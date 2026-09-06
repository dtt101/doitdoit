package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func resizeModel(m Model, width, height int) Model {
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return updated.(Model)
}

func pressKeyCode(m Model, code rune) Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: code})
	return updated.(Model)
}

func assertFitsTerminal(t *testing.T, m Model) {
	t.Helper()
	view := m.View().Content
	if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
		t.Fatalf("view %dx%d exceeds terminal %dx%d:\n%s", lipgloss.Width(view), lipgloss.Height(view), m.width, m.height, ansi.Strip(view))
	}
}

func TestResponsiveColumnsPreserveSelectionAndStorage(t *testing.T) {
	m := newFeedbackTestModel(t)
	m.VisibleDays = 7
	m.updateDateKeys()
	m.ColIdx = 5
	m.Data[m.dateKeys[5]] = []Task{{ID: "selected", Title: "Keep me selected"}}
	m.Data["Future"] = []Task{{ID: "later", Title: "Later", DueDate: dayKey(8)}}
	m.persist()
	before, err := os.ReadFile(m.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ width, count int }{{240, 7}, {100, 3}, {80, 2}, {48, 1}, {24, 1}, {240, 7}} {
		m = resizeModel(m, tc.width, 24)
		if len(m.visibleKeys()) != tc.count || len(m.dateKeys) != 7 || m.VisibleDays != 7 {
			t.Fatalf("width=%d: visible=%v logical=%v maximum=%d", tc.width, m.visibleKeys(), m.dateKeys, m.VisibleDays)
		}
		if m.getCurrentKey() != dayKey(5) || m.Data[m.getCurrentKey()][m.RowIdx].ID != "selected" {
			t.Fatal("resizing changed the selected task")
		}
		if !strings.Contains(ansi.Strip(m.View().Content), "> [ ] Keep me") {
			t.Fatal("selected task fell outside the rendered columns")
		}
		assertFitsTerminal(t, m)
	}
	after, err := os.ReadFile(m.FilePath)
	if err != nil || string(before) != string(after) || len(m.Data["Future"]) != 1 {
		t.Fatalf("resizing changed storage: %v", err)
	}
}

func TestResponsiveNavigationShowsFocusedDay(t *testing.T) {
	m := resizeModel(newFeedbackTestModel(t), 40, 14)
	for i := 0; i < 10; i++ {
		if got := m.visibleKeys(); len(got) != 1 || got[0] != dayKey(i) {
			t.Fatalf("day %d: visible=%v selected=%s", i, got, m.getCurrentKey())
		}
		m = pressRune(m, 'l')
	}
	for i := 10; i > 0; i-- {
		m = pressRune(m, 'h')
		if m.visibleKeys()[0] != dayKey(i-1) {
			t.Fatalf("returning left showed %v", m.visibleKeys())
		}
	}
	m = pressRune(m, 'h')
	if m.visibleKeys()[0] != dayKey(0) {
		t.Fatal("navigation moved before Today")
	}
}

func TestSingleDayArgumentStillAllowsCalendarNavigation(t *testing.T) {
	m := newFeedbackTestModel(t)
	m.VisibleDays = 1
	m.updateDateKeys()
	m = resizeModel(m, 200, 24)
	m = pressRune(m, 'l')
	if len(m.visibleKeys()) != 1 || m.getCurrentKey() != dayKey(1) || m.FocusToday {
		t.Fatal("-days 1 must be a navigable calendar, independent of Today focus")
	}
	m = pressRune(m, 't')
	if len(m.visibleKeys()) != 1 || m.getCurrentKey() != dayKey(0) {
		t.Fatal("Today jump did not preserve the one-day maximum")
	}
}

