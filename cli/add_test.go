package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dtt101/doitdoit/config"
)

func TestRunAddCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "tasks.json")
	cfg := &config.Config{StoragePath: path}
	cfg.SetRetention(0)
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := RunAddCommand([]string{"--when", "future", "write", "postcard"}, &out); code != 0 {
		t.Fatalf("code=%d output=%q", code, out.String())
	}
	contents, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(contents), "write postcard") {
		t.Fatalf("contents=%q err=%v", contents, err)
	}
}

func TestRunAddCommandRequiresTitleAndPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	if code := RunAddCommand(nil, &out); code != 2 {
		t.Fatalf("empty title code=%d", code)
	}
	out.Reset()
	if code := RunAddCommand([]string{"Task"}, &out); code != 1 || !strings.Contains(out.String(), "No storage path") {
		t.Fatalf("code=%d output=%q", code, out.String())
	}
}
