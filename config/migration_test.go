package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationUsesConfiguredAnchorWithoutChangingSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{"Tasks", "local"} {
		if err := os.Mkdir(filepath.Join(home, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, retention := range []string{"", `,"retention_days":0`, `,"retention_days":30`} {
		t.Run(retention, func(t *testing.T) {
			configRaw := []byte(`{"storage_path":"~/Tasks/tasks.json","theme":"omarchy","unrecognized_setting":"preserve"` + retention + `}`)
			configPath := filepath.Join(home, ".doitdoit_config.json")
			if err := os.WriteFile(configPath, configRaw, 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			days, decided := cfg.Retention()
			raw := []byte(`{"2020-01-01":[{"id":"old","title":"Retain before maintenance","completed":true,"created_at":"2020-01-01T00:00:00Z"}]}`)
			anchor := filepath.Join(home, "Tasks", "tasks.json")
			if err := os.WriteFile(anchor, raw, 0600); err != nil {
				t.Fatal(err)
			}
			m, err := cfg.RecordMigration(filepath.Join(home, "local"))
			if err != nil {
				t.Fatal(err)
			}
			if m.Anchor != anchor || m.Store.Root != anchor+".store" {
				t.Fatalf("wrong discovery: %+v", m)
			}
			result, err := m.Run()
			if err != nil || result.Status != "ready" || len(result.View.Snapshot["2020-01-01"]) != 1 {
				t.Fatalf("migration pruned or rolled over: %+v %v", result, err)
			}
			if got, _ := os.ReadFile(configPath); !bytes.Equal(got, configRaw) {
				t.Fatal("configuration changed")
			}
			if got, _ := os.ReadFile(anchor); !bytes.Equal(got, raw) {
				t.Fatal("legacy input changed")
			}
			if d, c := cfg.Retention(); d != days || c != decided || cfg.Theme != "omarchy" || cfg.StoragePath != "~/Tasks/tasks.json" {
				t.Fatal("in-memory settings changed")
			}
		})
	}
}
