package recordstore

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const MaxBytes = 16 << 20

// Linux and macOS both provide atomic, create-only hard-link publication.
func publishFile(source, target string) error { return os.Link(source, target) }

// Store is deliberately not wired into taskstore.Store until replay/migration.
// Root is a synced store directory; Pending is a device-local, store-scoped path.
// Each path's parent must already exist and be durable; Store creates only its
// own directories below those boundaries. No default paths or user files are
// accessed by constructing a Store.
type Store struct {
	Root, Pending string
	// Fault injection stays instance-local so parallel tests/writers cannot race.
	write    func(*os.File, []byte) error
	syncFile func(*os.File) error
	syncDir  func(string) error
	publish  func(string, string) error
}

type Issue struct {
	Path string
	Err  error
}
type Report struct {
	Records []Record
	Issues  []Issue
}

func validatePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("store and pending paths must be absolute")
	}
	if info, err := os.Lstat(path); err == nil && !info.IsDir() {
		return "", fmt.Errorf("not a plain directory: %s", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return resolvePath(path)
}

func separatePaths(root, pending string) error {
	for _, pair := range [][2]string{{root, pending}, {pending, root}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err != nil {
			return err
		}
		if rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("store and pending paths must not overlap")
		}
	}
	return nil
}

func (s Store) validatePaths() error {
	root, err := validatePath(s.Root)
	if err != nil {
		return err
	}
	pending, err := validatePath(s.Pending)
	if err != nil {
		return err
	}
	return separatePaths(root, pending)
}

// resolvePath resolves existing ancestors as well as paths not created yet.
// Lexical comparison alone misses overlap through aliases such as macOS /var.
func resolvePath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolved, err = resolvePath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

// Queue acknowledges an edit only after its immutable local pending file is
// durable. The caller must retain its ID for retries, rather than minting a nonce.
func (s Store) Queue(body []byte) (string, error) {
	if err := s.validatePaths(); err != nil {
		return "", err
	}
	record, err := Parse(body)
	if err != nil {
		return "", err
	}
	if err := s.put(s.Pending, record.ID, record.Body); err != nil {
		return "", err
	}
	return record.ID, nil
}

// Scan reports malformed entries alongside valid records, without deleting either.
// It intentionally performs no replay and does not require parents to arrive first.
func (s Store) Scan() Report {
	if err := s.validatePaths(); err != nil {
		return Report{Issues: []Issue{{s.Root, err}}}
	}
	return scan(filepath.Join(s.Root, "records"))
}

// ScanPending exposes acknowledged local edits for replay and recovery after a
// restart, including edits that could not yet be published to the synced store.
func (s Store) ScanPending() Report {
	pending, err := validatePath(s.Pending)
	if err != nil {
		return Report{Issues: []Issue{{s.Pending, err}}}
	}
	root, rootErr := validatePath(s.Root)
	if rootErr == nil {
		if err := separatePaths(root, pending); err != nil {
			return Report{Issues: []Issue{{s.Pending, err}}}
		}
	}
	report := scan(s.Pending)
	if rootErr != nil {
		report.Issues = append(report.Issues, Issue{s.Root, rootErr})
	}
	return report
}

// Flush retries all pending records. Published entries are verified before their
// pending copy is removed. A crash between publication and removal is idempotent.
func (s Store) Flush() error {
	report := s.Scan()
	if len(report.Issues) != 0 {
		return fmt.Errorf("record store requires recovery: %s: %w", report.Issues[0].Path, report.Issues[0].Err)
	}
	pending := s.ScanPending()
	if len(pending.Issues) != 0 {
		return fmt.Errorf("pending store requires recovery: %s: %w", pending.Issues[0].Path, pending.Issues[0].Err)
	}
	for _, record := range pending.Records {
		if err := s.put(filepath.Join(s.Root, "records"), record.ID, record.Body); err != nil {
			return err
		}
		path := filepath.Join(s.Pending, record.ID+".json")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := s.syncDirectories(s.Pending); err != nil {
			return err
		}
	}
	return nil
}

// Backup preserves exact legacy bytes, independently of normalized import IDs.
// Actual legacy task validation and activation matching belong to migration.
func (s Store) Backup(raw []byte) (string, error) {
	if err := s.validatePaths(); err != nil {
		return "", err
	}
	if _, err := Canonical(raw); err != nil {
		return "", err
	}
	id := digest(raw)
	return id, s.put(filepath.Join(s.Root, "legacy"), id, raw)
}

