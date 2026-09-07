// Package taskstore contains task data, operations, and the legacy storage adapter.
// It has no terminal, clipboard, or configuration dependencies.
package taskstore

import (
	"sort"
	"time"
)

const DateLayout = "2006-01-02"

// StartOfDay returns t truncated to midnight in its own location, so dates can
// be compared without the current clock time skewing the result.
func StartOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// ParseDate parses a YYYY-MM-DD key as a local calendar day (midnight local),
// so it lines up with StartOfDay(time.Now()) rather than a UTC midnight.
func ParseDate(s string) (time.Time, error) {
	return time.ParseInLocation(DateLayout, s, time.Local)
}

type Task struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
	DueDate   string    `json:"due_date,omitempty"`
}

// Data maps a date string (YYYY-MM-DD) to a list of tasks
type Data map[string][]Task

func (d Data) CarryForwardCount() int {
	today := StartOfDay(time.Now())
	count := 0
	for key, tasks := range d {
		date, err := ParseDate(key)
		if err != nil || !date.Before(today) {
			continue
		}
		for _, task := range tasks {
			if !task.Completed {
				count++
			}
		}
	}
	return count
}

// GroupTasksByCompletion restores the stable per-bucket ordering invariant.
// Relative order within the active and completed groups is preserved.
func (d Data) GroupTasksByCompletion() bool {
	changed := false
	for key, tasks := range d {
		seenCompleted := false
		needsGrouping := false
		for _, task := range tasks {
			if task.Completed {
				seenCompleted = true
			} else if seenCompleted {
				needsGrouping = true
				break
			}
		}
		if needsGrouping {
			d[key] = GroupTasksByCompletion(tasks)
			changed = true
		}
	}
	return changed
}

func (d Data) RollOverIncompleteTasks() bool {
	now := time.Now()
	todayStr := now.Format(DateLayout)
	normalizedNow := StartOfDay(now)
	tasksToRollOver := make([]Task, 0)
	datesToRemove := make([]string, 0)
	changed := false

	for dateStr, tasks := range d {
		if dateStr == "Future" {
			continue
		}

		parsedDate, err := ParseDate(dateStr)
		if err != nil {
			continue // Skip invalid date strings
		}

		if parsedDate.Before(normalizedNow) {
			remainingTasks := make([]Task, 0, len(tasks))
			for _, task := range tasks {
				if !task.Completed {
					task.DueDate = todayStr // Update due date to today
					tasksToRollOver = append(tasksToRollOver, task)
				} else {
					remainingTasks = append(remainingTasks, task)
				}
			}
			if len(remainingTasks) > 0 {
				d[dateStr] = remainingTasks
			} else {
				datesToRemove = append(datesToRemove, dateStr)
			}
		}
	}

	// Add rolled over tasks to today
	if len(tasksToRollOver) > 0 {
		// If today already has tasks, append to them.
		// Otherwise, create a new entry for today.
		if existingTasks, ok := d[todayStr]; ok {
			d[todayStr] = append(existingTasks, tasksToRollOver...)
		} else {
			d[todayStr] = tasksToRollOver
		}
		changed = true
	}

	// Clean up empty dates that were rolled over
	for _, date := range datesToRemove {
		delete(d, date)
		changed = true
	}

	// Additionally, if today's entry exists but is now empty, remove it.
	if tasks, ok := d[todayStr]; ok && len(tasks) == 0 {
		delete(d, todayStr)
	}
	if d.GroupTasksByCompletion() {
		changed = true
	}

	return changed
}

func (d Data) PruneOldTasks(retentionDays int) bool {
	if retentionDays <= 0 {
		return false
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	cutoffStr := cutoff.Format(DateLayout)
	changed := false

	for dateStr := range d {
		if dateStr == "Future" {
			// Prune completed tasks from Future
			tasks := d[dateStr]
			activeTasks := make([]Task, 0, len(tasks))
			for _, t := range tasks {
				if !t.Completed {
					activeTasks = append(activeTasks, t)
				}
			}
			if len(activeTasks) != len(tasks) {
				changed = true
			}
			d[dateStr] = activeTasks
			continue
		}
		if dateStr < cutoffStr {
			delete(d, dateStr)
			changed = true
		}
	}

	return changed
}

// DistributeFutureTasks moves tasks from "Future" to specific dates if they are
// due within the initial viewport starting today.
func (d Data) DistributeFutureTasks(visibleDays int) {
	d.DistributeFutureTasksThrough(StartOfDay(time.Now()).AddDate(0, 0, visibleDays-1))
}

// DistributeFutureTasksThrough moves dated tasks out of Future once their date
// has been loaded by the scrolling viewport. Undated tasks always remain in the
// separate Future list.
func (d Data) DistributeFutureTasksThrough(lastVisible time.Time) bool {
	futureTasks, ok := d["Future"]
	if !ok || len(futureTasks) == 0 {
		return false
	}

	today := StartOfDay(time.Now())
	todayStr := today.Format(DateLayout)

	remainingFuture := make([]Task, 0)
	changed := false

	for _, task := range futureTasks {
		if task.DueDate == "" {
			remainingFuture = append(remainingFuture, task)
			continue
		}

		dueDate, err := ParseDate(task.DueDate)
		if err != nil {
			remainingFuture = append(remainingFuture, task)
			continue
		}

		// If due date falls within visible range
		if !dueDate.After(lastVisible) {
			targetDate := task.DueDate
			// If overdue, move to today
			if dueDate.Before(today) {
				targetDate = todayStr
			}

			// Add to target date
			d[targetDate] = append(d[targetDate], task)
			changed = true
		} else {
			remainingFuture = append(remainingFuture, task)
		}
	}

	d["Future"] = remainingFuture
	if d.GroupTasksByCompletion() {
		changed = true
	}
	return changed
}

// Helper to get sorted keys
func (d Data) SortedKeys() []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func InsertAt(tasks []Task, idx int, task Task) []Task {
	tasks = append(tasks, Task{})
	copy(tasks[idx+1:], tasks[idx:])
	tasks[idx] = task
	return tasks
}

// GroupTasksByCompletion preserves the relative order within the incomplete
// and completed groups while restoring the list invariant that completed
// tasks follow all incomplete tasks.
func GroupTasksByCompletion(tasks []Task) []Task {
	grouped := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if !task.Completed {
			grouped = append(grouped, task)
		}
	}
	for _, task := range tasks {
		if task.Completed {
			grouped = append(grouped, task)
		}
	}
	return grouped
}

func Clone(data Data) Data {
	cloned := make(Data, len(data))
	for key, tasks := range data {
		cloned[key] = append([]Task(nil), tasks...)
	}
	return cloned
}
