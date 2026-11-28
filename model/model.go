package model

import (
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/dtt101/doitdoit/styles"
)

type State int

const (
	Browsing State = iota
	Adding
	Editing
	ChoosingMoveDestination
	SettingMoveDate
	EditingNotes
)

// moveTarget is either an exact calendar date or the undated Future list.
type moveTarget struct {
	Date   string
	Future bool
}

type moveUndoSnapshot struct {
	Data          TodoData
	ShowFuture    bool
	HideCompleted bool
	DateKeys      []string
	ColIdx        int
	RowIdx        int
}

type Model struct {
	cloud       *serviceState
	ShowOverdue bool
	ShowDeleted bool
	Data        TodoData
	VisibleDays int

	// Navigation
	ColIdx int
	RowIdx int

	// State
	State        State
	TextInput    textinput.Model
	notesInput   textarea.Model
	notesKey     string
	notesRow     int
	notesPreview bool
	notesOffset  int

	// Cache for date keys to keep order stable during a frame
	dateKeys []string
	todayKey string

	// Terminal dimensions
	width  int
	height int

	// Error handling
	Err     error
	saveErr error

	// Last successful move, delete, or undo; cleared when task history changes.
	feedback string

	// Views
	ShowFuture    bool
	ShowHelp      bool
	HideCompleted bool

	// Presentation state is independent of the date window used for scheduling.
	columnOffset  int
	scrollOffsets map[string]int
	helpOffset    int

	// Brief flash on copy
	copyFlash bool

	taskAnimationID     uint64
	taskFrame           int
	taskGlow            float64
	taskAnimationTaskID string
	taskCompleting      bool

	// Click-only animation for the footer wordmark.
	brandFrame       int
	brandAnimationID uint64

	// Session-only move history.
	lastMoveTarget *moveTarget
	moveUndo       *moveUndoSnapshot
}

func (m *Model) updateDateKeys() {
	// Reset the viewport to the next N days starting from today.
	today := startOfDay(m.serviceNow())
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
	return startOfDay(m.serviceNow())
}

// shiftDateWindow moves the viewport by one day. It never permits dates before
// today, so navigation is infinite in the forward direction only.
func (m *Model) shiftDateWindow(days int) bool {
	firstDay := m.firstVisibleDate().AddDate(0, 0, days)
	if firstDay.Before(startOfDay(m.serviceNow())) {
		return false
	}
	m.updateDateKeysFrom(firstDay)
	return true
}

func (m Model) Init() tea.Cmd {
	if m.cloud != nil {
		return tea.Batch(textinput.Blink, dateTick(), serviceTick(), m.serviceRead(true))
	}
	return tea.Batch(textinput.Blink, dateTick())
}

func (m Model) getCurrentKey() string {
	if m.ShowDeleted {
		return "Deleted"
	}
	if m.ShowOverdue {
		return "Overdue"
	}
	if m.ShowFuture {
		return "Future"
	}
	return m.dateKeys[m.ColIdx]
}

func (m *Model) clampRow() {
	rows := m.taskRows(m.getCurrentKey())
	if len(rows) == 0 {
		m.RowIdx = 0
		return
	}
	for _, row := range rows {
		if row == m.RowIdx {
			return
		}
	}
	for _, row := range rows {
		if row >= m.RowIdx {
			m.RowIdx = row
			return
		}
	}
	m.RowIdx = rows[len(rows)-1]
}

func (m *Model) configureTextInput(placeholder string) tea.Cmd {
	m.TextInput.Reset()
	m.TextInput.Placeholder = placeholder
	textInputStyles := m.TextInput.Styles()
	textInputStyles.Focused.Placeholder = styles.SubtleStyle
	textInputStyles.Focused.Text = styles.TextStyle
	textInputStyles.Blurred.Placeholder = textInputStyles.Focused.Placeholder
	textInputStyles.Blurred.Text = textInputStyles.Focused.Text
	m.TextInput.SetStyles(textInputStyles)
	m.TextInput.Prompt = ""
	m.resizeTextInput()
	return m.TextInput.Focus()
}
