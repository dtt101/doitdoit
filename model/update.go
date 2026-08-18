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
	case reloadTickMsg:
		return m.handleReloadTick()
	case dataFileCheckedMsg:
		return m.handleDataFileChecked(msg)
	case ThemeReloadMsg:
		return m.handleThemeReload(msg)
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	default:
		return m, nil
	}
}

func (m Model) handleDateTick() (tea.Model, tea.Cmd) {
	todayKey := time.Now().Format(dateLayout)
	dayChanged := m.todayKey != "" && m.todayKey != todayKey
	if m.todayKey == "" && (len(m.dateKeys) == 0 || m.firstVisibleDate().Before(startOfDay(time.Now()))) {
		dayChanged = true
	}
	if dayChanged {
		focusedDate := ""
		if !m.ShowFuture && m.ColIdx >= 0 && m.ColIdx < len(m.dateKeys) {
			focusedDate = m.dateKeys[m.ColIdx]
		}

		m.Data.rollOverIncompleteTasks()
		m.Data.pruneOldTasks()
		m.clearMoveUndo()
		firstDay := m.firstVisibleDate()
		if firstDay.Before(startOfDay(time.Now())) {
			firstDay = startOfDay(time.Now())
		}
		m.updateDateKeysFrom(firstDay)
		m.todayKey = todayKey
		m.Data.distributeFutureTasksThrough(m.lastVisibleDate())
		m.ColIdx = 0
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
	case ChoosingMoveDestination:
		return m.handleChoosingMoveDestinationKey(msg)
	case SettingMoveDate:
		return m.handleSettingMoveDateKey(msg)
	default:
		return m, nil
	}
}

func (m Model) handleAddingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if m.TextInput.Value() != "" {
			m.clearMoveUndo()
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
		if !m.ShowFuture {
			if m.ColIdx < len(m.dateKeys)-1 {
				m.ColIdx++
			} else if m.shiftDateWindow(1) {
				if m.Data.distributeFutureTasksThrough(m.lastVisibleDate()) {
					m.persist()
				}
			}
			m.clampRow()
		}
	case "left", "h":
		if !m.ShowFuture {
			if m.ColIdx > 0 {
				m.ColIdx--
			} else {
				m.shiftDateWindow(-1)
			}
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
		if m.deleteTask() {
			m.clearMoveUndo()
			m.persist()
		}
	case "enter", " ":
		if m.toggleTask() {
			m.clearMoveUndo()
			m.persist()
		}
	case "m":
		currentKey := m.getCurrentKey()
		if m.RowIdx >= 0 && m.RowIdx < len(m.Data[currentKey]) {
			m.State = ChoosingMoveDestination
		}
	case "J":
		if m.reorderTask(1) {
			m.persist()
		}
	case "K":
		if m.reorderTask(-1) {
			m.persist()
		}
	case ".":
		if m.repeatMove() {
			m.persist()
		}
	case "u":
		if m.undoMove() {
			m.persist()
		}
	case "f":
		m.ShowFuture = !m.ShowFuture
		m.RowIdx = 0
		m.clampRow()
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

func (m Model) handleChoosingMoveDestinationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.State = Browsing
	case "t":
		moved := m.scheduleTask(moveTarget{Date: time.Now().Format(dateLayout)})
		m.State = Browsing
		if moved {
			m.persist()
		}
	case "f":
		moved := m.scheduleTask(moveTarget{Future: true})
		m.State = Browsing
		if moved {
			m.persist()
		}
	case "d":
		m.State = SettingMoveDate
		m.configureTextInput("YYYY-MM-DD or MM-DD")
		return m, nil
	case "1", "2", "3", "4", "5", "6", "7":
		days := int(msg.String()[0] - '0')
		moved := m.scheduleTask(m.relativeMoveTarget(days))
		m.State = Browsing
		if moved {
			m.persist()
		}
	}

	return m, nil
}

func (m Model) handleSettingMoveDateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		normalizedDate, err := normalizeDueDateInput(m.TextInput.Value())
		if err != nil {
			m.Err = err
			return m, nil
		}
		m.Err = nil
		moved := m.scheduleTask(moveTarget{Date: normalizedDate})
		m.TextInput.Reset()
		m.State = Browsing
		if moved {
			m.persist()
		}
	case tea.KeyEsc:
		m.TextInput.Reset()
		m.Err = nil
		m.State = ChoosingMoveDestination
	default:
		var cmd tea.Cmd
		m.TextInput, cmd = m.TextInput.Update(msg)
		return m, cmd
	}

	return m, nil
}
