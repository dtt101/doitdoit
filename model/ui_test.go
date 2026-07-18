package model

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func pressRune(m Model, r rune) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	return updated.(Model)
}

func nextWeekday(after time.Time, weekday time.Weekday) time.Time {
	date := startOfDay(after).AddDate(0, 0, 1)
	for date.Weekday() != weekday {
		date = date.AddDate(0, 0, 1)
	}
	return date
}

func TestMovePickerMovesFridayTaskToMonday(t *testing.T) {
	friday := nextWeekday(time.Now(), time.Friday)
	monday := friday.AddDate(0, 0, 3)
	fridayKey := friday.Format(dateLayout)
	mondayKey := monday.Format(dateLayout)
	m := Model{
		Data:        TodoData{fridayKey: {{ID: "1", Title: "Task 1"}}},
		VisibleDays: 14,
		State:       Browsing,
		dateKeys:    []string{fridayKey},
	}

	m = pressRune(m, 'm')
	if m.State != ChoosingMoveDestination {
		t.Fatalf("expected destination picker, got %v", m.State)
	}
	m = pressRune(m, '3')

	if m.State != Browsing {
		t.Fatalf("expected move to return to browsing, got %v", m.State)
	}
	if got := m.Data[mondayKey]; len(got) != 1 || got[0].DueDate != mondayKey {
		t.Fatalf("expected task on Monday with matching due date, got %v", got)
	}
}

func TestMovePickerFutureOffsetUsesToday(t *testing.T) {
	tomorrow := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	m := Model{
		Data:        TodoData{"Future": {{ID: "1", Title: "Task 1"}}},
		VisibleDays: 3,
		State:       Browsing,
		ShowFuture:  true,
	}

	m = pressRune(pressRune(m, 'm'), '1')

	if got := m.Data[tomorrow]; len(got) != 1 || got[0].DueDate != tomorrow {
		t.Fatalf("expected Future task tomorrow, got %v", got)
	}
	if !m.ShowFuture || m.RowIdx != 0 {
		t.Fatalf("expected focus to remain in Future, got show=%v row=%d", m.ShowFuture, m.RowIdx)
	}
}

func TestMoveBeyondVisibleWindowIsHeldInFuture(t *testing.T) {
	today := time.Now().Format(dateLayout)
	target := time.Now().AddDate(0, 0, 7).Format(dateLayout)
	m := Model{
		Data:        TodoData{today: {{ID: "1", Title: "Task 1"}}},
		VisibleDays: 3,
		State:       Browsing,
		dateKeys:    []string{today},
	}

	m = pressRune(pressRune(m, 'm'), '7')

	if got := m.Data["Future"]; len(got) != 1 || got[0].DueDate != target {
		t.Fatalf("expected dated task held in Future, got %v", got)
	}
}

func TestMoveToFutureClearsDueDate(t *testing.T) {
	today := time.Now().Format(dateLayout)
	m := Model{
		Data:        TodoData{today: {{ID: "1", Title: "Task 1", DueDate: today}}},
		VisibleDays: 3,
		State:       Browsing,
		dateKeys:    []string{today},
	}

	m = pressRune(pressRune(m, 'm'), 'f')

	if got := m.Data["Future"]; len(got) != 1 || got[0].DueDate != "" {
		t.Fatalf("expected undated Future task, got %v", got)
	}
}

func TestRepeatMoveUsesAbsoluteDestination(t *testing.T) {
	today := time.Now().Format(dateLayout)
	target := time.Now().AddDate(0, 0, 2).Format(dateLayout)
	m := Model{
		Data: TodoData{today: {
			{ID: "1", Title: "Task 1"},
			{ID: "2", Title: "Task 2"},
		}},
		VisibleDays: 3,
		State:       Browsing,
		dateKeys:    []string{today},
	}

	m = pressRune(pressRune(m, 'm'), '2')
	m = pressRune(m, '.')

	if got := m.Data[target]; len(got) != 2 || got[0].ID != "1" || got[1].ID != "2" {
		t.Fatalf("expected both tasks at the repeated destination, got %v", got)
	}
}

