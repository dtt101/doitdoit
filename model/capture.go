package model

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"time"
)

// CaptureTask adds one task without constructing the interactive model.
// Far-future tasks use the Future bucket so they remain discoverable there.
func CaptureTask(path, title, when string, retentionDays int) (Task, string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Task{}, "", fmt.Errorf("task title cannot be empty")
	}

	now := time.Now()
	target, future, err := captureTarget(when, now)
	if err != nil {
		return Task{}, "", err
	}

	data, err := Load(path, retentionDays)
	if err != nil {
		return Task{}, "", err
	}
	contents, readErr := os.ReadFile(path)
	exists := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return Task{}, "", readErr
	}
	var expectedHash [sha256.Size]byte
	if exists {
		expectedHash = sha256.Sum256(contents)
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
	tasks := data[key]
	insertIdx := len(tasks)
	for i, existing := range tasks {
		if existing.Completed {
			insertIdx = i
			break
		}
	}
	if insertIdx == len(tasks) {
		data[key] = append(tasks, task)
	} else {
		data[key] = insertAt(tasks, insertIdx, task)
	}
	if err := data.SaveIfUnchanged(path, &expectedHash, exists); err != nil {
		return Task{}, "", err
	}
	return task, key, nil
}

func captureTarget(when string, now time.Time) (target string, future bool, err error) {
	when = strings.ToLower(strings.TrimSpace(when))
	today := startOfDay(now)
	switch when {
	case "", "today":
		return today.Format(dateLayout), false, nil
	case "tomorrow":
		return today.AddDate(0, 0, 1).Format(dateLayout), false, nil
	case "future":
		return "", true, nil
	default:
		date, parseErr := parseDate(when)
		if parseErr != nil {
			return "", false, fmt.Errorf("invalid --when value %q; use today, tomorrow, future, or YYYY-MM-DD", when)
		}
		if date.Before(today) {
			date = today
		}
		// Match the default three-day TUI window. More distant dates stay in
		// Future until the corresponding day enters the visible window.
		return date.Format(dateLayout), date.After(today.AddDate(0, 0, 2)), nil
	}
}
