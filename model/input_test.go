package model

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		newM, _ := m.Update(msg)
		m = newM.(Model)
	}

	if m.TextInput.Value() != "Hello" {
		t.Errorf("Expected input 'Hello', got '%s'", m.TextInput.Value())
	}

	// Simulate Enter
	msg := tea.KeyMsg{Type: tea.KeyEnter}
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
	msg := tea.KeyMsg{Type: tea.KeyEsc}
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

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
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

	m.ShowFuture = true
	m.State = Browsing
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	updated, _ = m.Update(msg)
	m = updated.(Model)

	if m.State != SettingDate {
		t.Fatalf("expected state SettingDate after 't', got %v", m.State)
	}
	if m.TextInput.Placeholder != "YYYY-MM-DD or MM-DD" {
		t.Fatalf("expected date placeholder, got %q", m.TextInput.Placeholder)
	}
	if m.TextInput.Value() != "" {
		t.Fatalf("expected input to be reset for date entry, got %q", m.TextInput.Value())
	}
}
