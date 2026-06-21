package model

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type copyFlashDoneMsg struct{}

type dateTickMsg time.Time

// dateTick schedules a wake-up so the visible date columns can be refreshed
// when the day rolls over while the app is left running.
func dateTick() tea.Cmd {
	return tea.Tick(time.Minute, func(t time.Time) tea.Msg {
		return dateTickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)
	case copyFlashDoneMsg:
		m.copyFlash = false
		return m, nil
	case dateTickMsg:
		return m.handleDateTick()
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	default:
		return m, nil
	}
}

func (m Model) handleDateTick() (tea.Model, tea.Cmd) {
	if len(m.dateKeys) == 0 || m.dateKeys[0] != time.Now().Format("2006-01-02") {
		focusedDate := ""
		if !m.ShowFuture && m.ColIdx >= 0 && m.ColIdx < len(m.dateKeys) {
			focusedDate = m.dateKeys[m.ColIdx]
		}

		m.Data.rollOverIncompleteTasks()
		m.Data.pruneOldTasks()
		m.Data.DistributeFutureTasks(m.VisibleDays)
		m.updateDateKeys()
		for i, dateKey := range m.dateKeys {
			if dateKey == focusedDate {
				m.ColIdx = i
				break
			}
		}
		m.clampRow()
		m.persist()
	}
	return m, dateTick()
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
	case ">":
		if !m.ShowFuture {
			m.postponeTask()
			m.persist()
		}
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
	case "y":
		m.copyTask()
		if m.copyFlash {
			return m, tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
				return copyFlashDoneMsg{}
			})
		}
	}

	return m, nil
}

func (m Model) handleMovingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "m":
		m.State = Browsing
	case "y":
		m.copyTask()
		if m.copyFlash {
			return m, tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
				return copyFlashDoneMsg{}
			})
		}
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
