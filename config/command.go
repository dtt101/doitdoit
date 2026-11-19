package config

import (
	"fmt"
	"io"
	"strings"

	"github.com/dtt101/doitdoit/styles"
)

const configUsage = "Usage: doitdoit config show | theme [name] | omarchy-hook install|status|remove"

// RunCommand executes a `config` subcommand (args[0] is expected to be
// "config") and returns the process exit code. All output is written to out.
func RunCommand(args []string, out io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(out, configUsage)
		return 1
	}
	switch args[1] {
	case "show":
		return runShow(out)
	case "theme":
		return runTheme(args[2:], out)
	case "omarchy-hook":
		return runOmarchyHook(args[2:], out)
	default:
		fmt.Fprintln(out, configUsage)
		return 1
	}
}

func runShow(out io.Writer) int {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(out, "Error loading config: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "Service: %s\n", cfg.ResolvedServiceOrigin(""))
	fmt.Fprintf(out, "Theme: %s\n", themeDescription(cfg))
	return 0
}

func themeDescription(cfg *Config) string {
	if cfg.Theme == "" {
		return "system (follows Omarchy when present)"
	}
	return cfg.Theme
}

func runTheme(args []string, out io.Writer) int {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(out, "Error loading config: %v\n", err)
		return 1
	}

	if len(args) < 1 {
		fmt.Fprintf(out, "Current theme: %s\n", themeDescription(cfg))
		names := append([]string{styles.ThemeNameSystem}, styles.BuiltinThemeNames()...)
		fmt.Fprintf(out, "Available themes: %s\n", strings.Join(names, ", "))
		return 0
	}

	name := args[0]
	if !styles.ValidThemeName(name) {
		fmt.Fprintf(out, "Unknown theme %q. Run 'doitdoit config theme' to list available themes.\n", name)
		return 1
	}

	cfg.Theme = name
	if err := SaveConfig(cfg); err != nil {
		fmt.Fprintf(out, "Error saving config: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "Theme set to: %s\n", name)
	return 0
}
