package model

import (
	"encoding/json"
	"os"
	"sort"
	"time"
)

type Task struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
	DueDate   string    `json:"due_date,omitempty"`
}

// TodoData maps a date string (YYYY-MM-DD) to a list of tasks
type TodoData map[string][]Task

func Load(path string) (TodoData, error) {
	data := make(TodoData)

	// If file doesn't exist, return empty data
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return data, nil
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, err
	}

	// Roll over incomplete tasks
	data.rollOverIncompleteTasks()

	// Prune old tasks
	data.pruneOldTasks()

	return data, nil
}

func (d TodoData) rollOverIncompleteTasks() {
	todayStr := time.Now().Format("2006-01-02")
	tasksToRollOver := make([]Task, 0)
	datesToRemove := make([]string, 0)

	for dateStr, tasks := range d {
		if dateStr == "Future" {
			continue
		}

		parsedDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue // Skip invalid date strings
		}

		now := time.Now()
		normalizedNow := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		normalizedParsedDate := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, parsedDate.Location())

		if normalizedParsedDate.Before(normalizedNow) {
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
	}

	// Clean up empty dates that were rolled over
	for _, date := range datesToRemove {
		delete(d, date)
	}

	// Additionally, if today's entry exists but is now empty, remove it.
	if tasks, ok := d[todayStr]; ok && len(tasks) == 0 {
		delete(d, todayStr)
	}
}

func (d TodoData) Save(path string) error {
	bytes, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0644)
}

func (d TodoData) pruneOldTasks() {
	cutoff := time.Now().AddDate(0, 0, -5)
	cutoffStr := cutoff.Format("2006-01-02")

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
			d[dateStr] = activeTasks
			continue
		}
		if dateStr < cutoffStr {
			delete(d, dateStr)
		}
	}
}

// DistributeFutureTasks moves tasks from "Future" to specific dates if they are due
func (d TodoData) DistributeFutureTasks(visibleDays int) {
	futureTasks, ok := d["Future"]
	if !ok || len(futureTasks) == 0 {
		return
	}

	today := time.Now()
	// Calculate the last visible date
	lastVisible := today.AddDate(0, 0, visibleDays-1).Format("2006-01-02")
	todayStr := today.Format("2006-01-02")

	remainingFuture := make([]Task, 0)

	for _, task := range futureTasks {
		if task.DueDate == "" {
			remainingFuture = append(remainingFuture, task)
			continue
		}

		// If due date is valid
		if task.DueDate <= lastVisible {
			targetDate := task.DueDate
			// If overdue, move to today
			if targetDate < todayStr {
				targetDate = todayStr
			}

			// Add to target date
			d[targetDate] = append(d[targetDate], task)
		} else {
			remainingFuture = append(remainingFuture, task)
		}
	}

	d["Future"] = remainingFuture
}

// Helper to get sorted keys
func (d TodoData) SortedKeys() []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
