package model

import (
	"fmt"

	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vs/doitdoit/styles"
)

type State int

const (
	Browsing State = iota
	Adding
	Moving
	SettingDate
)

type Model struct {
	Data        TodoData
	FilePath    string
	VisibleDays int

	// Navigation
	ColIdx int
	RowIdx int

	// State
	State     State
	TextInput textinput.Model

	// Cache for date keys to keep order stable during a frame
	dateKeys []string

	// Terminal dimensions
	width  int
	height int

	// Error handling
	Err error

	// Future View
	ShowFuture bool
}

func NewModel(filePath string, visibleDays int) (Model, error) {
	data, err := Load(filePath)
	if err != nil {
		return Model{}, err
	}

	ti := textinput.New()
	ti.Placeholder = "New task..."
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	ti.TextStyle = lipgloss.NewStyle().Foreground(styles.Text)
	ti.Prompt = ""
	ti.Width = 30
	ti.Focus()

	m := Model{
		Data:        data,
		FilePath:    filePath,
		VisibleDays: visibleDays,
		State:       Browsing,
		TextInput:   ti,
	}
	m.Data.DistributeFutureTasks(visibleDays)
	m.updateDateKeys()
	return m, nil
}

func (m *Model) updateDateKeys() {
	// Generate keys for the next N days starting from today
	keys := make([]string, m.VisibleDays)
	today := time.Now()
	for i := 0; i < m.VisibleDays; i++ {
		date := today.AddDate(0, 0, i)
		keys[i] = date.Format("2006-01-02")
	}
	m.dateKeys = keys
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch m.State {
		case Adding:
			switch msg.String() {
			case "enter":
				if m.TextInput.Value() != "" {
					m.addTask(m.TextInput.Value())
					m.TextInput.Reset()
					m.State = Browsing
					m.Data.Save(m.FilePath)
				}
			case "esc":
				m.TextInput.Reset()
				m.State = Browsing
			default:
				m.TextInput, cmd = m.TextInput.Update(msg)
			}
			return m, cmd

		case Browsing:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "right", "l":
				if !m.ShowFuture && m.ColIdx < m.VisibleDays-1 {
					m.ColIdx++
					m.clampRow()
				}
			case "left", "h":
				if !m.ShowFuture && m.ColIdx > 0 {
					m.ColIdx--
					m.clampRow()
				}
			case "up", "k":
				if m.RowIdx > 0 {
					m.RowIdx--
				}
			case "down", "j":
				currentDate := m.getCurrentKey()
				if m.RowIdx < len(m.Data[currentDate])-1 {
					m.RowIdx++
				}
			case "a":
				m.State = Adding
				return m, nil
			case "d":
				m.deleteTask()
				m.Data.Save(m.FilePath)
			case "enter", " ":
				m.toggleTask()
				m.Data.Save(m.FilePath)
			case "m":
				m.State = Moving
			case "f":
				m.ShowFuture = !m.ShowFuture
				m.RowIdx = 0
				m.clampRow()
			case "t":
				if m.ShowFuture {
					m.State = SettingDate
					m.TextInput.Reset() // Clear previous input
					m.TextInput.Placeholder = "YYYY-MM-DD or MM-DD"
					m.TextInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
					m.TextInput.TextStyle = lipgloss.NewStyle().Foreground(styles.Text)
					m.TextInput.Focus()
					return m, nil
				}
			}

		case Moving:
			switch msg.String() {
			case "esc", "m":
				m.State = Browsing
			case "right", "l":
				if !m.ShowFuture {
					m.moveTask(1)
					m.Data.Save(m.FilePath)
				}
			case "left", "h":
				if !m.ShowFuture {
					m.moveTask(-1)
					m.Data.Save(m.FilePath)
				}
			case "up", "k":
				m.reorderTask(-1)
				m.Data.Save(m.FilePath)
			case "down", "j":
				m.reorderTask(1)
				m.Data.Save(m.FilePath)
			}

		case SettingDate:
			switch msg.String() {
			case "enter":
				m.setTaskDate(m.TextInput.Value())
				m.TextInput.Reset()
				m.State = Browsing
				m.Data.Save(m.FilePath)
			case "esc":
				m.TextInput.Reset()
				m.State = Browsing
			default:
				m.TextInput, cmd = m.TextInput.Update(msg)
			}
			return m, cmd
		}
	}

	return m, cmd
}

