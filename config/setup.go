package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// ResolveStoragePath determines the storage path interactively, prompting via
// in/out when the config has no path or the configured file is missing. It
// saves the chosen path to config when it differs from the existing one and
// returns the resolved path.
func ResolveStoragePath(cfg *Config, in io.Reader, out io.Writer) (string, error) {
	reader := bufio.NewReader(in)
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
