package model

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func TestAddingTask_Enter(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	m := Model{
		Data:        make(TodoData),
		VisibleDays: 3,
		State:       Adding,
		TextInput:   textinput.New(),
		dateKeys:    []string{today}, // Mock dateKeys
		ColIdx:      0,
	}
	m.TextInput.Focus()
	// Ensure the map entry exists
	m.Data[today] = []Task{}

	// Simulate typing "Hello"
	runes := []rune("Hello")
	for _, r := range runes {
		msg := tea.KeyPressMsg{Code: r, Text: string(r)}
		newM, _ := m.Update(msg)
		m = newM.(Model)
	}

	if m.TextInput.Value() != "Hello" {
		t.Errorf("Expected input 'Hello', got '%s'", m.TextInput.Value())
	}

	// Simulate Enter
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	newM, _ := m.Update(msg)
	m = newM.(Model)

	// Verify
	if m.State != Browsing {
		t.Errorf("Expected state Browsing after Enter, got %v", m.State)
	}

	// Check if task was added
	tasks := m.Data[today]
	if len(tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(tasks))
	} else if tasks[0].Title != "Hello" {
		t.Errorf("Expected task 'Hello', got '%s'", tasks[0].Title)
	}
}

func TestAddingTask_Esc(t *testing.T) {
	m := Model{
		Data:        make(TodoData),
		VisibleDays: 3,
		State:       Adding,
		TextInput:   textinput.New(),
	}
	m.TextInput.Focus()
	// Type something
	m.TextInput.SetValue("Partial")

	// Simulate Esc
	msg := tea.KeyPressMsg{Code: tea.KeyEsc}
	newM, _ := m.Update(msg)
	m = newM.(Model)

	if m.State != Browsing {
		t.Errorf("Expected state Browsing after Esc, got %v", m.State)
	}
	if m.TextInput.Value() != "" {
		t.Errorf("Expected empty input after Esc, got '%s'", m.TextInput.Value())
	}
}

func TestInputConfiguredOnModeSwitch(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	m := Model{
		Data:        TodoData{today: {}, "Future": {}},
		VisibleDays: 1,
		State:       Browsing,
		TextInput:   textinput.New(),
		dateKeys:    []string{today},
	}
	m.configureTextInput("initial")

	msg := tea.KeyPressMsg{Code: 'a', Text: "a"}
	updated, _ := m.Update(msg)
	m = updated.(Model)

	if m.State != Adding {
		t.Fatalf("expected state Adding after 'a', got %v", m.State)
	}
	if m.TextInput.Placeholder != "New task..." {
		t.Fatalf("expected placeholder 'New task...', got %q", m.TextInput.Placeholder)
	}
	if m.TextInput.Value() != "" {
		t.Fatalf("expected input to be reset, got %q", m.TextInput.Value())
	}

	m.Data[today] = []Task{{ID: "1", Title: "Task"}}
	m.State = Browsing
	msg = tea.KeyPressMsg{Code: 'm', Text: "m"}
	updated, _ = m.Update(msg)
	m = updated.(Model)

	if m.State != ChoosingMoveDestination {
		t.Fatalf("expected destination picker after 'm', got %v", m.State)
	}

	msg = tea.KeyPressMsg{Code: 'd', Text: "d"}
	updated, _ = m.Update(msg)
	m = updated.(Model)

	if m.State != SettingMoveDate {
		t.Fatalf("expected state SettingMoveDate after 'md', got %v", m.State)
	}
	if m.TextInput.Placeholder != "YYYY-MM-DD or MM-DD" {
		t.Fatalf("expected date placeholder, got %q", m.TextInput.Placeholder)
	}
	if m.TextInput.Value() != "" {
		t.Fatalf("expected input to be reset for date entry, got %q", m.TextInput.Value())
	}
}

func TestTerminalPasteIntoTextInput(t *testing.T) {
	for name, state := range map[string]State{"add": Adding, "edit": Editing, "date": SettingMoveDate} {
		t.Run(name, func(t *testing.T) {
			m := Model{State: state, TextInput: textinput.New(), Data: make(TodoData)}
			m.configureTextInput("input")
			m.TextInput.SetValue("before after")
			m.TextInput.SetCursor(7)
			updated, _ := m.Update(tea.PasteMsg{Content: "café 📋\nnext\t"})
			m = updated.(Model)
			if got := m.TextInput.Value(); got != "before café 📋 next after" {
				t.Fatalf("paste at cursor: %q", got)
			}
			if m.State != state || len(m.Data) != 0 {
				t.Fatal("paste must only update the draft, without submitting")
			}
		})
	}
}

func TestTerminalPasteIgnoredOutsideActiveInput(t *testing.T) {
	for _, state := range []State{Browsing, ChoosingMoveDestination, Adding} {
		m := Model{State: state, TextInput: textinput.New(), ShowHelp: state == Adding}
		m.configureTextInput("input")
		m.TextInput.SetValue("unchanged")
		updated, _ := m.Update(tea.PasteMsg{Content: "q\na\nd\n"})
		m = updated.(Model)
		if m.TextInput.Value() != "unchanged" || m.State != state {
			t.Fatal("paste changed an inactive input or triggered shortcuts")
		}
	}
}
