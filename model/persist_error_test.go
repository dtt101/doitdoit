package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistReportsErrors(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.Chmod(tmpDir, 0500); err != nil {
		t.Fatalf("chmod temp dir: %v", err)
	}
	defer os.Chmod(tmpDir, 0700)

	m := Model{
		Data:     make(TodoData),
		FilePath: filepath.Join(tmpDir, "tasks.json"),
	}

	m.persist()
	if m.Err == nil {
		t.Fatalf("expected persist to surface error when directory is not writable")
	}
}
