package model

import (
	"fmt"
	"os"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dtt101/doitdoit/styles"
)

type State int

const (
	Browsing State = iota
	Adding
	ChoosingMoveDestination
	SettingMoveDate
)

// moveTarget is either an exact calendar date or the undated Future list.
type moveTarget struct {
	Date   string
	Future bool
}

type moveUndoSnapshot struct {
	Data       TodoData
	ShowFuture bool
	ColIdx     int
	RowIdx     int
}

type Model struct {
	Data        TodoData
	FilePath    string
	VisibleDays int
	// RetentionDays is zero for forever and positive for pruning completed
	// history older than that many days.
	RetentionDays int

	// Navigation
	ColIdx int
	RowIdx int

	// State
	State     State
	TextInput textinput.Model

	// Cache for date keys to keep order stable during a frame
	dateKeys []string
	todayKey string

	// Last observed state of the data file, so background reload checks can
	// tell external writes apart from our own.
	dataModTime time.Time
	dataSize    int64

	// Terminal dimensions
	width  int
	height int

	// Error handling
	Err error

	// Future View
	ShowFuture bool
	ShowHelp   bool

	// Brief flash on copy
	copyFlash bool

	// Click-only animation for the footer wordmark.
	brandFrame       int
	brandAnimationID uint64

	// Session-only move history.
	lastMoveTarget *moveTarget
	moveUndo       *moveUndoSnapshot
}

func NewModel(filePath string, visibleDays int) (Model, error) {
	return NewModelWithRetention(filePath, visibleDays, 0)
}

// NewModelWithRetention creates a model after applying the explicit retention
// period selected by the user. Zero means completed history is kept forever.
func NewModelWithRetention(filePath string, visibleDays, retentionDays int) (Model, error) {
	if visibleDays < 1 {
		return Model{}, fmt.Errorf("visible days must be at least 1")
	}

	data, err := Load(filePath, retentionDays)
	if err != nil {
		return Model{}, err
	}

	m := Model{
		Data:          data,
		FilePath:      filePath,
		VisibleDays:   visibleDays,
		RetentionDays: retentionDays,
		State:         Browsing,
		TextInput:     textinput.New(),
		todayKey:      time.Now().Format(dateLayout),
	}
	m.configureTextInput("New task...")
	m.Data.DistributeFutureTasks(visibleDays)
	m.updateDateKeys()
	m.trackFileState()
	return m, nil
}

// trackFileState records the data file's mtime and size so the reload ticker
// can ignore writes made by this process.
func (m *Model) trackFileState() {
	if fi, err := os.Stat(m.FilePath); err == nil {
		m.dataModTime = fi.ModTime()
		m.dataSize = fi.Size()
	}
}

func (m *Model) updateDateKeys() {
	// Reset the viewport to the next N days starting from today.
	today := startOfDay(time.Now())
	m.todayKey = today.Format(dateLayout)
	m.updateDateKeysFrom(today)
}

func (m *Model) updateDateKeysFrom(firstDay time.Time) {
	keys := make([]string, m.VisibleDays)
	for i := 0; i < m.VisibleDays; i++ {
		date := firstDay.AddDate(0, 0, i)
		keys[i] = date.Format("2006-01-02")
	}
	m.dateKeys = keys
}

func (m Model) firstVisibleDate() time.Time {
	if len(m.dateKeys) > 0 {
		if date, err := parseDate(m.dateKeys[0]); err == nil {
			return date
		}
	}
	return startOfDay(time.Now())
}

func (m Model) lastVisibleDate() time.Time {
	return m.firstVisibleDate().AddDate(0, 0, m.VisibleDays-1)
}

// shiftDateWindow moves the viewport by one day. It never permits dates before
// today, so navigation is infinite in the forward direction only.
func (m *Model) shiftDateWindow(days int) bool {
	firstDay := m.firstVisibleDate().AddDate(0, 0, days)
	if firstDay.Before(startOfDay(time.Now())) {
		return false
	}
	m.updateDateKeysFrom(firstDay)
	return true
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, dateTick(), reloadTick())
}

// persist is last-writer-wins: an external edit landing between the previous
// reload check and this save is overwritten, the same trade-off the web app
// makes. The reload ticker keeps that window to a few seconds.
func (m *Model) persist() {
	if err := m.Data.Save(m.FilePath); err != nil {
		m.Err = err
		return
	}
	m.Err = nil
	m.trackFileState()
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
	textInputStyles := m.TextInput.Styles()
	textInputStyles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	textInputStyles.Focused.Text = lipgloss.NewStyle().Foreground(styles.Text)
	textInputStyles.Blurred.Placeholder = textInputStyles.Focused.Placeholder
	textInputStyles.Blurred.Text = textInputStyles.Focused.Text
	m.TextInput.SetStyles(textInputStyles)
	m.TextInput.Prompt = ""
	m.TextInput.SetWidth(30)
	m.TextInput.Focus()
}
