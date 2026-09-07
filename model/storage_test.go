package model

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/dtt101/doitdoit/taskstore"
)

type interveningStore struct {
	taskstore.JSONStore
	afterLoad func()
	afterSave func()
}

func TestModelStorageFollowsPublicFilePath(t *testing.T) {
	dir := t.TempDir()
	oldPath, newPath := filepath.Join(dir, "old.json"), filepath.Join(dir, "new.json")
	data := TodoData{"Future": {{ID: "a", Title: "original"}}}
	for _, path := range []string{oldPath, newPath} {
		if err := data.Save(path); err != nil {
			t.Fatal(err)
		}
	}
	m, err := NewModel(oldPath, 3)
	if err != nil {
		t.Fatal(err)
	}
	m.FilePath, m.ShowFuture = newPath, true
	m.addTask("new")
	m.persist()
	if m.Err != nil {
		t.Fatal(m.Err)
	}
	old, err := loadRaw(oldPath)
	if err != nil || len(old["Future"]) != 1 {
		t.Fatalf("old path changed: %v %v", old, err)
	}
	current, err := loadRaw(newPath)
	if err != nil || len(current["Future"]) != 2 {
		t.Fatalf("new path not saved: %v %v", current, err)
	}
}

func (s *interveningStore) Load() (taskstore.Snapshot, error) {
	snapshot, err := s.JSONStore.Load()
	if s.afterLoad != nil {
		fn := s.afterLoad
		s.afterLoad = nil
		fn()
	}
	return snapshot, err
}

func (s *interveningStore) Save(data taskstore.Data, expected *taskstore.Revision) (taskstore.Revision, error) {
	revision, err := s.JSONStore.Save(data, expected)
	if err == nil && s.afterSave != nil {
		fn := s.afterSave
		s.afterSave = nil
		fn()
	}
	return revision, err
}

func TestModelKeepsRevisionPairedWithLoadedOrSavedData(t *testing.T) {
	for _, stage := range []string{"startup", "save"} {
		t.Run(stage, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tasks.json")
			store := &interveningStore{JSONStore: taskstore.NewJSON(path)}
			initial := taskstore.Data{"Future": {{ID: "a", Title: "original"}}}
			if _, err := store.JSONStore.Save(initial, nil); err != nil {
				t.Fatal(err)
			}
			externalWrite := func() {
				if _, err := store.JSONStore.Save(taskstore.Data{"Future": {{ID: "b", Title: "external"}}}, nil); err != nil {
					t.Fatal(err)
				}
			}
			if stage == "startup" {
				store.afterLoad = externalWrite
			}
			m, err := newModelWithStore(path, 3, 0, store)
			if err != nil {
				t.Fatal(err)
			}
			m.ShowFuture = true
			if stage == "save" {
				store.afterSave = externalWrite
				m.addTask("first")
				m.persist()
				if m.Err != nil {
					t.Fatal(m.Err)
				}
			}
			m.addTask("second")
			m.persist()
			if !errors.Is(m.Err, ErrDataConflict) {
				t.Fatalf("expected visible same-bucket conflict: %v", m.Err)
			}
			current, err := store.Load()
			if err != nil || len(current.Data["Future"]) != 1 || current.Data["Future"][0].ID != "b" {
				t.Fatalf("external write overwritten: %+v %v", current, err)
			}
		})
	}
}
