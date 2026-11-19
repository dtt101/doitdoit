package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dtt101/doitdoit/config"
	"github.com/dtt101/doitdoit/taskclient"
)

func TestRunAddCommandServiceAndNotes(t *testing.T) {
	for _, notes := range []string{"", "Include expenses", "  café 📝\n\tSecond line\n"} {
		t.Run(notes, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			var got taskclient.CreateRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/v1/tasks" {
					http.NotFound(w, r)
					return
				}
				json.NewDecoder(r.Body).Decode(&got)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"task": taskclient.Task{ID: "11111111-1111-4111-8111-111111111111", Title: got.Title, Notes: got.Notes, Status: "open", Position: "a", Revision: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
				})
			}))
			defer server.Close()
			if err := config.SaveConfig(&config.Config{ServiceOrigin: server.URL}); err != nil {
				t.Fatal(err)
			}
			credPath, err := taskclient.CredentialPath(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if err := taskclient.SaveCredentials(credPath, taskclient.Credentials{Origin: server.URL, ClientID: "test", AccessToken: "access", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if code := RunAddCommand([]string{"--when", "future", "--notes", notes, "send", "invoice"}, &out); code != 0 {
				t.Fatalf("code=%d output=%q", code, out.String())
			}
			if got.Title != "send invoice" || got.Notes != notes || got.ScheduledDate != nil {
				t.Fatalf("unexpected create payload: %+v", got)
			}
		})
	}
}

func TestRunAddCommandInvalidArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--notes"}, {"--notes", "only notes"}, {"--when", "invalid", "--notes", "details", "Task"}, {"--file", "/tmp/old.json", "Task"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			var out bytes.Buffer
			if code := RunAddCommand(args, &out); code != 2 {
				t.Fatalf("args=%v code=%d output=%q", args, code, out.String())
			}
		})
	}
}

func TestRunAddCommandHelp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Help must succeed without reading even a broken config.
	if err := os.WriteFile(filepath.Join(home, ".doitdoit_config.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, arg := range []string{"--help", "-h"} {
		var out bytes.Buffer
		if code := RunAddCommand([]string{arg}, &out); code != 0 {
			t.Fatalf("%s: code=%d output=%q", arg, code, out.String())
		}
		if !strings.Contains(out.String(), "Usage: doitdoit add") {
			t.Fatalf("%s: missing usage: %q", arg, out.String())
		}
	}
}
