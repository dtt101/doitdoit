package model

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMoveToFuture(t *testing.T) {
	// Setup
	today := time.Now().Format("2006-01-02")
	m := Model{
		Data:        make(TodoData),
		VisibleDays: 3,
		State:       Browsing,
		dateKeys:    []string{today},
	}
	m.Data[today] = []Task{
		{ID: "1", Title: "Task 1", Completed: false},
	}
	m.Data["Future"] = []Task{}

	// Enter Move Mode
	m.State = Moving
	m.ColIdx = 0
	m.RowIdx = 0

	// Simulate 'f' key press
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}
	newM, _ := m.Update(msg)
	m = newM.(Model)

	// Verify
	// 1. Task should be gone from today
	if len(m.Data[today]) != 0 {
		t.Errorf("Expected 0 tasks in today, got %d", len(m.Data[today]))
	}

	// 2. Task should be in Future
	if len(m.Data["Future"]) != 1 {
		t.Errorf("Expected 1 task in Future, got %d", len(m.Data["Future"]))
	}
	if m.Data["Future"][0].Title != "Task 1" {
		t.Errorf("Expected task title 'Task 1', got '%s'", m.Data["Future"][0].Title)
	}

	// 3. State should be Browsing
	if m.State != Browsing {
		t.Errorf("Expected state Browsing, got %v", m.State)
	}
}

func TestPostponeMovesTaskToNextDay(t *testing.T) {
	today := time.Now()
	todayStr := today.Format("2006-01-02")
	tomorrowStr := today.AddDate(0, 0, 1).Format("2006-01-02")

	m := Model{
		Data: TodoData{
			todayStr: {
				{ID: "1", Title: "Task 1"},
				{ID: "2", Title: "Task 2"},
			},
		},
		VisibleDays: 3,
		State:       Browsing,
	}
	m.updateDateKeys()
	m.ColIdx = 0
	m.RowIdx = 0

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'>'}}
	newM, _ := m.Update(msg)
	m = newM.(Model)

	if got := len(m.Data[todayStr]); got != 1 {
		t.Fatalf("Expected 1 task remaining today, got %d", got)
	}
	if m.Data[todayStr][0].ID != "2" {
		t.Errorf("Expected Task 2 to remain today, got %s", m.Data[todayStr][0].ID)
	}

	tomorrow := m.Data[tomorrowStr]
	if len(tomorrow) != 1 || tomorrow[0].ID != "1" {
		t.Fatalf("Expected Task 1 on tomorrow, got %v", tomorrow)
	}
	if tomorrow[0].DueDate != tomorrowStr {
		t.Errorf("Expected due date %s, got %s", tomorrowStr, tomorrow[0].DueDate)
	}
}

func TestPostponeFromLastColumnHoldsInFuture(t *testing.T) {
	today := time.Now()
	lastVisible := today.AddDate(0, 0, 2) // VisibleDays = 3, so index 2 is the last column
	lastVisibleStr := lastVisible.Format("2006-01-02")
	beyondStr := lastVisible.AddDate(0, 0, 1).Format("2006-01-02")

	m := Model{
		Data: TodoData{
			lastVisibleStr: {
				{ID: "1", Title: "Task 1"},
			},
		},
		VisibleDays: 3,
		State:       Browsing,
	}
	m.updateDateKeys()
	m.ColIdx = 2
	m.RowIdx = 0

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'>'}}
	newM, _ := m.Update(msg)
	m = newM.(Model)

	if got := len(m.Data[lastVisibleStr]); got != 0 {
		t.Errorf("Expected task removed from last column, got %d", got)
	}

	future := m.Data["Future"]
	if len(future) != 1 || future[0].ID != "1" {
		t.Fatalf("Expected task held in Future, got %v", future)
	}
	if future[0].DueDate != beyondStr {
		t.Errorf("Expected due date %s, got %s", beyondStr, future[0].DueDate)
	}
}

func TestPostponeNoOpInFutureView(t *testing.T) {
	m := Model{
		Data: TodoData{
			"Future": {{ID: "1", Title: "Task 1"}},
		},
		VisibleDays: 3,
		State:       Browsing,
		ShowFuture:  true,
	}
	m.updateDateKeys()
	m.RowIdx = 0

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'>'}}
	newM, _ := m.Update(msg)
	m = newM.(Model)

	if len(m.Data["Future"]) != 1 {
		t.Errorf("Expected Future task untouched, got %d tasks", len(m.Data["Future"]))
	}
}

func TestFutureShortcutMoveToToday(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	m := Model{
		Data: TodoData{
			"Future": {
				{ID: "F1", Title: "Task 1"},
				{ID: "F2", Title: "Task 2"},
			},
		},
		VisibleDays: 3,
		State:       Browsing,
		ShowFuture:  true,
	}
	m.updateDateKeys()
	m.RowIdx = 0

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}}
	newM, _ := m.Update(msg)
	m = newM.(Model)

	if m.ShowFuture {
		t.Errorf("Expected to leave future view after moving, still showing future")
	}
	if m.ColIdx != 0 || m.RowIdx != 0 {
		t.Errorf("Expected focus on today's first task, got col %d row %d", m.ColIdx, m.RowIdx)
	}

	todayTasks := m.Data[today]
	if len(todayTasks) != 1 {
		t.Fatalf("Expected 1 task in today, got %d", len(todayTasks))
	}
	if todayTasks[0].ID != "F1" {
		t.Errorf("Expected task F1 moved to today, got %s", todayTasks[0].ID)
	}
	if todayTasks[0].DueDate != today {
		t.Errorf("Expected moved task due date %s, got %s", today, todayTasks[0].DueDate)
	}

	futureTasks := m.Data["Future"]
	if len(futureTasks) != 1 || futureTasks[0].ID != "F2" {
		t.Errorf("Expected remaining future task F2, got %v", futureTasks)
	}
}
