package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vs/doitdoit/model"
)

func main() {
	// Default path in home directory
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not find home directory: %v\n", err)
		os.Exit(1)
	}
	defaultPath := filepath.Join(home, "Dropbox", "doitdoit.json")

	// Flags
	filePath := flag.String("file", defaultPath, "Path to the JSON data file (e.g., Dropbox path)")
	visibleDays := flag.Int("days", 3, "Number of days to display")
	flag.Parse()

	// Ensure directory exists
	dir := filepath.Dir(*filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Could not create directory %s: %v\n", dir, err)
		os.Exit(1)
	}

	// Initialize Model
	m, err := model.NewModel(*filePath, *visibleDays)
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
