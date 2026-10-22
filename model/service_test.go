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
	"github.com/dtt101/doitdoit/taskclient"
)

func cloudFixture(t *testing.T) (Model, *int) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	calls := new(int)
	stored := map[string]taskclient.Task{}
	receipts := map[string]taskclient.Task{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/changes" {
			json.NewEncoder(w).Encode(map[string]any{"events": []any{}, "cursor": 1, "highwater": 1, "next_cursor": nil})
			return
		}
		if r.URL.Path == "/v1/board" {
			list := []taskclient.Task{}
			for _, v := range stored {
				list = append(list, v)
			}
			json.NewEncoder(w).Encode(map[string]any{"tasks": list, "snapshot_cursor": 1, "timezone": "Europe/London", "next_cursor": nil})
			return
		}
		if r.URL.Path == "/v1/tasks" && r.Method == "POST" {
			*calls++
			var command taskclient.CreateRequest
			json.NewDecoder(r.Body).Decode(&command)
			if receipt, ok := receipts[command.OperationID]; ok {
				json.NewEncoder(w).Encode(map[string]any{"task": receipt})
				return
			}
			v := taskclient.Task{ID: "created", Title: command.Title, Notes: command.Notes, Status: "open", ScheduledDate: command.ScheduledDate, Position: "h", Revision: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
			stored[v.ID] = v
			receipts[command.OperationID] = v
			w.WriteHeader(503)
			fmt.Fprint(w, `{"error":{"code":"unavailable","message":"reply lost"}}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	client, _ := taskclient.New(server.URL, nil, func(context.Context) (string, error) { return "token", nil })
	m, err := NewServiceModel(client, server.URL, filepath.Join(t.TempDir(), "pending.json"))
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 100, 30
	m.serviceBoard(taskclient.Board{Timezone: "Europe/London"})
	return m, calls
}
func TestServiceUnknownWriteKeepsDurableCommandAndRetriesSameOperation(t *testing.T) {
	m, calls := cloudFixture(t)
	m.addTask("Preserve notes and date")
	m.persist()
	command := *m.cloud.queued
	m.cloud.queued = nil
	value := m.serviceWrite(command)()
	updated, _ := m.Update(value)
	m = updated.(Model)
	if m.cloud.pending == nil || m.saveErr == nil {
		t.Fatal("failed response lost draft")
	}
	data, err := os.ReadFile(m.cloud.draftPath)
	if err != nil {
		t.Fatal(err)
	}
	var retained taskclient.Command
	if json.Unmarshal(data, &retained) != nil || retained.Create.OperationID != command.Create.OperationID {
		t.Fatal("operation identity not retained")
	}
	retry := *m.cloud.pending
	m.queueService(retry)
	m.cloud.queued = nil
	updated, _ = m.Update(m.serviceWrite(retry)())
	m = updated.(Model)
	if m.cloud.pending != nil || m.saveErr != nil {
		t.Fatalf("retry failed: %v", m.Err)
	}
	if *calls != 2 || len(m.cloud.tasks) != 1 {
		t.Fatal("retry duplicated task")
	}
	if _, err := os.Stat(m.cloud.draftPath); !os.IsNotExist(err) {
		t.Fatal("acknowledged draft was not cleared")
	}
	if m.cloud.undo == nil || m.cloud.undo.Action != "delete" || m.cloud.undo.Mutation.ExpectedRevision != 1 {
		t.Fatal("create undo was not revision guarded")
	}
}
func TestServiceOverdueAndMidnightDoNotWriteOrReschedule(t *testing.T) {
	m, calls := cloudFixture(t)
	past := "2020-02-29"
	now := time.Now()
	original := taskclient.Task{ID: "old", Title: "Keep date", Notes: "note", Status: "open", ScheduledDate: &past, Position: "h", Revision: 8, CreatedAt: now, UpdatedAt: now}
	m.serviceBoard(taskclient.Board{Timezone: "Europe/London", Tasks: []taskclient.Task{original}})
	if m.Data["Overdue"][0].DueDate != past {
		t.Fatal("overdue date changed")
	}
	m.ShowOverdue = true
	if !strings.Contains(m.View().Content, past) {
		t.Fatal("overdue date not visible")
	}
	updated, _ := m.Update(dateTickMsg(now.Add(24 * time.Hour)))
	m = updated.(Model)
	if *calls != 0 || m.cloud.tasks["old"].Revision != 8 || *m.cloud.tasks["old"].ScheduledDate != past {
		t.Fatal("midnight mutated task")
	}
	m.cloud.sequence = 2
	updated, _ = m.Update(serviceResult{sequence: 1, board: &taskclient.Board{Timezone: "Europe/London"}})
	m = updated.(Model)
	if _, ok := m.cloud.tasks["old"]; !ok {
		t.Fatal("stale response replaced newer board")
	}
}
func TestServiceUndoOnlyRevertsEditedFieldsAtAcknowledgedRevision(t *testing.T) {
	m, _ := cloudFixture(t)
	date := "2030-01-02"
	before := taskclient.Task{ID: "task", Title: "Before", Notes: "Preserve", Status: "open", ScheduledDate: &date, Revision: 4}
	changed := "After"
	command := taskclient.Command{Action: "update", Before: &before, Update: &taskclient.UpdateRequest{Changes: taskclient.Changes{Title: &changed}}}
	after := before
	after.Title = changed
	after.Revision = 5
	m.serviceUndo(command, after)
	undo := m.cloud.undo
	if undo.Update.ExpectedRevision != 5 || undo.Update.Changes.Title == nil || *undo.Update.Changes.Title != "Before" || undo.Update.Changes.Notes != nil || undo.Update.Changes.ScheduledDate != nil {
		t.Fatal("undo would overwrite unrelated fields")
	}
}

func TestServiceRetainedInvalidDraftBlocksWritesUntilReviewed(t *testing.T) {
	m, calls := cloudFixture(t)
	original := []byte(`{"origin":"https://another.example","action":"create"}`)
	if err := os.WriteFile(m.cloud.draftPath, original, 0600); err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(m.serviceRead(true)())
	m = updated.(Model)
	if !m.cloud.recoveryBlocked {
		t.Fatal("invalid retained draft did not block editing")
	}
	for _, key := range []rune{'a', 'u'} {
		updated, _ = m.Update(tea.KeyPressMsg{Code: key})
		m = updated.(Model)
	}
	data, err := os.ReadFile(m.cloud.draftPath)
	if err != nil || string(data) != string(original) || *calls != 0 || m.State != Browsing {
		t.Fatal("invalid draft was overwritten or request sent")
	}
	if err := os.Rename(m.cloud.draftPath, m.cloud.draftPath+".reviewed"); err != nil {
		t.Fatal(err)
	}
	updated, command := m.Update(tea.KeyPressMsg{Code: 'r'})
	m = updated.(Model)
	updated, _ = m.Update(command())
	m = updated.(Model)
	if m.cloud.recoveryBlocked {
		t.Fatal("reviewed recovery could not be rechecked")
	}
}

func TestServiceMinuteTickPreservesBrowsedDateWindow(t *testing.T) {
	m, _ := cloudFixture(t)
	m.updateDateKeysFrom(startOfDay(m.serviceNow()).AddDate(0, 0, 10))
	first := m.dateKeys[0]
	updated, _ := m.Update(dateTickMsg(time.Now()))
	m = updated.(Model)
	if m.dateKeys[0] != first {
		t.Fatal("minute tick reset date navigation")
	}
}

func TestServiceIdlePollDoesNotReloadBoardOrMultiplyTimers(t *testing.T) {
	m, calls := cloudFixture(t)
	m.cloud.cursor = 1
	result := m.servicePoll()().(serviceResult)
	if result.err != nil || !result.polled || result.board != nil || *calls != 0 {
		t.Fatalf("unchanged poll did more than check changes: %v", result.err)
	}
	_, command := m.Update(result)
	if command != nil {
		t.Fatal("poll completion created a second periodic timer")
	}
}

func TestServiceEditorCanTypeQuitShortcut(t *testing.T) {
	m, _ := cloudFixture(t)
	m.cloud.busy = false
	m.State = Adding
	m.TextInput.Focus()
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(Model)
	if m.State != Adding || m.TextInput.Value() != "q" {
		t.Fatal("quit shortcut intercepted task text")
	}
}
