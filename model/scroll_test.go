package model

import (
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func pressBrowsingKey(t *testing.T, m Model, key rune) Model {
	t.Helper()
	updated, _ := m.handleBrowsingKey(tea.KeyPressMsg{Code: key, Text: string(key)})
	return updated.(Model)
}

func TestRightNavigationScrollsViewportAndLoadsDueTasks(t *testing.T) {
	today := dayKey(0)
	dueDate := dayKey(3)
	m := Model{
		Data:        TodoData{"Future": {{ID: "due", Title: "Due later", DueDate: dueDate}}},
		FilePath:    filepath.Join(t.TempDir(), "tasks.json"),
		VisibleDays: 3,
		State:       Browsing,
		todayKey:    today,
	}
	m.updateDateKeysFrom(startOfDayNow())

	m = pressBrowsingKey(t, m, 'l')
	m = pressBrowsingKey(t, m, 'l')
	if m.dateKeys[0] != today || m.ColIdx != 2 {
		t.Fatalf("before edge scroll, keys=%v col=%d", m.dateKeys, m.ColIdx)
	}

	m = pressBrowsingKey(t, m, 'l')
	if got := m.dateKeys; got[0] != dayKey(1) || got[2] != dueDate {
		t.Fatalf("after edge scroll, keys=%v, want %s through %s", got, dayKey(1), dueDate)
	}
	if m.ColIdx != 2 {
		t.Fatalf("focus column = %d, want right edge 2", m.ColIdx)
	}
	if got := m.Data[dueDate]; len(got) != 1 || got[0].ID != "due" {
		t.Fatalf("due-date tasks = %v, want task loaded from Future", got)
	}
	if len(m.Data["Future"]) != 0 {
		t.Fatalf("Future = %v, want dated task removed", m.Data["Future"])
	}
}

func TestLeftNavigationScrollsBackButStopsAtToday(t *testing.T) {
	m := Model{Data: TodoData{}, VisibleDays: 3, State: Browsing, todayKey: dayKey(0)}
	m.updateDateKeysFrom(startOfDayNow().AddDate(0, 0, 2))
	m.ColIdx = 0

	m = pressBrowsingKey(t, m, 'h')
	if m.dateKeys[0] != dayKey(1) || m.ColIdx != 0 {
		t.Fatalf("first left scroll: keys=%v col=%d", m.dateKeys, m.ColIdx)
	}
	m = pressBrowsingKey(t, m, 'h')
	if m.dateKeys[0] != dayKey(0) {
		t.Fatalf("second left scroll starts %s, want Today", m.dateKeys[0])
	}
	m = pressBrowsingKey(t, m, 'h')
	if m.dateKeys[0] != dayKey(0) || m.ColIdx != 0 {
		t.Fatalf("left boundary moved before Today: keys=%v col=%d", m.dateKeys, m.ColIdx)
	}
}

func TestScrollingDoesNotMoveUndatedFutureTasks(t *testing.T) {
	m := Model{
		Data:        TodoData{"Future": {{ID: "someday", Title: "Someday"}}},
		FilePath:    filepath.Join(t.TempDir(), "tasks.json"),
		VisibleDays: 1,
		State:       Browsing,
		todayKey:    dayKey(0),
	}
	m.updateDateKeysFrom(startOfDayNow())

	for range 10 {
		m = pressBrowsingKey(t, m, 'l')
	}
	if got := m.Data["Future"]; len(got) != 1 || got[0].ID != "someday" {
		t.Fatalf("undated Future tasks changed after scrolling: %v", got)
	}
}

func startOfDayNow() time.Time {
	return startOfDay(time.Now())
}
