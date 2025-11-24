package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
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

	// Handle CLI commands
	if len(flag.Args()) > 0 {
		cmd := flag.Arg(0)
		if cmd == "config" {
			if len(flag.Args()) > 1 && flag.Arg(1) == "show" {
				cfg, err := config.LoadConfig()
				if err != nil {
					fmt.Printf("Error loading config: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("Storage Path: %s\n", cfg.StoragePath)
				os.Exit(0)
			} else if len(flag.Args()) > 1 && flag.Arg(1) == "move" {
				if len(flag.Args()) < 3 {
					fmt.Println("Usage: doitdoit config move <new_file_path>")
					os.Exit(1)
				}
				newPath := flag.Arg(2)

				// Expand ~ if present
				if strings.HasPrefix(newPath, "~/") {
					home, _ := os.UserHomeDir()
					newPath = filepath.Join(home, newPath[2:])
				}

				cfg, err := config.LoadConfig()
				if err != nil {
					fmt.Printf("Error loading config: %v\n", err)
					os.Exit(1)
				}

				oldPath := cfg.StoragePath
				if oldPath == "" {
					fmt.Println("No storage path currently configured.")
					os.Exit(1)
				}

				if same, _ := filepath.Abs(oldPath); same != "" {
					if target, _ := filepath.Abs(newPath); target != "" && same == target {
						fmt.Println("New path is the same as the current path; nothing to do.")
						os.Exit(0)
					}
				}

				// Create destination directory
				newDir := filepath.Dir(newPath)
				if err := os.MkdirAll(newDir, 0755); err != nil {
					fmt.Printf("Error creating directory for new path: %v\n", err)
					os.Exit(1)
				}

				// First try simple rename (atomic on same filesystem)
				if err := os.Rename(oldPath, newPath); err != nil {
					// Fall back to copy+rename for cross-filesystem moves
					sourceFile, errOpen := os.Open(oldPath)
					if errOpen != nil {
						fmt.Printf("Error opening current storage file: %v\n", errOpen)
						os.Exit(1)
					}
					defer sourceFile.Close()

					tempFile, errTemp := os.CreateTemp(newDir, "doitdoit-move-*")
					if errTemp != nil {
						fmt.Printf("Error creating temp file in destination: %v\n", errTemp)
						os.Exit(1)
					}
					tempPath := tempFile.Name()

					if _, errCopy := io.Copy(tempFile, sourceFile); errCopy != nil {
						tempFile.Close()
						os.Remove(tempPath)
						fmt.Printf("Error copying data: %v\n", errCopy)
						os.Exit(1)
					}
					if errSync := tempFile.Sync(); errSync != nil {
						tempFile.Close()
						os.Remove(tempPath)
						fmt.Printf("Error flushing data: %v\n", errSync)
						os.Exit(1)
					}
					if errClose := tempFile.Close(); errClose != nil {
						os.Remove(tempPath)
						fmt.Printf("Error closing temp file: %v\n", errClose)
						os.Exit(1)
					}
					if errChmod := os.Chmod(tempPath, 0600); errChmod != nil {
						os.Remove(tempPath)
						fmt.Printf("Error setting permissions: %v\n", errChmod)
						os.Exit(1)
					}

					if errRename := os.Rename(tempPath, newPath); errRename != nil {
						os.Remove(tempPath)
						fmt.Printf("Error moving temp file into place: %v\n", errRename)
						os.Exit(1)
					}

					if errRemove := os.Remove(oldPath); errRemove != nil {
						fmt.Printf("Warning: Could not remove old file: %v\n", errRemove)
					}
				}

				// Update config
				cfg.StoragePath = newPath
				if err := config.SaveConfig(cfg); err != nil {
					fmt.Printf("Error saving config: %v\n", err)
					// Try to cleanup? No, better to leave both than lose data.
					os.Exit(1)
				}

				// Remove old file
				if err := os.Remove(oldPath); err != nil {
					fmt.Printf("Warning: Could not remove old file: %v\n", err)
				}

				fmt.Printf("Successfully moved storage to: %s\n", newPath)
				os.Exit(0)

			} else {
				fmt.Println("Usage: doitdoit config show | move <path>")
				os.Exit(1)
			}
		}
	}

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
