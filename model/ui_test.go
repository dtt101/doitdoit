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