func scan(dir string) Report {
	report := Report{}
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return report
	}
	if err != nil {
		report.Issues = append(report.Issues, Issue{dir, err})
		return report
	}
	if !info.IsDir() {
		report.Issues = append(report.Issues, Issue{dir, fmt.Errorf("not a plain directory")})
		return report
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		report.Issues = append(report.Issues, Issue{dir, err})
		return report
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !validID(id) || entry.Name() != id+".json" {
			report.Issues = append(report.Issues, Issue{path, fmt.Errorf("unexpected record filename")})
			continue
		}
		raw, err := readPlain(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		} // another process completed this pending entry
		if err == nil {
			record, parseErr := Parse(raw)
			err = parseErr
			if err == nil && (record.ID != id || !bytes.Equal(raw, record.Body)) {
				err = fmt.Errorf("record content/hash mismatch")
			}
			if err == nil {
				report.Records = append(report.Records, record)
				continue
			}
		}
		report.Issues = append(report.Issues, Issue{path, err})
	}
	sort.Slice(report.Records, func(i, j int) bool { return report.Records[i].ID < report.Records[j].ID })
	return report
}

func readPlain(path string) ([]byte, error) {
	f, raw, err := readPlainFile(path)
	if f != nil {
		defer f.Close()
	}
	return raw, err
}

// Keep the verified descriptor open for publication's durability check. Opening
// without following symlinks or blocking on special files also handles an entry
// being replaced between directory discovery and reading.
func readPlainFile(path string) (*os.File, []byte, error) {
	f, err := openPlain(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("not a regular file: %s", path)
	}
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if len(raw) > MaxBytes {
		err = fmt.Errorf("record exceeds size limit: %s", path)
	}
	return f, raw, err
}

func ensureDirectory(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("not a plain directory: %s", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return err
	}
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	// A concurrent creator must also have created a directory, not a symlink/file.
	info, err = os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a plain directory")
	}
	return nil
}

func (s Store) put(dir, id string, raw []byte) error {
	if dir == "" || !validID(id) || len(raw) > MaxBytes {
		return fmt.Errorf("invalid publication target or size")
	}
	// The caller supplies durable existing parents. Create only store-owned
	// directories; never infer a safe boundary from whichever ancestor happens
	// to exist during concurrent or interrupted initialization.
	owned := s.Root
	if filepath.Clean(dir) == filepath.Clean(s.Pending) {
		owned = s.Pending
	}
	parent := filepath.Dir(owned)
	if info, err := os.Stat(parent); err != nil {
		return fmt.Errorf("store parent must already exist: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("store parent is not a directory: %s", parent)
	}
	if err := ensureDirectory(owned); err != nil {
		return err
	}
	if err := ensureDirectory(dir); err != nil {
		return err
	}
	// Staging is a sibling of the public record namespace, never an entry in it.
	staging := filepath.Join(filepath.Dir(dir), ".staging")
	if err := ensureDirectory(staging); err != nil {
		return err
	}
	temp, err := os.CreateTemp(staging, "record-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	if err := temp.Chmod(0600); err != nil {
		return err
	}
	write := s.write
	if write == nil {
		write = func(f *os.File, b []byte) error {
			n, err := f.Write(b)
			if err == nil && n != len(b) {
				err = io.ErrShortWrite
			}
			return err
		}
	}
	if err := write(temp, raw); err != nil {
		return err
	}
	flush := s.syncFile
	if flush == nil {
		flush = (*os.File).Sync
	}
	if err := flush(temp); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	target := filepath.Join(dir, id+".json")
	publish := s.publish
	if publish == nil {
		publish = publishFile
	}
	var published *os.File
	var stored []byte
	for attempt := 0; ; attempt++ {
		err = publish(temp.Name(), target)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		// Existing IDs are successful retries only if exact bytes match. Never
		// replace. A concurrent Flush can consume a pending entry between the
		// link and open, so republish our still-durable staging inode and retry.
		published, stored, err = readPlainFile(target)
		if filepath.Clean(dir) == filepath.Clean(s.Pending) && errors.Is(err, os.ErrNotExist) && attempt < 8 {
			continue
		}
		break
	}
	if published != nil {
		defer published.Close()
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(stored, raw) {
		return fmt.Errorf("immutable record collision at %s", target)
	}
	// A matching existing file may have arrived from another process or folder
	// sync. Flush that actual inode too; flushing our unused staging file cannot
	// make an existing target durable. Restore private permissions on retries.
	if err := published.Chmod(0600); err != nil {
		return err
	}
	if err := flush(published); err != nil {
		return err
	}
	return s.syncDirectories(dir)
}

func (s Store) syncDirectories(path string) error {
	// Always sync through the owned directory's pre-existing parent, including
	// retries. Seeing a directory created by another process is not proof its
	// entry is durable. Ancestors above this explicit boundary belong to callers.
	owned := s.Root
	if filepath.Clean(path) == filepath.Clean(s.Pending) {
		owned = s.Pending
	}
	boundary, err := filepath.EvalSymlinks(filepath.Dir(owned))
	if err != nil {
		return err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	flush := s.syncDir
	if flush == nil {
		flush = syncDirectory
	}
	for {
		if err := flush(path); err != nil {
			return fmt.Errorf("sync directory %s: %w", path, err)
		}
		if path == boundary {
			return nil
		}
		parent := filepath.Dir(path)
		if parent == path {
			return fmt.Errorf("directory is outside durability boundary")
		}
		path = parent
	}
}
