package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dtt101/doitdoit/taskclient"
)

func TestServiceCaptureExplicitDatesPersistBeforeNetworkWithoutBoardFetch(t *testing.T) {
	for _, when := range []string{"future", "2020-02-29"} {
		t.Run(when, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			var credential string
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/tasks" || r.Method != "POST" {
					t.Errorf("unnecessary metadata request: %s", r.URL)
					http.Error(w, "unexpected", 500)
					return
				}
				posts.Add(1)
				drafts, _ := filepath.Glob(filepath.Join(filepath.Dir(credential), "capture-*.json"))
				if len(drafts) != 1 {
					t.Error("capture was not retained before network request")
				}
				var input taskclient.CreateRequest
				json.NewDecoder(r.Body).Decode(&input)
				if when == "future" && input.ScheduledDate != nil {
					t.Error("future was scheduled")
				}
				if when != "future" && (input.ScheduledDate == nil || *input.ScheduledDate != when) {
					t.Error("explicit date changed")
				}
				w.WriteHeader(503)
				w.Write([]byte(`{"error":{"code":"unavailable"}}`))
			}))
			defer server.Close()
			credential, _ = taskclient.CredentialPath(server.URL)
			if err := taskclient.SaveCredentials(credential, taskclient.Credentials{Origin: server.URL, ClientID: "capture", AccessToken: "access", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if addServiceTask(server.URL, "Keep title", "Keep notes", when, &out) != 1 || posts.Load() != 1 {
				t.Fatal("capture did not attempt direct create")
			}
			drafts, _ := filepath.Glob(filepath.Join(filepath.Dir(credential), "capture-*.json"))
			retained, err := taskclient.LoadCommand(drafts[0], server.URL)
			if err != nil || retained.Create.Title != "Keep title" || retained.Create.Notes != "Keep notes" {
				t.Fatalf("failed capture lost fields: %v", err)
			}
		})
	}
}
func TestServiceRelativeCaptureRetainsIntentAndFreezesDateBeforeRetry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var metadata, posts atomic.Int32
	var credential string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/board" {
			if r.URL.Query().Get("limit") != "1" {
				t.Error("capture fetched full board")
			}
			if metadata.Add(1) == 1 {
				w.WriteHeader(503)
				w.Write([]byte(`{"error":{"code":"unavailable"}}`))
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"timezone": "Pacific/Auckland", "tasks": []any{}})
			return
		}
		if r.URL.Path == "/v1/tasks" {
			posts.Add(1)
			drafts, _ := filepath.Glob(filepath.Join(filepath.Dir(credential), "capture-*.json"))
			retained, err := taskclient.LoadCommand(drafts[0], "http://"+r.Host)
			if err != nil || retained.RelativeDate != "" || retained.Create.ScheduledDate == nil {
				t.Error("resolved date was not durable before create")
			}
			w.WriteHeader(503)
			w.Write([]byte(`{"error":{"code":"unavailable"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	credential, _ = taskclient.CredentialPath(server.URL)
	if err := taskclient.SaveCredentials(credential, taskclient.Credentials{Origin: server.URL, ClientID: "capture", AccessToken: "access", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if addServiceTask(server.URL, "Relative title", "Preserve notes", "tomorrow", &out) != 1 {
		t.Fatal("expected offline metadata failure")
	}
	drafts, _ := filepath.Glob(filepath.Join(filepath.Dir(credential), "capture-*.json"))
	if len(drafts) != 1 {
		t.Fatal("offline capture lost")
	}
	intent, err := taskclient.LoadCommand(drafts[0], server.URL)
	if err != nil || intent.RelativeDate != "tomorrow" || intent.CapturedAt == nil || intent.Create.Notes != "Preserve notes" {
		t.Fatal("relative capture intent lost")
	}
	// Move the retained capture instant across a year boundary to prove retry uses
	// the captured instant in the owner timezone, rather than the retry machine's day.
	captured := time.Date(2020, 12, 31, 12, 0, 0, 0, time.UTC)
	intent.CapturedAt = &captured
	raw, _ := intent.JSON()
	if err := taskclient.WritePrivate(drafts[0], raw); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if RunServiceCommand([]string{"retry", "--url", server.URL, "--file", drafts[0]}, &out) != 1 {
			t.Fatal("expected lost acknowledgement")
		}
	}
	resolved, err := taskclient.LoadCommand(drafts[0], server.URL)
	location, _ := time.LoadLocation("Pacific/Auckland")
	want := captured.In(location).AddDate(0, 0, 1).Format("2006-01-02")
	if err != nil || resolved.Create.OperationID != intent.Create.OperationID || resolved.Create.ScheduledDate == nil || *resolved.Create.ScheduledDate != want || resolved.RelativeDate != "" || resolved.CapturedAt != nil {
		t.Fatal("retry changed capture identity or date")
	}
	if metadata.Load() != 2 || posts.Load() != 2 {
		t.Fatal("retry re-resolved an already frozen date")
	}
}
