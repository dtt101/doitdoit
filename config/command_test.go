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
		{"config", "move"},
		{"config", "retention"},
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
	cfg := &Config{ServiceOrigin: "https://tasks.example.com", Theme: "nord"}
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if code := RunCommand([]string{"config", "show"}, &out); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Service: https://tasks.example.com") {
		t.Errorf("expected service origin in output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Theme: nord") {
		t.Errorf("expected theme in output, got %q", out.String())
	}
}

func TestLoadConfigIgnoresLegacyJSONFields(t *testing.T) {
	home := withTempHome(t)
	raw := `{"backend":"service","service_origin":"https://tasks.example.com","storage_path":"/tmp/old.json","retention_days":30,"theme":"tokyo-night"}`
	if err := os.WriteFile(filepath.Join(home, ".doitdoit_config.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.ServiceOrigin != "https://tasks.example.com" || cfg.Theme != "tokyo-night" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}
