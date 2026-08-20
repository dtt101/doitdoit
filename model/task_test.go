package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImportTextIsIgnoredAndPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "tasks.json")
	importPath := filepath.Join(tmpDir, "import.txt")
	initialData := TodoData{"Future": {{ID: "1", Title: "Existing Task"}}}
	bytes, _ := json.Marshal(initialData)
	if err := os.WriteFile(jsonPath, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	importContent := []byte("must remain untouched\n")
	if err := os.WriteFile(importPath, importContent, 0600); err != nil {
		t.Fatal(err)
	}

	data, err := Load(jsonPath, 0)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := data["Future"]; len(got) != 1 || got[0].Title != "Existing Task" {
		t.Fatalf("import.txt affected tasks: %#v", got)
	}
	gotImport, err := os.ReadFile(importPath)
	if err != nil {
		t.Fatalf("import.txt was removed: %v", err)
	}
	if string(gotImport) != string(importContent) {
		t.Errorf("import.txt changed: %q", gotImport)
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
	_ = data.pruneOldTasks(5)

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
