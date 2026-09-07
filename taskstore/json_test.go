package taskstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJSONStoreRevisionBackupAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := NewJSON(path)
	empty, err := store.Load()
	if err != nil || empty.Revision.Exists || len(empty.Data) != 0 {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
	data := Data{"Future": {{ID: "a", Title: "original"}}}
	first, err := store.Save(data, &empty.Revision)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Revision != first {
		t.Fatalf("load revision=%+v err=%v", loaded.Revision, err)
	}
	data["Future"][0].Title = "external"
	if _, err := store.Save(data, &first); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || string(backup) != string(original) {
		t.Fatalf("backup=%q err=%v", backup, err)
	}
	if _, err := store.Save(loaded.Data, &loaded.Revision); !errors.Is(err, ErrDataConflict) {
		t.Fatalf("stale save: %v", err)
	}
	current, err := store.Load()
	if err != nil || current.Data["Future"][0].Title != "external" {
		t.Fatalf("current=%+v err=%v", current, err)
	}
	for _, name := range []string{path, path + ".bak"} {
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("permissions for %s: %v %v", name, info, err)
		}
	}
}

// Inject an external write between the service's load and save. The expected
// revision must come from the loaded data, including when startup did maintenance.
type conflictingStore struct {
	JSONStore
	beforeSave func()
}

func (s conflictingStore) Save(data Data, expected *Revision) (Revision, error) {
	s.beforeSave()
	return s.JSONStore.Save(data, expected)
}

func TestCaptureAndMaintenanceUseLoadedRevision(t *testing.T) {
	for _, maintenance := range []bool{false, true} {
		t.Run(map[bool]string{false: "capture", true: "maintenance"}[maintenance], func(t *testing.T) {
			store := NewJSON(filepath.Join(t.TempDir(), "tasks.json"))
			key := "Future"
			if maintenance {
				key = time.Now().AddDate(0, 0, -1).Format(DateLayout)
			}
			if _, err := store.Save(Data{key: {{ID: "a", Title: "original"}}}, nil); err != nil {
				t.Fatal(err)
			}
			external := Data{key: {{ID: "b", Title: "external"}}}
			injected := conflictingStore{JSONStore: store, beforeSave: func() {
				if _, err := store.Save(external, nil); err != nil {
					t.Fatal(err)
				}
			}}
			_, _, err := Capture(injected, "capture", "future", 0)
			if !errors.Is(err, ErrDataConflict) {
				t.Fatalf("capture error=%v", err)
			}
			current, err := store.Load()
			if err != nil || current.Data[key][0].ID != "b" {
				t.Fatalf("external edit lost: %+v %v", current, err)
			}
		})
	}
}

func TestLoadStoreDoesNotWriteUnchangedOrInvalidData(t *testing.T) {
	store := NewJSON(filepath.Join(t.TempDir(), "tasks.json"))
	if _, err := store.Save(Data{"Future": {{ID: "a"}}}, nil); err != nil {
		t.Fatal(err)
	}
	noWrites := conflictingStore{JSONStore: store, beforeSave: func() { t.Fatal("unexpected maintenance write") }}
	if _, _, err := LoadStore(noWrites, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadStore(noWrites, 0); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
