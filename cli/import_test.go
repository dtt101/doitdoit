package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportDryRunFreezesExactSourceAndPreservesRecoveryWithoutNetworkOrConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	source := filepath.Join(dir, "tasks.json")
	raw := []byte("{\n\"2020-02-29\":[{\"id\":\"legacy\",\"title\":\"Keep\",\"notes\":\"  note\\n\",\"completed\":true,\"created_at\":\"2019-01-01T00:00:00Z\"}]}\n")
	os.WriteFile(source, raw, 0600)
	os.WriteFile(source+".bak", raw, 0600)
	var out bytes.Buffer
	err := runServiceImport(context.Background(), "https://offline.invalid", filepath.Join(dir, "private", "credentials.json"), source, false, false, &out)
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
	if !strings.Contains(out.String(), "Dry-run only") || !strings.Contains(out.String(), "tasks.json.bak") {
		t.Fatal("dry-run did not report preserved recovery")
	}
}
