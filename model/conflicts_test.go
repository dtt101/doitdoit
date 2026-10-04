package model

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dtt101/doitdoit/taskstore"
)

func conflictFixture(t *testing.T, current, alternative taskstore.Data) (string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.json")
	if _, err := taskstore.NewJSON(path).Save(current, nil); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(filepath.Dir(path), "tasks.json (Someone's conflicted copy 2026-09-19).bak")
	if _, err := taskstore.NewJSON(source).Save(alternative, nil); err != nil {
		t.Fatal(err)
	}
	return path, source
}

func TestConflictKeepCurrentDeletesCopyWithoutRewriting(t *testing.T) {
	today := time.Now().Format(dateLayout)
	current := taskstore.Data{today: {{ID: "beacon", Title: "Get KTLO on Beacon down", Completed: true}, {ID: "test", Title: "test", Completed: true}}}
	alternative := taskstore.Data{today: {{ID: "beacon", Title: "Get KTLO on Beacon down"}, {ID: "tes", Title: "Tes"}}}
	path, source := conflictFixture(t, current, alternative)
	original, _ := os.ReadFile(path)
	// Verify no-op resolution preserves formatting too.
	original = append(original, '\n')
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	backup := path + ".bak"
	if err := os.WriteFile(backup, []byte("regular backup stays"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewModelWithRetention(path, 3, 5)
	if err != nil {
		t.Fatal(err)
	}
	if m.review == nil {
		t.Fatal("startup did not open review")
	}
	m = pressRune(m, 'K')
	m = pressRune(m, 'p')
	if !m.review.previewing {
		t.Fatal("no preview")
	}
	m = pressKeyCode(m, tea.KeyEnter)
	actual, _ := os.ReadFile(path)
	if !bytes.Equal(original, actual) {
		t.Fatalf("no-op resolution rewrote file: %s", actual)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatal("reviewed source remains")
	}
	if b, _ := os.ReadFile(backup); string(b) != "regular backup stays" {
		t.Fatal("regular backup changed")
	}
	dir, _ := taskstore.RecoveryDirectory(path)
	exports, _ := filepath.Glob(filepath.Join(dir, "review-*.json"))
	if len(exports) != 1 {
		t.Fatalf("alternatives not archived: %v", exports)
	}
}

func TestConflictStartupSilentlyDeletesRedundantSubset(t *testing.T) {
	current := taskstore.Data{"Future": {{ID: "a", Title: "same"}, {ID: "b", Title: "newer"}}}
	path, source := conflictFixture(t, current, taskstore.Data{"Future": {{ID: "a", Title: "same"}}})
	before, _ := os.ReadFile(path)
	m, err := NewModel(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	if m.review != nil || m.conflictStatus() != "" {
		t.Fatalf("redundant copy interrupted startup: %s", m.conflictStatus())
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatal("redundant copy remains")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("cleanup changed tasks")
	}
}

func TestConflictImportExplicitDeletionAndMultipleCopies(t *testing.T) {
	path, source := conflictFixture(t, taskstore.Data{"Future": {{ID: "a", Title: "delete me"}}}, taskstore.Data{"Future": {{ID: "b", Title: "import me"}}})
	second := filepath.Join(filepath.Dir(path), "tasks (Zed's conflicted copy 2026-09-20).json")
	if _, err := taskstore.NewJSON(second).Save(taskstore.Data{"Future": {{ID: "c", Title: "another task"}}}, nil); err != nil {
		t.Fatal(err)
	}
	m, err := NewModel(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	// Whichever filename sorts first, explicitly select its entire alternative.
	firstPath := m.review.source.Path
	for i := range m.review.comparison.Differences {
		m.review.comparison.Differences[i].Choice = taskstore.UseAlternative
	}
	m = pressRune(m, 'p')
	m = pressKeyCode(m, tea.KeyEnter)
	if _, err := os.Stat(firstPath); !os.IsNotExist(err) {
		t.Fatal("first copy not cleaned")
	}
	if m.review == nil || m.review.source.Path == firstPath {
		t.Fatal("did not recalculate next copy")
	}
	m = pressRune(m, 'K')
	m = pressRune(m, 'p')
	m = pressKeyCode(m, tea.KeyEnter)
	for _, p := range []string{source, second} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("copy remains", p)
		}
	}
	loaded, _ := taskstore.NewJSON(path).Load()
	if len(loaded.Data["Future"]) != 1 || loaded.Data["Future"][0].ID == "a" {
		t.Fatal("import/delete not applied", loaded.Data)
	}
}

func TestConflictChangesDuringReviewInvalidatePreview(t *testing.T) {
	for _, change := range []string{"current", "source"} {
		t.Run(change, func(t *testing.T) {
			path, source := conflictFixture(t, taskstore.Data{"Future": {{ID: "a", Title: "current"}}}, taskstore.Data{"Future": {{ID: "a", Title: "alternative"}}})
			m, _ := NewModel(path, 3)
			m = pressRune(m, '2')
			m = pressRune(m, 'p')
			target := path
			if change == "source" {
				target = source
			}
			if _, err := taskstore.NewJSON(target).Save(taskstore.Data{"Future": {{ID: "a", Title: "changed again"}}}, nil); err != nil {
				t.Fatal(err)
			}
			m = pressKeyCode(m, tea.KeyEnter)
			if m.review == nil || m.review.previewing || !strings.Contains(m.review.message, "changed") {
				t.Fatal("stale choices applied")
			}
			if m.review.comparison.Differences[0].Choice != taskstore.Undecided {
				t.Fatal("stale choice retained")
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatal("changed copy deleted")
			}
		})
	}
}

func TestConflictDraftRestartAndFailedRecoveryBlocksQuit(t *testing.T) {
	m := newDraftTestModel(t)
	m.editTask("local edit")
	writeDraftTestRemote(t, m, TodoData{"Future": {{ID: "a", Title: "remote edit"}}})
	m.persist()
	restarted, err := NewModel(m.FilePath, 3)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.review == nil || restarted.Data["Future"][0].Title != "local edit" {
		t.Fatal("restart lost pending draft")
	}
	restarted = pressRune(restarted, '2')
	restarted = pressRune(restarted, 'p')
	restarted = pressKeyCode(restarted, tea.KeyEnter)
	loaded, _ := taskstore.NewJSON(m.FilePath).Load()
	if loaded.Data["Future"][0].Title != "local edit" {
		t.Fatal("draft resolution not saved")
	}
	if pending, err := taskstore.ReadRecovery(m.FilePath); err != nil || pending != nil {
		t.Fatal("pending recovery not cleared")
	}
	// A filesystem error prevents persistent recovery and must block ordinary quit.
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocked)
	m = newDraftTestModel(t)
	m.editTask("unsafe to quit")
	writeDraftTestRemote(t, m, TodoData{"Future": {{ID: "a", Title: "remote"}}})
	m.persist()
	if m.recoveryErr == nil {
		t.Fatal("expected recovery storage failure")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = next.(Model)
	if cmd != nil || m.pending == nil || m.review == nil {
		t.Fatal("quit discarded unprotected draft")
	}
	m = pressRune(m, 'D')
	m = pressRune(m, 'y')
	if m.pending != nil || m.Data["Future"][0].Title != "remote" {
		t.Fatal("explicit discard did not release blocked draft")
	}
}

func TestConflictFailuresRetainCopies(t *testing.T) {
	path, source := conflictFixture(t, taskstore.Data{"Future": {{ID: "a", Title: "old"}}}, taskstore.Data{"Future": {{ID: "a", Title: "new"}}})
	m, _ := NewModel(path, 3)
	m.store = &failingDraftStore{JSONStore: taskstore.NewJSON(path), err: errors.New("disk full")}
	m.storePath = path
	m = pressRune(m, '2')
	m = pressRune(m, 'p')
	m = pressKeyCode(m, tea.KeyEnter)
	if m.review == nil || !strings.Contains(m.review.message, "disk full") {
		t.Fatal("save failure not visible")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("failed save deleted copy")
	}
	m.store = &interveningStore{JSONStore: taskstore.NewJSON(path), afterSave: func() { os.WriteFile(source, []byte(`{"Future":[{"id":"a","title":"later"}]}`), 0600) }}
	m = pressKeyCode(m, tea.KeyEnter)
	if !strings.Contains(m.conflictNotice, "cleanup") {
		t.Fatal("cleanup failure not visible", m.conflictNotice)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("changed source deleted after save")
	}
}

func TestConflictBackgroundDiscoveryDefersAndRawRetentionPreview(t *testing.T) {
	old := time.Now().AddDate(0, 0, -20).Format(dateLayout)
	path, source := conflictFixture(t, taskstore.Data{"Future": {{ID: "a", Title: "existing"}}}, taskstore.Data{old: {{ID: "done", Title: "old completed", Completed: true}, {ID: "open", Title: "old open"}}})
	m, err := NewModelWithRetention(path, 3, 5)
	if err != nil {
		t.Fatal(err)
	}
	if m.review == nil || len(m.review.comparison.Differences) != 3 {
		t.Fatal("startup pruned raw evidence")
	}
	for i, d := range m.review.comparison.Differences {
		if d.Kind == "add" {
			m.review.comparison.Differences[i].Choice = taskstore.UseAlternative
		}
	}
	m = pressRune(m, 'p')
	body := m.reviewBody()
	if !strings.Contains(body, "Your history setting will also remove") || !strings.Contains(body, "old completed") || !strings.Contains(body, "Carried forward to Today") {
		t.Fatal("preview hid lifecycle effects", body)
	}
	m = pressKeyCode(m, tea.KeyEsc)
	m = pressKeyCode(m, tea.KeyEsc)
	if m.review != nil {
		t.Fatal("cancel did not close review")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("cancel removed source")
	}
	next, _ := m.Update(reloadTickMsg(time.Now()))
	m = next.(Model)
	if m.review != nil || len(m.conflictPaths) != 1 {
		t.Fatal("background discovery interrupted user")
	}
}

func TestConflictDialogFitsTerminalAndKeepsControls(t *testing.T) {
	path, _ := conflictFixture(t, taskstore.Data{"Future": {{ID: "a", Title: "current"}}}, taskstore.Data{"Future": {{ID: "a", Title: "copy", Notes: strings.Repeat("long notes\n", 100)}}})
	m, _ := NewModel(path, 3)
	for _, size := range [][2]int{{80, 24}, {120, 40}, {24, 10}} {
		m.width, m.height = size[0], size[1]
		view := ansi.Strip(m.conflictView())
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("dialog overflow at %v", size)
		}
		if !strings.Contains(view, "p preview") {
			t.Fatal("controls clipped", view)
		}
	}
}

func TestReviewUsesTaskActionsAndHidesStorageDetails(t *testing.T) {
	path, _ := conflictFixture(t, taskstore.Data{"Future": {{ID: "a", Title: "Existing task"}}}, taskstore.Data{"Future": {{ID: "a", Title: "Existing task"}, {ID: "b", Title: "Buy coffee"}}})
	m, err := NewModel(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 100, 30
	view := ansi.Strip(m.conflictView())
	for _, want := range []string{"Review tasks", "Missing task", "Buy coffee", "Don't add", "Add task"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing task-focused guidance %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"conflicted copy", "tasks.json", "Current file", "Conflict copy", "alternative", "Created:"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("storage detail leaked into normal review: %q", unwanted)
		}
	}
	m = pressRune(m, '2')
	m = pressRune(m, 'p')
	body := m.reviewBody()
	if !strings.Contains(body, "Add task:\nBuy coffee") || strings.Contains(body, "Existing task") {
		t.Fatalf("preview should focus on changes:\n%s", body)
	}
	if !strings.Contains(ansi.Strip(m.conflictView()), "enter save & finish") {
		t.Fatal("missing plain save action")
	}
	m = pressRune(m, 'v')
	if !strings.Contains(m.reviewBody(), "Existing task") {
		t.Fatal("complete resulting task list unavailable")
	}
}

func TestReviewDescribesStatusChangeAsTaskAction(t *testing.T) {
	path, _ := conflictFixture(t, taskstore.Data{"Future": {{ID: "a", Title: "Send update", Completed: true}}}, taskstore.Data{"Future": {{ID: "a", Title: "Send update"}}})
	m, err := NewModel(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	body := ansi.Strip(m.conflictView())
	for _, want := range []string{"Status: Done → Not done", "Keep as is", "Mark not done"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q:\n%s", want, body)
		}
	}
}

func TestReviewPanelMatchesNotesAndOverlaysBoard(t *testing.T) {
	path, _ := conflictFixture(t, taskstore.Data{"Future": {{ID: "a", Title: "Existing task"}}}, taskstore.Data{"Future": {{ID: "a", Title: "Existing task"}, {ID: "b", Title: "Buy coffee", Notes: "The usual beans."}}})
	m, err := NewModel(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 120, 40
	panel := ansi.Strip(m.conflictView())
	if lipgloss.Width(panel) > 64 || lipgloss.Height(panel) > 20 {
		t.Fatalf("short task should use a compact notes-style panel: %dx%d", lipgloss.Width(panel), lipgloss.Height(panel))
	}
	if strings.Count(panel, "Buy coffee") != 1 {
		t.Fatal("task title duplicated in panel", panel)
	}
	if !strings.Contains(panel, "1/1") || !strings.Contains(panel, "╭") || !strings.Contains(panel, "╯") {
		t.Fatal("missing progress or app panel border")
	}
	overlay := ansi.Strip(m.View().Content)
	if lipgloss.Width(overlay) > 120 || lipgloss.Width(overlay) < lipgloss.Width(panel) || lipgloss.Height(overlay) != 40 || !strings.Contains(overlay, "Today") {
		t.Fatalf("review overlay size %dx%d, Today visible=%v:\n%s", lipgloss.Width(overlay), lipgloss.Height(overlay), strings.Contains(overlay, "Today"), overlay)
	}
	m = pressRune(m, '2')
	if !strings.Contains(ansi.Strip(m.conflictView()), "Add task ✓") {
		t.Fatal("selected choice has no visible feedback")
	}
}
