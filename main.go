package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vs/doitdoit/config"
	"github.com/vs/doitdoit/model"
)

func main() {
	// Flags
	filePathFlag := flag.String("file", "", "Path to the JSON data file (overrides config)")
	visibleDays := flag.Int("days", 3, "Number of days to display")
	flag.Parse()

	var finalPath string

	// Helper to expand ~
	expandPath := func(path string) string {
		if strings.HasPrefix(path, "~/") {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, path[2:])
		}
		return path
	}

	if *filePathFlag != "" {
		finalPath = expandPath(*filePathFlag)
	} else {
		// Load Config
		cfg, err := config.LoadConfig()
		if err != nil {
			// If error loading config, assume empty/new
			cfg = &config.Config{}
		}

		candidatePath := cfg.StoragePath
		reader := bufio.NewReader(os.Stdin)

		for {
			if candidatePath == "" {
				fmt.Println("Welcome to DoItDoIt! Please configure your storage location.")
				fmt.Print("Enter the path for your tasks file (e.g. ~/Dropbox/doitdoit.json): ")
				input, _ := reader.ReadString('\n')
				candidatePath = expandPath(strings.TrimSpace(input))
				if candidatePath == "" {
					continue
				}
			}

			// Check if file exists
			if _, err := os.Stat(candidatePath); os.IsNotExist(err) {
				fmt.Printf("File not found at: %s\n", candidatePath)
				fmt.Print("Do you want to (c)reate a new file here or (s)pecify a different location? (c/s): ")
				choice, _ := reader.ReadString('\n')
				choice = strings.ToLower(strings.TrimSpace(choice))

				if choice == "s" {
					candidatePath = "" // Reset to prompt again
					continue
				} else if choice == "c" || choice == "" {
					// Create (default)
					// We proceed with this path
				} else {
					// Invalid choice, loop again
					continue
				}
			}

			// If we got here, either file exists or user chose to create it
			finalPath = candidatePath

			// Save to config
			if cfg.StoragePath != finalPath {
				cfg.StoragePath = finalPath
				if err := config.SaveConfig(cfg); err != nil {
					fmt.Printf("Warning: Could not save config: %v\n", err)
				} else {
					fmt.Printf("Configuration saved.\n")
				}
			}
			break
		}
	}

	// Ensure directory exists
	dir := filepath.Dir(finalPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Could not create directory %s: %v\n", dir, err)
		os.Exit(1)
	}

	// Initialize Model
	m, err := model.NewModel(finalPath, *visibleDays)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing model: %v\n", err)
		os.Exit(1)
	}

	// Run Program
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
