package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ResolveStoragePath determines the storage path interactively, prompting via
// in/out when the config has no path or the configured file is missing. It
// saves the chosen path to config when it differs from the existing one and
// returns the resolved path.
func ResolveStoragePath(cfg *Config, in io.Reader, out io.Writer) (string, error) {
	reader := bufferedReader(in)
	candidatePath := cfg.StoragePath

	for {
		if candidatePath == "" {
			fmt.Fprintln(out, "Welcome to DoItDoIt! Please configure your storage location.")
			fmt.Fprint(out, "Enter the path for your tasks file (e.g. ~/Dropbox/doitdoit.json): ")
			input, err := reader.ReadString('\n')
			if input == "" && err != nil {
				return "", fmt.Errorf("reading storage path: %w", err)
			}
			candidatePath, _ = ExpandPath(strings.TrimSpace(input))
			if candidatePath == "" {
				continue
			}
		}

		if _, err := os.Stat(candidatePath); os.IsNotExist(err) {
			fmt.Fprintf(out, "File not found at: %s\n", candidatePath)
			fmt.Fprint(out, "Do you want to (c)reate a new file here or (s)pecify a different location? (c/s): ")
			choice, _ := reader.ReadString('\n')
			choice = strings.ToLower(strings.TrimSpace(choice))

			if choice == "s" {
				candidatePath = ""
				continue
			} else if choice != "c" && choice != "" {
				continue
			}
			// "c" or empty: fall through and create at this path.
		}

		finalPath := candidatePath
		if cfg.StoragePath != finalPath {
			cfg.StoragePath = finalPath
			if err := SaveConfig(cfg); err != nil {
				fmt.Fprintf(out, "Warning: Could not save config: %v\n", err)
			} else {
				fmt.Fprintln(out, "Configuration saved.")
			}
		}
		return finalPath, nil
	}
}

func bufferedReader(in io.Reader) *bufio.Reader {
	if reader, ok := in.(*bufio.Reader); ok {
		return reader
	}
	return bufio.NewReader(in)
}

// ResolveRetention ensures that an explicit retention choice is persisted
// before task data can be loaded. Empty input and EOF safely mean forever.
func ResolveRetention(cfg *Config, in io.Reader, out io.Writer) (int, error) {
	if days, decided := cfg.Retention(); decided {
		return days, nil
	}

	reader := bufferedReader(in)
	for {
		fmt.Fprintln(out, "Choose how long to keep completed task history.")
		fmt.Fprint(out, "Enter 'forever' or a positive number of days [forever]: ")
		input, err := reader.ReadString('\n')
		choice := strings.ToLower(strings.TrimSpace(input))
		if err != nil && err != io.EOF {
			return 0, fmt.Errorf("reading retention choice: %w", err)
		}
		if choice == "" || choice == "forever" {
			cfg.SetRetention(0)
			if err := SaveConfig(cfg); err != nil {
				return 0, fmt.Errorf("saving retention choice: %w", err)
			}
			fmt.Fprintln(out, "Completed task history will be kept forever.")
			return 0, nil
		}

		days, parseErr := strconv.Atoi(choice)
		if parseErr == nil && days > 0 {
			cfg.SetRetention(days)
			if err := SaveConfig(cfg); err != nil {
				return 0, fmt.Errorf("saving retention choice: %w", err)
			}
			fmt.Fprintf(out, "Completed task history will be kept for %d days.\n", days)
			return days, nil
		}

		fmt.Fprintln(out, "Please enter 'forever' or a positive whole number of days.")
		if err == io.EOF {
			cfg.SetRetention(0)
			if saveErr := SaveConfig(cfg); saveErr != nil {
				return 0, fmt.Errorf("saving retention choice: %w", saveErr)
			}
			fmt.Fprintln(out, "Completed task history will be kept forever.")
			return 0, nil
		}
	}
}
