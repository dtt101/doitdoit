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

	// Prune old tasks
	data.pruneOldTasks()

	return data, nil
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

// Helper to get sorted keys
func (d TodoData) SortedKeys() []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
