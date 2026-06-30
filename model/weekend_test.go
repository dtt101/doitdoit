package model

import (
	"reflect"
	"testing"
)

// 2026-06-26 is a Friday, so the following two days are the weekend and the day
// after returns to Monday.
const (
	friday   = "2026-06-26"
	saturday = "2026-06-27"
	sunday   = "2026-06-28"
	monday   = "2026-06-29"
)

func TestColumnGroupsStacksWeekend(t *testing.T) {
	m := Model{VisibleDays: 4}
	keys := []string{friday, saturday, sunday, monday}

	got := m.columnGroups(keys)
	want := [][]int{{0}, {1, 2}, {3}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("columnGroups(%v) = %v, want %v", keys, got, want)
	}
}

func TestColumnGroupsLoneSaturday(t *testing.T) {
	m := Model{VisibleDays: 2}
	keys := []string{friday, saturday}

	got := m.columnGroups(keys)
	want := [][]int{{0}, {1}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("columnGroups(%v) = %v, want %v", keys, got, want)
	}
}

func TestColumnGroupsLoneSunday(t *testing.T) {
	m := Model{VisibleDays: 2}
	keys := []string{sunday, monday}

	got := m.columnGroups(keys)
	want := [][]int{{0}, {1}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("columnGroups(%v) = %v, want %v", keys, got, want)
	}
}

func TestColumnGroupsSingleDayViewNeverStacks(t *testing.T) {
	m := Model{VisibleDays: 1}
	keys := []string{saturday}

	got := m.columnGroups(keys)
	want := [][]int{{0}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("columnGroups(%v) = %v, want %v", keys, got, want)
	}
}

func TestColumnGroupsFutureViewSingleColumn(t *testing.T) {
	m := Model{VisibleDays: 3, ShowFuture: true}
	keys := []string{"Future"}

	got := m.columnGroups(keys)
	want := [][]int{{0}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("columnGroups(%v) = %v, want %v", keys, got, want)
	}
}

func TestGroupFocusedMatchesAnyDayInColumn(t *testing.T) {
	m := Model{VisibleDays: 4, ColIdx: 2} // Sunday focused
	weekendColumn := []int{1, 2}

	if !m.groupFocused(weekendColumn) {
		t.Errorf("expected weekend column to be focused when its Sunday is selected")
	}

	weekdayColumn := []int{0}
	if m.groupFocused(weekdayColumn) {
		t.Errorf("did not expect weekday column to be focused")
	}
}
