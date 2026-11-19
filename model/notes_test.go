package model

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func notesModel(t *testing.T) Model {
	t.Helper()
	m := newReloadTestModel(t)
	m.addTask("A task")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	return next.(Model)
}

func notesKey(m Model, code rune, mod tea.KeyMod) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: code, Mod: mod})
	return next.(Model)
}

func TestNotesSaveOnClosePasteUndo(t *testing.T) {
	m := notesModel(t)
	text := "Hello café\nhttps://example.com/a?q=1\n" + strings.Repeat("another line\n", 150)
	next, _ := m.Update(tea.PasteMsg{Content: text})
	m = next.(Model)
	if m.notesInput.Value() != text {
		t.Fatal("multiline paste truncated or changed")
	}
	if m.Data[m.notesKey][0].Notes != "" {
		t.Fatal("draft changed data before save")
	}
	m = notesKey(m, tea.KeyEsc, 0)
	if m.State != Browsing || m.Err != nil || m.Data[m.notesKey][0].Notes != text {
		t.Fatalf("save: %v %v", m.State, m.Err)
	}
	opened, _ := m.openNotes()
	m = opened.(Model)
	m.notesInput.SetValue("updated")
	m = notesKey(m, tea.KeyTab, 0)
	m = notesKey(m, tea.KeyEsc, 0)
	if m.State != Browsing || m.Data[m.notesKey][0].Notes != "updated" {
		t.Fatal("closing link view did not save")
	}
	if !m.undoMove() || m.Data[m.notesKey][0].Notes != text {
		t.Fatal("undo did not restore notes")
	}
}

func TestNotesEmptySaveAndDateTickDeferral(t *testing.T) {
	m := notesModel(t)
	m.notesInput.SetValue("old")
	m = notesKey(m, tea.KeyEsc, 0)
	opened, _ := m.openNotes()
	m = opened.(Model)
	m.notesInput.SetValue("")
	m.todayKey = time.Now().AddDate(0, 0, -1).Format(dateLayout)
	next, _ := m.Update(dateTickMsg(time.Now()))
	m = next.(Model)
	if m.todayKey == time.Now().Format(dateLayout) {
		t.Fatal("date tick ran during notes editing")
	}
	m = notesKey(m, tea.KeyEsc, 0)
	if m.Data[m.notesKey][0].Notes != "" {
		t.Fatal("could not clear notes")
	}
}

func TestNotesLinksAndLayout(t *testing.T) {
	rendered := linkedNotes("See (https://example.com/a_(b)). javascript:alert(1)\x1b[31m")
	if !strings.Contains(rendered, ansi.SetHyperlink("https://example.com/a_(b)")) {
		t.Fatal("missing hyperlink")
	}
	if strings.Contains(rendered, "\x1b[31m") || strings.Contains(rendered, ansi.SetHyperlink("javascript:alert(1)")) {
		t.Fatal("unsafe escape or link")
	}
	m := notesModel(t)
	m.notesInput.SetValue(strings.Repeat("https://example.com/long/path words\n", 100))
	for _, size := range [][2]int{{80, 24}, {24, 10}, {120, 40}} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(Model)
		for _, preview := range []bool{false, true} {
			m.notesPreview = preview
			view := m.View()
			if preview && !strings.Contains(view.Content, ansi.SetHyperlink("https://example.com/long/path")) {
				t.Fatal("rendered preview lost hyperlinks")
			}
			if lipgloss.Width(view.Content) > size[0] || lipgloss.Height(view.Content) > size[1] {
				t.Fatalf("notes overflow %v", size)
			}
			if view.MouseMode != tea.MouseModeNone {
				t.Fatal("terminal selection disabled")
			}
		}
	}
}

func TestNotesTerminalClipboard(t *testing.T) {
	m := notesModel(t)
	if m.notesInput.KeyMap.Paste.Enabled() {
		t.Fatal("editor must not read the system clipboard itself")
	}
	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("terminal text selection must remain available")
	}
	m.notesInput.SetValue("before after")
	m.notesInput.SetCursorColumn(7)
	next, _ := m.Update(tea.PasteMsg{Content: "café\r\nhttps://example.com\n"})
	m = next.(Model)
	if got := m.notesInput.Value(); got != "before café\nhttps://example.com\nafter" {
		t.Fatalf("terminal paste at cursor: %q", got)
	}
	if m.State != EditingNotes || m.Data[m.notesKey][0].Notes != "" {
		t.Fatal("terminal paste should only change the editor draft")
	}
}

func TestNotesPanelGrowsAndScrollsWithinBounds(t *testing.T) {
	m := notesModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 50})
	m = next.(Model)
	shortHeight := lipgloss.Height(m.notesView())
	if shortHeight > 12 || lipgloss.Width(m.notesView()) > 70 {
		t.Fatal("short notes should open in a compact panel")
	}
	text := strings.Repeat("another line\n", 100)
	next, _ = m.Update(tea.PasteMsg{Content: text})
	m = next.(Model)
	if lipgloss.Height(m.notesView()) <= shortHeight || lipgloss.Height(m.notesView()) > 22 {
		t.Fatal("notes panel should grow to a bounded height")
	}
	if m.notesInput.Value() != text {
		t.Fatal("resizing truncated notes")
	}
	m = notesKey(m, tea.KeyTab, 0)
	m = notesKey(m, tea.KeyPgDown, 0)
	if m.notesOffset == 0 {
		t.Fatal("long notes preview should scroll")
	}
}
