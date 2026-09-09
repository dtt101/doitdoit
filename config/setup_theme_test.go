package config

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dtt101/doitdoit/styles"
)

func fakeActiveTheme(t *testing.T) {
	t.Helper()
	path, err := styles.OmarchyColorsPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("foreground = '#ffffff'\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestThemeSetupRequiresExplicitOptIn(t *testing.T) {
	for _, answer := range []string{"", "\n", "n\n", "no\n", "maybe\n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			withTempHome(t)
			fakeActiveTheme(t)
			var out bytes.Buffer
			OfferOmarchyHook(&Config{}, strings.NewReader(answer), &out)
			path, _ := omarchyHookPath()
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("setup installed a hook without an explicit yes")
			}
			if !strings.Contains(out.String(), "[y/N]") || !strings.Contains(out.String(), "doitdoit config omarchy-hook install") {
				t.Fatalf("setup did not explain opt-in and later installation: %q", out.String())
			}
		})
	}
}

func TestThemeSetupSharesInputAndInstallsManagedHook(t *testing.T) {
	target := setupFakeOmarchy(t, true)
	fakeActiveTheme(t)
	input := bufio.NewReader(strings.NewReader("forever\nyes\n"))
	cfg := &Config{}
	var out bytes.Buffer
	if _, err := ResolveRetention(cfg, input, &out); err != nil {
		t.Fatal(err)
	}
	OfferOmarchyHook(cfg, input, &out)
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != managedOmarchyHook || !strings.Contains(out.String(), "omarchy-hook remove") {
		t.Fatalf("explicit opt-in failed: %v, %q", err, out.String())
	}
	loaded, err := LoadConfig()
	if err != nil || loaded.RetentionDays == nil || *loaded.RetentionDays != 0 {
		t.Fatal("hook setup disturbed retention settings")
	}
}

func TestThemeSetupPreservesExistingHooks(t *testing.T) {
	for _, content := range []string{managedOmarchyHook, "#!/bin/sh\necho custom\n"} {
		withTempHome(t)
		fakeActiveTheme(t)
		path, _ := omarchyHookPath()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0700); err != nil {
			t.Fatal(err)
		}
		input := strings.NewReader("yes\n")
		var out bytes.Buffer
		OfferOmarchyHook(&Config{}, input, &out)
		after, _ := os.ReadFile(path)
		if string(after) != content || input.Len() != 4 || strings.Contains(out.String(), "[y/N]") {
			t.Fatalf("existing hook was changed or prompted for replacement: %q", out.String())
		}
	}
}

func TestThemeSetupSkipsAbsentOmarchyAndFixedPalette(t *testing.T) {
	withTempHome(t)
	input := strings.NewReader("yes\n")
	var out bytes.Buffer
	OfferOmarchyHook(&Config{}, input, &out)
	if out.Len() != 0 || input.Len() != 4 {
		t.Fatal("setup prompted on a system without Omarchy")
	}
	fakeActiveTheme(t)
	OfferOmarchyHook(&Config{Theme: "nord"}, input, &out)
	if out.Len() != 0 || input.Len() != 4 {
		t.Fatal("setup offered live following for an explicitly fixed palette")
	}
}

func TestThemeSetupInstallationFailureIsNonFatal(t *testing.T) {
	target := setupFakeOmarchy(t, false)
	fakeActiveTheme(t)
	var out bytes.Buffer
	OfferOmarchyHook(&Config{}, strings.NewReader("y\n"), &out)
	if !strings.Contains(out.String(), "simulated failure") || !strings.Contains(out.String(), "enable live updates later") {
		t.Fatalf("failure did not leave a recovery instruction: %q", out.String())
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("failed installation left a managed hook")
	}
}

type unreadableSetupInput struct{}

func (unreadableSetupInput) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestThemeSetupReadFailureDoesNotInstall(t *testing.T) {
	withTempHome(t)
	fakeActiveTheme(t)
	var out bytes.Buffer
	OfferOmarchyHook(&Config{}, unreadableSetupInput{}, &out)
	path, _ := omarchyHookPath()
	if _, err := os.Lstat(path); !os.IsNotExist(err) || !strings.Contains(out.String(), "could not be read") {
		t.Fatalf("read failure was not handled conservatively: %q", out.String())
	}
}
