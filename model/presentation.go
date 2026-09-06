package model

// taskSection groups rows for display without rearranging the task file. Row
// indices always refer to the original bucket, including when rows are hidden.
type taskSection struct {
	label     string
	rows      []int
	collapsed bool
}

func (m Model) taskSections(key string) []taskSection {
	active := taskSection{}
	done := taskSection{label: "Completed", collapsed: m.HideCompleted}
	for i, task := range m.Data[key] {
		if task.Completed {
			done.rows = append(done.rows, i)
		} else {
			active.rows = append(active.rows, i)
		}
	}
	sections := []taskSection{active}
	if len(done.rows) > 0 {
		sections = append(sections, done)
	}
	return sections
}

func (m Model) taskRows(key string) []int {
	var rows []int
	for _, section := range m.taskSections(key) {
		if !section.collapsed {
			rows = append(rows, section.rows...)
		}
	}
	return rows
}

func (m Model) hasSelectedTask() bool {
	tasks := m.Data[m.getCurrentKey()]
	return m.RowIdx >= 0 && m.RowIdx < len(tasks) && (!m.HideCompleted || !tasks[m.RowIdx].Completed)
}

func (m *Model) moveSelection(direction int) {
	rows := m.taskRows(m.getCurrentKey())
	for i, row := range rows {
		if row == m.RowIdx {
			m.RowIdx = rows[min(max(0, i+direction), len(rows)-1)]
			return
		}
	}
	m.clampRow()
}
