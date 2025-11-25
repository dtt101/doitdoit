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
		},
	}

	payload, _ := json.Marshal(initial)
	if err := os.WriteFile(jsonPath, payload, 0600); err != nil {
		t.Fatalf("write initial json: %v", err)
	}
	if err := os.WriteFile(importPath, []byte("Imported Task\n"), 0600); err != nil {
		t.Fatalf("write import file: %v", err)
	}

	data, err := Load(jsonPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	today := time.Now().Format("2006-01-02")

	if _, ok := data[sixDaysAgo]; ok {
		t.Fatalf("expected old date bucket %s to be pruned", sixDaysAgo)
	}
	if tasks := data[today]; len(tasks) != 1 || tasks[0].ID != "old" {
		t.Fatalf("expected rolled over task in today, got %#v", tasks)
	}
	if _, err := os.Stat(importPath); !os.IsNotExist(err) {
		t.Fatalf("expected import file to be removed, got err=%v", err)
	}

	persistedBytes, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read persisted json: %v", err)
	}
	var persisted TodoData
	if err := json.Unmarshal(persistedBytes, &persisted); err != nil {
		t.Fatalf("unmarshal persisted json: %v", err)
	}

	if _, ok := persisted[sixDaysAgo]; ok {
		t.Fatalf("persisted data still contains old date %s", sixDaysAgo)
	}
	if tasks := persisted[today]; len(tasks) != 1 || tasks[0].ID != "old" {
		t.Fatalf("persisted data missing rolled task, got %#v", tasks)
	}
	if tasks := persisted["Future"]; len(tasks) != 1 || tasks[0].Title != "Imported Task" {
		t.Fatalf("expected imported task persisted in Future, got %#v", tasks)
	}
}