func TestBoardFitsAcrossSizesAndInteractionStates(t *testing.T) {
	for _, size := range [][2]int{{24, 10}, {30, 12}, {40, 12}, {48, 16}, {60, 20}, {80, 24}, {100, 30}} {
		for _, state := range []State{Browsing, Adding, Editing, ChoosingMoveDestination, SettingMoveDate} {
			for _, future := range []bool{false, true} {
				t.Run(fmt.Sprintf("%dx%d/state%d/future%v", size[0], size[1], state, future), func(t *testing.T) {
					m := newFeedbackTestModel(t)
					m.ShowFuture = future
					key := m.getCurrentKey()
					for i := 0; i < 30; i++ {
						m.Data[key] = append(m.Data[key], Task{ID: fmt.Sprint(i), Title: strings.Repeat("Long task 界 ", 8)})
					}
					m.State, m.RowIdx = state, 20
					m.configureTextInput("New task...")
					m.TextInput.SetValue(strings.Repeat("typing a long title ", 8))
					m = resizeModel(m, size[0], size[1])
					assertFitsTerminal(t, m)
					if state == Browsing {
						m.feedback = "Moved to " + dayKey(10)
						m.moveUndo = &moveUndoSnapshot{}
						assertFitsTerminal(t, m)
						m.ShowHelp = true
						assertFitsTerminal(t, m)
						m.ShowHelp = false
					}
					if state == Adding || state == Editing || state == SettingMoveDate {
						if !strings.Contains(ansi.Strip(m.View().Content), "esc") {
							t.Fatal("input controls fell outside the screen")
						}
					} else if !strings.Contains(ansi.Strip(m.View().Content), "m move") && state == Browsing {
						t.Fatal("browsing footer fell outside the screen")
					}
					m.Err = fmt.Errorf("could not save: %s", strings.Repeat("long file path ", 30))
					assertFitsTerminal(t, m)
					if !strings.Contains(m.View().Content, "Error:") {
						t.Fatal("save error was hidden")
					}
				})
			}
		}
	}
}

func TestInputRemainsOnScreenAtEndOfLongList(t *testing.T) {
	for _, state := range []State{Adding, Editing, SettingMoveDate} {
		m := newFeedbackTestModel(t)
		key := m.getCurrentKey()
		for i := 0; i < 50; i++ {
			m.Data[key] = append(m.Data[key], Task{ID: fmt.Sprint(i), Title: "Earlier task"})
		}
		m.State, m.RowIdx = state, len(m.Data[key])-1
		m.configureTextInput("Type here")
		m.TextInput.SetValue("INPUT_MARKER")
		m = resizeModel(m, 40, 12)
		if !strings.Contains(ansi.Strip(m.View().Content), "INPUT_MARKER") {
			t.Fatalf("input scrolled off screen in state %d", state)
		}
	}
}

func TestVerticalNavigationKeepsSelectedTaskVisible(t *testing.T) {
	m := newFeedbackTestModel(t)
	key := m.getCurrentKey()
	m.Data[key] = nil
	for i := 0; i < 40; i++ {
		m.Data[key] = append(m.Data[key], Task{ID: fmt.Sprint(i), Title: fmt.Sprintf("Task %02d wraps onto another line", i)})
	}
	m = resizeModel(m, 40, 12)
	for i := 0; i < 40; i++ {
		if !strings.Contains(ansi.Strip(m.View().Content), fmt.Sprintf("Task %02d", i)) {
			t.Fatalf("selected task %d is off screen", i)
		}
		assertFitsTerminal(t, m)
		m = pressRune(m, 'j')
	}
	for i := 38; i >= 0; i-- {
		m = pressRune(m, 'k')
		if !strings.Contains(ansi.Strip(m.View().Content), fmt.Sprintf("Task %02d", i)) {
			t.Fatalf("selected task %d is off screen moving upward", i)
		}
	}
}

func TestPageScrollReadsOversizedTaskAndContinuesToNext(t *testing.T) {
	m := newFeedbackTestModel(t)
	key := m.getCurrentKey()
	m.Data[key] = []Task{{ID: "long", Title: strings.Repeat("wrapped line\n", 30) + "END OF LONG TASK"}, {ID: "next", Title: "Next task"}}
	m = resizeModel(m, 40, 12)
	sawEnd := false
	for i := 0; i < 50; i++ {
		m = pressKeyCode(m, tea.KeyPgDown)
		if strings.Contains(m.View().Content, "END OF LONG TASK") {
			sawEnd = true
		}
		if m.RowIdx == 1 {
			break
		}
	}
	if !sawEnd || m.RowIdx != 1 || !strings.Contains(m.View().Content, "Next task") {
		t.Fatalf("could not page through long task: sawEnd=%v row=%d", sawEnd, m.RowIdx)
	}
	for i := 0; i < 50; i++ {
		m = pressKeyCode(m, tea.KeyPgUp)
	}
	if m.RowIdx != 0 || m.scrollOffsets[key] != 0 {
		t.Fatalf("could not page back to top: row=%d offset=%d", m.RowIdx, m.scrollOffsets[key])
	}
}

