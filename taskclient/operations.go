package taskclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"time"
)

// OperationID is generated before a write and retained for identical retries.
func OperationID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

type Mutation struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}
type ReorderRequest struct {
	Mutation
	AfterID  *string `json:"after_id"`
	BeforeID *string `json:"before_id"`
}
type Event struct {
	Cursor      int64  `json:"cursor"`
	TaskID      string `json:"task_id"`
	OperationID string `json:"operation_id"`
	Actor       string `json:"actor"`
	OccurredAt  string `json:"occurred_at"`
	Action      string `json:"action"`
	Before      *Task  `json:"before"`
	After       *Task  `json:"after"`
}
type EventPage struct {
	Events     []Event `json:"events"`
	NextCursor *int64  `json:"next_cursor"`
	Cursor     int64   `json:"cursor"`
	Highwater  int64   `json:"highwater"`
}
type Board struct {
	Tasks          []Task `json:"tasks"`
	SnapshotCursor int64  `json:"snapshot_cursor"`
	Timezone       string `json:"timezone"`
}
type Export struct {
	SchemaVersion int `json:"schema_version"`
	Board
}

// OwnerTimezone reads only the smallest board page, never the whole task list.
func (c *Client) OwnerTimezone(ctx context.Context) (*time.Location, error) {
	var metadata struct {
		Timezone string `json:"timezone"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/board?limit=1", nil, &metadata); err != nil {
		return nil, err
	}
	if metadata.Timezone == "" {
		return nil, fmt.Errorf("task service: missing owner timezone")
	}
	return time.LoadLocation(metadata.Timezone)
}

func (c *Client) Delete(ctx context.Context, id string, command Mutation) (Task, error) {
	return c.mutate(ctx, "/v1/tasks/"+url.PathEscape(id), http.MethodDelete, command)
}
func (c *Client) Restore(ctx context.Context, id string, command Mutation) (Task, error) {
	return c.mutate(ctx, "/v1/tasks/"+url.PathEscape(id)+"/restore", http.MethodPost, command)
}
func (c *Client) Reorder(ctx context.Context, id string, command ReorderRequest) (Task, error) {
	return c.mutate(ctx, "/v1/tasks/"+url.PathEscape(id)+"/reorder", http.MethodPost, command)
}
func (c *Client) mutate(ctx context.Context, path, method string, input any) (Task, error) {
	var r struct {
		Task Task `json:"task"`
	}
	err := c.do(ctx, method, path, input, &r)
	if err == nil {
		err = r.Task.validate()
	}
	return r.Task, err
}
func (c *Client) History(ctx context.Context, id string, after int64) (EventPage, error) {
	var p EventPage
	err := c.do(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(id)+"/history?after="+strconv.FormatInt(after, 10), nil, &p)
	return p, err
}
func (c *Client) Changes(ctx context.Context, after int64) (EventPage, error) {
	var p EventPage
	err := c.do(ctx, http.MethodGet, "/v1/changes?after="+strconv.FormatInt(after, 10), nil, &p)
	if err != nil {
		return p, err
	}
	invalid := p.Cursor < after || p.Highwater < p.Cursor || p.Highwater > 9_007_199_254_740_991
	if p.NextCursor != nil {
		invalid = invalid || *p.NextCursor != p.Cursor || p.Cursor <= after || len(p.Events) == 0
	} else {
		invalid = invalid || p.Cursor != p.Highwater
	}
	previous := after
	for _, event := range p.Events {
		if event.Cursor <= previous || event.Cursor > p.Cursor || event.After == nil || event.TaskID != event.After.ID || event.After.validate() != nil {
			invalid = true
		}
		previous = event.Cursor
	}
	if p.NextCursor != nil && previous != p.Cursor {
		invalid = true
	}
	if invalid {
		return EventPage{}, fmt.Errorf("task service: invalid change page")
	}
	return p, err
}
func (c *Client) Board(ctx context.Context) (Board, error) { return c.snapshot(ctx, false) }
func (c *Client) Export(ctx context.Context) (Export, error) {
	b, err := c.snapshot(ctx, true)
	return Export{1, b}, err
}
func (c *Client) snapshot(ctx context.Context, export bool) (Board, error) {
	var board Board
	path := "/v1/board"
	if export {
		path = "/v1/export"
	}
	after := ""
	seen := map[string]bool{}
	for page := 0; page < 10000; page++ {
		query := url.Values{"limit": {"50"}}
		if page > 0 {
			query.Set("after", after)
			query.Set("snapshot_cursor", strconv.FormatInt(board.SnapshotCursor, 10))
		}
		var result struct {
			Board
			NextCursor    *string `json:"next_cursor"`
			SchemaVersion int     `json:"schema_version"`
		}
		if err := c.do(ctx, http.MethodGet, path+"?"+query.Encode(), nil, &result); err != nil {
			return Board{}, err
		}
		if result.SnapshotCursor < 0 || result.Timezone == "" || (export && result.SchemaVersion != 1) {
			return Board{}, fmt.Errorf("task service: invalid snapshot metadata")
		}
		if page == 0 {
			board.SnapshotCursor = result.SnapshotCursor
			board.Timezone = result.Timezone
		} else if result.SnapshotCursor != board.SnapshotCursor || result.Timezone != board.Timezone {
			return Board{}, fmt.Errorf("task service: inconsistent snapshot")
		}
		for _, t := range result.Tasks {
			if seen[t.ID] || t.validate() != nil {
				return Board{}, fmt.Errorf("task service: invalid snapshot task")
			}
			seen[t.ID] = true
			board.Tasks = append(board.Tasks, t)
		}
		if result.NextCursor == nil {
			sort.SliceStable(board.Tasks, func(i, j int) bool {
				a, b := board.Tasks[i], board.Tasks[j]
				if a.Position == b.Position {
					return a.ID < b.ID
				}
				return a.Position < b.Position
			})
			return board, nil
		}
		if *result.NextCursor <= after || len(result.Tasks) == 0 || *result.NextCursor != result.Tasks[len(result.Tasks)-1].ID {
			return Board{}, fmt.Errorf("task service: invalid snapshot cursor")
		}
		after = *result.NextCursor
	}
	return Board{}, fmt.Errorf("task service: snapshot page limit exceeded")
}

// Command is a durable task-oriented draft, never a whole-board replacement.
type Command struct {
	Origin         string          `json:"origin,omitempty"`
	Action         string          `json:"action"`
	ID             string          `json:"id,omitempty"`
	Create         *CreateRequest  `json:"create,omitempty"`
	Update         *UpdateRequest  `json:"update,omitempty"`
	Mutation       *Mutation       `json:"mutation,omitempty"`
	Reorder        *ReorderRequest `json:"reorder,omitempty"`
	PreviousAfter  *string         `json:"previous_after,omitempty"`
	PreviousBefore *string         `json:"previous_before,omitempty"`
	Before         *Task           `json:"before,omitempty"`
	RelativeDate   string          `json:"relative_date,omitempty"`
	CapturedAt     *time.Time      `json:"captured_at,omitempty"`
}

// ResolveCommand freezes relative capture dates before the command is sent.
// Callers must persist the returned command before Execute, including on retry.
func (c *Client) ResolveCommand(ctx context.Context, command Command) (Command, error) {
	if command.RelativeDate == "" {
		return command, nil
	}
	if command.Origin != c.base || command.Action != "create" || command.Create == nil || command.CapturedAt == nil || command.CapturedAt.IsZero() || (command.RelativeDate != "today" && command.RelativeDate != "tomorrow") {
		return command, fmt.Errorf("invalid relative capture command")
	}
	location, err := c.OwnerTimezone(ctx)
	if err != nil {
		return command, err
	}
	date := command.CapturedAt.In(location)
	if command.RelativeDate == "tomorrow" {
		date = date.AddDate(0, 0, 1)
	}
	value := date.Format("2006-01-02")
	create := *command.Create
	create.ScheduledDate = &value
	command.Create, command.RelativeDate, command.CapturedAt = &create, "", nil
	return command, nil
}

func (c *Client) Execute(ctx context.Context, command Command) (Task, error) {
	if command.RelativeDate != "" || command.CapturedAt != nil {
		return Task{}, fmt.Errorf("relative capture date must be resolved and persisted before sending")
	}
	if command.Origin != "" && command.Origin != c.base {
		return Task{}, fmt.Errorf("recovery command belongs to a different service")
	}
	switch command.Action {
	case "create":
		if command.Create != nil {
			return c.Create(ctx, *command.Create)
		}
	case "update":
		if command.Update != nil {
			return c.Update(ctx, command.ID, *command.Update)
		}
	case "delete":
		if command.Mutation != nil {
			return c.Delete(ctx, command.ID, *command.Mutation)
		}
	case "restore":
		if command.Mutation != nil {
			return c.Restore(ctx, command.ID, *command.Mutation)
		}
	case "reorder":
		if command.Reorder != nil {
			return c.Reorder(ctx, command.ID, *command.Reorder)
		}
	}
	return Task{}, fmt.Errorf("task service: invalid recovery command")
}
func (c Command) JSON() ([]byte, error) { return json.MarshalIndent(c, "", "  ") }

func LoadCommand(path, origin string) (Command, error) {
	var command Command
	info, err := os.Lstat(path)
	if err != nil {
		return command, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return command, fmt.Errorf("retained command must be a private regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return command, err
	}
	if len(data) > 1<<20 {
		return command, fmt.Errorf("retained command exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&command); err != nil {
		return command, fmt.Errorf("retained command cannot be decoded; preserve it for review")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return command, fmt.Errorf("retained command has trailing data")
	}
	count := 0
	for _, present := range []bool{command.Create != nil, command.Update != nil, command.Mutation != nil, command.Reorder != nil} {
		if present {
			count++
		}
	}
	valid := (command.Action == "create" && command.Create != nil) || (command.Action == "update" && command.Update != nil) || ((command.Action == "delete" || command.Action == "restore") && command.Mutation != nil) || (command.Action == "reorder" && command.Reorder != nil)
	if command.Origin != origin || count != 1 || !valid || (command.Action != "create" && command.ID == "") {
		return command, fmt.Errorf("invalid retained command or different service origin")
	}
	if command.RelativeDate != "" || command.CapturedAt != nil {
		if command.Action != "create" || command.Create.ScheduledDate != nil || command.CapturedAt == nil || command.CapturedAt.IsZero() || (command.RelativeDate != "today" && command.RelativeDate != "tomorrow") {
			return command, fmt.Errorf("invalid retained relative capture intent")
		}
	}
	return command, nil
}
