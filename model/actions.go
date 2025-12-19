package model

import (
	"fmt"
	"strings"
	"time"
)

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
		m.Data[currentDate] = append(tasks[:insertIdx], append([]Task{newTask}, tasks[insertIdx:]...)...)
	}
}

func (m *Model) deleteTask() {
	currentDate := m.getCurrentKey()
	tasks := m.Data[currentDate]
	if len(tasks) == 0 || m.RowIdx >= len(tasks) {
		return
	}

	m.Data[currentDate] = append(tasks[:m.RowIdx], tasks[m.RowIdx+1:]...)
	m.clampRow()
}

func (m *Model) toggleTask() {
	currentDate := m.getCurrentKey()
	tasks := m.Data[currentDate]
	if m.RowIdx >= len(tasks) {
		return
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
			tasks = append(tasks[:insertIdx], append([]Task{task}, tasks[insertIdx:]...)...)
		}

		// Update the map with the reordered slice
		m.Data[currentDate] = tasks
	}
}

func (m *Model) moveTask(direction int) {
	if m.ShowFuture {
		return
	}
	currentDate := m.dateKeys[m.ColIdx]
	tasks := m.Data[currentDate]
	if len(tasks) == 0 || m.RowIdx >= len(tasks) {
		return
	}

	targetColIdx := m.ColIdx + direction
	if targetColIdx < 0 || targetColIdx >= len(m.dateKeys) {
		return
	}

	targetDate := m.dateKeys[targetColIdx]
	taskToMove := tasks[m.RowIdx]

	// Remove from current
	m.Data[currentDate] = append(tasks[:m.RowIdx], tasks[m.RowIdx+1:]...)

	// Add to target
	targetTasks := m.Data[targetDate]
	insertIdx := m.RowIdx
	if insertIdx > len(targetTasks) {
		insertIdx = len(targetTasks)
	}

	if insertIdx == len(targetTasks) {
		m.Data[targetDate] = append(targetTasks, taskToMove)
	} else {
		m.Data[targetDate] = append(targetTasks[:insertIdx], append([]Task{taskToMove}, targetTasks[insertIdx:]...)...)
	}

	// Follow the task
	m.ColIdx = targetColIdx
	m.RowIdx = insertIdx
}

func (m *Model) reorderTask(direction int) {
	currentDate := m.getCurrentKey()
	tasks := m.Data[currentDate]
	if len(tasks) == 0 {
		return
	}

	newRowIdx := m.RowIdx + direction
	if newRowIdx < 0 || newRowIdx >= len(tasks) {
		return
	}

	// Swap
	tasks[m.RowIdx], tasks[newRowIdx] = tasks[newRowIdx], tasks[m.RowIdx]
	m.RowIdx = newRowIdx
}

func (m *Model) moveFutureTaskToToday() {
	if !m.ShowFuture {
		return
	}

	futureTasks := m.Data["Future"]
	if len(futureTasks) == 0 || m.RowIdx >= len(futureTasks) {
		return
	}

	todayStr := time.Now().Format("2006-01-02")
	task := futureTasks[m.RowIdx]
	task.DueDate = todayStr

	// Remove from Future
	m.Data["Future"] = append(futureTasks[:m.RowIdx], futureTasks[m.RowIdx+1:]...)

	// Insert into Today, keeping incomplete tasks above completed ones
	todayTasks := m.Data[todayStr]
	insertIdx := len(todayTasks)
	for i, t := range todayTasks {
		if t.Completed {
			insertIdx = i
			break
		}
	}

	if insertIdx == len(todayTasks) {
		m.Data[todayStr] = append(todayTasks, task)
	} else {
		m.Data[todayStr] = append(todayTasks[:insertIdx], append([]Task{task}, todayTasks[insertIdx:]...)...)
	}

	// Jump back to today with the moved task focused
	m.ShowFuture = false
	m.ColIdx = 0
	m.RowIdx = insertIdx
}

func (m *Model) setTaskDate(dateStr string) error {
	currentDate := m.getCurrentKey()
	tasks := m.Data[currentDate]
	if len(tasks) == 0 || m.RowIdx >= len(tasks) {
		return nil
	}

	normalizedDate, err := normalizeDueDateInput(dateStr)
	if err != nil {
		m.Err = err
		return err
	}
	m.Err = nil

	// Update task
	taskID := tasks[m.RowIdx].ID
	tasks[m.RowIdx].DueDate = normalizedDate
	m.Data[currentDate] = tasks

	// Redistribute
	m.Data.DistributeFutureTasks(m.VisibleDays)
	m.updateDateKeys()

	// Check if we need to switch view
	// We need to find where the task went
	found := false
	if !m.ShowFuture {
		// Already in daily view, nothing to do (though this function is only called in Future view currently)
	} else {
		// Check visible days
		for colIdx, dateKey := range m.dateKeys {
			for rowIdx, t := range m.Data[dateKey] {
				if t.ID == taskID {
					// Found it in a visible day!
					m.ShowFuture = false
					m.ColIdx = colIdx
					m.RowIdx = rowIdx
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	if !found {
		// Still in future or somewhere else
		m.clampRow()
	}

	return nil
}

func normalizeDueDateInput(dateStr string) (string, error) {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return "", fmt.Errorf("date is required in YYYY-MM-DD or MM-DD format")
	}

	// If only M-D provided, append current year
	if len(dateStr) == 5 {
		dateStr = fmt.Sprintf("%d-%s", time.Now().Year(), dateStr)
	}

	parsed, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", fmt.Errorf("invalid date; use YYYY-MM-DD or MM-DD")
	}

	return parsed.Format("2006-01-02"), nil
}
