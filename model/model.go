package model

import (
	"crypto/sha256"
	"fmt"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dtt101/doitdoit/styles"
	"github.com/dtt101/doitdoit/taskstore"
)

type State int

const (
	Browsing State = iota
	Adding
	Editing
	ChoosingMoveDestination
	SettingMoveDate
)

// moveTarget is either an exact calendar date or the undated Future list.
type moveTarget struct {
	Date   string
	Future bool
}

type moveUndoSnapshot struct {
	Data          TodoData
	ShowFuture    bool
	FocusToday    bool
	HideCompleted bool
	DateKeys      []string
	ColIdx        int
	RowIdx        int
}

type Model struct {
	Data        TodoData
	FilePath    string
	VisibleDays int
	// RetentionDays is zero for forever and positive for pruning completed
	// history older than that many days.
	RetentionDays int

	store     taskstore.Store
	storePath string

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
	dataHash    [sha256.Size]byte
	dataExists  bool

	// Terminal dimensions
	width  int
	height int

	// Error handling
	Err error
	// A failed save leaves a draft in memory. Input validation and cancellation
	// may clear Err, but only a successful save may make this draft reloadable.
	saveErr error

	// Last successful move, delete, or undo; cleared when task history changes.
	feedback string

	// Session-only explanation of the last observed rollover; never stored.
	carriedForward int

	// Future View
	ShowFuture    bool
	ShowHelp      bool
	FocusToday    bool
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
	return newModelWithStore(filePath, visibleDays, retentionDays, taskstore.NewJSON(filePath))
}

func newModelWithStore(filePath string, visibleDays, retentionDays int, store taskstore.Store) (Model, error) {
	if visibleDays < 1 {
		return Model{}, fmt.Errorf("visible days must be at least 1")
	}

	snapshot, carriedForward, err := taskstore.LoadStore(store, retentionDays)
	if err != nil {
		return Model{}, err
	}

	m := Model{
		Data:           TodoData(snapshot.Data),
		store:          store,
		storePath:      filePath,
		carriedForward: carriedForward,
		FilePath:       filePath,
		VisibleDays:    visibleDays,
		RetentionDays:  retentionDays,
		State:          Browsing,
		TextInput:      textinput.New(),
		todayKey:       time.Now().Format(dateLayout),
	}
	m.configureTextInput("New task...")
	m.Data.DistributeFutureTasks(visibleDays)
	m.updateDateKeys()
	m.recordRevision(snapshot.Revision)
	return m, nil
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

func (m Model) getCurrentKey() string {
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

func (m *Model) configureTextInput(placeholder string) {
	m.TextInput.Reset()
	m.TextInput.Placeholder = placeholder
	textInputStyles := m.TextInput.Styles()
	textInputStyles.Focused.Placeholder = lipgloss.NewStyle().Foreground(styles.Subtle)
	textInputStyles.Focused.Text = lipgloss.NewStyle().Foreground(styles.Text)
	textInputStyles.Blurred.Placeholder = textInputStyles.Focused.Placeholder
	textInputStyles.Blurred.Text = textInputStyles.Focused.Text
	m.TextInput.SetStyles(textInputStyles)
	m.TextInput.Prompt = ""
	m.resizeTextInput()
	m.TextInput.Focus()
}
