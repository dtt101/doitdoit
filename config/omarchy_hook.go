package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const managedOmarchyHook = "#!/bin/sh\npkill -SIGUSR2 -x doitdoit\n"

type hookState int

const (
	hookMissing hookState = iota
	hookManaged
	hookModified
)

func omarchyHookPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "omarchy", "hooks", "theme-set.d", "doitdoit"), nil
}

func inspectOmarchyHook(path string) (hookState, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return hookMissing, nil
	}
	if err != nil {
		return hookModified, err
	}
	if !info.Mode().IsRegular() {
		return hookModified, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return hookModified, err
	}
	if bytes.Equal(data, []byte(managedOmarchyHook)) {
		return hookManaged, nil
	}
	return hookModified, nil
}

func runOmarchyHook(args []string, out io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(out, "Usage: doitdoit config omarchy-hook install|status|remove")
		return 1
	}
	path, err := omarchyHookPath()
	if err != nil {
		fmt.Fprintf(out, "Error locating Omarchy hook: %v\n", err)
		return 1
	}
	state, err := inspectOmarchyHook(path)
	if err != nil {
		fmt.Fprintf(out, "Error inspecting Omarchy hook: %v\n", err)
		return 1
	}

	switch args[0] {
	case "status":
		switch state {
		case hookMissing:
			fmt.Fprintf(out, "Omarchy hook is not installed (%s).\n", path)
		case hookManaged:
			fmt.Fprintf(out, "Omarchy hook is installed and managed by doitdoit (%s).\n", path)
		case hookModified:
			fmt.Fprintf(out, "A non-managed file, directory, or symlink exists at %s.\n", path)
		}
		return 0

	case "install":
		if state == hookManaged {
			fmt.Fprintf(out, "Omarchy hook is already installed (%s).\n", path)
			return 0
		}
		if state == hookModified {
			fmt.Fprintf(out, "Refusing to overwrite non-managed hook at %s.\n", path)
			return 1
		}
		if _, err := exec.LookPath("omarchy"); err != nil {
			fmt.Fprintln(out, "Cannot install hook: the 'omarchy' command was not found.")
			return 1
		}
		tempDir, err := os.MkdirTemp("", "doitdoit-omarchy-hook-")
		if err != nil {
			fmt.Fprintf(out, "Error creating temporary hook: %v\n", err)
			return 1
		}
		defer os.RemoveAll(tempDir)
		tempHook := filepath.Join(tempDir, "doitdoit")
		if err := os.WriteFile(tempHook, []byte(managedOmarchyHook), 0700); err != nil {
			fmt.Fprintf(out, "Error writing temporary hook: %v\n", err)
			return 1
		}
		command := exec.Command("omarchy", "hook", "install", "theme-set", tempHook)
		if output, err := command.CombinedOutput(); err != nil {
			fmt.Fprintf(out, "Omarchy hook installation failed: %v", err)
			if len(output) > 0 {
				fmt.Fprintf(out, ": %s", bytes.TrimSpace(output))
			}
			fmt.Fprintln(out)
			return 1
		}
		state, err = inspectOmarchyHook(path)
		if err != nil || state != hookManaged {
			fmt.Fprintf(out, "Omarchy did not install the expected managed hook at %s.\n", path)
			return 1
		}
		fmt.Fprintf(out, "Installed Omarchy theme hook at %s.\n", path)
		return 0

	case "remove":
		if state == hookMissing {
			fmt.Fprintf(out, "Omarchy hook is not installed (%s).\n", path)
			return 0
		}
		if state == hookModified {
			fmt.Fprintf(out, "Refusing to remove non-managed hook at %s.\n", path)
			return 1
		}
		if err := os.Remove(path); err != nil {
			fmt.Fprintf(out, "Error removing Omarchy hook: %v\n", err)
			return 1
		}
		fmt.Fprintf(out, "Removed Omarchy theme hook at %s.\n", path)
		return 0

	default:
		fmt.Fprintln(out, "Usage: doitdoit config omarchy-hook install|status|remove")
		return 1
	}
}
