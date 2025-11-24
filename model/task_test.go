package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImportTasks(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "doitdoit_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	jsonPath := filepath.Join(tmpDir, "tasks.json")
	importPath := filepath.Join(tmpDir, "import.txt")

	// Create initial JSON
	initialData := TodoData{
		"Future": []Task{
			{ID: "1", Title: "Existing Task", Completed: false},
		},
	}
	bytes, _ := json.Marshal(initialData)
	if err := os.WriteFile(jsonPath, bytes, 0644); err != nil {
		t.Fatal(err)
	}

	// Create import.txt
	importContent := "New Task 1\nNew Task 2\n   Trimmed Task   \n"
	if err := os.WriteFile(importPath, []byte(importContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load (should trigger import)
	data, err := Load(jsonPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify tasks in memory
	futureTasks := data["Future"]
	if len(futureTasks) != 4 { // 1 existing + 3 new
		t.Errorf("Expected 4 future tasks, got %d", len(futureTasks))
	}

	// Verify titles
	titles := make(map[string]bool)
	for _, t := range futureTasks {
		titles[t.Title] = true
	}

	if !titles["New Task 1"] {
		t.Error("Missing 'New Task 1'")
	}
	if !titles["New Task 2"] {
		t.Error("Missing 'New Task 2'")
	}
	if !titles["Trimmed Task"] {
		t.Error("Missing 'Trimmed Task'")
	}

	// Verify import.txt is deleted
	if _, err := os.Stat(importPath); !os.IsNotExist(err) {
		t.Error("import.txt was not deleted")
	}

	// Verify JSON file on disk is updated
	// Re-load raw file to check persistence
	// Note: Load might do rollover/prune, but our data is simple enough that it shouldn't change much unless dates are involved.
	// But we specifically want to check if the NEW tasks are saved.
	bytes, _ = os.ReadFile(jsonPath)
	var savedData TodoData
	json.Unmarshal(bytes, &savedData)
	if len(savedData["Future"]) != 4 {
		t.Errorf("Persisted data has %d future tasks, expected 4", len(savedData["Future"]))
	}
}

func TestPruningAndRollover(t *testing.T) {
	today := time.Now()
	todayStr := today.Format("2006-01-02")

	sixDaysAgo := today.AddDate(0, 0, -6).Format("2006-01-02")
	fourDaysAgo := today.AddDate(0, 0, -4).Format("2006-01-02")

	data := TodoData{
		sixDaysAgo: []Task{
			{ID: "A", Title: "Old Completed", Completed: true},
			{ID: "C", Title: "Old Incomplete", Completed: false},
		},
		fourDaysAgo: []Task{
			{ID: "B", Title: "Recent Completed", Completed: true},
			{ID: "D", Title: "Recent Incomplete", Completed: false},
		},
		"Future": []Task{
			{ID: "E", Title: "Future Completed", Completed: true},
			{ID: "F", Title: "Future Incomplete", Completed: false},
		},
	}

	// Execute the logic in the same order as Load()
	_ = data.rollOverIncompleteTasks()
	_ = data.pruneOldTasks()

	// Assertions

	// 1. Check 6 days ago bucket
	if _, exists := data[sixDaysAgo]; exists {
		t.Errorf("Expected bucket %s to be deleted (contained only old completed tasks or moved tasks)", sixDaysAgo)
	}

	// 2. Check 4 days ago bucket
	recentBucket, exists := data[fourDaysAgo]
	if !exists {
		t.Errorf("Expected bucket %s to exist", fourDaysAgo)
	} else {
		if len(recentBucket) != 1 {
			t.Errorf("Expected 1 task in %s, got %d", fourDaysAgo, len(recentBucket))
		} else if recentBucket[0].ID != "B" {
			t.Errorf("Expected task B in %s, got %s", fourDaysAgo, recentBucket[0].ID)
		}
	}

	// 3. Check Today's bucket (should contain rolled over tasks C and D)
	todayBucket, exists := data[todayStr]
	if !exists {
		t.Error("Expected today's bucket to exist with rolled over tasks")
	} else {
		foundC := false
		foundD := false
		for _, task := range todayBucket {
			if task.ID == "C" {
				foundC = true
			}
			if task.ID == "D" {
				foundD = true
			}
		}
		if !foundC {
			t.Error("Expected Task C (Old Incomplete) to be rolled over to today")
		}
		if !foundD {
			t.Error("Expected Task D (Recent Incomplete) to be rolled over to today")
		}
	}

	// 4. Check Future bucket
	futureBucket, exists := data["Future"]
	if !exists {
		t.Error("Expected Future bucket to exist")
	} else {
		if len(futureBucket) != 1 {
			t.Errorf("Expected 1 task in Future, got %d", len(futureBucket))
		} else if futureBucket[0].ID != "F" {
			t.Errorf("Expected Task F (Future Incomplete) in Future, got %s", futureBucket[0].ID)
		}
	}
}
