package model

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDateTickRollsOverAtMidnight(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	m := Model{
		Data:        make(TodoData),
		FilePath:    filepath.Join(t.TempDir(), "tasks.json"),
		VisibleDays: 3,
		State:       Browsing,
		// Stale keys, as if the app were opened yesterday and left running.
		dateKeys: []string{yesterday},
	}
	m.Data[yesterday] = []Task{
		{ID: "1", Title: "Carry me over", Completed: false},
		{ID: "2", Title: "Done already", Completed: true},
	}

	newM, cmd := m.Update(dateTickMsg(time.Now()))
	m = newM.(Model)

	if cmd == nil {
		t.Error("expected the tick to reschedule itself")
	}
	if m.dateKeys[0] != today {
		t.Errorf("dateKeys[0] = %q, want today %q", m.dateKeys[0], today)
	}

	todayTasks := m.Data[today]
	if len(todayTasks) != 1 {
		t.Fatalf("expected 1 task rolled over to today, got %d", len(todayTasks))
	}
	if todayTasks[0].ID != "1" {
		t.Errorf("rolled task ID = %q, want %q", todayTasks[0].ID, "1")
	}
	// The completed task stays put; only the incomplete one rolls over.
	if got := m.Data[yesterday]; len(got) != 1 || got[0].ID != "2" {
		t.Errorf("expected only completed task to remain under yesterday, got %v", got)
	}
}

func TestDateTickNoChangeSameDay(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	m := Model{
		Data:        TodoData{today: {{ID: "1", Title: "Task"}}},
		FilePath:    filepath.Join(t.TempDir(), "tasks.json"),
		VisibleDays: 3,
		State:       Browsing,
	}
	m.updateDateKeys()

	newM, cmd := m.Update(dateTickMsg(time.Now()))
	m = newM.(Model)

	if cmd == nil {
		t.Error("expected the tick to reschedule itself")
	}
	// No persist should have been needed, so no error recorded.
	if m.Err != nil {
		t.Errorf("unexpected error on same-day tick: %v", m.Err)
	}
	if len(m.Data[today]) != 1 {
		t.Errorf("expected today's task to be untouched, got %d", len(m.Data[today]))
	}
}

func TestDateTickPreservesFocusedDateAfterRollover(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	m := Model{
		Data:        TodoData{today: {{ID: "1", Title: "Keep focus here"}}},
		FilePath:    filepath.Join(t.TempDir(), "tasks.json"),
		VisibleDays: 3,
		State:       Adding,
		ColIdx:      1,
		dateKeys:    []string{yesterday, today, tomorrow},
	}

	newM, _ := m.Update(dateTickMsg(time.Now()))
	m = newM.(Model)

	if m.dateKeys[m.ColIdx] != today {
		t.Errorf("focused date = %q, want %q", m.dateKeys[m.ColIdx], today)
	}
}
