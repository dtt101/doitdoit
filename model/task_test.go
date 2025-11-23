package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
