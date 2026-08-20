package config

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func setupFakeOmarchy(t *testing.T, succeed bool) string {
	t.Helper()
	home := withTempHome(t)
	bin := t.TempDir()
	script := "#!/bin/sh\n"
	if succeed {
		script += "mkdir -p \"$(dirname \"$FAKE_HOOK_TARGET\")\"\ncp \"$4\" \"$FAKE_HOOK_TARGET\"\n"
	} else {
		script += "echo simulated failure >&2\nexit 23\n"
	}
	if err := os.WriteFile(filepath.Join(bin, "omarchy"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	target := filepath.Join(home, ".config", "omarchy", "hooks", "theme-set.d", "doitdoit")
	t.Setenv("FAKE_HOOK_TARGET", target)
	return target
}

func TestOmarchyHookInstallStatusRemove(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	target := setupFakeOmarchy(t, true)
	var out bytes.Buffer
	if code := RunCommand([]string{"config", "omarchy-hook", "install"}, &out); code != 0 {
		t.Fatalf("install code=%d output=%q", code, out.String())
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != managedOmarchyHook {
		t.Fatalf("installed hook=%q err=%v", got, err)
	}
	if !strings.Contains(string(got), "pkill -SIGUSR2 -x doitdoit") {
		t.Fatalf("hook does not target every exact-name process: %q", got)
	}

	out.Reset()
	if code := RunCommand([]string{"config", "omarchy-hook", "status"}, &out); code != 0 || !strings.Contains(out.String(), "installed and managed") {
		t.Fatalf("status code=%d output=%q", code, out.String())
	}
	out.Reset()
	if code := RunCommand([]string{"config", "omarchy-hook", "install"}, &out); code != 0 || !strings.Contains(out.String(), "already installed") {
		t.Fatalf("second install code=%d output=%q", code, out.String())
	}
	out.Reset()
	if code := RunCommand([]string{"config", "omarchy-hook", "remove"}, &out); code != 0 {
		t.Fatalf("remove code=%d output=%q", code, out.String())
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("managed hook still exists: %v", err)
	}
}

func TestOmarchyHookRefusesModifiedFileAndSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink and shell fixture")
	}
	target := setupFakeOmarchy(t, true)
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho mine\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"install", "remove"} {
		var out bytes.Buffer
		if code := RunCommand([]string{"config", "omarchy-hook", action}, &out); code != 1 || !strings.Contains(out.String(), "Refusing") {
			t.Errorf("%s code=%d output=%q", action, code, out.String())
		}
	}
	if got, _ := os.ReadFile(target); string(got) != "#!/bin/sh\necho mine\n" {
		t.Fatalf("modified hook changed: %q", got)
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(filepath.Dir(target), "real")
	if err := os.WriteFile(real, []byte(managedOmarchyHook), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, target); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := RunCommand([]string{"config", "omarchy-hook", "remove"}, &out); code != 1 {
		t.Fatalf("symlink remove code=%d output=%q", code, out.String())
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("symlink was removed: %v", err)
	}
}

func TestOmarchyHookRefusesDirectory(t *testing.T) {
	target := setupFakeOmarchy(t, true)
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := RunCommand([]string{"config", "omarchy-hook", "install"}, &out); code != 1 || !strings.Contains(out.String(), "Refusing") {
		t.Fatalf("directory install code=%d output=%q", code, out.String())
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("directory changed: info=%v err=%v", info, err)
	}
}

func TestOmarchyHookAbsentAndCommandFailure(t *testing.T) {
	withTempHome(t)
	t.Setenv("PATH", t.TempDir())
	var out bytes.Buffer
	if code := RunCommand([]string{"config", "omarchy-hook", "install"}, &out); code != 1 || !strings.Contains(out.String(), "not found") {
		t.Fatalf("absent code=%d output=%q", code, out.String())
	}

	setupFakeOmarchy(t, false)
	out.Reset()
	if code := RunCommand([]string{"config", "omarchy-hook", "install"}, &out); code != 1 || !strings.Contains(out.String(), "simulated failure") {
		t.Fatalf("failure code=%d output=%q", code, out.String())
	}
}
