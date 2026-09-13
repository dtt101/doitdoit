package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/dtt101/doitdoit/cli"
	"github.com/dtt101/doitdoit/config"
	"github.com/dtt101/doitdoit/model"
	"github.com/dtt101/doitdoit/styles"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "add":
			os.Exit(cli.RunAddCommand(os.Args[2:], os.Stdout))
		case "config":
			os.Exit(config.RunCommand(os.Args[1:], os.Stdout))
		}
	}
	filePathFlag := flag.String("file", "", "Path to the JSON data file (overrides config)")
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), `doitdoit — a terminal task manager

Usage:
  doitdoit [--file <path>]
  doitdoit add [--when <target>] [--file <path>] [--notes <text>] <title>
  doitdoit config <command>

Run without a command to open the interactive task manager.
The layout adapts to the terminal width. Use left/right to navigate days,
t to return to Today, or T to toggle Today focus.

Interactive options:
`)
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), `
Add options (put flags before the title):
  --when <target>  today (default), tomorrow, future, or YYYY-MM-DD
  --file <path>    Task JSON file for this command (overrides config)
  --notes <text>   Plain-text notes; preserves whitespace and line breaks
  Titles can be quoted or supplied as multiple arguments.

Config commands:
  show                          Show storage path, theme, and retention
  move <path>                   Move the task file and update config
  theme [name]                  Show available themes or select a theme
  retention [forever|days]       Show or set completed-history retention
                                Use forever or a positive number of days
  omarchy-hook install|status|remove
                                Manage opt-in live Omarchy theme updates

Examples:
  doitdoit --file ~/tasks.json
  doitdoit add --when tomorrow --notes "Include expenses" "Send invoice"
  doitdoit config retention 30

Use --help or -h to show help; use doitdoit add --help for capture options.
`)
	}
	flag.Parse()

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

	_, setupComplete := cfg.Retention()
	retentionDays, err := config.ResolveRetention(cfg, input, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error configuring retention: %v\n", err)
		os.Exit(1)
	}
	if !setupComplete {
		config.OfferOmarchyHook(cfg, input, os.Stdout)
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

	// The calendar window controls presentation only.
	m, err := model.NewModelWithRetention(finalPath, 3, retentionDays)
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
