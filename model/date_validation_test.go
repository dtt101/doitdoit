package model

import (
	"fmt"
	"testing"
	"time"
)

func TestSetTaskDateRejectsInvalidDate(t *testing.T) {
	m := Model{
		Data:        TodoData{"Future": {{ID: "1", Title: "Task"}}},
		VisibleDays: 3,
		ShowFuture:  true,
		RowIdx:      0,
	}

	if err := m.setTaskDate("24-11"); err == nil {
		t.Fatalf("expected invalid date to return an error")
	}

	if m.Data["Future"][0].DueDate != "" {
		t.Fatalf("expected due date to remain unset after invalid input, got %q", m.Data["Future"][0].DueDate)
	}

	if m.Err == nil {
		t.Fatalf("expected validation error to be recorded in model.Err")
	}
}

func TestSetTaskDateNormalizesValidDate(t *testing.T) {
	visibleDays := 370
	m := Model{
		Data:        TodoData{"Future": {{ID: "1", Title: "Task"}}},
		VisibleDays: visibleDays,
		ShowFuture:  true,
		RowIdx:      0,
	}

	if err := m.setTaskDate("12-31"); err != nil {
		t.Fatalf("expected valid date to be accepted, got %v", err)
	}

	expectedDate := fmt.Sprintf("%04d-12-31", time.Now().Year())

	if m.Err != nil {
		t.Fatalf("expected validation error to be cleared, got %v", m.Err)
	}

	if len(m.Data["Future"]) != 0 {
		t.Fatalf("expected task to leave Future after scheduling, still have %d entries", len(m.Data["Future"]))
	}

	tasksOnDate := m.Data[expectedDate]
	if len(tasksOnDate) != 1 {
		t.Fatalf("expected task to be scheduled on %s, found %d tasks", expectedDate, len(tasksOnDate))
	}

	if tasksOnDate[0].DueDate != expectedDate {
		t.Fatalf("expected due date %s, got %s", expectedDate, tasksOnDate[0].DueDate)
	}

	if m.ShowFuture {
		t.Fatalf("expected model to exit Future view after scheduling")
	}
}
