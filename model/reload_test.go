package model

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newReloadTestModel(t *testing.T) Model {
	t.Helper()
	m := Model{
		Data:        make(TodoData),
		FilePath:    filepath.Join(t.TempDir(), "tasks.json"),
		VisibleDays: 3,
		State:       Browsing,
	}
	m.updateDateKeys()
	return m
}

func TestReloadAppliesExternalChangeAndFollowsCursor(t *testing.T) {
	today := time.Now().Format(dateLayout)
	m := newReloadTestModel(t)
	m.Data[today] = []Task{
		{ID: "a", Title: "First"},
		{ID: "b", Title: "Second"},
	}
	m.RowIdx = 1 // cursor on "b"

	// Externally, "b" moved to the top and a new task appeared.
	external := TodoData{today: {
		{ID: "b", Title: "Second"},
		{ID: "c", Title: "Added on web"},
	}}

	newM, cmd := m.Update(dataFileCheckedMsg{data: external, modTime: time.Now(), size: 1})
	m = newM.(Model)

	if cmd == nil {
		t.Error("expected the reload check to reschedule itself")
	}
	if len(m.Data[today]) != 2 || m.Data[today][1].ID != "c" {
		t.Errorf("external data not applied, got %v", m.Data[today])
	}
	if m.RowIdx != 0 {
		t.Errorf("RowIdx = %d, want 0 (cursor should follow task b)", m.RowIdx)
	}
}

func TestReloadIgnoresStaleCheckResult(t *testing.T) {
	today := time.Now().Format(dateLayout)
	m := newReloadTestModel(t)
	m.Data[today] = []Task{{ID: "a", Title: "Keep me"}}
	m.dataModTime = time.Now()

	stale := dataFileCheckedMsg{
		data:    TodoData{today: {{ID: "z", Title: "Old snapshot"}}},
		modTime: m.dataModTime.Add(-time.Second),
		size:    1,
	}
	newM, cmd := m.Update(stale)
	m = newM.(Model)

	if cmd == nil {
		t.Error("expected the reload check to reschedule itself")
	}
	if got := m.Data[today]; len(got) != 1 || got[0].ID != "a" {
		t.Errorf("stale reload should be discarded, got %v", got)
	}
}

func TestReloadDiffGateKeepsModelUntouched(t *testing.T) {
	today := time.Now().Format(dateLayout)
	m := newReloadTestModel(t)
	m.Data[today] = []Task{{ID: "a", Title: "Same"}}
	m.moveUndo = &moveUndoSnapshot{Data: cloneTodoData(m.Data)}

	same := TodoData{today: {{ID: "a", Title: "Same"}}}
	newMod := time.Now()
	newM, _ := m.Update(dataFileCheckedMsg{data: same, modTime: newMod, size: 42})
	m = newM.(Model)

	if m.moveUndo == nil {
		t.Error("identical content should not clear the move undo snapshot")
	}
	if !m.dataModTime.Equal(newMod) || m.dataSize != 42 {
		t.Error("tracked file state should still advance on an mtime-only change")
	}
}

func TestReloadTickSkippedWhileTyping(t *testing.T) {
	m := newReloadTestModel(t)
	m.State = Adding

	newM, cmd := m.Update(reloadTickMsg(time.Now()))
	m = newM.(Model)

	if cmd == nil {
		t.Error("expected the tick to reschedule itself")
	}
	if m.State != Adding {
		t.Errorf("State = %v, want Adding", m.State)
	}
}

func TestCheckDataFileEndToEnd(t *testing.T) {
	today := time.Now().Format(dateLayout)
	m := newReloadTestModel(t)

	external := TodoData{today: {{ID: "w", Title: "From the web"}}}
	if err := external.Save(m.FilePath); err != nil {
		t.Fatal(err)
	}

	msg := checkDataFile(m.FilePath, time.Time{}, 0)()
	checked, ok := msg.(dataFileCheckedMsg)
	if !ok || checked.data == nil || checked.err != nil {
		t.Fatalf("expected a changed-file result, got %#v", msg)
	}
	newM, _ := m.Update(checked)
	m = newM.(Model)
	if got := m.Data[today]; len(got) != 1 || got[0].ID != "w" {
		t.Fatalf("external file not loaded, got %v", got)
	}

	// A corrupt file (e.g. mid-sync) must not disturb the current data.
	if err := os.WriteFile(m.FilePath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	msg = checkDataFile(m.FilePath, m.dataModTime.Add(-time.Second), 0)()
	newM, cmd := m.Update(msg)
	m = newM.(Model)
	if cmd == nil {
		t.Error("expected the reload check to reschedule itself")
	}
	if got := m.Data[today]; len(got) != 1 || got[0].ID != "w" {
		t.Errorf("corrupt file should leave data untouched, got %v", got)
	}
}

func TestPersistPreventsSelfDetection(t *testing.T) {
	today := time.Now().Format(dateLayout)
	m := newReloadTestModel(t)
	m.Data[today] = []Task{{ID: "a", Title: "Mine"}}

	m.persist()
	if m.Err != nil {
		t.Fatal(m.Err)
	}

	msg := checkDataFile(m.FilePath, m.dataModTime, m.dataSize)()
	checked := msg.(dataFileCheckedMsg)
	if checked.data != nil {
		t.Error("check after own persist should report the file as unchanged")
	}
}

func TestReloadRollsOverInMemoryWithoutSaving(t *testing.T) {
	today := time.Now().Format(dateLayout)
	yesterday := time.Now().AddDate(0, 0, -1).Format(dateLayout)
	m := newReloadTestModel(t)

	external := TodoData{yesterday: {{ID: "r", Title: "Left behind", Completed: false}}}
	newM, _ := m.Update(dataFileCheckedMsg{data: external, modTime: time.Now(), size: 1})
	m = newM.(Model)

	if got := m.Data[today]; len(got) != 1 || got[0].ID != "r" {
		t.Errorf("incomplete task should roll over to today in memory, got %v", got)
	}
	if _, err := os.Stat(m.FilePath); !os.IsNotExist(err) {
		t.Error("reload must not write the data file")
	}
}
