package recordstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func migrationStore(t *testing.T) (Migrator, []byte) {
	t.Helper()
	base := t.TempDir()
	synced, local := filepath.Join(base, "synced"), filepath.Join(base, "local")
	for _, dir := range []string{synced, local} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	raw := []byte(" { \"Future\": [{\"id\":\"a\",\"title\":\"Original\",\"completed\":false,\"created_at\":\"2020-01-01T12:00:00.123456789+01:30\"}] }\n")
	anchor := filepath.Join(synced, "tasks.json")
	if err := os.WriteFile(anchor, raw, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewMigrator(anchor, local)
	if err != nil {
		t.Fatal(err)
	}
	return m, raw
}
func mustRun(t *testing.T, m Migrator) MigrationResult {
	t.Helper()
	r, err := m.Run()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func assertOriginal(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("changed %s: %s %v", path, got, err)
	}
}

func TestMigrationPreservesExactInputAndRestarts(t *testing.T) {
	m, raw := migrationStore(t)
	plan, err := PrepareMigration(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.Store.Root); !os.IsNotExist(err) {
		t.Fatal("constructor touched store")
	}
	for i := 0; i < 3; i++ {
		restarted, err := NewMigrator(m.Anchor, m.LocalParent)
		if err != nil {
			t.Fatal(err)
		}
		result := mustRun(t, restarted)
		if result.Status != "ready" || result.View.UniqueImports != 1 || result.View.CompletionEvents != 0 || len(result.View.Conflicts) != 0 {
			t.Fatalf("result=%+v", result)
		}
		if got := result.View.Snapshot["Future"][0]; got.ID != "a" || got.CreatedAt != "2020-01-01T12:00:00.123456789+01:30" || got.Completed {
			t.Fatalf("task changed: %+v", got)
		}
		archive := m.Store.Inspect()
		if len(archive.Records) != 2 || len(archive.Backups) != 1 || len(archive.Issues) != 0 {
			t.Fatalf("archive=%+v", archive)
		}
		assertOriginal(t, m.Anchor, raw)
		for _, path := range []string{filepath.Join(m.Store.Root, "legacy", plan.BackupID+".json"), filepath.Join(m.scope, "migration", "legacy", plan.BackupID+".json")} {
			assertOriginal(t, path, raw)
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("permissions=%v %v", info, err)
			}
		}
	}
}
func TestMigrationFaultBoundariesResumeFromJournal(t *testing.T) {
	for _, point := range []string{"read", "scope", "journal", "backup", "import", "verified", "activation", "replay", "witness", "finished"} {
		t.Run(point, func(t *testing.T) {
			m, raw := migrationStore(t)
			failure := errors.New("interrupted " + point)
			m.step = func(step string) error {
				if step == point {
					return failure
				}
				return nil
			}
			if result, err := m.Run(); !errors.Is(err, failure) || result.Status == "ready" {
				t.Fatalf("acknowledged interruption: %+v %v", result, err)
			}
			assertOriginal(t, m.Anchor, raw)
			restarted, err := NewMigrator(m.Anchor, m.LocalParent)
			if err != nil {
				t.Fatal(err)
			}
			result := mustRun(t, restarted)
			if result.Status != "ready" || len(result.View.Issues) > 0 || result.View.UniqueImports != 1 {
				t.Fatalf("retry=%+v", result)
			}
		})
	}
}
func TestMigrationRetainsJournalWhenLegacyChangesDuringInterruption(t *testing.T) {
	m, raw := migrationStore(t)
	m.step = func(step string) error {
		if step == "journal" {
			return errors.New("stop")
		}
		return nil
	}
	if _, err := m.Run(); err == nil {
		t.Fatal("expected interruption")
	}
	changed := bytes.Replace(raw, []byte("Original"), []byte("Changed"), 1)
	if err := os.WriteFile(m.Anchor, changed, 0600); err != nil {
		t.Fatal(err)
	}
	m.step = nil
	result := mustRun(t, m)
	if result.Status != "conflict" || result.View.UniqueImports != 2 || len(result.View.Conflicts) != 1 {
		t.Fatalf("lost observed original: %+v", result)
	}
	archive := m.Store.Inspect()
	if len(archive.Backups) != 2 {
		t.Fatal("recovery copies missing")
	}
	assertOriginal(t, m.Anchor, changed)
}
func TestMigrationWaitsForSyncedDependenciesWithoutReimportingLegacy(t *testing.T) {
	for _, missing := range []string{"import", "backup", "activation"} {
		t.Run(missing, func(t *testing.T) {
			m, raw := migrationStore(t)
			p, err := PrepareMigration(raw)
			if err != nil {
				t.Fatal(err)
			}
			if missing != "backup" {
				if _, err := m.Store.Backup(raw); err != nil {
					t.Fatal(err)
				}
			}
			for _, r := range []Record{p.Import, p.Activation} {
				if missing == "import" && r.ID == p.Import.ID || missing == "activation" && r.ID == p.Activation.ID {
					continue
				}
				if err := m.Store.put(filepath.Join(m.Store.Root, "records"), r.ID, r.Body); err != nil {
					t.Fatal(err)
				}
			}
			changed := bytes.Replace(raw, []byte("Original"), []byte("Stale local"), 1)
			if err := os.WriteFile(m.Anchor, changed, 0600); err != nil {
				t.Fatal(err)
			}
			result := mustRun(t, m)
			if result.Status != "waiting" {
				t.Fatalf("did not wait: %+v", result)
			}
			if _, err := os.Stat(m.scope); !os.IsNotExist(err) {
				t.Fatal("waiting imported legacy input")
			}
			if missing == "backup" {
				if _, err := m.Store.Backup(raw); err != nil {
					t.Fatal(err)
				}
			} else {
				r := p.Import
				if missing == "activation" {
					r = p.Activation
				}
				if err := m.Store.put(filepath.Join(m.Store.Root, "records"), r.ID, r.Body); err != nil {
					t.Fatal(err)
				}
			}
			result = mustRun(t, m)
			if result.Status != "conflict" || result.View.UniqueImports != 2 {
				t.Fatalf("late snapshot not reconciled: %+v", result)
			}
		})
	}
}
func TestMigrationCannotFallbackAfterSidecarDamage(t *testing.T) {
	for _, damage := range []string{"missing-root", "root-file", "missing-import", "missing-backup", "missing-activation"} {
		t.Run(damage, func(t *testing.T) {
			m, raw := migrationStore(t)
			mustRun(t, m)
			p, _ := PrepareMigration(raw)
			// An acknowledged unpublished edit must remain recoverable even with a bad root.
			id, err := m.Store.Queue(body(9))
			if err != nil {
				t.Fatal(err)
			}
			switch damage {
			case "missing-root":
				err = os.Rename(m.Store.Root, m.Store.Root+".removed")
			case "root-file":
				err = os.Rename(m.Store.Root, m.Store.Root+".removed")
				if err == nil {
					err = os.WriteFile(m.Store.Root, []byte("damage"), 0600)
				}
			case "missing-import":
				err = os.Remove(filepath.Join(m.Store.Root, "records", p.Import.ID+".json"))
			case "missing-activation":
				err = os.Remove(filepath.Join(m.Store.Root, "records", p.Activation.ID+".json"))
			case "missing-backup":
				err = os.Remove(filepath.Join(m.Store.Root, "legacy", p.BackupID+".json"))
			}
			if err != nil {
				t.Fatal(err)
			}
			restarted, err := NewMigrator(m.Anchor, m.LocalParent)
			if err != nil {
				t.Fatal(err)
			}
			result, err := restarted.Run()
			if !errors.Is(err, ErrMigrationRecovery) || len(result.Pending) != 1 || result.Pending[0].ID != id {
				t.Fatalf("unsafe fallback: %+v %v", result, err)
			}
			assertOriginal(t, m.Anchor, raw)
			if damage == "missing-root" {
				if _, err := os.Stat(m.Store.Root); !os.IsNotExist(err) {
					t.Fatal("recreated missing sidecar")
				}
			}
		})
	}
}
func TestMigrationRejectsInvalidInputBeforeWriting(t *testing.T) {
	for _, raw := range [][]byte{[]byte("{broken"), []byte("null"), []byte(`{"Future":[{"title":"bad"}]}`)} {
		t.Run(string(raw), func(t *testing.T) {
			m, _ := migrationStore(t)
			if err := os.WriteFile(m.Anchor, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Run(); err == nil {
				t.Fatal("invalid input migrated")
			}
			for _, path := range []string{m.scope, m.Store.Root} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("invalid input touched store")
				}
			}
			assertOriginal(t, m.Anchor, raw)
		})
	}
}
func TestMigrationSelectedCopiesAndFreshInstall(t *testing.T) {
	t.Run("explicit copies", func(t *testing.T) {
		m, raw := migrationStore(t)
		copyPath := m.Anchor + " (conflicted copy)"
		changed := bytes.Replace(raw, []byte("Original"), []byte("Other device"), 1)
		if err := os.WriteFile(copyPath, changed, 0600); err != nil {
			t.Fatal(err)
		}
		if r := mustRun(t, m); r.View.UniqueImports != 1 {
			t.Fatal("guessed a conflict-copy filename")
		}
		r, err := m.Run(copyPath)
		if err != nil || r.Status != "conflict" || r.View.UniqueImports != 2 {
			t.Fatalf("selected copy=%+v %v", r, err)
		}
		assertOriginal(t, copyPath, changed)
	})
	t.Run("fresh install", func(t *testing.T) {
		m, _ := migrationStore(t)
		if err := os.Remove(m.Anchor); err != nil {
			t.Fatal(err)
		}
		r := mustRun(t, m)
		if r.Status != "ready" || r.View.Snapshot == nil || len(r.View.Snapshot) != 0 {
			t.Fatalf("fresh=%+v", r)
		}
		if _, err := os.Stat(m.Anchor); !os.IsNotExist(err) {
			t.Fatal("created legacy authority")
		}
		if r = mustRun(t, m); r.View.UniqueImports != 1 {
			t.Fatal("duplicated empty import")
		}
	})
}
func TestMigrationDetectsChangedLegacyWithoutOverwrite(t *testing.T) {
	m, raw := migrationStore(t)
	changed := bytes.Replace(raw, []byte("Original"), []byte("Newer"), 1)
	m.step = func(step string) error {
		if step == "read" {
			return os.WriteFile(m.Anchor, changed, 0600)
		}
		return nil
	}
	if r, err := m.Run(); !errors.Is(err, ErrLegacyChanged) || r.Status == "ready" {
		t.Fatalf("stale completion: %+v %v", r, err)
	}
	m.step = nil
	r := mustRun(t, m)
	if r.View.UniqueImports != 2 || r.Status != "conflict" {
		t.Fatal("lost observed version")
	}
	assertOriginal(t, m.Anchor, changed)
}

