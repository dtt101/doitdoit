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
