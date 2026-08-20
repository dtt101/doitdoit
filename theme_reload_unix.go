//go:build unix

package main

import (
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/dtt101/doitdoit/config"
	"github.com/dtt101/doitdoit/model"
	"github.com/dtt101/doitdoit/styles"
)

// watchThemeReload re-resolves and re-applies the theme whenever the process
// receives SIGUSR2 — the same reload convention Omarchy's theme switcher
// uses for btop, helix, and friends. On Omarchy, wire it up with a theme-set
// hook that runs `pkill -SIGUSR2 -x doitdoit`.
func watchThemeReload(p *tea.Program) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGUSR2)
	go func() {
		for range signals {
			cfg, err := config.LoadConfig()
			if err != nil {
				continue
			}
			theme, err := styles.ResolveTheme(cfg.Theme)
			if err != nil {
				continue
			}
			p.Send(model.ThemeReloadMsg{Theme: theme})
		}
	}()
}
