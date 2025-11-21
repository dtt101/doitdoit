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