func TestResizeInputPreservesTextAndCursor(t *testing.T) {
	for _, state := range []State{Adding, Editing, SettingMoveDate} {
		m := newFeedbackTestModel(t)
		m.State = state
		m.configureTextInput("Task")
		value := strings.Repeat("abc界 def ", 30)
		m.TextInput.SetValue(value)
		m.TextInput.SetCursor(80)
		for _, width := range []int{150, 24, 80, 40, 150} {
			m = resizeModel(m, width, 24)
			if m.TextInput.Value() != value || m.TextInput.Position() != 80 {
				t.Fatalf("resizing state %d changed input or cursor", state)
			}
			available := m.columnGeometry(m.visibleColumnCount()).contentWidth
			if got := lipgloss.Width(m.inputPrefix() + m.TextInput.View()); got > available {
				t.Fatalf("input width=%d exceeds %d in state %d at terminal %d", got, available, state, width)
			}
			assertFitsTerminal(t, m)
		}
	}
}

func TestTodayFocusAndJumpPreserveMaximumAndUndoDate(t *testing.T) {
	m := resizeModel(newFeedbackTestModel(t), 120, 24)
	m.updateDateKeysFrom(startOfDayNow().AddDate(0, 0, 5))
	key := m.dateKeys[1]
	m.ColIdx = 1
	m.Data[key] = []Task{{ID: "restore", Title: "Restore here"}}
	m.persist()
	m = pressRune(m, 'd')
	m = pressRune(m, 'T')
	if !m.FocusToday || m.ShowFuture || m.getCurrentKey() != dayKey(0) || len(m.visibleKeys()) != 1 || m.VisibleDays != 3 {
		t.Fatalf("focus did not show Today: %+v", m.visibleKeys())
	}
	m = pressRune(m, 'l')
	if m.getCurrentKey() != dayKey(0) {
		t.Fatal("Today focus navigated to another date")
	}
	m = pressRune(m, 'f')
	m = pressRune(m, 't')
	if m.ShowFuture || !m.FocusToday || m.getCurrentKey() != dayKey(0) {
		t.Fatal("Today jump did not restore focused Today from Future")
	}
	m = pressRune(m, 'u')
	if m.FocusToday || m.getCurrentKey() != key || m.Data[key][m.RowIdx].ID != "restore" {
		t.Fatal("undo after changing views did not restore its original date and task")
	}
	m = pressRune(pressRune(m, 'T'), 'T')
	if m.FocusToday || len(m.visibleKeys()) != 3 || m.getCurrentKey() != dayKey(0) {
		t.Fatal("leaving Today focus did not restore the requested day layout")
	}
}

func TestFocusFollowsTodayAcrossMidnight(t *testing.T) {
	m := newFeedbackTestModel(t)
	m.FocusToday = true
	m.todayKey = dayKey(-1)
	m.updateDateKeysFrom(startOfDayNow().AddDate(0, 0, -1))
	updated, _ := m.Update(dateTickMsg(time.Now()))
	m = updated.(Model)
	if m.getCurrentKey() != dayKey(0) || len(m.visibleKeys()) != 1 {
		t.Fatal("focus did not follow Today after midnight")
	}
}

func TestShortHelpCanScrollToEveryShortcut(t *testing.T) {
	m := resizeModel(newFeedbackTestModel(t), 40, 12)
	m = pressRune(m, '?')
	seenQuit := false
	for i := 0; i < 80; i++ {
		assertFitsTerminal(t, m)
		view := ansi.Strip(m.helpModalView())
		if !strings.Contains(view, "Press Esc to close") {
			t.Fatal("help close control is hidden")
		}
		if strings.Contains(view, "quit") {
			seenQuit = true
		}
		m = pressRune(m, 'j')
	}
	if !seenQuit {
		t.Fatal("could not scroll to the final shortcut")
	}
	m = pressKeyCode(m, tea.KeyEsc)
	if m.ShowHelp {
		t.Fatal("Esc did not close help")
	}
}

func TestTinyWindowPausesEditsAndRecovers(t *testing.T) {
	m := resizeModel(newFeedbackTestModel(t), 18, 6)
	assertFitsTerminal(t, m)
	m = pressRune(m, 'd')
	if len(m.Data[m.getCurrentKey()]) != 2 {
		t.Fatal("tiny window allowed an invisible deletion")
	}
	m = resizeModel(m, 80, 24)
	if !strings.Contains(m.View().Content, "First task") {
		t.Fatal("board did not recover after resizing")
	}
}
