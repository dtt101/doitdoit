package recordstore

import (
	"bytes"
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

func body(n int) []byte {
	return []byte(fmt.Sprintf(`{"schema":1,"kind":"import","snapshot":{"Future":[{"id":"a","title":"task %d","completed":false,"created_at":"2026-09-07T12:00:00Z","due_date":""}]}}`, n))
}
func newStore(t *testing.T) Store {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"synced", "local"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return Store{Root: filepath.Join(dir, "synced", "tasks.json.store"), Pending: filepath.Join(dir, "local", "pending")}
}

func TestQueueRestartPublishAndBackup(t *testing.T) {
	s := newStore(t)
	id, err := s.Queue(body(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("queue touched synced storage")
	}
	restarted := Store{Root: s.Root, Pending: s.Pending}
	if err := restarted.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Queue(body(1)); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Flush(); err != nil {
		t.Fatal(err)
	}
	report := restarted.Scan()
	if len(report.Issues) != 0 || len(report.Records) != 1 || report.Records[0].ID != id {
		t.Fatalf("report=%+v", report)
	}
	if len(scan(s.Pending).Records) != 0 {
		t.Fatal("acknowledged entry still pending")
	}
	original := []byte("{ \"Future\": [] }\n")
	backup, err := s.Backup(original)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(s.Root, "legacy", backup+".json"))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("backup=%q err=%v", got, err)
	}
	info, err := os.Stat(filepath.Join(s.Root, "records", id+".json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissions=%v %v", info, err)
	}
}

func TestFailureNeverAcknowledgesOrDiscardsPending(t *testing.T) {
	for _, point := range []string{"partial-write", "permission", "sync", "publish", "after-publish"} {
		t.Run(point, func(t *testing.T) {
			s := newStore(t)
			failure := errors.New("simulated disk full or interrupted I/O")
			switch point {
			case "partial-write":
				s.write = func(f *os.File, b []byte) error { _, _ = f.Write(b[:len(b)/2]); return failure }
			case "permission":
				s.write = func(*os.File, []byte) error { return os.ErrPermission }
			case "sync":
				s.syncFile = func(*os.File) error { return failure }
			case "publish":
				s.publish = func(string, string) error { return failure }
			case "after-publish":
				s.publish = func(a, b string) error {
					if err := publishFile(a, b); err != nil {
						return err
					}
					return failure
				}
			}
			if _, err := s.Queue(body(1)); err == nil {
				t.Fatal("failed write acknowledged")
			}
			clean := Store{Root: s.Root, Pending: s.Pending}
			if _, err := clean.Queue(body(1)); err != nil {
				t.Fatal(err)
			}
			if err := s.Flush(); err == nil {
				t.Fatal("failed publication acknowledged")
			}
			if len(scan(s.Pending).Records) != 1 {
				t.Fatal("pending edit lost")
			}
			if err := clean.Flush(); err != nil {
				t.Fatal(err)
			}
			if len(clean.Scan().Records) != 1 {
				t.Fatal("retry lost or duplicated record")
			}
		})
	}
}

