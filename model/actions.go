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

func (m *Model) addTask(title string) {
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

	// Toggle completion
	tasks[m.RowIdx].Completed = !tasks[m.RowIdx].Completed

	if tasks[m.RowIdx].Completed {
		// If completed and not already at the bottom, move to bottom
		if m.RowIdx < len(tasks)-1 {
			task := tasks[m.RowIdx]
			// Remove task at RowIdx
			tasks = append(tasks[:m.RowIdx], tasks[m.RowIdx+1:]...)
			// Append task to end
			tasks = append(tasks, task)

			// Update the map with the reordered slice
			m.Data[currentDate] = tasks
		}
	} else {
		// If uncompleted, move it above completed tasks
		task := tasks[m.RowIdx]
		// Remove task at current position
		tasks = append(tasks[:m.RowIdx], tasks[m.RowIdx+1:]...)

		// Find the first completed task to insert before it
		insertIdx := len(tasks)
		for i, t := range tasks {
			if t.Completed {
				insertIdx = i
				break
			}
		}

		// Insert at the appropriate position
		if insertIdx == len(tasks) {
			tasks = append(tasks, task)
		} else {
			tasks = insertAt(tasks, insertIdx, task)
		}

		// Update the map with the reordered slice
		m.Data[currentDate] = tasks
	}
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

		lastVisible := today.AddDate(0, 0, m.VisibleDays-1)
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