func TestUndoRestoresLastMoveAndFocus(t *testing.T) {
	today := time.Now().Format(dateLayout)
	tomorrow := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	m := Model{
		Data: TodoData{today: {
			{ID: "1", Title: "Task 1"},
			{ID: "2", Title: "Task 2"},
		}},
		VisibleDays: 3,
		State:       Browsing,
		dateKeys:    []string{today},
		RowIdx:      1,
	}

	m = pressRune(pressRune(m, 'm'), '1')
	m = pressRune(m, 'u')

	if got := m.Data[today]; len(got) != 2 || got[1].ID != "2" {
		t.Fatalf("expected original source ordering, got %v", got)
	}
	if len(m.Data[tomorrow]) != 0 || m.RowIdx != 1 || m.moveUndo != nil {
		t.Fatalf("expected destination cleared and focus restored, row=%d undo=%v", m.RowIdx, m.moveUndo)
	}
	if m.lastMoveTarget == nil || m.lastMoveTarget.Date != tomorrow {
		t.Fatal("expected undo to preserve the repeat destination")
	}
}

func TestReorderAndUndo(t *testing.T) {
	today := time.Now().Format(dateLayout)
	m := Model{
		Data: TodoData{today: {
			{ID: "1", Title: "Task 1"},
			{ID: "2", Title: "Task 2"},
		}},
		VisibleDays: 1,
		State:       Browsing,
		dateKeys:    []string{today},
	}

	m = pressRune(m, 'J')
	if got := m.Data[today]; got[0].ID != "2" || m.RowIdx != 1 {
		t.Fatalf("expected J to reorder down, got %v at row %d", got, m.RowIdx)
	}
	m = pressRune(m, 'u')
	if got := m.Data[today]; got[0].ID != "1" || m.RowIdx != 0 {
		t.Fatalf("expected undo to restore ordering, got %v at row %d", got, m.RowIdx)
	}
}

func TestCompletedTasksStayBelowMovedIncompleteTask(t *testing.T) {
	today := time.Now().Format(dateLayout)
	tomorrow := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	m := Model{
		Data: TodoData{
			today:    {{ID: "move", Title: "Move"}},
			tomorrow: {{ID: "done", Title: "Done", Completed: true}},
		},
		VisibleDays: 3,
		State:       Browsing,
		dateKeys:    []string{today},
	}

	m = pressRune(pressRune(m, 'm'), '1')
	if got := m.Data[tomorrow]; len(got) != 2 || got[0].ID != "move" || got[1].ID != "done" {
		t.Fatalf("expected moved task above completed task, got %v", got)
	}
}

func TestLegacyMoveAliasesDoNothing(t *testing.T) {
	today := time.Now().Format(dateLayout)
	for _, showFuture := range []bool{false, true} {
		for _, key := range []rune{'>', 't', 'T'} {
			data := TodoData{
				today:    {{ID: "dated", Title: "Dated"}},
				"Future": {{ID: "future", Title: "Future"}},
			}
			m := Model{Data: data, VisibleDays: 3, State: Browsing, ShowFuture: showFuture, dateKeys: []string{today}}
			m = pressRune(m, key)
			if len(m.Data[today]) != 1 || len(m.Data["Future"]) != 1 || m.State != Browsing {
				t.Fatalf("expected legacy key %q to do nothing in Future=%v", key, showFuture)
			}
		}
	}
}

func TestSameDestinationClosesPickerWithoutChangingHistory(t *testing.T) {
	today := time.Now().Format(dateLayout)
	m := Model{
		Data:        TodoData{today: {{ID: "1", Title: "Task 1", DueDate: today}}},
		VisibleDays: 1,
		State:       Browsing,
		dateKeys:    []string{today},
	}

	m = pressRune(pressRune(m, 'm'), 't')
	if m.State != Browsing || m.moveUndo != nil || m.lastMoveTarget != nil {
		t.Fatalf("expected same-date target to close without history, got state=%v", m.State)
	}
}

func TestMoveDateEscapeReturnsToPicker(t *testing.T) {
	today := time.Now().Format(dateLayout)
	m := Model{
		Data:        TodoData{today: {{ID: "1", Title: "Task 1"}}},
		VisibleDays: 1,
		State:       Browsing,
		TextInput:   textinput.New(),
		dateKeys:    []string{today},
	}
	m = pressRune(pressRune(m, 'm'), 'd')

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.State != ChoosingMoveDestination {
		t.Fatalf("expected Esc to return to picker, got %v", m.State)
	}
}

