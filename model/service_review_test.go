package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dtt101/doitdoit/taskclient"
)

func serviceKey(m Model, key rune) Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
	return updated.(Model)
}
func reviewTask(id, date string) taskclient.Task {
	now := time.Now().UTC()
	return taskclient.Task{ID: id, Title: id, Status: "open", ScheduledDate: &date, Position: id, Revision: 1, CreatedAt: now, UpdatedAt: now}
}
func TestServiceSingleColumnViewsUseSingleColumnGeometry(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(fmt.Sprint(deleted), func(t *testing.T) {
			m, _ := cloudFixture(t)
			m.width = 150
			m.ShowDeleted = deleted
			m.ShowOverdue = !deleted
			m.syncViewport()
			if m.visibleColumnCount() != 1 || len(m.visibleKeys()) != 1 {
				t.Fatal("special view uses multiple columns")
			}
			want := m.columnGeometry(1).contentWidth - lipgloss.Width(m.inputPrefix()) - 1
			if m.TextInput.Width() != want {
				t.Fatalf("input width %d, want %d", m.TextInput.Width(), want)
			}
		})
	}
}
func TestServiceOverdueNavigationAndRelativeMovesUseToday(t *testing.T) {
	m, _ := cloudFixture(t)
	m.cloud.busy = false
	m.ShowOverdue = true
	m.updateDateKeysFrom(startOfDay(m.serviceNow()).AddDate(0, 0, 10))
	m.ColIdx = 2
	keys := strings.Join(m.dateKeys, ",")
	for _, key := range []rune{'h', 'l'} {
		m = serviceKey(m, key)
	}
	if m.ColIdx != 2 || strings.Join(m.dateKeys, ",") != keys {
		t.Fatal("overdue navigation moved hidden day columns")
	}
	if got := m.relativeMoveTarget(1).Date; got != m.serviceNow().AddDate(0, 0, 1).Format(dateLayout) {
		t.Fatalf("relative move used hidden day: %s", got)
	}
}
func TestServiceReturnToTodayResetsOverdueSelection(t *testing.T) {
	for _, key := range []rune{'t', 'T'} {
		m, _ := cloudFixture(t)
		today := m.serviceNow().Format(dateLayout)
		m.serviceBoard(taskclient.Board{Timezone: "Europe/London", Tasks: []taskclient.Task{reviewTask("a", today), reviewTask("b", today), reviewTask("c", "2020-01-01"), reviewTask("d", "2020-01-01")}})
		m.cloud.busy = false
		m.ShowOverdue = true
		m.RowIdx = 1
		m = serviceKey(m, key)
		if m.ShowOverdue || m.getCurrentKey() != today || m.RowIdx != 0 {
			t.Fatalf("%c retained overdue selection", key)
		}
	}
}
func TestServiceAddFromOverdueVisiblyTargetsToday(t *testing.T) {
	m, _ := cloudFixture(t)
	m.cloud.busy = false
	m.ShowOverdue = true
	m = serviceKey(m, 'a')
	if m.ShowOverdue || m.State != Adding || m.getCurrentKey() != m.serviceNow().Format(dateLayout) {
		t.Fatal("add stayed in synthetic Overdue bucket")
	}
	m.TextInput.SetValue("New task")
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.cloud.pending == nil || *m.cloud.pending.Create.ScheduledDate != m.serviceNow().Format(dateLayout) {
		t.Fatal("create did not target Today")
	}
	for _, task := range m.Data[m.getCurrentKey()] {
		if task.DueDate == "Overdue" {
			t.Fatal("synthetic date leaked into task")
		}
	}
}
func TestServiceUncertainDraftCanBeReviewedAndPrivatelyArchived(t *testing.T) {
	m, calls := cloudFixture(t)
	m.cloud.busy = false
	command := taskclient.Command{Origin: m.cloud.origin, Action: "create", Create: &taskclient.CreateRequest{OperationID: taskclient.OperationID(), Title: "Keep uncertain draft", Notes: "Private notes"}}
	m.cloud.pending = &command
	data, _ := command.JSON()
	if err := taskclient.WritePrivate(m.cloud.draftPath, data); err != nil {
		t.Fatal(err)
	}
	m = serviceKey(m, 'd')
	if !m.cloud.discardReview || m.cloud.pending == nil || !strings.Contains(m.View().Content, "does not cancel") {
		t.Fatal("archive did not require outcome review")
	}
	m = serviceKey(m, 'd')
	files, _ := filepath.Glob(m.cloud.draftPath + ".archived-*")
	if len(files) != 1 || m.cloud.pending != nil || *calls != 0 {
		t.Fatal("archive lost command or sent a mutation")
	}
	archived, err := taskclient.LoadCommand(files[0], m.cloud.origin)
	if err != nil || archived.Create.OperationID != command.Create.OperationID || archived.Create.Notes != command.Create.Notes {
		t.Fatal("archive changed operation or notes")
	}
	if _, err := os.Stat(m.cloud.draftPath); !os.IsNotExist(err) {
		t.Fatal("active draft not cleared")
	}
}
func TestServiceConflictOnlyOffersApplicableUpdates(t *testing.T) {
	for _, action := range []string{"delete", "restore", "reorder", "deleted-update"} {
		t.Run(action, func(t *testing.T) {
			m, _ := cloudFixture(t)
			m.cloud.busy = false
			current := reviewTask("task", "2020-01-01")
			command := taskclient.Command{Action: action, ID: current.ID}
			if action == "deleted-update" {
				now := time.Now()
				current.DeletedAt = &now
				command.Action = "update"
				command.Update = &taskclient.UpdateRequest{}
			}
			m.cloud.pending = &command
			m.cloud.conflict = &taskclient.APIError{Status: 409, Code: "revision_conflict", Current: &current}
			if strings.Contains(m.serviceConflictView(), "a apply UPDATE") {
				t.Fatal("offered impossible apply")
			}
			m = serviceKey(m, 'a')
			if m.cloud.busy || m.cloud.queued != nil {
				t.Fatal("impossible update was queued")
			}
		})
	}
}
func TestServiceFooterBoundsAndHelpMatchRenderedState(t *testing.T) {
	for _, state := range []string{"normal", "feedback", "error"} {
		t.Run(state, func(t *testing.T) {
			m, _ := cloudFixture(t)
			m.cloud.busy = false
			m.width, m.height = 120, 40
			if state == "feedback" {
				m.feedback = "Saved to service"
			}
			if state == "error" {
				m.Err = fmt.Errorf("A failed request with details")
			}
			if state == "normal" && m.footerView() != m.serviceStatusView()+"\n"+m.helpView() {
				t.Fatal("normal footer inserted an empty status row")
			}
			if state == "feedback" && !strings.Contains(ansi.Strip(m.footerView()), "Saved to service") {
				t.Fatal("feedback hidden")
			}
			x, y, _, ok := m.brandBounds()
			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			if !ok || y >= len(lines) || strings.Index(lines[y], "doitdoit") != x {
				t.Fatalf("brand hitbox wrong: %d,%d", x, y)
			}
		})
	}
	m, _ := cloudFixture(t)
	m.width, m.height = 120, 100
	help := ansi.Strip(m.helpModalView())
	if strings.Contains(help, "carry forward") || !strings.Contains(help, "original dates") {
		t.Fatal("service help describes rollover")
	}
	for _, obsolete := range []string{"rescan copies", "retained command"} {
		if strings.Contains(help, obsolete) {
			t.Fatalf("service help contains obsolete wording %q", obsolete)
		}
	}
	found := map[string]bool{}
	for _, item := range m.helpItems() {
		found[item.key] = true
	}
	for _, key := range []string{"o", "x", "r"} {
		if !found[key] {
			t.Fatalf("missing service shortcut %s", key)
		}
	}
	for _, deleted := range []bool{false, true} {
		m.ShowDeleted, m.ShowOverdue = deleted, !deleted
		if text := strings.Join(m.dayContent(m.getCurrentKey(), 0, 80).lines, "\n"); strings.Contains(text, "Plan something") {
			t.Fatal("special empty view described a day")
		}
	}
}
func TestServiceBackgroundBoardCannotReplaceActiveEditor(t *testing.T) {
	m, _ := cloudFixture(t)
	m.cloud.busy = false
	m.State = EditingNotes
	m.Data = TodoData{"Future": {{ID: "task", Title: "Draft", Notes: "Retain edits"}}}
	updated, _ := m.Update(serviceResult{sequence: m.cloud.sequence, board: &taskclient.Board{Timezone: "Europe/London", SnapshotCursor: 90}})
	m = updated.(Model)
	if len(m.Data["Future"]) != 1 || m.Data["Future"][0].Notes != "Retain edits" || m.cloud.cursor == 90 {
		t.Fatal("background snapshot replaced editor")
	}
}

