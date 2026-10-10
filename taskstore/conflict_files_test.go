package taskstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConflictDiscoveryStrictReadingAndCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	store := NewJSON(path)
	revision, err := store.Save(Data{"Future": {{ID: "a"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	matching := []string{"tasks (Someone's conflicted copy 2026-09-19).json", "tasks.json (Someone's conflicted copy 2026-09-19).bak", "tasks (Someone's conflicted copy 2026-09-19).json.bak"}
	for _, name := range append(append([]string{}, matching...), "tasks.json.bak", "tasks.json.pre-merge", "other (Someone's conflicted copy 2026-09-19).json") {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"Future":[{"id":"a","title":"","completed":false,"created_at":"0001-01-01T00:00:00Z"}]}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := ConflictPaths(path)
	if err != nil || len(paths) != len(matching) {
		t.Fatalf("discovery: %v %v", paths, err)
	}
	source, err := ReadReviewFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source.Path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(RemoveReviewedCopy(source, store, revision), ErrDataConflict) {
		t.Fatal("deleted changed source")
	}
	source, _ = ReadReviewFile(paths[1])
	if _, err := store.Save(Data{}, &revision); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(RemoveReviewedCopy(source, store, revision), ErrDataConflict) {
		t.Fatal("deleted against stale current file")
	}
	link := filepath.Join(dir, "symlink.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReviewFile(link); err == nil {
		t.Fatal("followed symlink")
	}
}

func TestReviewDecoderRejectsLossyInputs(t *testing.T) {
	for _, input := range []string{`null`, `[]`, `{`, `{} {}`, `{"Future":[],"Future":[]}`, `{"Future":[{"id":"a","updated_at":"unknown"}]}`, `{"Future":[{"id":"a","id":"b"}]}`} {
		if _, err := DecodeReviewData([]byte(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
}

func TestRecoveryIsScopedAtomicAndPrivate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	path := filepath.Join(t.TempDir(), "tasks.json")
	recovery := Recovery{StorePath: path, Base: Data{}, Draft: Data{"Future": {{ID: "a", Title: "draft"}}}, CurrentBytes: []byte("exact source bytes")}
	file, err := WriteRecovery(path, recovery, true)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v %v", info, err)
	}
	loaded, err := ReadRecovery(path)
	if err != nil || loaded.Draft["Future"][0].Title != "draft" || string(loaded.CurrentBytes) != "exact source bytes" {
		t.Fatalf("recovery: %v %v", loaded, err)
	}
	if loaded, err := ReadRecovery(path + ".other"); err != nil || loaded != nil {
		t.Fatal("recovery crossed stores")
	}
	archive, err := WriteRecovery(path, recovery, false)
	if err != nil || !strings.Contains(archive, "review-") {
		t.Fatal(archive, err)
	}
	if err := ClearRecovery(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatal("clearing pending removed export")
	}
}