func TestMigrationProcess(t *testing.T) {
	if os.Getenv("DOITDOIT_MIGRATION_TEST") != "1" {
		return
	}
	m, err := NewMigrator(os.Getenv("DOITDOIT_MIGRATION_ANCHOR"), os.Getenv("DOITDOIT_MIGRATION_LOCAL"))
	if err != nil {
		t.Fatal(err)
	}
	crash := os.Getenv("DOITDOIT_MIGRATION_CRASH")
	m.step = func(step string) error {
		if step == crash {
			os.Exit(0)
		}
		return nil
	}
	if _, err := m.Run(); err != nil {
		t.Fatal(err)
	}
}
func TestMigrationProcessesAndCrashRestart(t *testing.T) {
	for _, point := range []string{"journal", "import", "activation", "witness"} {
		t.Run(point, func(t *testing.T) {
			m, _ := migrationStore(t)
			run := func(crash string) error {
				cmd := exec.Command(os.Args[0], "-test.run=^TestMigrationProcess$")
				cmd.Env = append(os.Environ(), "DOITDOIT_MIGRATION_TEST=1", "DOITDOIT_MIGRATION_ANCHOR="+m.Anchor, "DOITDOIT_MIGRATION_LOCAL="+m.LocalParent, "DOITDOIT_MIGRATION_CRASH="+crash)
				out, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("%w: %s", err, out)
				}
				return nil
			}
			if err := run(point); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			errs := make(chan error, 4)
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- run("") }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			if r := mustRun(t, m); r.Status != "ready" || r.View.UniqueImports != 1 {
				t.Fatalf("process recovery=%+v", r)
			}
		})
	}
}
func TestMigrationIOfailuresNeverAcknowledge(t *testing.T) {
	for i, point := range []string{"write", "file-sync", "directory-sync", "publish"} {
		t.Run(strconv.Itoa(i)+point, func(t *testing.T) {
			m, raw := migrationStore(t)
			failure := errors.New("I/O failure")
			switch point {
			case "write":
				m.Store.write = func(*os.File, []byte) error { return failure }
			case "file-sync":
				m.Store.syncFile = func(*os.File) error { return failure }
			case "directory-sync":
				m.Store.syncDir = func(string) error { return failure }
			case "publish":
				m.Store.publish = func(string, string) error { return failure }
			}
			if r, err := m.Run(); !errors.Is(err, failure) || r.Status == "ready" {
				t.Fatalf("acknowledged failure: %+v %v", r, err)
			}
			assertOriginal(t, m.Anchor, raw)
			clean, err := NewMigrator(m.Anchor, m.LocalParent)
			if err != nil {
				t.Fatal(err)
			}
			if r := mustRun(t, clean); r.Status != "ready" {
				t.Fatal("could not retry")
			}
		})
	}
}

