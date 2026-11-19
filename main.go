package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dtt101/doitdoit/cli"
	"github.com/dtt101/doitdoit/config"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "service":
			os.Exit(cli.RunServiceCommand(os.Args[2:], os.Stdout))
		case "add":
			os.Exit(cli.RunAddCommand(os.Args[2:], os.Stdout))
		case "config":
			os.Exit(config.RunCommand(os.Args[1:], os.Stdout))
		}
	}
	serviceFlag := flag.String("service", "", "Use the task service at this origin for this session")
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), `doitdoit — a terminal task manager

Usage:
  doitdoit [--service <origin>]
  doitdoit add [--when <target>] [--service <origin>] [--notes <text>] <title>
  doitdoit config <command>
  doitdoit service login|status|logout [--url <origin>]
  doitdoit service export --out <file> [--url <origin>]
  doitdoit service import --file <file> [--dry-run | --apply]
  doitdoit service retry --file <retained-command>
  doitdoit service history --id <id>
  doitdoit service restore --id <id> --revision <reviewed-revision>

Run without a command to open the interactive task manager.
The layout adapts to the terminal width. Use left/right to navigate days,
t to return to Today.

Interactive options:
`)
		flag.PrintDefaults()
		fmt.Fprint(flag.CommandLine.Output(), `
Add options (put flags before the title):
  --when <target>     today (default), tomorrow, future, or YYYY-MM-DD
  --service <origin>  Task service origin for this command (overrides config)
  --notes <text>      Plain-text notes; preserves whitespace and line breaks
  Titles can be quoted or supplied as multiple arguments.

Config commands:
  show                          Show service origin and theme
  theme [name]                  Show available themes or select a theme
  omarchy-hook install|status|remove
                                Manage opt-in live Omarchy theme updates

Examples:
  doitdoit service login
  doitdoit add --when tomorrow --notes "Include expenses" "Send invoice"
  doitdoit config theme tokyo-night

Use --help or -h to show help; use doitdoit add --help for capture options.
`)
	}
	flag.Parse()

	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	origin := cfg.ResolvedServiceOrigin(*serviceFlag)
	if err := runServiceTUI(origin, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
