package model

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCaptureTaskTargetsAndOrdering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	today := time.Now().Format(dateLayout)
	seed := TodoData{today: {{ID: "done", Title: "Done", Completed: true}}}
	if err := seed.Save(path); err != nil {
		t.Fatal(err)
	}
	if _, key, err := CaptureTask(path, "  Buy milk  ", "today", 0); err != nil || key != today {
		t.Fatalf("capture key=%q err=%v", key, err)
	}
	data, err := loadRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := data[today]; len(got) != 2 || got[0].Title != "Buy milk" || !got[1].Completed {
		t.Fatalf("unexpected ordering: %#v", got)
	}
	if backup, err := loadRaw(path + ".bak"); err != nil || len(backup[today]) != 1 {
		t.Fatalf("backup=%#v err=%v", backup, err)
	}
}

func TestCaptureTaskFutureAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	if _, key, err := CaptureTask(path, "Someday", "future", 0); err != nil || key != "Future" {
		t.Fatalf("capture key=%q err=%v", key, err)
	}
	if _, _, err := CaptureTask(path, "", "today", 0); err == nil {
		t.Fatal("expected empty-title error")
	}
	if _, _, err := CaptureTask(path, "Task", "whenever", 0); err == nil {
		t.Fatal("expected target error")
	}
}

func TestSaveIfUnchangedDetectsConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	data := TodoData{"Future": {{ID: "1", Title: "Original"}}}
	if err := data.Save(path); err != nil {
		t.Fatal(err)
	}
	contents, _ := os.ReadFile(path)
	hash := sha256.Sum256(contents)
	if err := os.WriteFile(path, []byte(`{"Future":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveIfUnchanged(path, &hash, true); !errors.Is(err, ErrDataConflict) {
		t.Fatalf("err=%v, want conflict", err)
	}
}

func TestPersistRebasesSeparateBucketsAndUndoPreservesExternalData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	today := time.Now().Format(dateLayout)
	tomorrow := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	seed := TodoData{today: {{ID: "base", Title: "Base"}}}
	if err := seed.Save(path); err != nil {
		t.Fatal(err)
	}
	m, err := NewModel(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	external := cloneTodoData(seed)
	external[tomorrow] = []Task{{ID: "remote", Title: "Remote"}}
	if err := external.Save(path); err != nil {
		t.Fatal(err)
	}
	m.addTask("Local")
	m.persist()
	if m.Err != nil || len(m.Data[today]) != 2 || len(m.Data[tomorrow]) != 1 {
		t.Fatalf("merge failed: err=%v data=%#v", m.Err, m.Data)
	}
	if !m.undoMove() {
		t.Fatal("expected local add to remain undoable")
	}
	m.persist()
	if m.Err != nil || len(m.Data[today]) != 1 || len(m.Data[tomorrow]) != 1 {
		t.Fatalf("undo should preserve remote bucket: err=%v data=%#v", m.Err, m.Data)
	}
}

func TestPersistRejectsCompetingChangesInSameBucket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	today := time.Now().Format(dateLayout)
	seed := TodoData{today: {{ID: "base", Title: "Base"}}}
	if err := seed.Save(path); err != nil {
		t.Fatal(err)
	}
	m, err := NewModel(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	external := TodoData{today: {{ID: "base", Title: "Changed elsewhere"}}}
	if err := external.Save(path); err != nil {
		t.Fatal(err)
	}
	m.addTask("Local")
	m.persist()
	if !errors.Is(m.Err, ErrDataConflict) {
		t.Fatalf("err=%v, want conflict", m.Err)
	}
	disk, err := loadRaw(path)
	if err != nil || len(disk[today]) != 1 || disk[today][0].Title != "Changed elsewhere" {
		t.Fatalf("external data was overwritten: %#v err=%v", disk, err)
	}
}
