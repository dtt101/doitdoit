package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
)

func insertAt(tasks []Task, idx int, task Task) []Task {
	tasks = append(tasks, Task{})
	copy(tasks[idx+1:], tasks[idx:])
	tasks[idx] = task
	return tasks
}

// groupTasksByCompletion preserves the relative order within the incomplete
// and completed groups while restoring the list invariant that completed
// tasks follow all incomplete tasks.
func groupTasksByCompletion(tasks []Task) []Task {
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

func (m *Model) addTask(title string) {
	m.captureMoveUndo()
	title = strings.TrimSpace(title)
	currentDate := m.getCurrentKey()
	newTask := Task{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Title:     title,
		CreatedAt: time.Now(),
		Completed: false,
	}

	tasks := m.Data[currentDate]
	insertIdx := len(tasks)
	for i, t := range tasks {
		if t.Completed {
			insertIdx = i
			break
		}
	}

	if insertIdx == len(tasks) {
		m.Data[currentDate] = append(tasks, newTask)
	} else {
		m.Data[currentDate] = insertAt(tasks, insertIdx, newTask)
	}
}

func (m *Model) deleteTask() bool {
	currentDate := m.getCurrentKey()
	tasks := m.Data[currentDate]
	if len(tasks) == 0 || m.RowIdx >= len(tasks) {
		return false
	}

	m.captureMoveUndo()
	m.Data[currentDate] = append(tasks[:m.RowIdx], tasks[m.RowIdx+1:]...)
	m.clampRow()
	return true
}

func (m *Model) toggleTask() bool {
	currentDate := m.getCurrentKey()
	tasks := m.Data[currentDate]
	if m.RowIdx >= len(tasks) {
		return false
	}

	m.captureMoveUndo()
	task := tasks[m.RowIdx]
	task.Completed = !task.Completed

	// Remove the toggled task before grouping the remainder. This repairs any
	// pre-existing interleaving in the list, while still placing a newly
	// completed task at the very bottom.
	tasks = append(tasks[:m.RowIdx], tasks[m.RowIdx+1:]...)
	tasks = groupTasksByCompletion(tasks)

	if task.Completed {
		tasks = append(tasks, task)
	} else {
		insertIdx := len(tasks)
		for i, existing := range tasks {
			if existing.Completed {
				insertIdx = i
				break
			}
		}
		tasks = insertAt(tasks, insertIdx, task)
	}

	m.Data[currentDate] = tasks
	return true
}

func (m *Model) editTask(title string) bool {
	title = strings.TrimSpace(title)
	currentDate := m.getCurrentKey()
	tasks := m.Data[currentDate]
	if title == "" || m.RowIdx < 0 || m.RowIdx >= len(tasks) || tasks[m.RowIdx].Title == title {
		return false
	}
	m.captureMoveUndo()
	tasks[m.RowIdx].Title = title
	m.Data[currentDate] = tasks
	return true
}

func cloneTodoData(data TodoData) TodoData {
	cloned := make(TodoData, len(data))
	for key, tasks := range data {
		cloned[key] = append([]Task(nil), tasks...)
	}
	return cloned
}

func (m *Model) captureMoveUndo() {
	m.moveUndo = &moveUndoSnapshot{
		Data:       cloneTodoData(m.Data),
		ShowFuture: m.ShowFuture,
		ColIdx:     m.ColIdx,
		RowIdx:     m.RowIdx,
	}
}

func (m *Model) clearMoveUndo() {
	m.moveUndo = nil
}

func (m *Model) undoMove() bool {
	if m.moveUndo == nil {
		return false
	}

	snapshot := m.moveUndo
	m.Data = cloneTodoData(snapshot.Data)
	m.ShowFuture = snapshot.ShowFuture
	m.ColIdx = snapshot.ColIdx
	m.RowIdx = snapshot.RowIdx
	m.moveUndo = nil
	m.clampRow()
	return true
}

func (m *Model) reorderTask(direction int) bool {
	currentDate := m.getCurrentKey()
	tasks := m.Data[currentDate]
	if len(tasks) == 0 {
		return false
	}

	newRowIdx := m.RowIdx + direction
	if newRowIdx < 0 || newRowIdx >= len(tasks) {
		return false
	}
	// Reordering is allowed within the active and completed groups, but not
	// across their boundary. Completion grouping is an invariant of every
	// task bucket.
	if tasks[m.RowIdx].Completed != tasks[newRowIdx].Completed {
		return false
	}

	m.captureMoveUndo()
	tasks[m.RowIdx], tasks[newRowIdx] = tasks[newRowIdx], tasks[m.RowIdx]
	m.RowIdx = newRowIdx
	return true
}

func (m Model) moveBaseDate() time.Time {
	if !m.ShowFuture && m.ColIdx >= 0 && m.ColIdx < len(m.dateKeys) {
		if parsed, err := parseDate(m.dateKeys[m.ColIdx]); err == nil {
			return parsed
		}
	}
	return startOfDay(time.Now())
}

func (m Model) relativeMoveTarget(days int) moveTarget {
	return moveTarget{Date: m.moveBaseDate().AddDate(0, 0, days).Format(dateLayout)}
}

func (m *Model) scheduleTask(target moveTarget) bool {
	sourceKey := m.getCurrentKey()
	tasks := m.Data[sourceKey]
	if len(tasks) == 0 || m.RowIdx < 0 || m.RowIdx >= len(tasks) {
		return false
	}

	task := tasks[m.RowIdx]
	targetKey := "Future"
	dueDate := ""
	if !target.Future {
		parsed, err := parseDate(target.Date)
		if err != nil {
			return false
		}
		today := startOfDay(time.Now())
		if parsed.Before(today) {
			parsed = today
		}
		dueDate = parsed.Format(dateLayout)
		target = moveTarget{Date: dueDate}

		lastVisible := m.lastVisibleDate()
		if !parsed.After(lastVisible) {
			targetKey = dueDate
		}
	}

	if sourceKey == targetKey && task.DueDate == dueDate {
		return false
	}

	m.captureMoveUndo()
	task.DueDate = dueDate

	if sourceKey == targetKey {
		tasks[m.RowIdx] = task
		m.Data[sourceKey] = tasks
	} else {
		m.Data[sourceKey] = append(tasks[:m.RowIdx], tasks[m.RowIdx+1:]...)

		targetTasks := m.Data[targetKey]
		insertIdx := len(targetTasks)
		for i, existing := range targetTasks {
			if existing.Completed {
				insertIdx = i
				break
			}
		}
		if insertIdx == len(targetTasks) {
			m.Data[targetKey] = append(targetTasks, task)
		} else {
			m.Data[targetKey] = insertAt(targetTasks, insertIdx, task)
		}
	}

	m.lastMoveTarget = &target
	m.clampRow()
	return true
}

func (m *Model) repeatMove() bool {
	if m.lastMoveTarget == nil {
		return false
	}
	return m.scheduleTask(*m.lastMoveTarget)
}

func (m *Model) copyTask() {
	currentDate := m.getCurrentKey()
	tasks := m.Data[currentDate]
	if len(tasks) == 0 || m.RowIdx >= len(tasks) {
		return
	}

	if err := clipboard.WriteAll(tasks[m.RowIdx].Title); err != nil {
		m.Err = err
		return
	}
	m.copyFlash = true
}

func normalizeDueDateInput(dateStr string) (string, error) {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return "", fmt.Errorf("date is required in YYYY-MM-DD or MM-DD format")
	}

	// If only M-D or MM-DD provided (no year), prepend current year
	parts := strings.Split(dateStr, "-")
	if len(parts) == 2 {
		dateStr = fmt.Sprintf("%d-%s", time.Now().Year(), dateStr)
	}

	parsed, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", fmt.Errorf("invalid date; use YYYY-MM-DD or MM-DD")
	}

	return parsed.Format("2006-01-02"), nil
}
