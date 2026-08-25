package model

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dtt101/doitdoit/styles"
)

func pressRune(m Model, r rune) Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	return updated.(Model)
}

func pressSpace(m Model) Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
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

func TestSpaceCompletesTaskAndGroupsCompletedTasksAtBottom(t *testing.T) {
	today := time.Now().Format(dateLayout)

	for _, tc := range []struct {
		name       string
		key        string
		showFuture bool
	}{
		{name: "dated list", key: today},
		{name: "Future list", key: "Future", showFuture: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := Model{
				Data: TodoData{tc.key: {
					{ID: "done", Title: "Already done", Completed: true},
					{ID: "target", Title: "Complete me"},
					{ID: "remaining", Title: "Still to do"},
				}},
				VisibleDays: 1,
				State:       Browsing,
				ShowFuture:  tc.showFuture,
				dateKeys:    []string{today},
				RowIdx:      1,
			}

			m = pressSpace(m)

			got := m.Data[tc.key]
			if len(got) != 3 || got[0].ID != "remaining" || got[1].ID != "done" || got[2].ID != "target" {
				t.Fatalf("expected unfinished task followed by completed tasks, got %v", got)
			}
			if !got[2].Completed {
				t.Fatal("expected space to mark the target task complete")
			}
		})
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
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

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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

func TestBrowsingHelpUsesCompactModalHint(t *testing.T) {
	mainHelp := (Model{State: Browsing}).helpView()
	if !strings.Contains(mainHelp, "help") || !strings.Contains(mainHelp, "?") {
		t.Fatalf("expected compact help hint, got %q", mainHelp)
	}
	if strings.Contains(mainHelp, "add") || strings.Contains(mainHelp, "navigate") || strings.Contains(mainHelp, "quit") {
		t.Fatalf("expected shortcut details to stay out of the footer, got %q", mainHelp)
	}

	mainModal := (Model{State: Browsing, width: 80}).helpModalView()
	if !strings.Contains(mainModal, "Future view") || !strings.Contains(mainModal, "arrows / hjkl") {
		t.Fatalf("expected main modal to describe Future toggle and full navigation, got %q", mainModal)
	}

	futureModal := (Model{State: Browsing, ShowFuture: true, width: 80}).helpModalView()
	if !strings.Contains(futureModal, "main view") || !strings.Contains(futureModal, "↑/↓ / k/j") {
		t.Fatalf("expected Future modal to describe main-view toggle and vertical navigation, got %q", futureModal)
	}
}

func TestMoveFooterShowsExactTargetsInFutureView(t *testing.T) {
	base := startOfDay(time.Now())
	for _, width := range []int{80, 48} {
		m := Model{State: ChoosingMoveDestination, ShowFuture: true, width: width}
		help := m.helpView()
		for days := 1; days <= 7; days++ {
			label := base.AddDate(0, 0, days).Format("Mon 02")
			if !strings.Contains(help, label) {
				t.Fatalf("expected Future move footer at width %d to contain %q, got %q", width, label, help)
			}
		}
		if !strings.Contains(help, "today") || !strings.Contains(help, "other date") || !strings.Contains(help, "future") {
			t.Fatalf("expected all move destinations in footer at width %d, got %q", width, help)
		}
		if got := lipgloss.Width(help); got > m.width-styles.AppStyle.GetHorizontalFrameSize() {
			t.Fatalf("move footer width = %d at terminal width %d, exceeds available width", got, width)
		}
	}

	m := Model{State: ChoosingMoveDestination, ShowFuture: true, width: 80}
	m.ShowHelp = false
	m = pressRune(m, '?')
	if m.ShowHelp {
		t.Fatal("expected move destinations to stay in the footer instead of opening the help modal")
	}
}

func TestHelpModalCapturesInputUntilClosed(t *testing.T) {
	today := time.Now().Format(dateLayout)
	m := Model{
		Data:        TodoData{today: {{ID: "1", Title: "Task"}}},
		VisibleDays: 1,
		State:       Browsing,
		dateKeys:    []string{today},
	}

	m = pressRune(m, '?')
	if !m.ShowHelp {
		t.Fatal("expected ? to open help")
	}
	m = pressRune(m, 'd')
	if !m.ShowHelp || len(m.Data[today]) != 1 {
		t.Fatal("expected help modal to capture task shortcuts")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if m.ShowHelp {
		t.Fatal("expected Esc to close help")
	}
}
