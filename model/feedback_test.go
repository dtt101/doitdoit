package model

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dtt101/doitdoit/styles"
)

func newFeedbackTestModel(t *testing.T) Model {
	t.Helper()
	m := newReloadTestModel(t)
	m.width, m.height = 80, 24
	m.Data[m.dateKeys[0]] = []Task{{ID: "1", Title: "First task"}, {ID: "2", Title: "Second task"}}
	m.persist()
	if m.Err != nil {
		t.Fatal(m.Err)
	}
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
			if lipgloss.Width(footer) > width-styles.AppStyle.GetHorizontalFrameSize() {
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

func TestDeleteFeedbackAndUndoMatchSavedData(t *testing.T) {
	m := newFeedbackTestModel(t)
	today := m.dateKeys[0]
	m = pressRune(m, 'd')
	footer := ansi.Strip(m.helpView())
	if !strings.Contains(footer, "Task deleted") || !strings.Contains(footer, "u undo") {
		t.Fatalf("missing delete confirmation and recovery: %q", footer)
	}
	saved, err := loadRaw(m.FilePath)
	if err != nil || len(saved[today]) != 1 || saved[today][0].ID != "2" {
		t.Fatalf("delete not saved: %v, %v", saved, err)
	}
	m = pressRune(m, 'u')
	footer = ansi.Strip(m.helpView())
	if !strings.Contains(footer, "Undid last change") || strings.Contains(footer, "u undo") || strings.Contains(footer, "Task deleted") {
		t.Fatalf("undo confirmation is stale: %q", footer)
	}
	saved, err = loadRaw(m.FilePath)
	if err != nil || len(saved[today]) != 2 || saved[today][0].ID != "1" {
		t.Fatalf("undo not saved: %v, %v", saved, err)
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
		{name: "past date", key: 'd', date: dayKey(-1), want: "Today"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newFeedbackTestModel(t)
			m = pressRune(pressRune(m, 'm'), tc.key)
			if tc.key == 'd' {
				m.TextInput.SetValue(tc.date)
				updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				m = updated.(Model)
			}
			footer := ansi.Strip(m.helpView())
			if m.Err != nil || !strings.Contains(footer, "Moved to "+tc.want) || !strings.Contains(footer, "u undo") {
				t.Fatalf("unexpected move result: err=%v footer=%q", m.Err, footer)
			}
			saved, err := loadRaw(m.FilePath)
			if err != nil || !sameJSON(saved, m.Data) {
				t.Fatalf("move confirmation does not match saved data: %v", err)
			}
		})
	}
	m := newFeedbackTestModel(t)
	m = pressRune(pressRune(m, 'm'), '1')
	m = pressRune(m, '.')
	if m.Err != nil || len(m.Data[dayKey(1)]) != 2 || !strings.Contains(m.helpView(), "Moved to tomorrow") {
		t.Fatalf("repeat move did not confirm its destination: %v, %q", m.Err, m.helpView())
	}
}

func TestFeedbackFollowsUndoHistory(t *testing.T) {
	m := newFeedbackTestModel(t)
	m = pressRune(m, 'd')
	m = pressRune(pressRune(m, 'f'), 'f')
	if !strings.Contains(m.helpView(), "Task deleted") {
		t.Fatal("navigation lost the last action feedback")
	}
	m = pressSpace(m)
	if strings.Contains(m.helpView(), "Task deleted") || m.feedback != "" {
		t.Fatal("a new mutation retained stale delete feedback")
	}
	m = pressRune(m, 'd')
	updated, _ := m.Update(dataFileCheckedMsg{
		data:    TodoData{m.dateKeys[0]: {{ID: "external", Title: "New task"}}},
		modTime: m.dataModTime.Add(time.Second),
	})
	m = updated.(Model)
	if m.feedback != "" || strings.Contains(m.helpView(), "u undo") {
		t.Fatal("external reload retained feedback for invalidated undo history")
	}
}

func TestFailedActionsDoNotClaimSuccess(t *testing.T) {
	for _, action := range []rune{'d', 'm', 'u'} {
		m := newFeedbackTestModel(t)
		if action == 'u' {
			m = pressRune(m, 'd')
		}
		// A regular file as the parent gives a deterministic save failure.
		m.FilePath = filepath.Join(m.FilePath, "tasks.json")
		m.dataExists = false
		m = pressRune(m, action)
		if action == 'm' {
			m = pressRune(m, '1')
		}
		if m.Err == nil || m.feedback != "" || strings.Contains(m.helpView(), "u undo") || m.errorView() == "" {
			t.Fatalf("failed %q claimed success: err=%v footer=%q", action, m.Err, m.helpView())
		}
	}
	m := newFeedbackTestModel(t)
	external := cloneTodoData(m.Data)
	external[m.dateKeys[0]][0].Title = "Edited elsewhere"
	if err := external.Save(m.FilePath); err != nil {
		t.Fatal(err)
	}
	m = pressRune(m, 'd')
	if !errors.Is(m.Err, ErrDataConflict) || m.feedback != "" {
		t.Fatalf("conflict claimed success: err=%v feedback=%q", m.Err, m.feedback)
	}
}

func TestNoOpAndCancelledMovesDoNotClaimSuccess(t *testing.T) {
	m := newFeedbackTestModel(t)
	m.Data[m.dateKeys[0]][0].DueDate = m.dateKeys[0]
	m.persist()
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
