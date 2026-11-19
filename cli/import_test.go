package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dtt101/doitdoit/taskclient"
)

func TestImportDryRunFreezesExactSourceWithoutNetworkOrConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	source := filepath.Join(dir, "export.json")
	now := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	date := "2020-02-29"
	raw, _ := json.MarshalIndent(taskclient.Export{
		SchemaVersion: 1,
		Board: taskclient.Board{
			Timezone: "Europe/London",
			Tasks: []taskclient.Task{{
				ID:            "11111111-1111-4111-8111-111111111111",
				Title:         "Keep",
				Notes:         "  note\n",
				Status:        "completed",
				ScheduledDate: &date,
				Position:      "a",
				Revision:      1,
				CreatedAt:     now,
				UpdatedAt:     now,
			}},
		},
	}, "", "  ")
	os.WriteFile(source, raw, 0600)
	var out bytes.Buffer
	err := runServiceImport(context.Background(), "https://offline.invalid", filepath.Join(dir, "private", "credentials.json"), source, false, &out)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, _ := os.ReadFile(source)
	if !bytes.Equal(unchanged, raw) {
		t.Fatal("source modified")
	}
	copies, _ := filepath.Glob(filepath.Join(dir, "private", "imports", "*", "source.snapshot.json"))
	if len(copies) != 1 {
		t.Fatal("snapshot missing")
	}
	frozen, _ := os.ReadFile(copies[0])
	if !bytes.Equal(frozen, raw) {
		t.Fatal("snapshot changed bytes")
	}
	info, _ := os.Stat(copies[0])
	if info.Mode().Perm() != 0600 {
		t.Fatal("snapshot not private")
	}
	if !strings.Contains(out.String(), "Dry-run only") || !strings.Contains(out.String(), "service-export-v1") {
		t.Fatalf("unexpected dry-run output: %q", out.String())
	}
}
