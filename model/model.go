package model

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dtt101/doitdoit/styles"
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

	m := Model{
		Data:        data,
		FilePath:    filePath,
		VisibleDays: visibleDays,
		State:       Browsing,
		TextInput:   textinput.New(),
	}
	m.configureTextInput("New task...")
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

func (m *Model) persist() {
	if err := m.Data.Save(m.FilePath); err != nil {
		m.Err = err
		return
	}
	m.Err = nil
}

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

func (m *Model) configureTextInput(placeholder string) {
	m.TextInput.Reset()
	m.TextInput.Placeholder = placeholder
	m.TextInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	m.TextInput.TextStyle = lipgloss.NewStyle().Foreground(styles.Text)
	m.TextInput.Prompt = ""
	m.TextInput.Width = 30
	m.TextInput.Focus()
}
