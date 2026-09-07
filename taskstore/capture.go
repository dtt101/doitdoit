package taskstore

import (
	"fmt"
	"strings"
	"time"
)

// CaptureTask adds one task without constructing the interactive model.
// Far-future tasks use the Future bucket so they remain discoverable there.
func CaptureTask(path, title, when string, retentionDays int) (Task, string, error) {
	return Capture(NewJSON(path), title, when, retentionDays)
}

// Capture runs non-interactive task creation against the same store as the TUI.
func Capture(store Store, title, when string, retentionDays int) (Task, string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Task{}, "", fmt.Errorf("task title cannot be empty")
	}

	now := time.Now()
	target, future, err := CaptureTarget(when, now)
	if err != nil {
		return Task{}, "", err
	}

	snapshot, _, err := LoadStore(store, retentionDays)
	data := snapshot.Data
	if err != nil {
		return Task{}, "", err
	}

	task := Task{ID: fmt.Sprintf("%d", now.UnixNano()), Title: title, CreatedAt: now}
	key := target
	if future {
		key = "Future"
		if target != "" {
			task.DueDate = target
		}
	} else {
		task.DueDate = target
	}
	data.Insert(key, task)
	if _, err := store.Save(data, &snapshot.Revision); err != nil {
		return Task{}, "", err
	}
	return task, key, nil
}

func CaptureTarget(when string, now time.Time) (target string, future bool, err error) {
	when = strings.ToLower(strings.TrimSpace(when))
	today := StartOfDay(now)
	switch when {
	case "", "today":
		return today.Format(DateLayout), false, nil
	case "tomorrow":
		return today.AddDate(0, 0, 1).Format(DateLayout), false, nil
	case "future":
		return "", true, nil
	default:
		date, parseErr := ParseDate(when)
		if parseErr != nil {
			return "", false, fmt.Errorf("invalid --when value %q; use today, tomorrow, future, or YYYY-MM-DD", when)
		}
		if date.Before(today) {
			date = today
		}
		// Match the default three-day TUI window. More distant dates stay in
		// Future until the corresponding day enters the visible window.
		return date.Format(DateLayout), date.After(today.AddDate(0, 0, 2)), nil
	}
}
