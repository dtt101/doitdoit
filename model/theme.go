package model

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dtt101/doitdoit/styles"
)

// ThemeReloadMsg carries a freshly resolved theme into the update loop, so
// styles are swapped on the same goroutine that renders.
type ThemeReloadMsg struct {
	Theme styles.Theme
}

func (m Model) handleThemeReload(msg ThemeReloadMsg) (tea.Model, tea.Cmd) {
	styles.Apply(msg.Theme)
	// The text input captures its style at configure time, so refresh it.
	m.TextInput.TextStyle = lipgloss.NewStyle().Foreground(styles.Text)
	return m, nil
}
