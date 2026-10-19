package taskclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../api/fixtures/task.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "test-token", nil })
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestClientPreservesSharedTaskContract(t *testing.T) {
	data := fixture(t)
	var expected Task
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing application credentials")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(append(append([]byte(`{"task":`), data...), '}'))
	}))
	defer server.Close()
	task, err := testClient(t, server).Get(t.Context(), expected.ID)
	if err != nil || task.ScheduledDate == nil || *task.ScheduledDate != "2020-02-29" || task.Notes != expected.Notes || task.Revision != 2 || task.CompletedAt != nil {
		t.Fatalf("contract mismatch: %+v, %v", task, err)
	}
}

func TestClientDistinguishesOmittedAndClearedDates(t *testing.T) {
	data := fixture(t)
	var bodies []map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Error("expected PATCH")
		}
		var command struct {
			OperationID string                     `json:"operation_id"`
			Revision    int64                      `json:"expected_revision"`
			Changes     map[string]json.RawMessage `json:"changes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
			t.Error(err)
		}
		if command.OperationID != "keep-this-id" || command.Revision != 2 {
			t.Error("request identity/revision changed")
		}
		bodies = append(bodies, command.Changes)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(append(append([]byte(`{"task":`), data...), '}'))
	}))
	defer server.Close()
	client := testClient(t, server)
	title := "updated"
	var clearDate *string
	scheduled := "2030-01-01"
	updates := []Changes{{Title: &title}, {ScheduledDate: &clearDate}}
	setDate := &scheduled
	updates = append(updates, Changes{ScheduledDate: &setDate})
	for _, changes := range updates {
		if _, err := client.Update(t.Context(), "task-id", UpdateRequest{"keep-this-id", 2, changes}); err != nil {
			t.Fatal(err)
		}
	}
	if _, exists := bodies[0]["scheduled_date"]; exists {
		t.Fatal("title edit touched schedule")
	}
	if string(bodies[1]["scheduled_date"]) != "null" || string(bodies[2]["scheduled_date"]) != `"2030-01-01"` {
		t.Fatal("clearing/setting a date encoded incorrectly", bodies)
	}
}

func TestClientReturnsConflictAndDoesNotRetry(t *testing.T) {
	data := fixture(t)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write(append(append([]byte(`{"error":{"code":"revision_conflict","message":"Review it","current":`), data...), []byte("}}")...))
	}))
	defer server.Close()
	_, err := testClient(t, server).Update(t.Context(), "task-id", UpdateRequest{OperationID: "stable", ExpectedRevision: 1})
	var conflict *APIError
	if !errors.As(err, &conflict) || conflict.Code != "revision_conflict" || conflict.Current == nil || conflict.Current.Revision != 2 {
		t.Fatalf("lost conflict context: %v", err)
	}
	if attempts.Load() != 1 {
		t.Fatal("stale mutation retried automatically")
	}
}

func TestClientNeverForwardsCredentialsOnRedirect(t *testing.T) {
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := testClient(t, server).List(t.Context())
	if err == nil || calls.Load() != 0 {
		t.Fatal("followed redirect")
	}
}

func TestClientRejectsUnsafeOriginsAndHidesCredentialFailures(t *testing.T) {
	source := func(context.Context) (string, error) { return "", errors.New("secret-value") }
	for _, origin := range []string{"http://example.com", "https://user:secret@example.com", "https://example.com/api", "https://example.com?x=y", "https://example.com#fragment"} {
		if _, err := New(origin, nil, source); err == nil {
			t.Errorf("accepted %q", origin)
		}
	}
	client, err := New("https://example.com", nil, source)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.List(t.Context())
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("credential failure leaked details: %v", err)
	}
}

func TestClientRejectsIncompleteAcknowledgements(t *testing.T) {
	for _, body := range []string{`{"task":null}`, `{"task":{}}`, `{}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			_, err := testClient(t, server).Create(t.Context(), CreateRequest{Title: "Retain my draft"})
			if err == nil {
				t.Fatal("invalid acknowledgement reported as a successful save")
			}
		})
	}
}
