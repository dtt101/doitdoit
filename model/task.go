package model

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	dirty := false

	// Check if file exists
	if _, err := os.Stat(path); err == nil {
		bytes, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(bytes, &data); err != nil {
			return nil, err
		}
	}

	// Import tasks from text file if it exists
	imported, err := data.importFromTextFile(path)
	if err != nil {
		return nil, err
	}
	dirty = dirty || imported

	// Roll over incomplete tasks
	if data.rollOverIncompleteTasks() {
		dirty = true
	}

	// Prune old tasks
	if data.pruneOldTasks() {
		dirty = true
	}

	// Persist any changes triggered during load so the file stays up to date
	if dirty {
		if err := data.Save(path); err != nil {
			return nil, err
		}
	}

	return data, nil
}

func (d TodoData) importFromTextFile(jsonPath string) (bool, error) {
	// Look for import.txt in the same directory as the JSON file
	dir := filepath.Dir(jsonPath)
	importPath := filepath.Join(dir, "import.txt")

	if _, err := os.Stat(importPath); os.IsNotExist(err) {
		return false, nil
	}

	file, err := os.Open(importPath)
	if err != nil {
		return false, err
	}
	// We defer close, but we also close explicitly before removing
	defer file.Close()

	var newTasks []Task
	scanner := bufio.NewScanner(file)

	// Seed for unique IDs in this batch
	baseTime := time.Now().UnixNano()
	idx := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		newTask := Task{
			ID:        fmt.Sprintf("%d-%d", baseTime, idx),
			Title:     line,
			Completed: false,
			CreatedAt: time.Now(),
		}
		newTasks = append(newTasks, newTask)
		idx++
	}

	if err := scanner.Err(); err != nil {
		return false, err
	}

	// If we found tasks, add them and save
	if len(newTasks) > 0 {
		if d["Future"] == nil {
			d["Future"] = make([]Task, 0)
		}
		d["Future"] = append(d["Future"], newTasks...)
	}

	// Close the file so we can delete it (important on Windows)
	file.Close()

	// Delete the import file
	if err := os.Remove(importPath); err != nil {
		return false, err
	}

	return len(newTasks) > 0, nil
}

func (d TodoData) rollOverIncompleteTasks() bool {
	todayStr := time.Now().Format("2006-01-02")
	tasksToRollOver := make([]Task, 0)
	datesToRemove := make([]string, 0)
	changed := false

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

	return changed
}

func (d TodoData) Save(path string) error {
	bytes, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	temp, err := os.CreateTemp(dir, "doitdoit-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()

	if _, err := temp.Write(bytes); err != nil {
		temp.Close()
		os.Remove(tempPath)
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		os.Remove(tempPath)
		return err
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		return err
	}

	// Restrict permissions to the owner for privacy
	if err := os.Chmod(tempPath, 0600); err != nil {
		os.Remove(tempPath)
		return err
	}

	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return err
	}

	return nil
}

func (d TodoData) pruneOldTasks() bool {
	cutoff := time.Now().AddDate(0, 0, -5)
	cutoffStr := cutoff.Format("2006-01-02")
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
