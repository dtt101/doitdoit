package taskclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBoardKeepsCursorAndRejectsDuplicatePages(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "duplicate"}[duplicate], func(t *testing.T) {
			n := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n++
				id := "a"
				var next *string
				if n == 1 {
					value := "a"
					next = &value
				} else {
					id = "b"
					if duplicate {
						id = "a"
					}
					if r.URL.Query().Get("snapshot_cursor") != "42" || r.URL.Query().Get("after") != "a" {
						t.Error("snapshot not fixed")
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"tasks": []Task{{ID: id, Title: id, Status: "open", Position: id, Revision: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}}, "timezone": "Europe/London", "snapshot_cursor": 42, "next_cursor": next})
			}))
			defer server.Close()
			c, _ := New(server.URL, nil, func(context.Context) (string, error) { return "test", nil })
			b, err := c.Board(context.Background())
			if duplicate {
				if err == nil {
					t.Fatal("duplicate snapshot task accepted")
				}
			} else if err != nil || len(b.Tasks) != 2 || b.SnapshotCursor != 42 {
				t.Fatalf("board failed: %v", err)
			}
		})
	}
}

func TestRetainedCommandRequiresPrivateValidOriginBoundPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.json")
	command := Command{Origin: "https://tasks.example", Action: "restore", ID: "task", Mutation: &Mutation{OperationID: OperationID(), ExpectedRevision: 4}}
	data, _ := command.JSON()
	if err := WritePrivate(path, data); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCommand(path, command.Origin)
	if err != nil || loaded.Mutation.OperationID != command.Mutation.OperationID {
		t.Fatalf("original operation lost: %v", err)
	}
	if _, err := LoadCommand(path, "https://other.example"); err == nil {
		t.Fatal("different service accepted")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCommand(path, command.Origin); err == nil {
		t.Fatal("public retained command accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`{`, string(data) + `{}`, `{"origin":"https://tasks.example","action":"restore","id":"task"}`} {
		if err := WritePrivate(path, []byte(invalid)); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadCommand(path, command.Origin); err == nil {
			t.Fatal("invalid retained command accepted")
		}
	}
}

func TestPortableExportNeverOverwritesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.json")
	if err := WritePrivateExclusive(path, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := WritePrivateExclusive(path, []byte("replacement")); err == nil {
		t.Fatal("export replaced existing file")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original" {
		t.Fatal("original export changed")
	}
}
