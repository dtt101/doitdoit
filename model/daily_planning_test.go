package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func futurePlanningModel(t *testing.T) Model {
	t.Helper()
	m := newFeedbackTestModel(t)
	m.Data["Future"] = []Task{
		{ID: "scheduled-a", Title: "Scheduled A", DueDate: dayKey(10)},
		{ID: "idea-a", Title: "Idea A"},
		{ID: "scheduled-b", Title: "Scheduled B", DueDate: dayKey(12)},
		{ID: "idea-b", Title: "Idea B"},
		{ID: "done", Title: "Finished idea", Completed: true},
	}
	m.persist()
	return pressRune(m, 'f')
}

func TestFutureGroupingDoesNotChangeStorage(t *testing.T) {
	m := futurePlanningModel(t)
	before, _ := os.ReadFile(m.FilePath)
	m = resizeModel(m, 80, 32)
	view := ansi.Strip(m.View().Content)
	ideas, scheduled, completed := strings.Index(view, "Ideas (undated) 2"), strings.Index(view, "Scheduled 2"), strings.Index(view, "Completed 1")
	if ideas < 0 || scheduled <= ideas || completed <= scheduled || !strings.Contains(view, "4 remaining") {
		t.Fatalf("Future groups missing or out of order:\n%s", view)
	}
	if !strings.Contains(view, "("+dayKey(10)+")") || !strings.Contains(view, "> [ ] Idea A") {
		t.Fatalf("Future lost date labels or initial selection:\n%s", view)
	}
	for _, id := range []string{"idea-a", "idea-b", "scheduled-a", "scheduled-b", "done"} {
		if m.Data["Future"][m.RowIdx].ID != id {
			t.Fatalf("navigation selected %q, want %q", m.Data["Future"][m.RowIdx].ID, id)
		}
		m = pressRune(m, 'j')
	}
	m = pressRune(m, 'c')
	after, _ := os.ReadFile(m.FilePath)
	if string(before) != string(after) || m.Data["Future"][0].ID != "scheduled-a" {
		t.Fatal("presentation changed Future storage order")
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
		t.Fatal("reorder crossed into Scheduled")
	}
	m = pressRune(m, 'j')
	m = pressRune(pressRune(m, 'm'), '1')
	if tasks := m.Data[dayKey(1)]; len(tasks) != 1 || tasks[0].ID != "scheduled-a" {
		t.Fatal("move targeted the wrong displayed Future task")
	}
}

func TestRolloverNoticeComesFromActualRollover(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	before := TodoData{
		dayKey(-1): {{ID: "carry-a", Title: "Carry A"}, {ID: "completed", Title: "Completed", Completed: true}},
		dayKey(-3): {{ID: "carry-b", Title: "Carry B"}},
		dayKey(0):  {{ID: "existing", Title: "Already planned", CreatedAt: time.Now().AddDate(0, 0, -10)}},
		"Future":   {{ID: "idea", Title: "An old idea", CreatedAt: time.Now().AddDate(0, 0, -10)}},
	}
	if err := before.Save(path); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(path)
	m, err := NewModel(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	m = resizeModel(m, 80, 24)
	if m.carriedForward != 2 || !strings.Contains(ansi.Strip(m.View().Content), "2 tasks carried forward to Today.") {
		t.Fatalf("incorrect rollover explanation: count=%d", m.carriedForward)
	}
	if len(m.Data[dayKey(0)]) != 3 || len(m.Data[dayKey(-1)]) != 1 || !m.Data[dayKey(-1)][0].Completed {
		t.Fatal("rollover semantics changed")
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || string(backup) != string(original) {
		t.Fatal("load did not preserve the pre-rollover backup")
	}
	saved, _ := os.ReadFile(path)
	if strings.Contains(string(saved), "carried") {
		t.Fatal("rollover metadata leaked into task JSON")
	}
	second, err := NewModel(path, 3)
	if err != nil || second.carriedForward != 0 {
		t.Fatal("already rolled tasks were reported as a fresh rollover")
	}
}

func TestRolloverNoticeOnDateTickAndExternalReload(t *testing.T) {
	m := newFeedbackTestModel(t)
	m.Data[dayKey(-1)] = []Task{{ID: "carry", Title: "Carry"}}
	m.todayKey = dayKey(-1)
	updated, _ := m.Update(dateTickMsg(time.Now()))
	m = updated.(Model)
	if m.carriedForward != 1 || !strings.Contains(ansi.Strip(m.View().Content), "1 task carried forward to Today.") {
		t.Fatal("midnight rollover was not explained")
	}
	updated, _ = m.Update(dataFileCheckedMsg{
		data:    TodoData{dayKey(-2): {{ID: "external", Title: "External"}, {ID: "external-2", Title: "External 2"}}},
		modTime: m.dataModTime.Add(time.Second),
	})
	m = updated.(Model)
	if m.carriedForward != 2 || len(m.Data[dayKey(0)]) != 2 {
		t.Fatal("external rollover was not explained")
	}
	m.todayKey = dayKey(-1)
	updated, _ = m.Update(dateTickMsg(time.Now()))
	m = updated.(Model)
	if m.carriedForward != 0 {
		t.Fatal("notice survived into a day with no rollover")
	}
}

func TestPlanningGuidanceFitsSmallWindows(t *testing.T) {
	m := futurePlanningModel(t)
	m.carriedForward = 3
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
	saved, err := Load(m.FilePath, m.RetentionDays)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range saved["Future"] {
		if task.ID == "idea-a" && task.DueDate == date {
			return
		}
	}
	t.Fatal("date entry did not save the selected Future idea's schedule")
}
