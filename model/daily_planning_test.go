package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func futurePlanningModel(t *testing.T) Model {
	t.Helper()
	m := newFeedbackTestModel(t)
	m.Data["Future"] = []Task{
		{ID: "scheduled-a", Title: "Invalid A", DueDate: "invalid-a"},
		{ID: "idea-a", Title: "Idea A"},
		{ID: "scheduled-b", Title: "Invalid B", DueDate: "invalid-b"},
		{ID: "idea-b", Title: "Idea B"},
		{ID: "done", Title: "Finished idea", Completed: true},
	}
	return pressRune(m, 'f')
}

func TestFutureGroupingDoesNotChangeOrder(t *testing.T) {
	m := futurePlanningModel(t)
	before := cloneTodoData(m.Data)
	m = resizeModel(m, 80, 32)
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "Ideas (undated)") {
		t.Fatal("Future should not repeat an undated heading")
	}
	ideas, scheduled, completed := strings.Index(view, "Idea A"), strings.Index(view, "Invalid dates 2"), strings.Index(view, "Completed 1")
	if ideas < 0 || scheduled <= ideas || completed <= scheduled || !strings.Contains(view, "4 remaining") {
		t.Fatalf("Future groups missing or out of order:\n%s", view)
	}
	if !strings.Contains(view, "(invalid-a)") || !strings.Contains(view, "[ ] Idea A") {
		t.Fatalf("Future lost date labels or initial selection:\n%s", view)
	}
	for _, id := range []string{"idea-a", "idea-b", "scheduled-a", "scheduled-b", "done"} {
		if m.Data["Future"][m.RowIdx].ID != id {
			t.Fatalf("navigation selected %q, want %q", m.Data["Future"][m.RowIdx].ID, id)
		}
		m = pressRune(m, 'j')
	}
	m = pressRune(m, 'c')
	if !sameJSON(before, m.Data) || m.Data["Future"][0].ID != "scheduled-a" {
		t.Fatal("presentation changed Future order")
	}
}

func TestFutureActionsUseDisplayedTaskAndSection(t *testing.T) {
	m := futurePlanningModel(t)
	original := cloneTodoData(m.Data)
	m = pressRune(m, 'd')
	for _, task := range m.Data["Future"] {
		if task.ID == "idea-a" {
			t.Fatal("delete did not remove the displayed idea")
		}
	}
	m = pressRune(m, 'u')
	if !sameJSON(original, m.Data) || m.Data["Future"][m.RowIdx].ID != "idea-a" {
		t.Fatal("undo did not restore the selected idea and original order")
	}
	m = pressRune(m, 'J')
	if m.Data["Future"][1].ID != "idea-b" || m.Data["Future"][3].ID != "idea-a" || m.Data["Future"][0].ID != "scheduled-a" {
		t.Fatal("reorder did not stay within the Ideas section")
	}
	before := cloneTodoData(m.Data)
	m = pressRune(m, 'J')
	if !sameJSON(before, m.Data) {
		t.Fatal("reorder crossed into Invalid dates")
	}
	m = pressRune(m, 'j')
	m = pressRune(pressRune(m, 'm'), '1')
	if tasks := m.Data[dayKey(1)]; len(tasks) != 1 || tasks[0].ID != "scheduled-a" {
		t.Fatal("move targeted the wrong displayed Future task")
	}
}

func TestPlanningGuidanceFitsSmallWindows(t *testing.T) {
	m := futurePlanningModel(t)
	for _, size := range [][2]int{{24, 10}, {24, 16}, {40, 12}, {48, 16}, {80, 24}} {
		m = resizeModel(m, size[0], size[1])
		m.feedback = "Moved to " + dayKey(10)
		m.moveUndo = &moveUndoSnapshot{}
		assertFitsTerminal(t, m)
		m = pressRune(m, '?')
		assertFitsTerminal(t, m)
		m.helpOffset = m.helpMaxOffset()
		assertFitsTerminal(t, m)
		m.ShowHelp = false
	}
}

func TestFutureDateGuidanceLeadsToScheduling(t *testing.T) {
	m := resizeModel(futurePlanningModel(t), 80, 24)
	if !strings.Contains(ansi.Strip(m.helpView()), "m move/date") {
		t.Fatal("Future footer does not expose date setting")
	}
	m = pressRune(pressRune(m, 'm'), 'd')
	if m.State != SettingMoveDate {
		t.Fatal("documented sequence did not open date input")
	}
	date := dayKey(20)
	m.TextInput.SetValue(date)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	for _, task := range m.Data["Future"] {
		if task.ID == "idea-a" {
			t.Fatal("scheduled idea remained in Future")
		}
	}
	for _, task := range m.Data[date] {
		if task.ID == "idea-a" && task.DueDate == date {
			return
		}
	}
	t.Fatal("date entry did not schedule the selected Future idea")
}
