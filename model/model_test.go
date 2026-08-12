package model

import (
	"strconv"
	"strings"
	"testing"
)

func TestNewModelRejectsNonPositiveVisibleDays(t *testing.T) {
	for _, visibleDays := range []int{0, -1} {
		t.Run(strconv.Itoa(visibleDays), func(t *testing.T) {
			_, err := NewModel(t.TempDir()+"/tasks.json", visibleDays)
			if err == nil {
				t.Fatalf("NewModel(%d) expected an error", visibleDays)
			}
			if !strings.Contains(err.Error(), "at least 1") {
				t.Fatalf("NewModel(%d) error = %q, want minimum-day guidance", visibleDays, err)
			}
		})
	}
}
