package model

import (
	"testing"
	"time"
)

// withTimeZone pins time.Local for the duration of a test so that date math is
// deterministic and we can prove behaviour holds in zones far from UTC. The
// previous implementation parsed date strings as UTC midnight while deriving
// "today" from local time, which skewed comparisons in such zones.
func withTimeZone(t *testing.T, name string, offsetHours int) {
	t.Helper()
	orig := time.Local
	time.Local = time.FixedZone(name, offsetHours*3600)
	t.Cleanup(func() { time.Local = orig })
}

// dayKey returns the YYYY-MM-DD key for today+offset using the same notion of
// "today" the production code uses, so assertions stay in sync with it.
func dayKey(offset int) string {
	return startOfDay(time.Now()).AddDate(0, 0, offset).Format(dateLayout)
}

// taskIDsByDate flattens the data into a date -> set-of-IDs view for assertions.
func taskIDsByDate(d TodoData) map[string]map[string]bool {
	out := make(map[string]map[string]bool)
	for date, tasks := range d {
		ids := make(map[string]bool, len(tasks))
		for _, task := range tasks {
			ids[task.ID] = true
		}
		out[date] = ids
	}
	return out
}

func TestDistributeFutureTasks(t *testing.T) {
	const visibleDays = 3

	// Each zone is exercised independently. The extreme offsets (±) are the
	// cases that broke under UTC-vs-local date comparisons.
	zones := []struct {
		name        string
		offsetHours int
	}{
		{"UTC", 0},
		{"Kiritimati", 14}, // UTC+14, the furthest-ahead real zone
		{"Pago Pago", -11}, // UTC-11, far behind
	}

	for _, z := range zones {
		z := z
		t.Run(z.name, func(t *testing.T) {
			withTimeZone(t, z.name, z.offsetHours)

			// Build the input fresh inside the zone so dayKey reflects it.
			data := TodoData{
				"Future": []Task{
					{ID: "no-due", Title: "No due date"},
					{ID: "bad-due", Title: "Invalid due date", DueDate: "not-a-date"},
					{ID: "overdue", Title: "Overdue", DueDate: dayKey(-3)},
					{ID: "today", Title: "Due today", DueDate: dayKey(0)},
					{ID: "in-range", Title: "Due tomorrow", DueDate: dayKey(1)},
					{ID: "last-visible", Title: "Due last visible day", DueDate: dayKey(visibleDays - 1)},
					{ID: "beyond", Title: "Just past the window", DueDate: dayKey(visibleDays)},
				},
			}

			data.DistributeFutureTasks(visibleDays)

			byDate := taskIDsByDate(data)

			// Tasks that should remain in Future, untouched.
			wantFuture := map[string]bool{"no-due": true, "bad-due": true, "beyond": true}
			gotFuture := byDate["Future"]
			if len(gotFuture) != len(wantFuture) {
				t.Fatalf("Future bucket = %v, want exactly %v", gotFuture, wantFuture)
			}
			for id := range wantFuture {
				if !gotFuture[id] {
					t.Errorf("expected %q to remain in Future, got %v", id, gotFuture)
				}
			}

			// Overdue and due-today both land on today.
			today := dayKey(0)
			if !byDate[today]["overdue"] {
				t.Errorf("expected overdue task on today (%s), got %v", today, byDate[today])
			}
			if !byDate[today]["today"] {
				t.Errorf("expected due-today task on today (%s), got %v", today, byDate[today])
			}

			// In-range and boundary tasks land on their exact dates.
			if d := dayKey(1); !byDate[d]["in-range"] {
				t.Errorf("expected in-range task on %s, got %v", d, byDate[d])
			}
			if d := dayKey(visibleDays - 1); !byDate[d]["last-visible"] {
				t.Errorf("expected last-visible task on %s, got %v", d, byDate[d])
			}

			// The beyond-window task must NOT have been placed on any date.
			beyondDate := dayKey(visibleDays)
			if byDate[beyondDate]["beyond"] {
				t.Errorf("task due beyond the window was distributed onto %s", beyondDate)
			}
		})
	}
}

func TestDistributeFutureTasksNoFutureBucket(t *testing.T) {
	// Missing bucket must not panic and must not create one spuriously.
	data := TodoData{}
	data.DistributeFutureTasks(3)
	if _, ok := data["Future"]; ok {
		t.Errorf("did not expect a Future bucket to be created, got %v", data)
	}

	// Empty bucket is left exactly as-is.
	empty := TodoData{"Future": []Task{}}
	empty.DistributeFutureTasks(3)
	if len(empty["Future"]) != 0 {
		t.Errorf("expected Future to remain empty, got %v", empty["Future"])
	}
}

// TestRollOverIncompleteTasksTimezone guards the same date-normalization fix on
// the rollover path: in any zone, yesterday's incomplete task rolls to today and
// completed ones are left behind.
func TestRollOverIncompleteTasksTimezone(t *testing.T) {
	zones := []struct {
		name        string
		offsetHours int
	}{
		{"UTC", 0},
		{"Kiritimati", 14},
		{"Pago Pago", -11},
	}
	for _, z := range zones {
		z := z
		t.Run(z.name, func(t *testing.T) {
			withTimeZone(t, z.name, z.offsetHours)

			yesterday := dayKey(-1)
			today := dayKey(0)

			data := TodoData{
				yesterday: []Task{
					{ID: "done", Title: "Completed", Completed: true},
					{ID: "todo", Title: "Incomplete", Completed: false},
				},
			}

			if !data.rollOverIncompleteTasks() {
				t.Fatal("expected rollover to report a change")
			}

			byDate := taskIDsByDate(data)
			if !byDate[today]["todo"] {
				t.Errorf("expected incomplete task rolled to today (%s), got %v", today, byDate[today])
			}
			if byDate[today]["done"] {
				t.Errorf("completed task should not roll over, got %v", byDate[today])
			}
			if !byDate[yesterday]["done"] {
				t.Errorf("expected completed task to stay on %s, got %v", yesterday, byDate[yesterday])
			}
			if byDate[yesterday]["todo"] {
				t.Errorf("incomplete task should have left %s, got %v", yesterday, byDate[yesterday])
			}
		})
	}
}
