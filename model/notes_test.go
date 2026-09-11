package model

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dtt101/doitdoit/taskstore"
)

func notesModel(t *testing.T) Model {
	t.Helper()
	m, err := NewModel(filepath.Join(t.TempDir(), "tasks.json"), 3)
	if err != nil {
		t.Fatal(err)
	}
	m.addTask("A task")
	m.persist()
	if m.Err != nil {
		t.Fatal(m.Err)
	}
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
	if m.State != Browsing || m.Err != nil {
		t.Fatalf("save: %v %v", m.State, m.Err)
	}
	loaded, err := taskstore.NewJSON(m.FilePath).Load()
	if err != nil || loaded.Data[m.notesKey][0].Notes != text {
		t.Fatalf("notes not persisted: %v", err)
	}
	opened, _ := m.openNotes()
	m = opened.(Model)
	m.notesInput.SetValue("updated")
	m = notesKey(m, tea.KeyTab, 0)
	m = notesKey(m, tea.KeyEsc, 0)
	loaded, err = taskstore.NewJSON(m.FilePath).Load()
	if err != nil || m.State != Browsing || loaded.Data[m.notesKey][0].Notes != "updated" {
		t.Fatalf("closing link view did not save: %v", err)
	}
	if !m.undoMove() || m.Data[m.notesKey][0].Notes != text {
		t.Fatal("undo did not restore notes")
	}
}

func TestNotesConflictKeepsDraft(t *testing.T) {
	m := notesModel(t)
	remote := taskstore.Clone(taskstore.Data(m.Data))
	remote[m.notesKey][0].Notes = "external notes"
	if _, err := taskstore.NewJSON(m.FilePath).Save(remote, nil); err != nil {
		t.Fatal(err)
	}
	m.notesInput.SetValue("local notes")
	m = notesKey(m, tea.KeyEsc, 0)
	if !errors.Is(m.saveErr, ErrDataConflict) || m.State != EditingNotes || m.notesInput.Value() != "local notes" {
		t.Fatalf("lost conflict draft: %v", m.saveErr)
	}
	loaded, _ := taskstore.NewJSON(m.FilePath).Load()
	if loaded.Data[m.notesKey][0].Notes != "external notes" {
		t.Fatal("overwrote external notes")
	}
	if m.canReload() {
		t.Fatal("conflicting notes allowed reload")
	}
}

func TestNotesEmptySaveAndRolloverDeferral(t *testing.T) {
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
		t.Fatal("rollover ran during notes editing")
	}
	m = notesKey(m, tea.KeyEsc, 0)
	loaded, _ := taskstore.NewJSON(m.FilePath).Load()
	if loaded.Data[m.notesKey][0].Notes != "" {
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

func TestNotesCloseUnchangedDoesNotWrite(t *testing.T) {
	m := notesModel(t)
	remote := taskstore.Clone(taskstore.Data(m.Data))
	remote[m.notesKey][0].Notes = "external notes"
	revision, err := taskstore.NewJSON(m.FilePath).Save(remote, nil)
	if err != nil {
		t.Fatal(err)
	}
	m = notesKey(m, tea.KeyEsc, 0)
	if m.State != Browsing || m.Err != nil {
		t.Fatalf("unchanged notes should close: %v", m.Err)
	}
	loaded, err := taskstore.NewJSON(m.FilePath).Load()
	if err != nil || loaded.Revision != revision {
		t.Fatalf("closing unchanged notes wrote to disk: %v", err)
	}
}

func TestNotesCloseRetriesFailedSave(t *testing.T) {
	m := notesModel(t)
	baseline := taskstore.Clone(taskstore.Data(m.Data))
	remote := taskstore.Clone(baseline)
	remote[m.notesKey][0].Notes = "external notes"
	store := taskstore.NewJSON(m.FilePath)
	if _, err := store.Save(remote, nil); err != nil {
		t.Fatal(err)
	}
	m.notesInput.SetValue("local notes")
	m = notesKey(m, tea.KeyEsc, 0)
	if m.State != EditingNotes || m.saveErr == nil {
		t.Fatal("failed save closed editor")
	}
	// Restore the loaded file to resolve the conflict, then retry closing.
	if _, err := store.Save(baseline, nil); err != nil {
		t.Fatal(err)
	}
	m = notesKey(m, tea.KeyEsc, 0)
	loaded, err := store.Load()
	if err != nil || m.State != Browsing || m.saveErr != nil || loaded.Data[m.notesKey][0].Notes != "local notes" {
		t.Fatalf("retry did not save draft and close: %v / %v", err, m.saveErr)
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
