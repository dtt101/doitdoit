package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveStoragePathExistingConfiguredFile(t *testing.T) {
	withTempHome(t)
	dir := t.TempDir()
	existing := filepath.Join(dir, "tasks.json")
	if err := os.WriteFile(existing, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{StoragePath: existing}

	var out bytes.Buffer
	got, err := ResolveStoragePath(cfg, strings.NewReader(""), &out)
	if err != nil {
		t.Fatalf("ResolveStoragePath: %v", err)
	}
	if got != existing {
		t.Errorf("path = %q, want %q", got, existing)
	}
	// File already configured and present: no prompting, no save message.
	if out.Len() != 0 {
		t.Errorf("expected no output, got %q", out.String())
	}
}

func TestResolveStoragePathPromptsAndSaves(t *testing.T) {
	withTempHome(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "tasks.json")
	if err := os.WriteFile(target, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}
	var out bytes.Buffer
	got, err := ResolveStoragePath(cfg, strings.NewReader(target+"\n"), &out)
	if err != nil {
		t.Fatalf("ResolveStoragePath: %v", err)
	}
	if got != target {
		t.Errorf("path = %q, want %q", got, target)
	}
	if !strings.Contains(out.String(), "Configuration saved.") {
		t.Errorf("expected save confirmation, got %q", out.String())
	}

	// The chosen path was persisted.
	saved, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if saved.StoragePath != target {
		t.Errorf("saved path = %q, want %q", saved.StoragePath, target)
	}
}

func TestResolveStoragePathCreatesMissingFile(t *testing.T) {
	withTempHome(t)
	dir := t.TempDir()
	missing := filepath.Join(dir, "new.json")

	cfg := &Config{}
	// Provide the path, then choose to create it.
	in := strings.NewReader(missing + "\nc\n")
	var out bytes.Buffer
	got, err := ResolveStoragePath(cfg, in, &out)
	if err != nil {
		t.Fatalf("ResolveStoragePath: %v", err)
	}
	if got != missing {
		t.Errorf("path = %q, want %q", got, missing)
	}
	if !strings.Contains(out.String(), "File not found") {
		t.Errorf("expected not-found prompt, got %q", out.String())
	}
}

func TestResolveStoragePathSpecifyDifferentLocation(t *testing.T) {
	withTempHome(t)
	dir := t.TempDir()
	existing := filepath.Join(dir, "tasks.json")
	if err := os.WriteFile(existing, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.json")

	cfg := &Config{}
	// First a missing path, choose (s)pecify, then give the existing file.
	in := strings.NewReader(missing + "\ns\n" + existing + "\n")
	var out bytes.Buffer
	got, err := ResolveStoragePath(cfg, in, &out)
	if err != nil {
		t.Fatalf("ResolveStoragePath: %v", err)
	}
	if got != existing {
		t.Errorf("path = %q, want %q", got, existing)
	}
}

func TestResolveStoragePathEOFWithoutInput(t *testing.T) {
	withTempHome(t)
	cfg := &Config{}
	var out bytes.Buffer
	if _, err := ResolveStoragePath(cfg, strings.NewReader(""), &out); err == nil {
		t.Fatal("expected an error when input ends before a path is given")
	}
}
