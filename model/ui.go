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
}

func NewModel(filePath string, visibleDays int) (Model, error) {
	data, err := Load(filePath)
	if err != nil {
		return Model{}, err
	}

	ti := textinput.New()
	ti.Placeholder = "New task..."
	ti.Focus()

	m := Model{
		Data:        data,
		FilePath:    filePath,
		VisibleDays: visibleDays,
		State:       Browsing,
		TextInput:   ti,
	}
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
				if m.ColIdx < m.VisibleDays-1 {
					m.ColIdx++
					m.clampRow()
				}
			case "left", "h":
				if m.ColIdx > 0 {
					m.ColIdx--
					m.clampRow()
				}
			case "up", "k":
				if m.RowIdx > 0 {
					m.RowIdx--
				}
			case "down", "j":
				currentDate := m.dateKeys[m.ColIdx]
				if m.RowIdx < len(m.Data[currentDate])-1 {
					m.RowIdx++
				}
			case "n":
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
			}

		case Moving:
			switch msg.String() {
			case "esc":
				m.State = Browsing
			case "right", "l":
				m.moveTask(1)
				m.Data.Save(m.FilePath)
				m.State = Browsing
			case "left", "h":
				m.moveTask(-1)
				m.Data.Save(m.FilePath)
				m.State = Browsing
			case "up", "k":
				m.reorderTask(-1)
				m.Data.Save(m.FilePath)
			case "down", "j":
				m.reorderTask(1)
				m.Data.Save(m.FilePath)
			}
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

	for i, dateStr := range m.dateKeys {
		isFocused := m.State != Adding && m.ColIdx == i

		// Header
		displayDate, _ := time.Parse("2006-01-02", dateStr)
		header := displayDate.Format("Mon, Jan 02")
		if dateStr == time.Now().Format("2006-01-02") {
			header = "Today"
		}

		title := styles.TitleStyle.Render(header)

		// Tasks
		var taskViews []string
		tasks := m.Data[dateStr]

		for j, task := range tasks {
			cursor := " "
			if isFocused && m.RowIdx == j {
				cursor = ">"
			}

			checked := "[ ]"
			if task.Completed {
				checked = "[x]"
			}

			taskStr := fmt.Sprintf("%s %s %s", cursor, checked, task.Title)

			if isFocused && m.RowIdx == j {
				if m.State == Moving {
					taskStr += " (Move: <- ->)"
				}
				taskViews = append(taskViews, styles.SelectedTaskStyle.Render(taskStr))
			} else {
				taskViews = append(taskViews, styles.TaskStyle.Render(taskStr))
			}
		}

		// Input field if adding to this column
		if m.State == Adding && m.ColIdx == i {
			taskViews = append(taskViews, lipgloss.NewStyle().Foreground(styles.Highlight).Render("> ")+m.TextInput.View())
		} else if len(tasks) == 0 && !(m.State == Adding && m.ColIdx == i) {
			taskViews = append(taskViews, lipgloss.NewStyle().Foreground(styles.Subtle).Render("No tasks"))
		}

		// Assemble column
		colContent := lipgloss.JoinVertical(lipgloss.Left, title, lipgloss.JoinVertical(lipgloss.Left, taskViews...))

		style := styles.ColumnStyle.Copy().Width(colWidth)
		if isFocused {
			style = styles.FocusedColumnStyle.Copy().Width(colWidth)
		}

		columns = append(columns, style.Render(colContent))
	}

	return styles.AppStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, columns...) + "\n\n" + m.helpView())
}

func (m Model) helpView() string {
	var help string
	switch m.State {
	case Browsing:
		help = "n: new • d: delete • space: toggle • m: move • arrows/hjkl: nav • q: quit"
	case Adding:
		help = "enter: save • esc: cancel"
	case Moving:
		help = "←/→/h/l: move day • ↑/↓/k/j: move up/down • esc: cancel"
	}
	return styles.HelpStyle.Render(help)
}

// Logic helpers

func (m *Model) clampRow() {
	currentDate := m.dateKeys[m.ColIdx]
	count := len(m.Data[currentDate])
	if m.RowIdx >= count {
		m.RowIdx = count - 1
	}
	if m.RowIdx < 0 {
		m.RowIdx = 0
	}
}

func (m *Model) addTask(title string) {
	currentDate := m.dateKeys[m.ColIdx]
	newTask := Task{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Title:     title,
		CreatedAt: time.Now(),
		Completed: false,
	}
	m.Data[currentDate] = append(m.Data[currentDate], newTask)
}

func (m *Model) deleteTask() {
	currentDate := m.dateKeys[m.ColIdx]
	tasks := m.Data[currentDate]
	if len(tasks) == 0 || m.RowIdx >= len(tasks) {
		return
	}

	m.Data[currentDate] = append(tasks[:m.RowIdx], tasks[m.RowIdx+1:]...)
	m.clampRow()
}

func (m *Model) toggleTask() {
	currentDate := m.dateKeys[m.ColIdx]
	if len(m.Data[currentDate]) > m.RowIdx {
		m.Data[currentDate][m.RowIdx].Completed = !m.Data[currentDate][m.RowIdx].Completed
	}
}

func (m *Model) moveTask(direction int) {
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
	m.Data[targetDate] = append(m.Data[targetDate], taskToMove)

	// Follow the task
	m.ColIdx = targetColIdx
	m.RowIdx = len(m.Data[targetDate]) - 1
}

func (m *Model) reorderTask(direction int) {
	currentDate := m.dateKeys[m.ColIdx]
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
