package config

import (
	"errors"
	"fmt"
	"io"
)

// RunCommand executes a `config` subcommand (args[0] is expected to be
// "config") and returns the process exit code. All output is written to out.
func RunCommand(args []string, out io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(out, "Usage: doitdoit config show | move <path>")
		return 1
	}
	switch args[1] {
	case "show":
		return runShow(out)
	case "move":
		return runMove(args[2:], out)
	default:
		fmt.Fprintln(out, "Usage: doitdoit config show | move <path>")
		return 1
	}
}

func runShow(out io.Writer) int {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(out, "Error loading config: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "Storage Path: %s\n", cfg.StoragePath)
	return 0
}

func runMove(args []string, out io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(out, "Usage: doitdoit config move <new_file_path>")
		return 1
	}

	newPath, err := ExpandPath(args[0])
	if err != nil {
		fmt.Fprintf(out, "Error expanding path: %v\n", err)
		return 1
	}

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(out, "Error loading config: %v\n", err)
		return 1
	}

	oldPath := cfg.StoragePath
	if oldPath == "" {
		fmt.Fprintln(out, "No storage path currently configured.")
		return 1
	}

	if SamePath(oldPath, newPath) {
		fmt.Fprintln(out, "New path is the same as the current path; nothing to do.")
		return 0
	}

	if err := MoveStorage(oldPath, newPath); err != nil {
		if errors.Is(err, ErrOldNotRemoved) {
			fmt.Fprintf(out, "Warning: %v\n", err)
		} else {
			fmt.Fprintf(out, "Error moving storage: %v\n", err)
			return 1
		}
	}

	cfg.StoragePath = newPath
	if err := SaveConfig(cfg); err != nil {
		fmt.Fprintf(out, "Error saving config: %v\n", err)
		return 1
	}

	fmt.Fprintf(out, "Successfully moved storage to: %s\n", newPath)
	return 0
}
