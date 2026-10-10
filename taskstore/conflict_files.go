package taskstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ReviewFile struct {
	Path     string
	Bytes    []byte
	Snapshot Snapshot
}

// DecodeReviewData refuses fields that the current writer would otherwise drop.
// The regular storage API remains backward compatible; recovery never silently
// interprets an unknown format as an empty or partially understood task list.
func DecodeReviewData(contents []byte) (Data, error) {
	// Detect repeated object keys as well as trailing JSON before typed decoding.
	decoder := json.NewDecoder(bytes.NewReader(contents))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				seen := map[string]bool{}
				for decoder.More() {
					key, err := decoder.Token()
					if err != nil {
						return err
					}
					name, ok := key.(string)
					if !ok || seen[name] {
						return fmt.Errorf("duplicate or invalid JSON key %q", key)
					}
					seen[name] = true
					if err := walk(); err != nil {
						return err
					}
				}
			case '[':
				for decoder.More() {
					if err := walk(); err != nil {
						return err
					}
				}
			default:
				return fmt.Errorf("unexpected JSON delimiter")
			}
			_, err = decoder.Token()
			return err
		}
		return nil
	}
	if err := walk(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing JSON")
	}
	decoder = json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var data Data
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}
	if data == nil {
		return nil, fmt.Errorf("expected a task object, not null")
	}
	var raw map[string][]json.RawMessage
	if err := json.Unmarshal(contents, &raw); err != nil {
		return nil, err
	}
	for _, tasks := range raw {
		for _, task := range tasks {
			if bytes.Equal(bytes.TrimSpace(task), []byte("null")) {
				return nil, fmt.Errorf("null task is not a task object")
			}
		}
	}
	return data, nil
}

func ReadReviewFile(path string) (ReviewFile, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return ReviewFile{}, err
	}
	if !info.Mode().IsRegular() {
		return ReviewFile{}, fmt.Errorf("not a regular file: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return ReviewFile{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return ReviewFile{}, err
	}
	if !os.SameFile(info, opened) {
		return ReviewFile{}, ErrDataConflict
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return ReviewFile{}, err
	}
	data, err := DecodeReviewData(contents)
	if err != nil {
		return ReviewFile{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return ReviewFile{Path: path, Bytes: contents, Snapshot: Snapshot{Data: data, Revision: Revision{Hash: sha256.Sum256(contents), Exists: true}, ModTime: opened.ModTime(), Size: int64(len(contents))}}, nil
}

func ConflictPaths(path string) ([]string, error) {
	name := filepath.Base(path)
	extension := filepath.Ext(name)
	stem := strings.TrimSuffix(name, extension)
	// Dropbox inserts its marker before the final extension. Include copies of
	// the backup and copies produced from the already-complete JSON basename.
	marker := ` \([^/\n]+ conflicted copy [0-9]{4}-[0-9]{2}-[0-9]{2}(?: [0-9]+)?\)`
	patterns := []string{
		regexp.QuoteMeta(stem) + marker + regexp.QuoteMeta(extension),
		regexp.QuoteMeta(name) + marker + `\.bak`,
		regexp.QuoteMeta(stem) + marker + regexp.QuoteMeta(extension) + `\.bak`,
	}
	match := regexp.MustCompile(`^(?:` + strings.Join(patterns, "|") + `)$`)
	entries, err := os.ReadDir(filepath.Dir(path))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if match.MatchString(entry.Name()) {
			paths = append(paths, filepath.Join(filepath.Dir(path), entry.Name()))
		}
	}
	return paths, nil
}

func VerifyReviewFile(expected ReviewFile) error {
	actual, err := ReadReviewFile(expected.Path)
	if err != nil {
		return err
	}
	if actual.Snapshot.Revision != expected.Snapshot.Revision {
		return ErrDataConflict
	}
	return nil
}

// RemoveReviewedCopy checks both inputs immediately before removal. Folder sync
// still has a narrow check-to-unlink race, just as conditional JSON replacement
// has a check-to-rename race; neither operation is a Dropbox transaction.
func RemoveReviewedCopy(source ReviewFile, store Store, expected Revision) error {
	current, err := store.Load()
	if err != nil {
		return err
	}
	if current.Revision != expected {
		return ErrDataConflict
	}
	if err := VerifyReviewFile(source); err != nil {
		return err
	}
	return os.Remove(source.Path)
}
