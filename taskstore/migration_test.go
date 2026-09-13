package taskstore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLoadMigratesDatedFutureBeforeLifecycle(t *testing.T) {
	store := NewJSON(filepath.Join(t.TempDir(), "tasks.json"))
	now := time.Now()
	past, today, far := now.AddDate(0, 0, -2).Format(DateLayout), now.Format(DateLayout), now.AddDate(2, 0, 0).Format(DateLayout)
	legacy := Task{ID: "far", Title: "Travel", Notes: "café\nDetails", DueDate: far, CreatedAt: now.UTC()}
	seed := Data{
		far: {{ID: "existing"}, {ID: "done", Completed: true}},
		"Future": {
			{ID: "idea"}, legacy,
			{ID: "past-open", DueDate: past},
			{ID: "past-done", DueDate: past, Completed: true},
			{ID: "invalid", DueDate: "broken"},
		},
	}
	if _, err := store.Save(seed, nil); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(store.path)
	snapshot, count, err := LoadStore(store, 0)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("carried=%d", count)
	}
	if got := snapshot.Data[far]; len(got) != 3 || got[0].ID != "existing" || !reflect.DeepEqual(got[1], legacy) || got[2].ID != "done" {
		t.Fatalf("far=%+v", got)
	}
	if got := snapshot.Data["Future"]; len(got) != 2 || got[0].ID != "idea" || got[1].DueDate != "broken" {
		t.Fatalf("Future=%+v", got)
	}
	if got := snapshot.Data[today]; len(got) != 1 || got[0].ID != "past-open" || got[0].DueDate != today {
		t.Fatalf("today=%+v", got)
	}
	if got := snapshot.Data[past]; len(got) != 1 || got[0].ID != "past-done" || got[0].DueDate != past {
		t.Fatalf("past=%+v", got)
	}
	backup, err := os.ReadFile(store.path + ".bak")
	if err != nil || string(backup) != string(original) {
		t.Fatalf("backup changed: %v", err)
	}
	for _, path := range []string{store.path, store.path + ".bak"} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("permissions=%v", info.Mode())
		}
	}
	noWrites := conflictingStore{JSONStore: store, beforeSave: func() { t.Fatal("repeat load wrote data") }}
	if _, _, err := LoadStore(noWrites, 0); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Revision != snapshot.Revision {
		t.Fatalf("revision mismatch: %v", err)
	}
}

func TestMigrationRejectsConcurrentWrite(t *testing.T) {
	store := NewJSON(filepath.Join(t.TempDir(), "tasks.json"))
	if _, err := store.Save(Data{"Future": {{ID: "legacy", DueDate: "2099-01-01"}}}, nil); err != nil {
		t.Fatal(err)
	}
	external := Data{"Future": {{ID: "external"}}}
	injected := conflictingStore{JSONStore: store, beforeSave: func() {
		if _, err := store.Save(external, nil); err != nil {
			t.Fatal(err)
		}
	}}
	if _, _, err := LoadStore(injected, 0); !errors.Is(err, ErrDataConflict) {
		t.Fatalf("err=%v", err)
	}
	loaded, err := store.Load()
	if err != nil || !reflect.DeepEqual(loaded.Data, external) {
		t.Fatalf("external data changed: %+v %v", loaded, err)
	}
}

func TestCaptureDistantDateUsesDateBucket(t *testing.T) {
	store := NewJSON(filepath.Join(t.TempDir(), "tasks.json"))
	date := time.Now().AddDate(3, 0, 0).Format(DateLayout)
	task, key, err := CaptureWithNotes(store, "Trip", date, "Details", 0)
	if err != nil || key != date || task.DueDate != date {
		t.Fatalf("task=%+v key=%s err=%v", task, key, err)
	}
	snapshot, err := store.Load()
	if err != nil || len(snapshot.Data["Future"]) != 0 || len(snapshot.Data[date]) != 1 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
}

func TestMigrationRunsBeforeRetention(t *testing.T) {
	store := NewJSON(filepath.Join(t.TempDir(), "tasks.json"))
	now := time.Now()
	old, recent := now.AddDate(0, 0, -30).Format(DateLayout), now.AddDate(0, 0, -1).Format(DateLayout)
	seed := Data{"Future": {
		{ID: "old-done", DueDate: old, Completed: true},
		{ID: "recent-done", DueDate: recent, Completed: true},
		{ID: "old-open", DueDate: old},
	}}
	if _, err := store.Save(seed, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := LoadStore(store, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Data[old]) != 0 || len(snapshot.Data[recent]) != 1 || len(snapshot.Data[now.Format(DateLayout)]) != 1 {
		t.Fatalf("migration did not respect history retention: %+v", snapshot.Data)
	}
}
