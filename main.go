package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dtt101/doitdoit/config"
	"github.com/dtt101/doitdoit/model"
)

func main() {
	filePathFlag := flag.String("file", "", "Path to the JSON data file (overrides config)")
	visibleDays := flag.Int("days", 3, "Number of days to display")
	flag.Parse()

	if args := flag.Args(); len(args) > 0 && args[0] == "config" {
		os.Exit(config.RunCommand(args, os.Stdout))
	}

	var finalPath string
	if *filePathFlag != "" {
		expanded, err := config.ExpandPath(*filePathFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error expanding path: %v\n", err)
			os.Exit(1)
		}
		finalPath = expanded
	} else {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = &config.Config{}
		}
		finalPath, err = config.ResolveStoragePath(cfg, os.Stdin, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}

	// Ensure directory exists
	dir := filepath.Dir(finalPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Could not create directory %s: %v\n", dir, err)
		os.Exit(1)
	}

	m, err := model.NewModel(finalPath, *visibleDays)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing model: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
