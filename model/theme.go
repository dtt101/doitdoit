package model

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	textInputStyles := m.TextInput.Styles()
	textInputStyles.Focused.Text = lipgloss.NewStyle().Foreground(styles.Text)
	textInputStyles.Blurred.Text = textInputStyles.Focused.Text
	m.TextInput.SetStyles(textInputStyles)
	return m, nil
}
