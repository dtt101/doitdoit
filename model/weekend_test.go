package model

import (
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

func TestRenderColumnsUsesOneColumnPerDayAcrossWeekend(t *testing.T) {
	m := Model{
		Data:        TodoData{},
		VisibleDays: 4,
		State:       Browsing,
		width:       120,
		height:      30,
	}
	keys := []string{friday, saturday, sunday, monday}

	if got := len(m.renderColumns(keys)); got != len(keys) {
		t.Errorf("renderColumns(%v) returned %d columns, want %d", keys, got, len(keys))
	}
}
