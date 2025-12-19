package model

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	default:
		return m, nil
	}
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	return m, nil
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.State {
	case Adding:
		return m.handleAddingKey(msg)
	case Browsing:
		return m.handleBrowsingKey(msg)
	case Moving:
		return m.handleMovingKey(msg)
	case SettingDate:
		return m.handleSettingDateKey(msg)
	default:
		return m, nil
	}
}

func (m Model) handleAddingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if m.TextInput.Value() != "" {
			m.addTask(m.TextInput.Value())
			m.TextInput.Reset()
			m.State = Browsing
			m.persist()
		}
	case tea.KeyEsc:
		m.TextInput.Reset()
		m.State = Browsing
	default:
		var cmd tea.Cmd
		m.TextInput, cmd = m.TextInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleBrowsingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		m.configureTextInput("New task...")
		return m, nil
	case "d":
		m.deleteTask()
		m.persist()
	case "enter", " ":
		m.toggleTask()
		m.persist()
	case "m":
		m.State = Moving
	case "f":
		m.ShowFuture = !m.ShowFuture
		m.RowIdx = 0
		m.clampRow()
	case "t":
		if m.ShowFuture {
			m.State = SettingDate
			m.configureTextInput("YYYY-MM-DD or MM-DD")
			return m, nil
		}
	case "T":
		if m.ShowFuture {
			m.moveFutureTaskToToday()
			m.persist()
		}
	}

	return m, nil
}

func (m Model) handleMovingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "m":
		m.State = Browsing
	case "right", "l":
		if !m.ShowFuture {
			m.moveTask(1)
			m.persist()
		}
	case "left", "h":
		if !m.ShowFuture {
			m.moveTask(-1)
			m.persist()
		}
	case "up", "k":
		m.reorderTask(-1)
		m.persist()
	case "down", "j":
		m.reorderTask(1)
		m.persist()
	case "f":
		if !m.ShowFuture {
			// Move to Future
			currentDate := m.getCurrentKey()
			tasks := m.Data[currentDate]
			if len(tasks) > 0 && m.RowIdx < len(tasks) {
				task := tasks[m.RowIdx]
				// Remove from current
				m.Data[currentDate] = append(tasks[:m.RowIdx], tasks[m.RowIdx+1:]...)

				// Add to Future
				futureTasks := m.Data["Future"]
				m.Data["Future"] = append(futureTasks, task)

				m.clampRow()
				m.State = Browsing
				m.persist()
			}
		}
	}

	return m, nil
}

func (m Model) handleSettingDateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if err := m.setTaskDate(m.TextInput.Value()); err != nil {
			return m, nil
		}
		m.TextInput.Reset()
		m.State = Browsing
		m.persist()
	case tea.KeyEsc:
		m.TextInput.Reset()
		m.State = Browsing
	default:
		var cmd tea.Cmd
		m.TextInput, cmd = m.TextInput.Update(msg)
		return m, cmd
	}

	return m, nil
}
