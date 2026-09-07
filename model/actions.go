package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/dtt101/doitdoit/taskstore"
)

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

	taskstore.Data(m.Data).Insert(currentDate, newTask)
}

func (m *Model) deleteTask() bool {
	currentDate := m.getCurrentKey()
	if !m.hasSelectedTask() {
		return false
	}

	m.captureMoveUndo()
	taskstore.Data(m.Data).Delete(currentDate, m.RowIdx)
	m.clampRow()
	return true
}

func (m *Model) toggleTask() bool {
	if !m.hasSelectedTask() {
		return false
	}
	m.captureMoveUndo()
	taskstore.Data(m.Data).Toggle(m.getCurrentKey(), m.RowIdx)
	return true
}

func (m *Model) editTask(title string) bool {
	title = strings.TrimSpace(title)
	currentDate := m.getCurrentKey()
	tasks := m.Data[currentDate]
	if title == "" || !m.hasSelectedTask() || tasks[m.RowIdx].Title == title {
		return false
	}
	m.captureMoveUndo()
	taskstore.Data(m.Data).Edit(currentDate, m.RowIdx, title)
	return true
}

func cloneTodoData(data TodoData) TodoData { return TodoData(taskstore.Clone(taskstore.Data(data))) }

func (m *Model) captureMoveUndo() {
	m.feedback = ""
	m.moveUndo = &moveUndoSnapshot{
		Data:          cloneTodoData(m.Data),
		ShowFuture:    m.ShowFuture,
		FocusToday:    m.FocusToday,
		HideCompleted: m.HideCompleted,
		DateKeys:      append([]string(nil), m.dateKeys...),
		ColIdx:        m.ColIdx,
		RowIdx:        m.RowIdx,
	}
}

func (m *Model) clearMoveUndo() {
	m.moveUndo = nil
	m.feedback = ""
}

func (m *Model) undoMove() bool {
	if m.moveUndo == nil {
		return false
	}

	snapshot := m.moveUndo
	m.Data = cloneTodoData(snapshot.Data)
	m.ShowFuture = snapshot.ShowFuture
	m.FocusToday = snapshot.FocusToday
	m.HideCompleted = snapshot.HideCompleted
	if len(snapshot.DateKeys) > 0 {
		m.dateKeys = append([]string(nil), snapshot.DateKeys...)
	}
	m.ColIdx = snapshot.ColIdx
	m.RowIdx = snapshot.RowIdx
	m.clearMoveUndo()
	m.clampRow()
	return true
}

func (m *Model) reorderTask(direction int) bool {
	currentDate := m.getCurrentKey()
	tasks := m.Data[currentDate]
	if !m.hasSelectedTask() {
		return false
	}

	newRowIdx := -1
	for _, section := range m.taskSections(currentDate) {
		if section.collapsed {
			continue
		}
		for i, row := range section.rows {
			if row == m.RowIdx && i+direction >= 0 && i+direction < len(section.rows) {
				newRowIdx = section.rows[i+direction]
				break
			}
		}
	}
	// Reordering stays within the displayed section, including Ideas and
	// Scheduled in Future. Completion grouping remains a bucket invariant.
	if newRowIdx < 0 || tasks[m.RowIdx].Completed != tasks[newRowIdx].Completed {
		return false
	}

	m.captureMoveUndo()
	taskstore.Data(m.Data).Reorder(currentDate, m.RowIdx, newRowIdx)
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
	if !m.hasSelectedTask() {
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
	taskstore.Data(m.Data).Move(sourceKey, m.RowIdx, targetKey, dueDate)

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
	if !m.hasSelectedTask() {
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
