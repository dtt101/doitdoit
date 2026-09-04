package model

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const dateLayout = "2006-01-02"

var ErrDataConflict = errors.New("task file changed externally; reload before trying again")

// startOfDay returns t truncated to midnight in its own location, so dates can
// be compared without the current clock time skewing the result.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// parseDate parses a YYYY-MM-DD key as a local calendar day (midnight local),
// so it lines up with startOfDay(time.Now()) rather than a UTC midnight.
func parseDate(s string) (time.Time, error) {
	return time.ParseInLocation(dateLayout, s, time.Local)
}

type Task struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
	DueDate   string    `json:"due_date,omitempty"`
}

// TodoData maps a date string (YYYY-MM-DD) to a list of tasks
type TodoData map[string][]Task

// loadRaw reads and parses the JSON file without any side effects. A missing
// file yields an empty map.
func loadRaw(path string) (TodoData, error) {
	data, _, _, err := loadRawState(path)
	return data, err
}

func loadRawState(path string) (TodoData, [sha256.Size]byte, bool, error) {
	data := make(TodoData)
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return data, [sha256.Size]byte{}, false, nil
	}
	if err != nil {
		return nil, [sha256.Size]byte{}, false, err
	}
	if err := json.Unmarshal(contents, &data); err != nil {
		return nil, [sha256.Size]byte{}, true, err
	}
	return data, sha256.Sum256(contents), true, nil
}

func Load(path string, retentionDays int) (TodoData, error) {
	data, hash, exists, err := loadRawState(path)
	if err != nil {
		return nil, err
	}
	dirty := false

	// Roll over incomplete tasks
	if data.rollOverIncompleteTasks() {
		dirty = true
	}

	// Prune only after a positive retention period has been explicitly passed.
	if retentionDays > 0 && data.pruneOldTasks(retentionDays) {
		dirty = true
	}

	// Repair files written by older versions or other clients that placed an
	// active task below a completed task.
	if data.groupTasksByCompletion() {
		dirty = true
	}

	// Persist any changes triggered during load so the file stays up to date
	if dirty {
		if err := data.SaveIfUnchanged(path, &hash, exists); err != nil {
			return nil, err
		}
	}

	return data, nil
}

// groupTasksByCompletion restores the stable per-bucket ordering invariant.
// Relative order within the active and completed groups is preserved.
func (d TodoData) groupTasksByCompletion() bool {
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
			d[key] = groupTasksByCompletion(tasks)
			changed = true
		}
	}
	return changed
}

func (d TodoData) rollOverIncompleteTasks() bool {
	now := time.Now()
	todayStr := now.Format(dateLayout)
	normalizedNow := startOfDay(now)
	tasksToRollOver := make([]Task, 0)
	datesToRemove := make([]string, 0)
	changed := false

	for dateStr, tasks := range d {
		if dateStr == "Future" {
			continue
		}

		parsedDate, err := parseDate(dateStr)
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
	if d.groupTasksByCompletion() {
		changed = true
	}

	return changed
}

func (d TodoData) Save(path string) error {
	return d.save(path, nil, false)
}

// SaveIfUnchanged refuses to replace a file whose contents changed since the
// caller last loaded it. A previous valid copy is retained at path + ".bak".
func (d TodoData) SaveIfUnchanged(path string, expectedHash *[sha256.Size]byte, expectedExists bool) error {
	return d.save(path, expectedHash, expectedExists)
}

func (d TodoData) save(path string, expectedHash *[sha256.Size]byte, expectedExists bool) error {
	bytes, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	current, readErr := os.ReadFile(path)
	currentExists := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	if expectedHash != nil && (currentExists != expectedExists || (currentExists && sha256.Sum256(current) != *expectedHash)) {
		return ErrDataConflict
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
	if currentExists {
		if err := writeBackup(path+".bak", current); err != nil {
			os.Remove(tempPath)
			return fmt.Errorf("writing backup: %w", err)
		}
	}

	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return err
	}

	return nil
}

func writeBackup(path string, contents []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "doitdoit-backup-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(contents); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, 0600); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func (d TodoData) pruneOldTasks(retentionDays int) bool {
	if retentionDays <= 0 {
		return false
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	cutoffStr := cutoff.Format(dateLayout)
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
func (d TodoData) DistributeFutureTasks(visibleDays int) {
	d.distributeFutureTasksThrough(startOfDay(time.Now()).AddDate(0, 0, visibleDays-1))
}

// distributeFutureTasksThrough moves dated tasks out of Future once their date
// has been loaded by the scrolling viewport. Undated tasks always remain in the
// separate Future list.
func (d TodoData) distributeFutureTasksThrough(lastVisible time.Time) bool {
	futureTasks, ok := d["Future"]
	if !ok || len(futureTasks) == 0 {
		return false
	}

	today := startOfDay(time.Now())
	todayStr := today.Format(dateLayout)

	remainingFuture := make([]Task, 0)
	changed := false

	for _, task := range futureTasks {
		if task.DueDate == "" {
			remainingFuture = append(remainingFuture, task)
			continue
		}

		dueDate, err := parseDate(task.DueDate)
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
	if d.groupTasksByCompletion() {
		changed = true
	}
	return changed
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