func TestMoveDateInputSchedulesExactDate(t *testing.T) {
	today := time.Now().Format(dateLayout)
	target := time.Now().AddDate(0, 0, 10).Format(dateLayout)
	m := Model{
		Data:        TodoData{today: {{ID: "1", Title: "Task 1"}}},
		FilePath:    filepath.Join(t.TempDir(), "tasks.json"),
		VisibleDays: 3,
		State:       Browsing,
		TextInput:   textinput.New(),
		dateKeys:    []string{today},
	}
	m = pressRune(pressRune(m, 'm'), 'd')
	m.TextInput.SetValue(target)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.State != Browsing || m.Err != nil {
		t.Fatalf("expected valid date to close cleanly, got state=%v err=%v", m.State, m.Err)
	}
	if got := m.Data["Future"]; len(got) != 1 || got[0].DueDate != target {
		t.Fatalf("expected exact date held in Future, got %v", got)
	}
	if m.lastMoveTarget == nil || m.lastMoveTarget.Date != target {
		t.Fatalf("expected exact date to become repeat target, got %v", m.lastMoveTarget)
	}
}

func TestNonMoveMutationClearsUndo(t *testing.T) {
	today := time.Now().Format(dateLayout)
	m := Model{
		Data:        TodoData{today: {{ID: "1", Title: "Task 1"}, {ID: "2", Title: "Task 2"}}},
		VisibleDays: 2,
		State:       Browsing,
		dateKeys:    []string{today},
	}
	m = pressRune(pressRune(m, 'm'), '1')
	if m.moveUndo == nil {
		t.Fatal("expected move to create undo history")
	}
	m = pressRune(m, ' ')
	if m.moveUndo != nil {
		t.Fatal("expected task mutation to clear move undo history")
	}
}

func TestMoveAndRepeatNoOpWithoutTaskOrTarget(t *testing.T) {
	today := time.Now().Format(dateLayout)
	m := Model{Data: TodoData{today: {}}, VisibleDays: 1, State: Browsing, dateKeys: []string{today}}
	m = pressRune(m, 'm')
	if m.State != Browsing || m.moveUndo != nil || m.lastMoveTarget != nil {
		t.Fatalf("expected empty move to be a no-op, got state=%v", m.State)
	}
	m = pressRune(m, '.')
	if m.moveUndo != nil || m.lastMoveTarget != nil {
		t.Fatal("expected repeat without a destination to be a no-op")
	}
}

func TestBrowsingHelpIsSpecificToMainAndFutureViews(t *testing.T) {
	mainHelp := (Model{State: Browsing}).helpView()
	if !strings.Contains(mainHelp, "future") || !strings.Contains(mainHelp, "arrows/hjkl") {
		t.Fatalf("expected main help to describe Future toggle and full navigation, got %q", mainHelp)
	}
	if strings.Contains(mainHelp, "postpone") || strings.Contains(mainHelp, "to today") {
		t.Fatalf("expected main help not to mention removed shortcuts, got %q", mainHelp)
	}

	futureHelp := (Model{State: Browsing, ShowFuture: true}).helpView()
	if !strings.Contains(futureHelp, "main view") || !strings.Contains(futureHelp, "↑/↓/k/j") {
		t.Fatalf("expected Future help to describe main-view toggle and vertical navigation, got %q", futureHelp)
	}
	if strings.Contains(futureHelp, "arrows/hjkl") || strings.Contains(futureHelp, "to today") {
		t.Fatalf("expected Future help not to show main-only or removed shortcuts, got %q", futureHelp)
	}
}

func TestMoveHelpShowsExactTargetsInFutureView(t *testing.T) {
	base := startOfDay(time.Now())
	help := (Model{State: ChoosingMoveDestination, ShowFuture: true}).helpView()
	for days := 1; days <= 7; days++ {
		label := base.AddDate(0, 0, days).Format("Mon 02")
		if !strings.Contains(help, label) {
			t.Fatalf("expected Future move help to contain %q, got %q", label, help)
		}
	}
	if !strings.Contains(help, "today") || !strings.Contains(help, "other date") || !strings.Contains(help, "future") {
		t.Fatalf("expected all move destinations in help, got %q", help)
	}
}