func TestServiceRefreshFailureDoesNotTrapActiveEditor(t *testing.T) {
	m, _ := cloudFixture(t)
	m.cloud.busy = false
	m.State = Adding
	m.TextInput.Focus()
	m.TextInput.SetValue("Keep draft")
	updated, _ := m.Update(serviceResult{sequence: m.cloud.sequence, err: fmt.Errorf("disconnected")})
	m = updated.(Model)
	m = serviceKey(m, 'q')
	if m.State != Adding || m.TextInput.Value() != "Keep draftq" {
		t.Fatal("background connection loss trapped the editor")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.cloud.pending == nil || m.cloud.pending.Create.Title != "Keep draftq" {
		t.Fatal("in-progress edit could not be retained after connection loss")
	}
}
func TestServiceAcknowledgementDoesNotWaitForBoardRead(t *testing.T) {
	m, _ := cloudFixture(t)
	m.addTask("Acknowledged independently")
	m.persist()
	command := *m.cloud.queued
	m.cloud.queued = nil
	// Fixture's first response is lost; the identical retry returns its receipt.
	_ = m.serviceWrite(command)()
	result := m.serviceWrite(command)().(serviceResult)
	if result.task == nil || result.board != nil {
		t.Fatal("write result fetched a full board")
	}
	updated, refresh := m.Update(result)
	m = updated.(Model)
	if m.cloud.busy || m.cloud.pending != nil || refresh == nil {
		t.Fatal("acknowledgement did not release editing before refresh")
	}
	raw, _ := json.Marshal(m.cloud.tasks)
	if !strings.Contains(string(raw), "Acknowledged independently") {
		t.Fatal("canonical acknowledgement missing")
	}
}

func TestServiceChangeReconciliationUsesPagedEventsAndPreservesNewerAcknowledgement(t *testing.T) {
	for _, deletedView := range []bool{false, true} {
		t.Run(fmt.Sprint(deletedView), func(t *testing.T) {
			a, b, c := reviewTask("a", "2030-01-01"), reviewTask("b", "2030-01-01"), reviewTask("c", "2030-01-01")
			a.Revision, a.Title = 4, "Keep acknowledged title"
			older := a
			older.Revision, older.Title = 3, "Older event"
			compacted := b
			compacted.Revision, compacted.Position = 2, "z"
			deleted := c
			now := time.Now().UTC()
			deleted.Revision, deleted.DeletedAt = 2, &now
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/changes" {
					t.Errorf("unexpected board fetch: %s", r.URL)
					http.Error(w, "unexpected", 500)
					return
				}
				events := []taskclient.Event{}
				page := taskclient.EventPage{Cursor: 6, Highwater: 6}
				switch r.URL.Query().Get("after") {
				case "2":
					events = []taskclient.Event{{Cursor: 3, TaskID: a.ID, After: &older}, {Cursor: 4, TaskID: b.ID, After: &compacted}}
					next := int64(4)
					page.Cursor, page.NextCursor = 4, &next
				case "4":
					events = []taskclient.Event{{Cursor: 5, TaskID: c.ID, After: &deleted}, {Cursor: 6, TaskID: a.ID, After: &a}}
				default:
					t.Errorf("wrong resume cursor: %s", r.URL)
				}
				page.Events = events
				json.NewEncoder(w).Encode(page)
			}))
			defer server.Close()
			client, _ := taskclient.New(server.URL, nil, func(context.Context) (string, error) { return "test", nil })
			m, _ := NewServiceModel(client, server.URL, filepath.Join(t.TempDir(), "draft.json"))
			m.serviceBoard(taskclient.Board{Timezone: "Europe/London", SnapshotCursor: 2, Tasks: []taskclient.Task{a, b, c}})
			m.cloud.deletedView = deletedView
			result := m.servicePoll()().(serviceResult)
			if result.err != nil || result.board == nil {
				t.Fatalf("reconciliation failed: %v", result.err)
			}
			got := map[string]taskclient.Task{}
			for _, task := range result.board.Tasks {
				got[task.ID] = task
			}
			if result.board.SnapshotCursor != 6 || got[a.ID].Title != a.Title || got[b.ID].Position != "z" {
				t.Fatal("events regressed acknowledgement or missed compaction")
			}
			_, hasDeleted := got[c.ID]
			if hasDeleted != deletedView {
				t.Fatal("deleted visibility differs from selected snapshot")
			}
		})
	}
}
