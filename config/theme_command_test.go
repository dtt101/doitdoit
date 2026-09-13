package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCommandThemeShow(t *testing.T) {
	withTempHome(t)
	var out bytes.Buffer
	if code := RunCommand([]string{"config", "theme"}, &out); code != 0 {
		t.Fatalf("code = %d, want 0; output %q", code, out.String())
	}
	if !strings.Contains(out.String(), "system") {
		t.Errorf("expected system theme by default, got %q", out.String())
	}
	if !strings.Contains(out.String(), "tokyo-night") {
		t.Errorf("expected available themes listed, got %q", out.String())
	}
}

func TestRunCommandThemeDescriptions(t *testing.T) {
	for _, tc := range []struct {
		theme string
		want  string
	}{
		{"", "system (follows Omarchy when present)"},
		{"system", "system"},
		{"nord", "nord"},
	} {
		t.Run("theme="+tc.theme, func(t *testing.T) {
			withTempHome(t)
			if err := SaveConfig(&Config{Theme: tc.theme}); err != nil {
				t.Fatal(err)
			}
			for _, command := range []struct {
				name   string
				prefix string
			}{
				{"show", "Theme: "},
				{"theme", "Current theme: "},
			} {
				var out bytes.Buffer
				if code := RunCommand([]string{"config", command.name}, &out); code != 0 {
					t.Fatalf("%s: code=%d output=%q", command.name, code, out.String())
				}
				want := command.prefix + tc.want
				found := false
				for _, line := range strings.Split(out.String(), "\n") {
					if line == want {
						found = true
					}
				}
				if !found {
					t.Errorf("%s: output=%q, want line %q", command.name, out.String(), want)
				}
			}
		})
	}
}

func TestRunCommandThemeSet(t *testing.T) {
	withTempHome(t)
	var out bytes.Buffer
	if code := RunCommand([]string{"config", "theme", "nord"}, &out); code != 0 {
		t.Fatalf("code = %d, want 0; output %q", code, out.String())
	}
	if !strings.Contains(out.String(), "Theme set to: nord") {
		t.Errorf("expected confirmation, got %q", out.String())
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "nord" {
		t.Errorf("config theme = %q, want nord", cfg.Theme)
	}
}

func TestRunCommandThemeSetPreservesStoragePath(t *testing.T) {
	withTempHome(t)
	if err := SaveConfig(&Config{StoragePath: "/tmp/tasks.json"}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if code := RunCommand([]string{"config", "theme", "gruvbox"}, &out); code != 0 {
		t.Fatalf("code = %d, want 0; output %q", code, out.String())
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StoragePath != "/tmp/tasks.json" {
		t.Errorf("storage path = %q, want it preserved", cfg.StoragePath)
	}
	if cfg.Theme != "gruvbox" {
		t.Errorf("theme = %q, want gruvbox", cfg.Theme)
	}
}

func TestRunCommandThemeSetUnknown(t *testing.T) {
	withTempHome(t)
	var out bytes.Buffer
	if code := RunCommand([]string{"config", "theme", "bogus"}, &out); code != 1 {
		t.Fatalf("code = %d, want 1; output %q", code, out.String())
	}
	if !strings.Contains(out.String(), "Unknown theme") {
		t.Errorf("expected unknown-theme message, got %q", out.String())
	}
}

func TestRunCommandThemeSetSystem(t *testing.T) {
	withTempHome(t)
	var out bytes.Buffer
	if code := RunCommand([]string{"config", "theme", "system"}, &out); code != 0 {
		t.Fatalf("code = %d, want 0; output %q", code, out.String())
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "system" {
		t.Errorf("theme = %q, want system", cfg.Theme)
	}
}
