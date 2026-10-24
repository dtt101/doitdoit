package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/dtt101/doitdoit/taskclient"
	"github.com/dtt101/doitdoit/tasktransfer"
)

func runExtended(ctx context.Context, origin string, out io.Writer, open func(string) error) (err error) {
	credentials, err := taskclient.Login(ctx, origin, out, open)
	if err != nil {
		return err
	}
	directory, err := os.MkdirTemp("", "doitdoit-smoke-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "credentials.json")
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		current, e := taskclient.LoadCredentials(path)
		if e != nil {
			current = credentials
		}
		if e := taskclient.Logout(cleanup, current); e != nil {
			if err == nil {
				err = e
			}
			fmt.Fprintln(out, "Temporary login revocation failed.")
		} else {
			fmt.Fprintln(out, "Temporary login revoked; no credentials retained.")
		}
	}()
	// Force the desktop cache to refresh immediately; server expiry remains normal.
	credentials.ExpiresAt = time.Now()
	if err = taskclient.SaveCredentials(path, credentials); err != nil {
		return err
	}
	tokens := taskclient.CredentialSource(origin, path)
	client, err := taskclient.New(origin, nil, tokens)
	if err != nil {
		return err
	}
	if _, err = client.Board(ctx); err != nil {
		return err
	}
	rotated, err := taskclient.LoadCredentials(path)
	if err != nil {
		return err
	}
	if rotated.RefreshToken == credentials.RefreshToken || rotated.AccessToken == credentials.AccessToken {
		return errors.New("desktop refresh failed to rotate credentials")
	}
	fmt.Fprintln(out, "PASS: desktop credential store refreshed, rotated and persisted new credentials.")
	date := "2099-10-22"
	a, err := client.Create(ctx, taskclient.CreateRequest{OperationID: taskclient.OperationID(), Title: "[extended smoke] A", Notes: "Disposable test task; no real task imports.", ScheduledDate: &date})
	if err != nil {
		return err
	}
	b, err := client.Create(ctx, taskclient.CreateRequest{OperationID: taskclient.OperationID(), Title: "[extended smoke] B", ScheduledDate: &date})
	if err != nil {
		return err
	}
	board, err := client.Board(ctx)
	if err != nil {
		return err
	}
	group := []taskclient.Task{}
	for _, t := range board.Tasks {
		if t.ScheduledDate != nil && *t.ScheduledDate == date && t.Status == "open" {
			group = append(group, t)
		}
	}
	var preceding *string
	for i, t := range group {
		if t.ID == a.ID && i > 0 {
			v := group[i-1].ID
			preceding = &v
		}
	}
	before := a.ID
	b, err = client.Reorder(ctx, b.ID, taskclient.ReorderRequest{Mutation: taskclient.Mutation{OperationID: taskclient.OperationID(), ExpectedRevision: b.Revision}, AfterID: preceding, BeforeID: &before})
	if err != nil {
		return err
	}
	deleted, err := client.Delete(ctx, b.ID, taskclient.Mutation{OperationID: taskclient.OperationID(), ExpectedRevision: b.Revision})
	if err != nil {
		return err
	}
	if deleted.DeletedAt == nil {
		return errors.New("delete was not recoverable")
	}
	b, err = client.Restore(ctx, b.ID, taskclient.Mutation{OperationID: taskclient.OperationID(), ExpectedRevision: deleted.Revision})
	if err != nil {
		return err
	}
	if b.ScheduledDate == nil || *b.ScheduledDate != date {
		return errors.New("restore changed date")
	}
	history, err := client.History(ctx, b.ID, 0)
	if err != nil {
		return err
	}
	if len(history.Events) < 4 {
		return errors.New("task history incomplete")
	}
	changes, err := client.Changes(ctx, board.SnapshotCursor)
	if err != nil || len(changes.Events) < 3 {
		return errors.New("change feed incomplete")
	}
	fmt.Fprintln(out, "PASS: reorder, recoverable deletion, restore, history and changes.")
	token, err := tokens(ctx)
	if err != nil {
		return err
	}
	c := smokeClient{origin: origin, http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, out: out}
	var rpc struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err = c.json(ctx, "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]any{}}, token, &rpc); err != nil {
		return err
	}
	var listed struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if json.Unmarshal(rpc.Result, &listed) != nil || len(listed.Tools) != 11 {
		return errors.New("MCP tool discovery incomplete")
	}
	rpc.Result = nil
	if err = c.json(ctx, "/mcp", map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "task_history", "arguments": map[string]any{"id": b.ID}}}, token, &rpc); err != nil {
		return err
	}
	var historyResult struct {
		IsError    bool `json:"isError"`
		Structured struct {
			Events []taskclient.Event `json:"events"`
		} `json:"structuredContent"`
	}
	if json.Unmarshal(rpc.Result, &historyResult) != nil || historyResult.IsError || len(historyResult.Structured.Events) < 4 {
		return errors.New("MCP task history failed")
	}
	fmt.Fprintln(out, "PASS: MCP discovers all 11 tools and reads canonical task history.")
	// Complete only the two tasks created by this test.
	completed := "completed"
	for _, task := range []taskclient.Task{a, b} {
		finished, e := client.Update(ctx, task.ID, taskclient.UpdateRequest{OperationID: taskclient.OperationID(), ExpectedRevision: task.Revision, Changes: taskclient.Changes{Status: &completed}})
		if e != nil {
			return e
		}
		if finished.ScheduledDate == nil || *finished.ScheduledDate != date {
			return errors.New("completion changed date")
		}
	}
	exported, err := client.Export(ctx)
	if err != nil {
		return err
	}
	data, err := json.Marshal(exported)
	if err != nil {
		return err
	}
	plan, err := tasktransfer.Build(data)
	if err != nil || len(plan.Report.Issues) > 0 {
		return errors.New("portable export failed strict validation")
	}
	if err = tasktransfer.Verify(exported.Tasks, plan.Tasks); err != nil {
		return err
	}
	fmt.Fprintln(out, "PASS: completed disposable tasks and verified portable export field equivalence.")
	// Verify revocation through the same desktop client (it still has a cached token).
	if err = taskclient.Logout(ctx, rotated); err != nil {
		return err
	}
	_, err = client.Board(ctx)
	var authError *taskclient.APIError
	if !errors.As(err, &authError) || authError.Status != 401 {
		return errors.New("revoked desktop access was not rejected")
	}
	fmt.Fprintln(out, "PASS: revoked desktop login rejected. Extended smoke passed.")
	return nil
}
