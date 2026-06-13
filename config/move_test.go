package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestMoveStorageRename(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.json")
	newPath := filepath.Join(dir, "sub", "new.json")
	writeFile(t, oldPath, `{"a":1}`)

	if err := MoveStorage(oldPath, newPath); err != nil {
		t.Fatalf("MoveStorage: %v", err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old file should be gone, stat err = %v", err)
	}
	got, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("reading new file: %v", err)
	}
	if string(got) != `{"a":1}` {
		t.Errorf("content = %q, want %q", got, `{"a":1}`)
	}
}

func TestMoveStorageCreatesDestDir(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.json")
	newPath := filepath.Join(dir, "a", "b", "c", "new.json")
	writeFile(t, oldPath, "data")

	if err := MoveStorage(oldPath, newPath); err != nil {
		t.Fatalf("MoveStorage: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected file at nested path: %v", err)
	}
}

func TestMoveStorageMissingSource(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "does-not-exist.json")
	newPath := filepath.Join(dir, "new.json")

	if err := MoveStorage(oldPath, newPath); err == nil {
		t.Fatal("expected error moving a missing source file")
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Errorf("destination should not exist after failed move")
	}
}

// copyFile is the cross-filesystem fallback path that MoveStorage's rename
// shortcut normally skips on the same filesystem, so exercise it directly.
func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.json")
	dst := filepath.Join(dir, "dst.json")
	writeFile(t, src, "hello world")

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading dst: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("content = %q, want %q", got, "hello world")
	}

	// copyFile leaves the source in place; MoveStorage removes it afterwards.
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source should still exist after copyFile: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("dst perm = %o, want 0600", perm)
	}
}

func TestSamePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.json")

	if !SamePath(p, p) {
		t.Error("identical paths should be the same")
	}
	if !SamePath(p, filepath.Join(dir, "sub", "..", "f.json")) {
		t.Error("paths resolving to the same location should be the same")
	}
	if SamePath(p, filepath.Join(dir, "other.json")) {
		t.Error("distinct paths should not be the same")
	}
}