func TestDiscoveryPreservesBadRecordsAndIgnoresStaging(t *testing.T) {
	s := newStore(t)
	id, err := s.Queue(body(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(s.Root, "records", digest([]byte("bad"))+".json")
	if err := os.WriteFile(badPath, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(s.Root, ".staging", "interrupted")
	if err := os.WriteFile(staging, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	report := s.Scan()
	if len(report.Records) != 1 || report.Records[0].ID != id || len(report.Issues) != 1 {
		t.Fatalf("report=%+v", report)
	}
	if _, err := s.Queue(body(2)); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err == nil {
		t.Fatal("published past malformed record")
	}
	if len(scan(s.Pending).Records) != 1 {
		t.Fatal("pending entry lost")
	}
	if got, _ := os.ReadFile(badPath); string(got) != "{broken" {
		t.Fatal("malformed evidence altered")
	}
}

func TestCollisionAndInvalidPaths(t *testing.T) {
	s := newStore(t)
	record, err := Parse(body(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureDirectory(s.Pending); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Pending, record.ID+".json")
	if err := os.WriteFile(path, []byte("different"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Queue(body(1)); err == nil {
		t.Fatal("collision overwritten")
	}
	if got, _ := os.ReadFile(path); string(got) != "different" {
		t.Fatal("collision altered")
	}
	for _, invalid := range []Store{{}, {Root: s.Root, Pending: s.Root}, {Root: s.Root, Pending: filepath.Join(s.Root, "records")}} {
		if _, err := invalid.Queue(body(1)); err == nil {
			t.Fatal("invalid paths accepted")
		}
	}
}

// Real subprocesses exercise create-only publication without a process-local lock.
func TestProcessWriter(t *testing.T) {
	mode := os.Getenv("DOITDOIT_RECORD_TEST_MODE")
	if mode == "" {
		return
	}
	s := Store{Root: os.Getenv("DOITDOIT_RECORD_TEST_ROOT"), Pending: os.Getenv("DOITDOIT_RECORD_TEST_PENDING")}
	n, _ := strconv.Atoi(os.Getenv("DOITDOIT_RECORD_TEST_NUMBER"))
	if mode == "init-crash" {
		// Stop after directory creation, before any file or directory sync.
		s.write = func(*os.File, []byte) error { os.Exit(0); return nil }
	}
	if _, err := s.Queue(body(n)); err != nil {
		t.Fatal(err)
	}
	if mode == "crash" {
		s.publish = func(a, b string) error {
			if err := publishFile(a, b); err != nil {
				return err
			}
			os.Exit(0)
			return nil
		}
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessesAndRestartAfterPublication(t *testing.T) {
	s := newStore(t)
	run := func(mode string, n int) error {
		cmd := exec.Command(os.Args[0], "-test.run=^TestProcessWriter$")
		cmd.Env = append(os.Environ(), "DOITDOIT_RECORD_TEST_MODE="+mode, "DOITDOIT_RECORD_TEST_ROOT="+s.Root, "DOITDOIT_RECORD_TEST_PENDING="+s.Pending, "DOITDOIT_RECORD_TEST_NUMBER="+strconv.Itoa(n))
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, out)
		}
		return nil
	}
	if err := run("crash", 0); err != nil {
		t.Fatal(err)
	}
	if len(scan(s.Pending).Records) != 1 {
		t.Fatal("crash lost pending entry")
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(n int) { defer wg.Done(); errs <- run("normal", n%3) }(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	report := s.Scan()
	if len(report.Issues) != 0 || len(report.Records) != 3 {
		t.Fatalf("concurrent records=%+v", report)
	}
}

func TestPendingRecoveryReportsValidAndDamagedEdits(t *testing.T) {
	s := newStore(t)
	id, err := s.Queue(body(1))
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(s.Pending, digest([]byte("damaged"))+".json")
	if err := os.WriteFile(bad, []byte("{partial"), 0600); err != nil {
		t.Fatal(err)
	}
	restarted := Store{Root: s.Root, Pending: s.Pending}
	report := restarted.ScanPending()
	if len(report.Records) != 1 || report.Records[0].ID != id || len(report.Issues) != 1 || report.Issues[0].Path != bad {
		t.Fatalf("pending recovery=%+v", report)
	}
	if err := restarted.Flush(); err == nil {
		t.Fatal("damaged pending record was ignored")
	}
	if got, err := os.ReadFile(bad); err != nil || string(got) != "{partial" {
		t.Fatalf("recovery evidence changed: %q %v", got, err)
	}
	if len(restarted.ScanPending().Records) != 1 {
		t.Fatal("valid pending edit lost")
	}
}

func TestPublicationSyncFailureRetainsPendingAndRetriesActualFile(t *testing.T) {
	for _, point := range []string{"file", "directory", "ancestor"} {
		t.Run(point, func(t *testing.T) {
			s := newStore(t)
			id, err := s.Queue(body(1))
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(s.Root, "records", id+".json")
			failure := errors.New("interrupted durability check")
			s.syncFile = func(f *os.File) error {
				if point == "file" && f.Name() == target {
					return failure
				}
				return f.Sync()
			}
			failedDir := filepath.Dir(target)
			if point == "ancestor" {
				failedDir = filepath.Dir(s.Root)
			}
			// macOS temporary directories can use a /var alias.
			failedDir, err = resolvePath(failedDir)
			if err != nil {
				t.Fatal(err)
			}
			s.syncDir = func(path string) error {
				if point != "file" && path == failedDir {
					return failure
				}
				return syncDirectory(path)
			}
			if err := s.Flush(); !errors.Is(err, failure) {
				t.Fatalf("flush=%v, want injected failure", err)
			}
			if len(s.ScanPending().Records) != 1 || len(s.Scan().Records) != 1 {
				t.Fatal("failed acknowledgement lost pending or published record")
			}
			// A retry must flush the existing target inode and its parent chain,
			// even though it did not create either of them itself.
			retry := Store{Root: s.Root, Pending: s.Pending}
			syncedFile, syncedAncestor := false, false
			retry.syncFile = func(f *os.File) error {
				if f.Name() == target {
					syncedFile = true
				}
				return f.Sync()
			}
			retry.syncDir = func(path string) error {
				if path == failedDir {
					syncedAncestor = true
				}
				return syncDirectory(path)
			}
			if err := retry.Flush(); err != nil {
				t.Fatal(err)
			}
			if !syncedFile || !syncedAncestor || len(retry.ScanPending().Records) != 0 {
				t.Fatal("retry acknowledged without persisting the existing publication")
			}
		})
	}
}

func TestQueueDirectoryFailureIsNotAcknowledged(t *testing.T) {
	s := newStore(t)
	failure := errors.New("directory sync failed")
	s.syncDir = func(string) error { return failure }
	if id, err := s.Queue(body(1)); id != "" || !errors.Is(err, failure) {
		t.Fatalf("queue acknowledged failed durability: %q %v", id, err)
	}
	// Retain a possibly durable operation rather than deleting evidence. Retrying
	// the same body must complete its durability checks and use the same ID.
	retry := Store{Root: s.Root, Pending: s.Pending}
	id, err := retry.Queue(body(1))
	if err != nil {
		t.Fatal(err)
	}
	if report := retry.ScanPending(); len(report.Records) != 1 || report.Records[0].ID != id {
		t.Fatalf("restart duplicated or lost operation: %+v", report)
	}
}

func TestRetryRestoresPrivatePermissions(t *testing.T) {
	s := newStore(t)
	id, err := s.Queue(body(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(s.Root, "records", id+".json")
	if err := os.Chmod(target, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Queue(body(1)); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("retry permissions=%v %v", info, err)
	}
}

func TestPathsCannotAliasSyncedAndPendingStorage(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	for _, pending := range []string{filepath.Join(alias, "store"), filepath.Join(alias, "store", "pending")} {
		s := Store{Root: filepath.Join(real, "store"), Pending: pending}
		if _, err := s.Queue(body(1)); err == nil {
			t.Fatal("aliased overlapping paths accepted")
		}
		if len(s.Scan().Issues) == 0 || len(s.ScanPending().Issues) == 0 {
			t.Fatal("discovery accepted aliased overlapping paths")
		}
	}
	if _, err := os.Stat(filepath.Join(real, "store")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid paths touched the store")
	}
}

func TestSymlinkRecordsArePreservedAndNeverFollowed(t *testing.T) {
	s := newStore(t)
	id, err := s.Queue(body(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureDirectory(s.Root); err != nil {
		t.Fatal(err)
	}
	if err := ensureDirectory(filepath.Join(s.Root, "records")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "original.json")
	if err := os.WriteFile(outside, []byte("private original"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(s.Root, "records", id+".json")
	if err := os.Symlink(outside, target); err != nil {
		t.Fatal(err)
	}
	if report := s.Scan(); len(report.Records) != 0 || len(report.Issues) != 1 {
		t.Fatalf("symlink discovery=%+v", report)
	}
	if err := s.Flush(); err == nil {
		t.Fatal("symlink accepted as publication")
	}
	if raw, _ := os.ReadFile(outside); string(raw) != "private original" {
		t.Fatal("symlink target modified")
	}
	if got, err := os.Readlink(target); err != nil || got != outside {
		t.Fatal("symlink evidence removed")
	}
}

func TestPendingRecoverySurvivesDamagedRoot(t *testing.T) {
	for _, damage := range []string{"file", "symlink", "missing-parent"} {
		t.Run(damage, func(t *testing.T) {
			s := newStore(t)
			id, err := s.Queue(body(1))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join(s.Pending, id+".json"))
			if err != nil {
				t.Fatal(err)
			}
			switch damage {
			case "file":
				err = os.WriteFile(s.Root, []byte("damaged root"), 0600)
			case "symlink":
				err = os.Symlink(t.TempDir(), s.Root)
			case "missing-parent":
				err = os.Remove(filepath.Dir(s.Root))
				if err == nil {
					err = os.WriteFile(filepath.Dir(s.Root), []byte("damaged ancestor"), 0600)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			restarted := Store{Root: s.Root, Pending: s.Pending}
			report := restarted.ScanPending()
			if len(report.Records) != 1 || report.Records[0].ID != id || !bytes.Equal(report.Records[0].Body, want) || len(report.Issues) != 1 || report.Issues[0].Path != s.Root {
				t.Fatalf("recovery=%+v", report)
			}
			if err := restarted.Flush(); err == nil {
				t.Fatal("published into damaged root")
			}
			got, err := os.ReadFile(filepath.Join(s.Pending, id+".json"))
			if err != nil || !bytes.Equal(got, want) {
				t.Fatal("pending evidence changed")
			}
			if damage == "file" {
				got, err := os.ReadFile(s.Root)
				if err != nil || string(got) != "damaged root" {
					t.Fatal("root evidence changed")
				}
			}
		})
	}
}

func TestDurabilityBoundaryBelowTraverseOnlyAncestor(t *testing.T) {
	base := t.TempDir()
	ancestor := filepath.Join(base, "traverse")
	parent := filepath.Join(ancestor, "owned-parent")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ancestor, 0100); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ancestor, 0700) })
	s := Store{Root: filepath.Join(parent, "store"), Pending: filepath.Join(parent, "pending")}
	boundary, err := filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatal(err)
	}
	s.syncDir = func(path string) error {
		rel, err := filepath.Rel(boundary, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("sync escaped owned boundary: %s", path)
		}
		return syncDirectory(path)
	}
	id, err := s.Queue(body(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if report := s.Scan(); len(report.Records) != 1 || report.Records[0].ID != id || len(report.Issues) != 0 {
		t.Fatalf("report=%+v", report)
	}
}

func TestStoreDoesNotCreateCallerOwnedAncestors(t *testing.T) {
	s := newStore(t)
	if err := os.Remove(filepath.Dir(s.Pending)); err != nil {
		t.Fatal(err)
	}
	if id, err := s.Queue(body(1)); err == nil || id != "" {
		t.Fatal("acknowledged without an existing durable parent")
	}
	if _, err := os.Stat(filepath.Dir(s.Pending)); !os.IsNotExist(err) {
		t.Fatal("created an ancestor outside store ownership")
	}
}

func TestInterruptedAndConcurrentInitialization(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(strconv.FormatBool(interrupted), func(t *testing.T) {
			s := newStore(t)
			run := func(mode string, n int) error {
				cmd := exec.Command(os.Args[0], "-test.run=^TestProcessWriter$")
				cmd.Env = append(os.Environ(), "DOITDOIT_RECORD_TEST_MODE="+mode, "DOITDOIT_RECORD_TEST_ROOT="+s.Root, "DOITDOIT_RECORD_TEST_PENDING="+s.Pending, "DOITDOIT_RECORD_TEST_NUMBER="+strconv.Itoa(n))
				out, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("%w: %s", err, out)
				}
				return nil
			}
			if interrupted {
				if err := run("init-crash", 0); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(s.Pending); err != nil {
					t.Fatal("child did not create pending directory", err)
				}
				// The restarted process must still persist the parent's entry for
				// the pending directory. Existing directories cannot bypass sync.
				parent, err := filepath.EvalSymlinks(filepath.Dir(s.Pending))
				if err != nil {
					t.Fatal(err)
				}
				failure := errors.New("parent sync failed after interrupted mkdir")
				s.syncDir = func(path string) error {
					if path == parent {
						return failure
					}
					return syncDirectory(path)
				}
				if id, err := s.Queue(body(0)); id != "" || !errors.Is(err, failure) {
					t.Fatalf("acknowledged interrupted directory: %q %v", id, err)
				}
				s.syncDir = nil
			}
			var wg sync.WaitGroup
			errs := make(chan error, 6)
			for i := 0; i < 6; i++ {
				wg.Add(1)
				go func(n int) { defer wg.Done(); errs <- run("normal", n) }(i)
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Flush(); err != nil {
				t.Fatal(err)
			}
			if report := s.Scan(); len(report.Records) != 6 || len(report.Issues) != 0 {
				t.Fatalf("initialized records=%+v", report)
			}
			if report := s.ScanPending(); len(report.Records) != 0 || len(report.Issues) != 0 {
				t.Fatalf("pending=%+v", report)
			}
		})
	}
}

func TestQueueRetriesEntryConsumedDuringPublication(t *testing.T) {
	s := newStore(t)
	clean := Store{Root: s.Root, Pending: s.Pending}
	consumed := false
	s.publish = func(source, target string) error {
		if err := publishFile(source, target); err != nil {
			return err
		}
		if !consumed {
			consumed = true
			// Another process can finish this same immutable operation before
			// Queue opens its pending entry to verify the publication.
			return clean.Flush()
		}
		return nil
	}
	id, err := s.Queue(body(1))
	if err != nil {
		t.Fatal(err)
	}
	if !consumed {
		t.Fatal("did not exercise concurrent pending removal")
	}
	if report := clean.ScanPending(); len(report.Records) != 1 || report.Records[0].ID != id || len(report.Issues) != 0 {
		t.Fatalf("pending=%+v", report)
	}
	if err := clean.Flush(); err != nil {
		t.Fatal(err)
	}
	if report := clean.Scan(); len(report.Records) != 1 || report.Records[0].ID != id || len(report.Issues) != 0 {
		t.Fatalf("published=%+v", report)
	}
}
