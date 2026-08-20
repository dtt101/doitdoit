package model

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func TestSetTaskDateRejectsInvalidDate(t *testing.T) {
	m := Model{
		Data:        TodoData{"Future": {{ID: "1", Title: "Task"}}},
		VisibleDays: 3,
		ShowFuture:  true,
		RowIdx:      0,
		State:       SettingMoveDate,
		TextInput:   textinput.New(),
	}
	m.TextInput.SetValue("24-11")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if m.Data["Future"][0].DueDate != "" {
		t.Fatalf("expected due date to remain unset after invalid input, got %q", m.Data["Future"][0].DueDate)
	}

	if m.Err == nil {
		t.Fatalf("expected validation error to be recorded in model.Err")
	}
	if m.State != SettingMoveDate {
		t.Fatalf("expected invalid input to remain open, got state %v", m.State)
	}
}

func TestSetTaskDateNormalizesValidDate(t *testing.T) {
	tomorrow := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	m := Model{
		Data:        TodoData{"Future": {{ID: "1", Title: "Task"}}},
		VisibleDays: 3,
		ShowFuture:  true,
		RowIdx:      0,
	}

	if !m.scheduleTask(moveTarget{Date: tomorrow}) {
		t.Fatal("expected valid date to schedule the task")
	}

	if m.Err != nil {
		t.Fatalf("expected validation error to be cleared, got %v", m.Err)
	}

	if len(m.Data["Future"]) != 0 {
		t.Fatalf("expected task to leave Future after scheduling, still have %d entries", len(m.Data["Future"]))
	}

	tasksOnDate := m.Data[tomorrow]
	if len(tasksOnDate) != 1 {
		t.Fatalf("expected task to be scheduled on %s, found %d tasks", tomorrow, len(tasksOnDate))
	}

	if tasksOnDate[0].DueDate != tomorrow {
		t.Fatalf("expected due date %s, got %s", tomorrow, tasksOnDate[0].DueDate)
	}

	if !m.ShowFuture {
		t.Fatalf("expected scheduling to retain source view focus")
	}
}

func TestMoveDateNormalizesPastDateToToday(t *testing.T) {
	today := time.Now().Format(dateLayout)
	yesterday := time.Now().AddDate(0, 0, -1).Format(dateLayout)
	m := Model{
		Data:        TodoData{"Future": {{ID: "1", Title: "Task"}}},
		VisibleDays: 3,
		ShowFuture:  true,
		RowIdx:      0,
	}

	if !m.scheduleTask(moveTarget{Date: yesterday}) {
		t.Fatal("expected past date to schedule the task")
	}
	if got := m.Data[today]; len(got) != 1 || got[0].DueDate != today {
		t.Fatalf("expected past target normalized to today, got %v", got)
	}
}