func mergeMigrationArchive(t *testing.T, target Migrator, source Archive) {
	t.Helper()
	for _, id := range keys(source.Backups) {
		if _, err := target.Store.Backup(source.Backups[id]); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range source.Records {
		if err := target.Store.put(filepath.Join(target.Store.Root, "records"), r.ID, r.Body); err != nil {
			t.Fatal(err)
		}
	}
}
func TestTwoOfflineMigrationsReconcileWithoutWinningDevice(t *testing.T) {
	for _, unequal := range []bool{false, true} {
		t.Run(strconv.FormatBool(unequal), func(t *testing.T) {
			a, raw := migrationStore(t)
			b, _ := migrationStore(t)
			other := append([]byte("\n"), raw...)
			if unequal {
				other = bytes.Replace(other, []byte("Original"), []byte("Offline edit"), 1)
			}
			if err := os.WriteFile(b.Anchor, other, 0600); err != nil {
				t.Fatal(err)
			}
			mustRun(t, a)
			mustRun(t, b)
			aa, bb := a.Store.Inspect(), b.Store.Inspect()
			// Transport may deliver activation before its backup and import.
			for _, r := range bb.Records {
				if recordKind(r).Kind == "activate" {
					if err := a.Store.put(filepath.Join(a.Store.Root, "records"), r.ID, r.Body); err != nil {
						t.Fatal(err)
					}
				}
			}
			if r := mustRun(t, a); r.Status != "waiting" {
				t.Fatalf("early activation did not wait: %+v", r)
			}
			mergeMigrationArchive(t, a, bb)
			mergeMigrationArchive(t, b, aa)
			left, right := mustRun(t, a), mustRun(t, b)
			if !equal(left.View, right.View) {
				t.Fatalf("offline devices diverged: %+v / %+v", left.View, right.View)
			}
			if unequal {
				if left.Status != "conflict" || left.View.UniqueImports != 2 || len(left.View.Conflicts) != 1 {
					t.Fatalf("lost alternatives: %+v", left)
				}
			} else {
				if left.Status != "ready" || left.View.UniqueImports != 1 || len(left.View.Snapshot["Future"]) != 1 {
					t.Fatalf("duplicated equivalent snapshot: %+v", left)
				}
			}
			assertOriginal(t, a.Anchor, raw)
			assertOriginal(t, b.Anchor, other)
		})
	}
}
func TestLateLegacyCannotOverwriteNewStateOrPendingEdits(t *testing.T) {
	m, raw := migrationStore(t)
	mustRun(t, m)
	plan, _ := PrepareMigration(raw)
	before, _ := NormalizeLegacy(raw)
	after := copySnapshot(before)
	after["Future"][0].Completed = true
	change := map[string]any{"schema": 1, "kind": "change", "parents": []string{plan.Activation.ID}, "origin": map[string]any{"device": strings.Repeat("1", 32), "nonce": strings.Repeat("2", 32), "at": "2026-09-10T12:00:00.000Z", "day": "2026-09-10", "offset_minutes": 0}, "action": "complete", "task_ids": []string{"a"}, "buckets": []any{map[string]any{"bucket": "Future", "before": before["Future"], "after": after["Future"]}}}
	encoded, _ := json.Marshal(change)
	id, err := m.Store.Queue(encoded)
	if err != nil {
		t.Fatal(err)
	}
	r := mustRun(t, m)
	if r.Status != "ready" || !r.View.Snapshot["Future"][0].Completed || len(r.Pending) != 1 || r.Pending[0].ID != id {
		t.Fatalf("pending edit lost: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(m.Store.Pending, id+".json")); err != nil {
		t.Fatal("migration removed pending edit")
	}
	if err := m.Store.Flush(); err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(raw, []byte("Original"), []byte("Late old client"), 1)
	if err := os.WriteFile(m.Anchor, changed, 0600); err != nil {
		t.Fatal(err)
	}
	r = mustRun(t, m)
	if r.Status != "conflict" || r.View.CompletionEvents != 1 || r.View.UniqueImports != 2 {
		t.Fatalf("late legacy won: %+v", r)
	}
	found := false
	for _, candidate := range r.View.Conflicts[0].Candidates {
		for _, task := range candidate.Snapshot["Future"] {
			found = found || task.Completed
		}
	}
	if !found {
		t.Fatal("new completion alternative lost")
	}
	assertOriginal(t, m.Anchor, changed)
}
func TestMigrationRepairsAreVisibleAndStableAcrossLanguages(t *testing.T) {
	raw, err := os.ReadFile("../docs/storage/fixtures/validation.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Legacy []struct {
			Raw   string `json:"raw"`
			Valid bool   `json:"valid"`
		} `json:"legacy"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	type envelope struct {
		ID   string `json:"id"`
		Body string `json:"body"`
	}
	type prepared struct {
		Raw        string           `json:"raw"`
		Backup     string           `json:"backup"`
		Import     envelope         `json:"import"`
		Activation envelope         `json:"activation"`
		Repairs    []IdentityRepair `json:"repairs"`
	}
	inputs := []string{}
	expected := []prepared{}
	for _, tc := range fixture.Legacy {
		if !tc.Valid {
			if _, err := PrepareMigration([]byte(tc.Raw)); err == nil {
				t.Fatal("invalid migration prepared")
			}
			continue
		}
		p, err := PrepareMigration([]byte(tc.Raw))
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, tc.Raw)
		expected = append(expected, prepared{tc.Raw, p.BackupID, envelope{p.Import.ID, string(p.Import.Body)}, envelope{p.Activation.ID, string(p.Activation.Body)}, p.Repairs})
		if len(p.Repairs) > 0 {
			m, _ := migrationStore(t)
			if err := os.WriteFile(m.Anchor, p.Raw, 0600); err != nil {
				t.Fatal(err)
			}
			r := mustRun(t, m)
			if !equal(r.Repairs, p.Repairs) {
				t.Fatalf("identity ambiguity hidden: %+v", r)
			}
			assertOriginal(t, m.Anchor, p.Raw)
		}
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node needed for cross-client migration conformance")
	}
	encoded, _ := json.Marshal(inputs)
	cmd := exec.Command("node", "testdata/migration.js")
	cmd.Stdin = bytes.NewReader(encoded)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("JS migration: %v %s", err, out)
	}
	var actual []prepared
	if err := json.Unmarshal(out, &actual); err != nil {
		t.Fatal(err)
	}
	if !equal(actual, expected) {
		t.Fatalf("migration identities differ: %s", out)
	}
}
func TestMigrationDamagedEvidenceIsNeverRepairedInPlace(t *testing.T) {
	for _, point := range []string{"unexpected-root-entry", "bad-backup", "journal-file", "pending-symlink", "dangling-source-symlink", "source-permission"} {
		t.Run(point, func(t *testing.T) {
			m, raw := migrationStore(t)
			var evidence string
			var want []byte
			switch point {
			case "unexpected-root-entry":
				if err := os.Mkdir(m.Store.Root, 0700); err != nil {
					t.Fatal(err)
				}
				evidence = filepath.Join(m.Store.Root, "unknown")
				want = []byte("retain me")
			case "bad-backup":
				mustRun(t, m)
				p, _ := PrepareMigration(raw)
				evidence = filepath.Join(m.Store.Root, "legacy", p.BackupID+".json")
				want = []byte("damaged backup")
			case "journal-file":
				if err := m.ensureScope(); err != nil {
					t.Fatal(err)
				}
				evidence = filepath.Join(m.scope, "migration")
				want = []byte("not a directory")
			case "pending-symlink":
				if err := m.ensureScope(); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), m.Store.Pending); err != nil {
					t.Fatal(err)
				}
			case "dangling-source-symlink":
				if err := os.Rename(m.Anchor, m.Anchor+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(m.Anchor+".missing", m.Anchor); err != nil {
					t.Fatal(err)
				}
			case "source-permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses file permissions")
				}
				if err := os.Chmod(m.Anchor, 0000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(m.Anchor, 0600) })
			}
			if evidence != "" {
				if err := os.WriteFile(evidence, want, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if r, err := m.Run(); err == nil || r.Status == "ready" {
				t.Fatalf("damaged evidence accepted: %+v %v", r, err)
			}
			if evidence != "" {
				assertOriginal(t, evidence, want)
			}
		})
	}
}
func TestMigrationVerifiesPrerequisitesBeforeActivation(t *testing.T) {
	m, raw := migrationStore(t)
	p, _ := PrepareMigration(raw)
	m.step = func(step string) error {
		if step == "import" {
			return os.WriteFile(filepath.Join(m.Store.Root, "legacy", p.BackupID+".json"), []byte("broken"), 0600)
		}
		return nil
	}
	if _, err := m.Run(); !errors.Is(err, ErrMigrationRecovery) {
		t.Fatalf("prerequisite corruption=%v", err)
	}
	if _, err := os.Stat(filepath.Join(m.Store.Root, "records", p.Activation.ID+".json")); !os.IsNotExist(err) {
		t.Fatal("activation preceded verified prerequisites")
	}
	assertOriginal(t, filepath.Join(m.scope, "migration", "legacy", p.BackupID+".json"), raw)
}

func TestMigrationDurabilityFailuresAtEachPublication(t *testing.T) {
	for _, point := range []string{"journal", "backup", "import", "activation", "witness"} {
		for _, kind := range []string{"file", "directory"} {
			t.Run(point+"/"+kind, func(t *testing.T) {
				m, raw := migrationStore(t)
				p, _ := PrepareMigration(raw)
				targets := map[string]string{
					"journal":    filepath.Join(m.scope, "migration", "legacy", p.BackupID+".json"),
					"backup":     filepath.Join(m.Store.Root, "legacy", p.BackupID+".json"),
					"import":     filepath.Join(m.Store.Root, "records", p.Import.ID+".json"),
					"activation": filepath.Join(m.Store.Root, "records", p.Activation.ID+".json"),
					"witness":    filepath.Join(m.scope, "witness", "records", p.Activation.ID+".json"),
				}
				target := targets[point]
				failure := errors.New("target durability failed")
				hit := false
				if kind == "file" {
					m.Store.syncFile = func(f *os.File) error {
						if f.Name() == target {
							hit = true
							return failure
						}
						return f.Sync()
					}
				} else {
					dir := filepath.Dir(target)
					m.Store.syncDir = func(path string) error {
						if path == dir {
							// Imports and activations share a directory; wait for the exact file.
							if _, err := os.Stat(target); err == nil {
								hit = true
								return failure
							}
						}
						return syncDirectory(path)
					}
				}
				if r, err := m.Run(); !hit || !errors.Is(err, failure) || r.Status == "ready" {
					t.Fatalf("acknowledged failed durability: hit=%v %+v %v", hit, r, err)
				}
				clean, err := NewMigrator(m.Anchor, m.LocalParent)
				if err != nil {
					t.Fatal(err)
				}
				if r := mustRun(t, clean); r.Status != "ready" {
					t.Fatalf("retry=%+v", r)
				}
				assertOriginal(t, m.Anchor, raw)
			})
		}
	}
}
func TestMigrationUsesResolvedScopeAndSafeBoundary(t *testing.T) {
	m, raw := migrationStore(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(filepath.Dir(m.Anchor), alias); err != nil {
		t.Fatal(err)
	}
	viaAlias, err := NewMigrator(filepath.Join(alias, filepath.Base(m.Anchor)), m.LocalParent)
	if err != nil {
		t.Fatal(err)
	}
	if viaAlias.Store.Root != m.Store.Root || viaAlias.RecoveryDirectory() != m.RecoveryDirectory() {
		t.Fatal("ancestor alias changed store identity")
	}
	if _, err := NewMigrator(m.Anchor, filepath.Dir(m.Anchor)); err == nil {
		t.Fatal("accepted local recovery parent overlapping synced root")
	}
	parent := filepath.Dir(filepath.Dir(m.Anchor))
	if err := os.Chmod(parent, 0100); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0700) })
	if r := mustRun(t, viaAlias); r.Status != "ready" {
		t.Fatal("traverse-only ancestor blocked migration")
	}
	assertOriginal(t, m.Anchor, raw)
}

func TestMigrationPreservesConfiguredLegacySymlink(t *testing.T) {
	m, raw := migrationStore(t)
	target := m.Anchor + ".target"
	if err := os.Rename(m.Anchor, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, m.Anchor); err != nil {
		t.Fatal(err)
	}
	r := mustRun(t, m)
	if r.Status != "ready" {
		t.Fatalf("symlink migration=%+v", r)
	}
	if got, err := os.Readlink(m.Anchor); err != nil || got != target {
		t.Fatal("legacy symlink replaced")
	}
	assertOriginal(t, target, raw)
	if _, err := os.Stat(target + ".store"); !os.IsNotExist(err) {
		t.Fatal("discovery followed file symlink instead of configured anchor")
	}
}

func TestIncomingMigrationIsDurableBeforeWitness(t *testing.T) {
	source, raw := migrationStore(t)
	mustRun(t, source)
	target, _ := migrationStore(t)
	if err := os.Remove(target.Anchor); err != nil {
		t.Fatal(err)
	}
	mergeMigrationArchive(t, target, source.Store.Inspect())
	plan, _ := PrepareMigration(raw)
	failure := errors.New("incoming import not synced")
	imported := filepath.Join(target.Store.Root, "records", plan.Import.ID+".json")
	target.Store.syncFile = func(f *os.File) error {
		if f.Name() == imported {
			return failure
		}
		return f.Sync()
	}
	if r, err := target.Run(); !errors.Is(err, failure) || r.Status == "ready" {
		t.Fatalf("incoming activation acknowledged prematurely: %+v %v", r, err)
	}
	assertOriginal(t, filepath.Join(target.scope, "migration", "legacy", plan.BackupID+".json"), raw)
	if _, err := os.Stat(filepath.Join(target.scope, "witness", "records", plan.Activation.ID+".json")); !os.IsNotExist(err) {
		t.Fatal("witness preceded durable prerequisites")
	}
	target.Store.syncFile = nil
	if r := mustRun(t, target); r.Status != "ready" {
		t.Fatalf("incoming retry=%+v", r)
	}
}
