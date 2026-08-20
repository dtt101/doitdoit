//go:build !unix

package main

import tea "charm.land/bubbletea/v2"

// watchThemeReload is a no-op on platforms without SIGUSR2.
func watchThemeReload(_ *tea.Program) {}
