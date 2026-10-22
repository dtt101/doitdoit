// Package taskclient provides the experimental HTTP client for the Cloudflare
// task service, used by the TUI and CLI after explicit backend selection.
package taskclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Task dates are local-calendar values. They are deliberately not time.Time.
type Task struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Notes         string     `json:"notes"`
	Status        string     `json:"status"`
	ScheduledDate *string    `json:"scheduled_date"`
	Position      string     `json:"position"`
	Revision      int64      `json:"revision"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	DeletedAt     *time.Time `json:"deleted_at"`
}

type CreateRequest struct {
	OperationID   string  `json:"operation_id"`
	Title         string  `json:"title"`
	Notes         string  `json:"notes"`
	ScheduledDate *string `json:"scheduled_date"`
}

// Changes distinguishes omitted dates from an explicit null (clear schedule).
// ScheduledDate points to nil to clear the date, or points to a date to set it.
type Changes struct {
	Title         *string  `json:"title,omitempty"`
	Notes         *string  `json:"notes,omitempty"`
	Status        *string  `json:"status,omitempty"`
	ScheduledDate **string `json:"scheduled_date,omitempty"`
}

type UpdateRequest struct {
	OperationID      string  `json:"operation_id"`
	ExpectedRevision int64   `json:"expected_revision"`
	Changes          Changes `json:"changes"`
}

type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Current *Task  `json:"current,omitempty"`
}

func (e *APIError) Error() string { return fmt.Sprintf("task service: HTTP %d (%s)", e.Status, e.Code) }

// TokenSource supplies application credentials, never Cloudflare management
// credentials. Login and refresh are intentionally separate from this transport.
type TokenSource func(context.Context) (string, error)

type Client struct {
	base   string
	http   *http.Client
	tokens TokenSource
}

func New(base string, transport *http.Client, tokens TokenSource) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("task service: expected an origin URL")
	}
	loopback := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, errors.New("task service: HTTPS is required outside loopback")
	}
	if tokens == nil {
		return nil, errors.New("task service: token source is required")
	}
	if transport == nil {
		transport = &http.Client{Timeout: 15 * time.Second}
	}
	copy := *transport
	// Never forward credentials or turn a redirected mutation into a new action.
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{base: strings.TrimSuffix(base, "/"), http: &copy, tokens: tokens}, nil
}

func (c *Client) List(ctx context.Context) ([]Task, error) {
	var result struct {
		Tasks []Task `json:"tasks"`
	}
	err := c.do(ctx, http.MethodGet, "/v1/tasks", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Tasks == nil {
		return nil, errors.New("task service: invalid task list")
	}
	for _, task := range result.Tasks {
		if err := task.validate(); err != nil {
			return nil, err
		}
	}
	return result.Tasks, nil
}

func (c *Client) Get(ctx context.Context, id string) (Task, error) {
	var result struct {
		Task Task `json:"task"`
	}
	err := c.do(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(id), nil, &result)
	if err == nil {
		err = result.Task.validate()
	}
	return result.Task, err
}

func (c *Client) Create(ctx context.Context, input CreateRequest) (Task, error) {
	var result struct {
		Task Task `json:"task"`
	}
	err := c.do(ctx, http.MethodPost, "/v1/tasks", input, &result)
	if err == nil {
		err = result.Task.validate()
	}
	return result.Task, err
}

func (c *Client) Update(ctx context.Context, id string, input UpdateRequest) (Task, error) {
	var result struct {
		Task Task `json:"task"`
	}
	err := c.do(ctx, http.MethodPatch, "/v1/tasks/"+url.PathEscape(id), input, &result)
	if err == nil {
		err = result.Task.validate()
	}
	return result.Task, err
}

func (t Task) validate() error {
	if t.ID == "" || t.Title == "" || t.Position == "" || t.Revision < 1 ||
		(t.Status != "open" && t.Status != "completed") ||
		t.CreatedAt.IsZero() || t.UpdatedAt.IsZero() {
		return errors.New("task service: invalid task response")
	}
	if t.ScheduledDate != nil {
		if _, err := time.Parse("2006-01-02", *t.ScheduledDate); err != nil {
			return errors.New("task service: invalid scheduled date")
		}
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return errors.New("task service: could not construct request")
	}
	token, err := c.tokens(ctx)
	if err != nil {
		return errors.New("task service: could not obtain credentials")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(req)
	if err != nil {
		// A failed response is an unknown outcome. Let callers retain the command
		// and retry its original operation ID; do not retry mutations implicitly.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("task service: request failed; write outcome may be unknown")
	}
	defer response.Body.Close()
	const limit = 2 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(data) > limit {
		return errors.New("task service: incomplete or oversized response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Error APIError `json:"error"`
		}
		if json.Unmarshal(data, &failure) != nil || failure.Error.Code == "" {
			return &APIError{Status: response.StatusCode, Code: "invalid_error_response"}
		}
		failure.Error.Status = response.StatusCode
		return &failure.Error
	}
	if json.Unmarshal(data, output) != nil {
		return errors.New("task service: invalid JSON response")
	}
	return nil
}
