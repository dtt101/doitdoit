package model

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func newFeedbackTestModel(t *testing.T) Model {
	t.Helper()
	m := newReloadTestModel(t)
	m.width, m.height = 80, 24
	m.Data[m.dateKeys[0]] = []Task{{ID: "1", Title: "First task"}, {ID: "2", Title: "Second task"}}
	return m
}

func TestBrowsingFooterShowsEverydayActionsAndWraps(t *testing.T) {
	for _, width := range []int{120, 80, 48, 30} {
		for _, future := range []bool{false, true} {
			m := Model{State: Browsing, width: width, ShowFuture: future}
			footer := m.helpView()
			plain := ansi.Strip(footer)
			for _, hint := range []string{"a add", "space complete", "m move", "? help"} {
				if !strings.Contains(plain, hint) {
					t.Errorf("width=%d future=%v: missing %q in %q", width, future, hint, plain)
				}
			}
			toggle := "f future"
			if future {
				toggle = "f days"
			}
			if !strings.Contains(plain, toggle) {
				t.Errorf("missing current view toggle %q in %q", toggle, plain)
			}
			if lipgloss.Width(footer) > width-m.appStyle().GetHorizontalFrameSize() {
				t.Errorf("footer exceeds width %d: %q", width, plain)
			}
		}
	}
}

func TestEmptyColumnGuidanceMatchesAddDestination(t *testing.T) {
	for _, tc := range []struct {
		name   string
		future bool
		column int
		want   string
	}{
		{"today", false, 0, "What needs doing today?"},
		{"later day", false, 1, "Plan something for this day."},
		{"future", true, 0, "Capture an idea for later."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newReloadTestModel(t)
			m.ShowFuture, m.ColIdx = tc.future, tc.column
			key := m.getCurrentKey()
			view := ansi.Strip(m.renderDaySection(key, tc.column, 60))
			if !strings.Contains(view, tc.want) || !strings.Contains(view, "Press a to add a task.") {
				t.Fatalf("missing guidance: %q", view)
			}
			m = pressRune(m, 'a')
			if m.State != Adding || m.getCurrentKey() != key {
				t.Fatal("add did not target the advertised column")
			}
			if strings.Contains(m.renderDaySection(key, tc.column, 60), "Press a") {
				t.Fatal("add guidance should disappear while entering a task")
			}
		})
	}
	m := newReloadTestModel(t)
	view := m.renderDaySection(m.dateKeys[1], 1, 60)
	if strings.Contains(view, "Press a") || !strings.Contains(view, "Select this day") {
		t.Fatalf("unfocused column advertised an action for another column: %q", view)
	}
}

func TestDeleteFeedbackAndUndo(t *testing.T) {
	m := newFeedbackTestModel(t)
	today := m.dateKeys[0]
	m = pressRune(m, 'd')
	footer := ansi.Strip(m.feedbackView())
	if !strings.Contains(footer, "Task deleted") || !strings.Contains(footer, "u undo") {
		t.Fatalf("missing delete confirmation and recovery: %q", footer)
	}
	if len(m.Data[today]) != 1 || m.Data[today][0].ID != "2" {
		t.Fatalf("delete not applied: %v", m.Data)
	}
	m = pressRune(m, 'u')
	footer = ansi.Strip(m.feedbackView())
	if !strings.Contains(footer, "Undid last change") || strings.Contains(footer, "u undo") || strings.Contains(footer, "Task deleted") {
		t.Fatalf("undo confirmation is stale: %q", footer)
	}
	if len(m.Data[today]) != 2 || m.Data[today][0].ID != "1" {
		t.Fatalf("undo not applied: %v", m.Data)
	}
}

func TestMoveFeedbackUsesActualDestination(t *testing.T) {
	for _, tc := range []struct {
		name, date, want string
		key              rune
	}{
		{name: "tomorrow", key: '1', want: "tomorrow"},
		{name: "future", key: 'f', want: "Future"},
		{name: "today", key: 't', want: "Today"},
		{name: "exact date", key: 'd', date: dayKey(10), want: dayKey(10)},
		{name: "past date preserved", key: 'd', date: dayKey(-1), want: dayKey(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newFeedbackTestModel(t)
			m = pressRune(pressRune(m, 'm'), tc.key)
			if tc.key == 'd' {
				m.TextInput.SetValue(tc.date)
				updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				m = updated.(Model)
			}
			footer := ansi.Strip(m.feedbackView())
			if m.Err != nil || !strings.Contains(footer, "Moved to "+tc.want) || !strings.Contains(footer, "u undo") {
				t.Fatalf("unexpected move result: err=%v footer=%q", m.Err, footer)
			}
		})
	}
	m := newFeedbackTestModel(t)
	m = pressRune(pressRune(m, 'm'), '1')
	m = pressRune(m, '.')
	if m.Err != nil || len(m.Data[dayKey(1)]) != 2 || !strings.Contains(m.feedbackView(), "Moved to tomorrow") {
		t.Fatalf("repeat move did not confirm its destination: %v, %q", m.Err, m.feedbackView())
	}
}

func TestFeedbackFollowsUndoHistory(t *testing.T) {
	m := newFeedbackTestModel(t)
	m = pressRune(m, 'd')
	m = pressRune(pressRune(m, 'f'), 'f')
	if !strings.Contains(m.feedbackView(), "Task deleted") {
		t.Fatal("navigation lost the last action feedback")
	}
	m = pressSpace(m)
	if strings.Contains(m.feedbackView(), "Task deleted") || m.feedback != "" {
		t.Fatal("a new mutation retained stale delete feedback")
	}
}

func TestNoOpAndCancelledMovesDoNotClaimSuccess(t *testing.T) {
	m := newFeedbackTestModel(t)
	m.Data[m.dateKeys[0]][0].DueDate = m.dateKeys[0]
	m = pressRune(pressRune(m, 'm'), 't')
	if m.feedback != "" {
		t.Fatal("same destination produced success feedback")
	}
	m = pressRune(m, 'm')
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if updated.(Model).feedback != "" {
		t.Fatal("cancelled move produced success feedback")
	}
}

// Exercise the rendered screen, not just the notification string: deleting and
// undoing must leave every keyboard hint and the task viewport in place.
func TestFeedbackDoesNotDisruptKeyboardHints(t *testing.T) {
	for _, size := range [][2]int{{24, 10}, {30, 12}, {48, 16}, {80, 24}, {120, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := resizeModel(newFeedbackTestModel(t), size[0], size[1])
			hints := ansi.Strip(m.helpView())
			geometry := m.columnGeometry(m.visibleColumnCount())
			hintRow := func(m Model) int {
				for i, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
					if strings.Contains(line, "a add") {
						return i
					}
				}
				t.Fatal("add shortcut is missing")
				return -1
			}
			row := hintRow(m)
			for _, action := range []rune{'d', 'u'} {
				m = pressRune(m, action)
				assertFitsTerminal(t, m)
				if ansi.Strip(m.helpView()) != hints || hintRow(m) != row {
					t.Fatal("feedback changed or moved keyboard hints")
				}
				if m.columnGeometry(m.visibleColumnCount()) != geometry {
					t.Fatal("feedback resized the task viewport")
				}
				if lipgloss.Height(m.feedbackView()) != 1 {
					t.Fatal("feedback must occupy exactly one line")
				}
			}
			m.feedback = strings.Repeat("界", 100)
			m.moveUndo = &moveUndoSnapshot{}
			assertFitsTerminal(t, m)
			if !strings.HasSuffix(ansi.Strip(m.feedbackView()), " · u undo") {
				t.Fatal("a long message hid the undo action")
			}
		})
	}
}
