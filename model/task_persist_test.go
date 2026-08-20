package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadPersistsMutations(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "tasks.json")
	importPath := filepath.Join(tmpDir, "import.txt")

	sixDaysAgo := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	initial := TodoData{
		sixDaysAgo: []Task{
			{ID: "old", Title: "Old Task", Completed: false},
			{ID: "done", Title: "Completed History", Completed: true},
		},
	}

	payload, _ := json.Marshal(initial)
	if err := os.WriteFile(jsonPath, payload, 0600); err != nil {
		t.Fatalf("write initial json: %v", err)
	}
	if err := os.WriteFile(importPath, []byte("Imported Task\n"), 0600); err != nil {
		t.Fatalf("write import file: %v", err)
	}

	data, err := Load(jsonPath, 0)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	today := time.Now().Format("2006-01-02")

	if tasks := data[sixDaysAgo]; len(tasks) != 1 || tasks[0].ID != "done" {
		t.Fatalf("expected completed history to be kept forever, got %#v", tasks)
	}
	if tasks := data[today]; len(tasks) != 1 || tasks[0].ID != "old" {
		t.Fatalf("expected rolled over task in today, got %#v", tasks)
	}
	if got, err := os.ReadFile(importPath); err != nil || string(got) != "Imported Task\n" {
		t.Fatalf("expected import file to remain untouched, got %q, err=%v", got, err)
	}

	persistedBytes, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read persisted json: %v", err)
	}
	var persisted TodoData
	if err := json.Unmarshal(persistedBytes, &persisted); err != nil {
		t.Fatalf("unmarshal persisted json: %v", err)
	}

	if tasks := persisted[sixDaysAgo]; len(tasks) != 1 || tasks[0].ID != "done" {
		t.Fatalf("persisted data lost completed history: %#v", tasks)
	}
	if tasks := persisted[today]; len(tasks) != 1 || tasks[0].ID != "old" {
		t.Fatalf("persisted data missing rolled task, got %#v", tasks)
	}
	if _, ok := persisted["Future"]; ok {
		t.Fatalf("import file unexpectedly created Future tasks: %#v", persisted["Future"])
	}
}

func TestLoadPrunesOnlyWithPositiveRetention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	oldDate := time.Now().AddDate(0, 0, -31).Format(dateLayout)
	payload, _ := json.Marshal(TodoData{oldDate: {{ID: "done", Completed: true}}})
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}

	data, err := Load(path, 30)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := data[oldDate]; ok {
		t.Fatalf("expected %s to be pruned with 30-day retention", oldDate)
	}
}
