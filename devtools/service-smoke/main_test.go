package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dtt101/doitdoit/taskclient"
)

func TestSmokeLoginAndTaskRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var mu sync.Mutex
	var redirect, challenge string
	const token = "private-test-token-never-log"
	const code = "private-authorization-code"
	var task taskclient.Task
	receipts := map[string]taskclient.Task{}
	created, mutated, revoked := 0, 0, false
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		respond := func(v any) {
			t.Helper()
			if err := json.NewEncoder(w).Encode(v); err != nil {
				t.Error(err)
			}
		}
		switch r.URL.Path {
		case "/health":
			respond(map[string]string{"status": "ok"})
		case "/oauth/register":
			var input struct {
				Redirects []string `json:"redirect_uris"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Redirects) != 1 {
				t.Error("invalid registration")
				w.WriteHeader(400)
				return
			}
			redirect = input.Redirects[0]
			if !strings.HasPrefix(redirect, "http://127.0.0.1:") {
				t.Error("callback is not loopback")
			}
			w.WriteHeader(201)
			respond(map[string]string{"client_id": "public-client"})
		case "/authorize":
			p := r.URL.Query()
			challenge = p.Get("code_challenge")
			if p.Get("redirect_uri") != redirect || p.Get("code_challenge_method") != "S256" || p.Get("resource") != origin {
				t.Error("invalid authorization parameters")
			}
			http.Redirect(w, r, redirect+"?"+url.Values{"state": {p.Get("state")}, "code": {code}}.Encode(), 302)
		case "/oauth/token":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if len(r.Form.Get("code_verifier")) < 43 || base64.RawURLEncoding.EncodeToString(hash[:]) != challenge || r.Form.Get("code") != code || r.Form.Get("redirect_uri") != redirect || r.Form.Get("resource") != origin || r.Form.Get("client_id") != "public-client" {
				t.Error("invalid token exchange")
			}
			respond(map[string]string{"access_token": token, "token_type": "Bearer"})
		case "/oauth/revoke":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("token") != token || r.Form.Get("client_id") != "public-client" {
				t.Error("invalid revocation")
			}
			revoked = true
			respond(map[string]any{})
		default:
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Error("missing credential")
				w.WriteHeader(401)
				return
			}
			switch {
			case r.URL.Path == "/v1/tasks" && r.Method == http.MethodPost:
				var input taskclient.CreateRequest
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
				}
				if input.ScheduledDate == nil || *input.ScheduledDate != "2020-02-29" {
					t.Error("smoke test must exercise past date")
				}
				task = taskclient.Task{ID: "test-task", Title: input.Title, Notes: input.Notes, Status: "open", ScheduledDate: input.ScheduledDate, Position: "a", Revision: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
				created++
				w.WriteHeader(201)
				respond(map[string]any{"task": task})
			case r.URL.Path == "/v1/tasks/test-task" && r.Method == http.MethodPatch:
				var input taskclient.UpdateRequest
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
				}
				if previous, ok := receipts[input.OperationID]; ok {
					respond(map[string]any{"task": previous})
					return
				}
				if input.ExpectedRevision != task.Revision {
					w.WriteHeader(409)
					respond(map[string]any{"error": map[string]any{"code": "revision_conflict", "current": task}})
					return
				}
				if input.Changes.ScheduledDate != nil {
					task.ScheduledDate = *input.Changes.ScheduledDate
				}
				if input.Changes.Status != nil {
					task.Status = *input.Changes.Status
				}
				task.Revision++
				mutated++
				receipts[input.OperationID] = task
				respond(map[string]any{"task": task})
			case r.URL.Path == "/mcp":
				var input struct {
					Method string `json:"method"`
					Params struct {
						Name      string `json:"name"`
						Arguments struct {
							ID string `json:"id"`
						} `json:"arguments"`
					} `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
				}
				if input.Method == "initialize" {
					respond(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]string{"protocolVersion": "2025-11-25"}})
				} else {
					if input.Method != "tools/call" || input.Params.Name != "get_task" || input.Params.Arguments.ID != task.ID {
						t.Error("wrong MCP tool call")
					}
					respond(map[string]any{"jsonrpc": "2.0", "id": 2, "result": map[string]any{"structuredContent": map[string]any{"task": task}}})
				}
			default:
				t.Error("unexpected endpoint", r.Method, r.URL.Path)
				w.WriteHeader(404)
			}
		}
	}))
	defer server.Close()
	origin = server.URL
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	open := func(location string) error {
		// A forged callback must not consume the genuine login waiting in the terminal.
		u, err := url.Parse(location)
		if err != nil {
			return err
		}
		target := u.Query().Get("redirect_uri")
		bad, err := http.Get(target + "?state=forged&code=forged")
		if err != nil {
			return err
		}
		bad.Body.Close()
		if bad.StatusCode != 400 {
			t.Errorf("forged callback: %d", bad.StatusCode)
		}
		browser, err := http.Get(location)
		if err != nil {
			return err
		}
		browser.Body.Close()
		if browser.StatusCode != 200 {
			return fmt.Errorf("callback failed: %d", browser.StatusCode)
		}
		return nil
	}
	if err := run(ctx, origin, &output, open); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if created != 1 || mutated != 2 || task.Status != "completed" || !revoked {
		t.Fatalf("unexpected outcome: created=%d mutated=%d status=%s revoked=%v", created, mutated, task.Status, revoked)
	}
	if strings.Contains(output.String(), token) || strings.Contains(output.String(), code) {
		t.Fatal("credentials leaked to terminal")
	}
	if !strings.Contains(output.String(), "Local smoke test passed") {
		t.Fatal("missing success message")
	}
}

func TestLoginCancellationAndRedirects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Run("cancel", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/oauth/register" {
				t.Error("unexpected request", r.URL.Path)
			}
			fmt.Fprintln(w, `{"client_id":"public-client"}`)
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		client := smokeClient{origin: server.URL, http: server.Client(), out: io.Discard}
		_, _, err := client.login(ctx, func(string) error { cancel(); return nil })
		if err != context.Canceled {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("service redirect rejected", func(t *testing.T) {
		contacted := false
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacted = true }))
		defer other.Close()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, 302) }))
		defer server.Close()
		err := run(context.Background(), server.URL, io.Discard, func(string) error { t.Fatal("must not start login"); return nil })
		if err == nil || contacted {
			t.Fatal("service redirect was followed")
		}
	})
}
