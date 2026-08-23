package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dtt101/doitdoit/styles"
)

func TestFooterWordmarkOnlyAnimatesWhenClicked(t *testing.T) {
	today := "2026-08-20"
	m := Model{
		Data:        TodoData{today: {{ID: "1", Title: "Task"}}},
		VisibleDays: 1,
		State:       Browsing,
		dateKeys:    []string{today},
		width:       80,
		height:      24,
	}

	if footer := ansi.Strip(m.helpView()); !strings.Contains(footer, "doitdoit. Press ? for help") {
		t.Fatalf("footer is missing the left-aligned wordmark and help prompt: %q", footer)
	}
	view := m.View()
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("mouse mode = %v, want cell motion", view.MouseMode)
	}

	x, y, width, ok := m.brandBounds()
	if !ok || width != len("doitdoit") {
		t.Fatalf("wordmark bounds = (%d, %d, %d, %v)", x, y, width, ok)
	}
	lines := strings.Split(ansi.Strip(view.Content), "\n")
	if y >= len(lines) || strings.Index(lines[y], "doitdoit") != x {
		t.Fatalf("wordmark bounds do not match rendered footer at (%d, %d): %q", x, y, view.Content)
	}

	outside, cmd := m.Update(tea.MouseClickMsg{X: x - 1, Y: y, Button: tea.MouseLeft})
	if got := outside.(Model); got.brandFrame != 0 || cmd != nil {
		t.Fatal("click outside the wordmark started its animation")
	}

	clicked, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	animated := clicked.(Model)
	if animated.brandFrame != 1 || cmd == nil {
		t.Fatal("left click on the wordmark did not start its animation")
	}
	if got := ansi.Strip(animated.brandView()); got == "doitdoit" || lipgloss.Width(got) != len("doitdoit") {
		t.Fatalf("wordmark did not enter its fixed-width matrix state: %q", got)
	}

	resolved := animated
	resolved.brandFrame = brandAnimationFrames
	if got := ansi.Strip(resolved.brandView()); got != "doitdoit" {
		t.Fatalf("matrix animation did not resolve to the wordmark: %q", got)
	}

	stopped, cmd := animated.Update(brandAnimationMsg{
		id:    animated.brandAnimationID,
		frame: brandAnimationFrames + 1,
	})
	if got := stopped.(Model); got.brandFrame != 0 || cmd != nil {
		t.Fatal("wordmark did not return to its quiet state")
	}
}

func TestColumnsFillAvailableWidthAndHaveEqualHeight(t *testing.T) {
	keys := []string{
		"2026-08-17",
		"2026-08-18",
		"2026-08-19",
		"2026-08-20",
		"2026-08-21",
	}
	m := Model{
		Data: TodoData{
			keys[0]: {{ID: "1", Title: strings.Repeat("A long task that wraps ", 4)}},
			keys[1]: {
				{ID: "2", Title: "First task"},
				{ID: "3", Title: "Second task"},
				{ID: "4", Title: "Third task"},
			},
			keys[2]: {},
			keys[3]: {{ID: "5", Title: "Short task"}},
			keys[4]: {},
		},
		VisibleDays: len(keys),
		State:       Browsing,
		dateKeys:    keys,
		width:       120,
		height:      30,
	}

	columns := m.renderColumns(keys)
	if len(columns) != len(keys) {
		t.Fatalf("rendered %d columns, want %d", len(columns), len(keys))
	}

	availableWidth := m.width - styles.AppStyle.GetHorizontalFrameSize()
	wantColumnWidth := availableWidth / len(columns)
	wantHeight := lipgloss.Height(columns[0])
	for i, column := range columns {
		if got := lipgloss.Width(column); got != wantColumnWidth {
			t.Errorf("column %d width = %d, want %d", i, got, wantColumnWidth)
		}
		if got := lipgloss.Height(column); got != wantHeight {
			t.Errorf("column %d height = %d, want %d", i, got, wantHeight)
		}
	}

	usedWidth := lipgloss.Width(lipgloss.JoinHorizontal(lipgloss.Top, columns...))
	if unusedWidth := availableWidth - usedWidth; unusedWidth < 0 || unusedWidth >= len(columns) {
		t.Errorf("columns use %d of %d available cells", usedWidth, availableWidth)
	}
}

func TestHelpModalRendersInsideTerminal(t *testing.T) {
	today := "2026-08-20"
	for _, width := range []int{80, 48} {
		m := Model{
			Data:        TodoData{today: {{ID: "1", Title: "Task"}}},
			VisibleDays: 1,
			State:       Browsing,
			ShowHelp:    true,
			dateKeys:    []string{today},
			width:       width,
			height:      24,
		}

		view := m.View()
		if !strings.Contains(view.Content, "Keyboard shortcuts") {
			t.Fatalf("help modal title is missing at width %d: %q", width, view.Content)
		}
		if got := lipgloss.Width(view.Content); got > m.width {
			t.Errorf("help overlay width = %d, terminal width = %d", got, m.width)
		}
		if got := lipgloss.Height(view.Content); got > m.height {
			t.Errorf("help overlay height = %d, terminal height = %d", got, m.height)
		}
	}
}

func TestHelpModalSeparatesKeysAndExplainsHowToClose(t *testing.T) {
	m := Model{State: Browsing, width: 80, height: 24}
	modal := ansi.Strip(m.helpModalView())

	if !strings.Contains(modal, "Press Esc to close") {
		t.Fatalf("help modal does not clearly explain how to close it: %q", modal)
	}

	for _, line := range strings.Split(modal, "\n") {
		keyStart := strings.Index(line, "arrows / hjkl")
		descriptionStart := strings.Index(line, "navigate")
		if keyStart < 0 || descriptionStart < 0 {
			continue
		}
		keyEnd := keyStart + len("arrows / hjkl")
		if descriptionStart-keyEnd < 2 {
			t.Fatalf("key and description need a visible gutter: %q", line)
		}
		return
	}
	t.Fatalf("navigation shortcut row is missing: %q", modal)
}

func TestHelpModalIsCenteredOverBackground(t *testing.T) {
	m := Model{
		State:  Browsing,
		width:  80,
		height: 24,
	}
	background := strings.Join([]string{
		"tasks remain visible",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"footer remains visible",
	}, "\n")

	overlay := m.renderHelpOverlay(background)
	if !strings.Contains(overlay, "tasks remain visible") || !strings.Contains(overlay, "footer remains visible") {
		t.Fatalf("help modal replaced its background: %q", overlay)
	}

	lines := strings.Split(overlay, "\n")
	modalTop := -1
	for i, line := range lines {
		if strings.Contains(line, "╭") {
			modalTop = i
			break
		}
	}
	if modalTop <= 0 {
		t.Fatalf("help modal was not vertically centered: %q", overlay)
	}
	if got, want := lipgloss.Width(lines[modalTop]), (m.width+lipgloss.Width(m.helpModalView()))/2; got != want {
		t.Fatalf("help modal right edge = %d, want %d for centered modal", got, want)
	}
}
