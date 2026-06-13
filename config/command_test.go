package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempHome points HOME at a temp dir so LoadConfig/SaveConfig operate on an
// isolated config file, and returns that dir.
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestRunCommandUsage(t *testing.T) {
	withTempHome(t)
	for _, args := range [][]string{
		{"config"},
		{"config", "bogus"},
	} {
		var out bytes.Buffer
		if code := RunCommand(args, &out); code != 1 {
			t.Errorf("args %v: code = %d, want 1", args, code)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Errorf("args %v: expected usage text, got %q", args, out.String())
		}
	}
}

func TestRunCommandShow(t *testing.T) {
	withTempHome(t)
	if err := SaveConfig(&Config{StoragePath: "/tmp/tasks.json"}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if code := RunCommand([]string{"config", "show"}, &out); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "/tmp/tasks.json") {
		t.Errorf("expected storage path in output, got %q", out.String())
	}
}

func TestRunCommandMoveMissingArg(t *testing.T) {
	withTempHome(t)
	var out bytes.Buffer
	if code := RunCommand([]string{"config", "move"}, &out); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "Usage: doitdoit config move") {
		t.Errorf("expected move usage, got %q", out.String())
	}
}

func TestRunCommandMoveNoConfiguredPath(t *testing.T) {
	withTempHome(t)
	var out bytes.Buffer
	if code := RunCommand([]string{"config", "move", "/tmp/new.json"}, &out); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "No storage path currently configured") {
		t.Errorf("expected no-path message, got %q", out.String())
	}
}

func TestRunCommandMoveSamePath(t *testing.T) {
	home := withTempHome(t)
	current := filepath.Join(home, "tasks.json")
	if err := SaveConfig(&Config{StoragePath: current}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if code := RunCommand([]string{"config", "move", current}, &out); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "nothing to do") {
		t.Errorf("expected same-path message, got %q", out.String())
	}
}

func TestRunCommandMoveSuccess(t *testing.T) {
	home := withTempHome(t)
	oldPath := filepath.Join(home, "old", "tasks.json")
	newPath := filepath.Join(home, "new", "tasks.json")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte(`{"a":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(&Config{StoragePath: oldPath}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if code := RunCommand([]string{"config", "move", newPath}, &out); code != 0 {
		t.Fatalf("code = %d, want 0; output %q", code, out.String())
	}
	if !strings.Contains(out.String(), "Successfully moved storage") {
		t.Errorf("expected success message, got %q", out.String())
	}

	// Data moved.
	if got, err := os.ReadFile(newPath); err != nil || string(got) != `{"a":1}` {
		t.Errorf("new file = %q, err %v", got, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old file should be gone, stat err = %v", err)
	}

	// Config now points at the new path.
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StoragePath != newPath {
		t.Errorf("config path = %q, want %q", cfg.StoragePath, newPath)
	}
}
