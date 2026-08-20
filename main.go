package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/dtt101/doitdoit/config"
	"github.com/dtt101/doitdoit/model"
	"github.com/dtt101/doitdoit/styles"
)

func main() {
	filePathFlag := flag.String("file", "", "Path to the JSON data file (overrides config)")
	visibleDays := flag.Int("days", 3, "Number of days to display")
	flag.Parse()

	if args := flag.Args(); len(args) > 0 && args[0] == "config" {
		os.Exit(config.RunCommand(args, os.Stdout))
	}
	if *visibleDays < 1 {
		fmt.Fprintln(os.Stderr, "Error: -days must be at least 1")
		os.Exit(2)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	input := bufio.NewReader(os.Stdin)

	var finalPath string
	if *filePathFlag != "" {
		expanded, err := config.ExpandPath(*filePathFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error expanding path: %v\n", err)
			os.Exit(1)
		}
		finalPath = expanded
	} else {
		finalPath, err = config.ResolveStoragePath(cfg, input, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}

	retentionDays, err := config.ResolveRetention(cfg, input, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error configuring retention: %v\n", err)
		os.Exit(1)
	}

	theme, err := styles.ResolveTheme(cfg.Theme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load theme %q (%v); using default\n", cfg.Theme, err)
		theme = styles.DefaultTheme()
	}
	styles.Apply(theme)

	// Ensure directory exists
	dir := filepath.Dir(finalPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Could not create directory %s: %v\n", dir, err)
		os.Exit(1)
	}

	m, err := model.NewModelWithRetention(finalPath, *visibleDays, retentionDays)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing model: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m)
	watchThemeReload(p)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
