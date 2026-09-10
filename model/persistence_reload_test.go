package model

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dtt101/doitdoit/taskstore"
)

func newDraftTestModel(t *testing.T) Model {
	t.Helper()
	m := newReloadTestModel(t)
	m.ShowFuture = true
	m.Data["Future"] = []Task{{ID: "a", Title: "original"}}
	m.persist()
	if m.Err != nil {
		t.Fatal(m.Err)
	}
	return m
}

func writeDraftTestRemote(t *testing.T, m Model, data TodoData) tea.Msg {
	t.Helper()
	if err := data.Save(m.FilePath); err != nil {
		t.Fatal(err)
	}
	// Avoid depending on the filesystem's timestamp resolution.
	later := m.dataModTime.Add(time.Second)
	if err := os.Chtimes(m.FilePath, later, later); err != nil {
		t.Fatal(err)
	}
	return checkDataFile(m.FilePath, m.dataHash, m.dataExists)()
}

func TestConflictDraftSurvivesReloadAndInputCancellation(t *testing.T) {
	m := newDraftTestModel(t)
	m.editTask("local edit")
	remote := TodoData{"Future": {{ID: "a", Title: "remote edit"}}}
	checked := writeDraftTestRemote(t, m, remote)
	m.persist()
	if !errors.Is(m.Err, ErrDataConflict) {
		t.Fatalf("expected conflict, got %v", m.Err)
	}
	// A check started before the failure must not replace the draft either.
	updated, _ := m.Update(checked)
	m = updated.(Model)
	m = pressRune(m, 'e')
	m.TextInput.SetValue("")
	m = pressKeyCode(m, tea.KeyEnter)
	if !strings.Contains(m.errorView(), "task title cannot be empty") {
		t.Fatal("draft warning hid input validation")
	}
	m = pressKeyCode(m, tea.KeyEsc)
	if m.Err != nil {
		t.Fatalf("input cancellation did not clear transient error: %v", m.Err)
	}
	if !strings.Contains(m.errorView(), "Changes not saved") {
		t.Fatal("input cancellation hid the unsaved draft warning")
	}
	// Repeated checks must leave data, revision and undo untouched.
	undo, hash, modTime := m.moveUndo, m.dataHash, m.dataModTime
	for range 2 {
		updated, cmd := m.Update(reloadTickMsg(time.Now()))
		m = updated.(Model)
		if cmd == nil {
			t.Fatal("paused reload must reschedule")
		}
		updated, _ = m.Update(checked)
		m = updated.(Model)
	}
	if m.Data["Future"][0].Title != "local edit" || m.moveUndo != undo || m.dataHash != hash || m.dataModTime != modTime {
		t.Fatal("reload replaced draft, undo or loaded revision")
	}
	current, err := loadRaw(m.FilePath)
	if err != nil || !sameJSON(current, remote) {
		t.Fatalf("remote changed: %v %v", current, err)
	}
}

func TestFurtherEditCannotMergeAwayConflictDraft(t *testing.T) {
	m := newDraftTestModel(t)
	m.editTask("local edit")
	remote := TodoData{"Future": {{ID: "a", Title: "remote edit"}}}
	writeDraftTestRemote(t, m, remote)
	m.persist()
	if !errors.Is(m.Err, ErrDataConflict) {
		t.Fatal(m.Err)
	}
	// This replaces the undo snapshot with a draft, not the loaded baseline.
	m.ShowFuture = false
	m.addTask("another local task")
	m.persist()
	if !errors.Is(m.Err, ErrDataConflict) || m.Data["Future"][0].Title != "local edit" {
		t.Fatalf("second edit merged away the draft: %v, %v", m.Data, m.Err)
	}
	current, err := loadRaw(m.FilePath)
	if err != nil || !sameJSON(current, remote) {
		t.Fatalf("second edit overwrote remote: %v %v", current, err)
	}
}

type failingDraftStore struct {
	taskstore.JSONStore
	err error
}

func (s *failingDraftStore) Save(data taskstore.Data, expected *taskstore.Revision) (taskstore.Revision, error) {
	if s.err != nil {
		return taskstore.Revision{}, s.err
	}
	return s.JSONStore.Save(data, expected)
}

func TestFailedSavePausesReloadUntilSuccessfulSave(t *testing.T) {
	m := newDraftTestModel(t)
	store := &failingDraftStore{JSONStore: taskstore.NewJSON(m.FilePath), err: errors.New("disk full")}
	m.store, m.storePath = store, m.FilePath
	m.editTask("local edit")
	m.persist()
	if m.canReload() || !strings.Contains(m.errorView(), "disk full") {
		t.Fatal("failed save did not pause reload with a useful warning")
	}
	updated, _ := m.Update(dataFileCheckedMsg{data: TodoData{"Future": {{ID: "a", Title: "incoming"}}}, modTime: m.dataModTime.Add(time.Second)})
	m = updated.(Model)
	if m.Data["Future"][0].Title != "local edit" {
		t.Fatal("reload discarded an ordinary failed save")
	}
	store.err = nil
	m.persist()
	if m.Err != nil || !m.canReload() || m.errorView() != "" {
		t.Fatalf("successful retry did not resume normal behavior: %v", m.Err)
	}
	checked := writeDraftTestRemote(t, m, TodoData{"Future": {{ID: "a", Title: "later external edit"}}})
	updated, _ = m.Update(checked)
	if updated.(Model).Data["Future"][0].Title != "later external edit" {
		t.Fatal("reload did not resume after successful save")
	}
}

func TestReloadResultWaitsForInputAndMove(t *testing.T) {
	for _, state := range []State{Adding, Editing, ChoosingMoveDestination, SettingMoveDate} {
		m := newDraftTestModel(t)
		checked := writeDraftTestRemote(t, m, TodoData{"Future": {{ID: "a", Title: "remote edit"}}})
		m.State = state
		updated, _ := m.Update(checked)
		m = updated.(Model)
		if m.Data["Future"][0].Title != "original" {
			t.Fatalf("in-flight reload applied during state %v", state)
		}
		m.State = Browsing
		updated, _ = m.Update(checked)
		if updated.(Model).Data["Future"][0].Title != "remote edit" {
			t.Fatalf("reload did not resume after state %v", state)
		}
	}
}
