//go:build !unix

package main

import tea "github.com/charmbracelet/bubbletea"

// watchThemeReload is a no-op on platforms without SIGUSR2.
func watchThemeReload(_ *tea.Program) {}
