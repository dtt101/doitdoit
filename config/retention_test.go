package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRetentionDefaultsToForeverOnEOF(t *testing.T) {
	withTempHome(t)
	cfg := &Config{StoragePath: "/tmp/tasks.json"}
	var out bytes.Buffer
	days, err := ResolveRetention(cfg, strings.NewReader(""), &out)
	if err != nil {
		t.Fatal(err)
	}
	if days != 0 {
		t.Fatalf("days = %d, want forever (0)", days)
	}
	saved, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got, decided := saved.Retention(); !decided || got != 0 {
		t.Fatalf("saved retention = %d, %v; want 0, true", got, decided)
	}
}

func TestResolveRetentionInputAndConfirmation(t *testing.T) {
	const prompt = "Choose how long to keep completed task history.\nEnter 'forever' or a positive number of days [forever]: "
	const invalid = "Please enter 'forever' or a positive whole number of days.\n"
	const forever = "Completed task history will be kept forever.\n"
	for _, tc := range []struct {
		name  string
		input string
		days  int
		out   string
	}{
		{"empty EOF", "", 0, prompt + forever},
		{"blank line", "\n", 0, prompt + forever},
		{"explicit forever", " FoReVeR \n", 0, prompt + forever},
		{"days at EOF", "14", 14, prompt + "Completed task history will be kept for 14 days.\n"},
		{"invalid EOF", "nope", 0, prompt + invalid + forever},
		{"zero EOF", "0", 0, prompt + invalid + forever},
		{"negative EOF", "-1", 0, prompt + invalid + forever},
		{"retry then EOF", "nope\n", 0, prompt + invalid + prompt + forever},
		{"retry then days", "nope\n14\n", 14, prompt + invalid + prompt + "Completed task history will be kept for 14 days.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := withTempHome(t)
			cfg := &Config{StoragePath: filepath.Join(home, "tasks.json"), Theme: "nord"}
			var out bytes.Buffer
			days, err := ResolveRetention(cfg, strings.NewReader(tc.input), &out)
			if err != nil || days != tc.days || out.String() != tc.out {
				t.Fatalf("days=%d err=%v output=%q; want days=%d output=%q", days, err, out.String(), tc.days, tc.out)
			}
			saved, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got, decided := saved.Retention(); !decided || got != tc.days {
				t.Fatalf("saved retention=%d,%v; want %d,true", got, decided, tc.days)
			}
			if saved.StoragePath != cfg.StoragePath || saved.Theme != cfg.Theme {
				t.Fatalf("other config fields changed: %#v", saved)
			}
		})
	}
}

func TestResolveRetentionSaveFailure(t *testing.T) {
	for _, input := range []string{"forever\n", "14\n", "invalid"} {
		t.Run(input, func(t *testing.T) {
			home := withTempHome(t)
			// A directory at the config path prevents atomic replacement on both supported platforms.
			if err := os.Mkdir(filepath.Join(home, ".doitdoit_config.json"), 0700); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			days, err := ResolveRetention(&Config{}, strings.NewReader(input), &out)
			if days != 0 || err == nil || !strings.HasPrefix(err.Error(), "saving retention choice: ") {
				t.Fatalf("days=%d err=%v", days, err)
			}
			if strings.Contains(out.String(), "Completed task history will be kept") {
				t.Fatalf("reported success after save failure: %q", out.String())
			}
		})
	}
}

func TestResolveRetentionCustomAndExisting(t *testing.T) {
	withTempHome(t)
	cfg := &Config{}
	var out bytes.Buffer
	days, err := ResolveRetention(cfg, strings.NewReader("nope\n14\n"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if days != 14 || !strings.Contains(out.String(), "positive whole number") {
		t.Fatalf("days = %d, output = %q", days, out.String())
	}

	out.Reset()
	days, err = ResolveRetention(cfg, strings.NewReader("30\n"), &out)
	if err != nil || days != 14 || out.Len() != 0 {
		t.Fatalf("existing decision changed: days=%d err=%v output=%q", days, err, out.String())
	}
}

func TestRunCommandRetention(t *testing.T) {
	withTempHome(t)
	for _, tc := range []struct {
		arg  string
		want int
	}{
		{"forever", 0},
		{"45", 45},
	} {
		var out bytes.Buffer
		if code := RunCommand([]string{"config", "retention", tc.arg}, &out); code != 0 {
			t.Fatalf("arg %q: code=%d output=%q", tc.arg, code, out.String())
		}
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if got, decided := cfg.Retention(); !decided || got != tc.want {
			t.Fatalf("arg %q: retention=%d,%v want=%d,true", tc.arg, got, decided, tc.want)
		}
	}

	for _, bad := range []string{"0", "-1", "1.5", "later"} {
		var out bytes.Buffer
		if code := RunCommand([]string{"config", "retention", bad}, &out); code != 1 {
			t.Errorf("bad arg %q: code=%d", bad, code)
		}
	}
}

func TestLoadConfigRejectsMalformedRetention(t *testing.T) {
	home := withTempHome(t)
	path := filepath.Join(home, ".doitdoit_config.json")
	for _, content := range []string{
		`{"retention_days":-1}`,
		`{"retention_days":"five"}`,
		`{"retention_days":`,
	} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(); err == nil {
			t.Errorf("LoadConfig accepted malformed config %q", content)
		}
	}
}
