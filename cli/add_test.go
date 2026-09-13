package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dtt101/doitdoit/config"
	"github.com/dtt101/doitdoit/taskstore"
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

func TestRunAddCommandNotes(t *testing.T) {
	for _, notes := range []string{"", "Include expenses", "  café 📝\n\tSecond line\n"} {
		t.Run(notes, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			path := filepath.Join(home, "tasks.json")
			var out bytes.Buffer
			args := []string{"--file", path, "--when", "future", "--notes", notes, "send", "invoice"}
			if code := RunAddCommand(args, &out); code != 0 {
				t.Fatalf("code=%d output=%q", code, out.String())
			}
			snapshot, err := taskstore.NewJSON(path).Load()
			if err != nil {
				t.Fatal(err)
			}
			tasks := snapshot.Data["Future"]
			if len(tasks) != 1 || tasks[0].Title != "send invoice" || tasks[0].Notes != notes {
				t.Fatalf("unexpected tasks: %#v", tasks)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if notes == "" && strings.Contains(string(contents), `"notes"`) {
				t.Fatal("empty notes should be omitted from JSON")
			}
		})
	}
}

func TestRunAddCommandInvalidNotesArgumentsDoNotWrite(t *testing.T) {
	for _, args := range [][]string{{"--notes"}, {"--notes", "only notes"}, {"--when", "invalid", "--notes", "details", "Task"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			path := filepath.Join(home, "tasks.json")
			var out bytes.Buffer
			if code := RunAddCommand(append([]string{"--file", path}, args...), &out); code == 0 {
				t.Fatalf("unexpected success: %q", out.String())
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("task file should not exist: %v", err)
			}
		})
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

func TestRunAddCommandHelp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Help must succeed without reading even a broken config or creating files.
	if err := os.WriteFile(filepath.Join(home, ".doitdoit_config.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, arg := range []string{"--help", "-h"} {
		var out bytes.Buffer
		if code := RunAddCommand([]string{arg}, &out); code != 0 {
			t.Fatalf("%s: code=%d output=%q", arg, code, out.String())
		}
		for _, want := range []string{"--notes", "today, tomorrow, future, or YYYY-MM-DD", "overrides config", "flags before the title"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%s: help missing %q", arg, want)
			}
		}
	}
}
