package model

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dtt101/doitdoit/styles"
)

func TestThemeReloadMsgAppliesTheme(t *testing.T) {
	t.Cleanup(func() { styles.Apply(styles.DefaultTheme()) })

	m := Model{
		State:     Browsing,
		TextInput: textinput.New(),
	}

	theme, err := styles.BuiltinTheme("nord")
	if err != nil {
		t.Fatal(err)
	}

	updated, cmd := m.Update(ThemeReloadMsg{Theme: theme})
	if cmd != nil {
		t.Error("expected no follow-up command")
	}

	if styles.Highlight != lipgloss.Color("#81a1c1") {
		t.Errorf("Highlight = %v, want the nord accent #81a1c1", styles.Highlight)
	}
	got := updated.(Model).TextInput.TextStyle.GetForeground()
	if got != lipgloss.Color("#d8dee9") {
		t.Errorf("TextInput foreground = %v, want the nord foreground #d8dee9", got)
	}
	var _ tea.Model = updated
}
