package model

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	got := updated.(Model).TextInput.Styles().Focused.Text.GetForeground()
	if got != lipgloss.Color("#d8dee9") {
		t.Errorf("TextInput foreground = %v, want the nord foreground #d8dee9", got)
	}
	var _ tea.Model = updated
}

func TestBuiltinOmarchyThemesRenderWithBubbleTeaV2(t *testing.T) {
	t.Cleanup(func() { styles.Apply(styles.DefaultTheme()) })

	today := time.Now().Format(dateLayout)
	m := Model{
		Data:        TodoData{today: {{ID: "1", Title: "Themed task"}}},
		VisibleDays: 1,
		State:       Browsing,
		dateKeys:    []string{today},
		width:       80,
		height:      24,
	}

	for _, name := range styles.BuiltinThemeNames() {
		t.Run(name, func(t *testing.T) {
			theme, err := styles.BuiltinTheme(name)
			if err != nil {
				t.Fatal(err)
			}
			styles.Apply(theme)

			view := m.View()
			if !view.AltScreen {
				t.Fatal("view must request the alternate screen")
			}
			if !strings.Contains(view.Content, "Themed task") {
				t.Fatalf("rendered view does not contain the task: %q", view.Content)
			}
			if !strings.Contains(view.Content, "\x1b[") {
				t.Fatalf("rendered view does not contain ANSI styling: %q", view.Content)
			}
		})
	}
}