func (m Model) View() string {
	var columns []string

	// Calculate dynamic width
	// App margins: 4 (2 left + 2 right)
	// Column margins: 2 per column (1 left + 1 right)
	// Column borders: 2 per column
	// Column padding: 2 per column
	// Total extra per column = 6

	availableWidth := m.width - 4
	if availableWidth < 0 {
		availableWidth = 0
	}

	colWidth := (availableWidth / m.VisibleDays) - 6
	if colWidth < 10 {
		colWidth = 10 // Minimum width
	}

	// Pre-calculate column contents to determine max height
	var colContents []string
	maxContentHeight := 0

	// Minimum height based on window size
	// App margins: 2 (1 top + 1 bottom)
	// Footer overhead: ~7 lines
	// Column overhead: 4 lines (2 border + 2 padding)
	minTotalHeight := m.height - 9
	if minTotalHeight < 10 {
		minTotalHeight = 10
	}
	minContentHeight := minTotalHeight - 4 // Subtract border+padding

	// If showing future, we just have one column
	keys := m.dateKeys
	if m.ShowFuture {
		keys = []string{"Future"}
		// Adjust colWidth for single column? Or keep it same for consistency?
		// Let's make it full width for Future view or maybe centered?
		// User said "purely a list of tasks", maybe similar to day view but one big column.
		// Let's stick to the calculated colWidth for now, or maybe make it wider.
		// Actually, if it's a single column, let's use the full available width.
		colWidth = availableWidth - 6
	}

	for i, dateStr := range keys {
		isFocused := m.State != Adding && (m.ShowFuture || m.ColIdx == i)

		// Header
		header := ""
		if m.ShowFuture {
			header = "Future"
		} else {
			displayDate, _ := time.Parse("2006-01-02", dateStr)
			header = displayDate.Format("Mon, Jan 02")
			if dateStr == time.Now().Format("2006-01-02") {
				header = "Today"
			}
		}

		title := styles.TitleStyle.Render(header)

		// Tasks
		var taskViews []string
		tasks := m.Data[dateStr]

		for j, task := range tasks {
			var style lipgloss.Style
			if task.Completed {
				style = styles.CompletedTaskStyle
			} else {
				style = styles.TaskStyle
			}

			title := task.Title
			if m.ShowFuture && task.DueDate != "" {
				title += fmt.Sprintf(" (%s)", task.DueDate)
			}

			if isFocused && m.RowIdx == j {
				if m.State == Moving {
					// Use special moving style with highlight background
					style = styles.MovingTaskStyle
				} else {
					// Normal selection highlight
					style = style.Copy().Foreground(styles.Highlight).Bold(true)
				}
			}

			// Calculate title width to ensure proper wrapping
			titleWidth := colWidth
			if titleWidth < 1 {
				titleWidth = 1
			}

			taskViews = append(taskViews, style.Width(titleWidth).Render(title))

			// Add a blank line between tasks
			if j < len(tasks)-1 {
				taskViews = append(taskViews, "")
			}
		}

		// Input field if adding to this column
		if (m.State == Adding || m.State == SettingDate) && (m.ShowFuture || m.ColIdx == i) {
			// Add spacing before input if there are tasks
			if len(tasks) > 0 {
				taskViews = append(taskViews, "")
			}

			// Match TaskStyle padding
			inputStyle := lipgloss.NewStyle()
			prefix := ""
			if m.State == SettingDate {
				prefix = "Due Date: "
			}
			taskViews = append(taskViews, inputStyle.Render(prefix+m.TextInput.View()))
		} else if len(tasks) == 0 && !((m.State == Adding || m.State == SettingDate) && (m.ShowFuture || m.ColIdx == i)) {
			taskViews = append(taskViews, lipgloss.NewStyle().Foreground(styles.Subtle).Render("No tasks"))
		}

		// Assemble content
		content := lipgloss.JoinVertical(lipgloss.Left, title, lipgloss.JoinVertical(lipgloss.Left, taskViews...))
		colContents = append(colContents, content)

		h := lipgloss.Height(content)
		if h > maxContentHeight {
			maxContentHeight = h
		}
	}

	// Ensure we meet the minimum window height
	if maxContentHeight < minContentHeight {
		maxContentHeight = minContentHeight
	}

	// Render columns with unified height
	for i, content := range colContents {
		isFocused := m.State != Adding && m.State != SettingDate && (m.ShowFuture || m.ColIdx == i)

		style := styles.ColumnStyle.Copy().Width(colWidth).Height(maxContentHeight)
		if isFocused {
			style = styles.FocusedColumnStyle.Copy().Width(colWidth).Height(maxContentHeight)
		}

		columns = append(columns, style.Render(content))
	}

	return styles.AppStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, columns...) + "\n\n" + m.helpView())
}

func (m Model) helpView() string {
	var help string
	switch m.State {
	case Browsing:
		help = "a: add • d: delete • space: toggle • m: move • f: future"
		if m.ShowFuture {
			help += " • t: date"
		}
		help += " • arrows/hjkl: nav • q: quit"
	case Adding:
		help = "enter: save • esc: cancel"
	case Moving:
		help = "←/→/h/l: move day • ↑/↓/k/j: move up/down • m/esc: done"
	case SettingDate:
		help = "enter: save date • esc: cancel"
	}
	return styles.HelpStyle.Render(help)
}

// Logic helpers

func (m Model) getCurrentKey() string {
	if m.ShowFuture {
		return "Future"
	}
	return m.dateKeys[m.ColIdx]
}

func (m *Model) clampRow() {
	currentDate := m.getCurrentKey()
	count := len(m.Data[currentDate])
	if m.RowIdx >= count {
		m.RowIdx = count - 1
	}
	if m.RowIdx < 0 {
		m.RowIdx = 0
	}
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

func (m *Model) setTaskDate(dateStr string) {
	currentDate := m.getCurrentKey()
	tasks := m.Data[currentDate]
	if len(tasks) == 0 || m.RowIdx >= len(tasks) {
		return
	}

	// Basic validation (could be improved)
	if len(dateStr) < 5 { // At least M-D
		return
	}

	// If only M-D provided, append current year
	if len(dateStr) == 5 {
		dateStr = fmt.Sprintf("%d-%s", time.Now().Year(), dateStr)
	}

	// Update task
	taskID := tasks[m.RowIdx].ID
	tasks[m.RowIdx].DueDate = dateStr
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
}
