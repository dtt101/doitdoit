package taskstore

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// JSONStore retains the single-file format and its atomic save/backup behavior.
type JSONStore struct{ path string }

func NewJSON(path string) JSONStore { return JSONStore{path: path} }

var _ Store = JSONStore{}

func (s JSONStore) Load() (Snapshot, error) {
	snapshot := Snapshot{Data: make(Data)}
	contents, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return snapshot, nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Revision = Revision{Hash: sha256.Sum256(contents), Exists: true}
	// Keep read and parse tied to the same contents even if a sync rename follows.
	info, err := os.Stat(s.path)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.ModTime, snapshot.Size = info.ModTime(), info.Size()
	err = json.Unmarshal(contents, &snapshot.Data)
	return snapshot, err
}

func (s JSONStore) Save(d Data, expected *Revision) (Revision, error) {
	bytes, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return Revision{}, err
	}
	if err := s.saveBytes(bytes, expected); err != nil {
		return Revision{}, err
	}
	return Revision{Hash: sha256.Sum256(bytes), Exists: true}, nil
}

func (s JSONStore) saveBytes(bytes []byte, expected *Revision) error {
	path := s.path
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	current, readErr := os.ReadFile(path)
	currentExists := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	if expected != nil && (currentExists != expected.Exists || (currentExists && sha256.Sum256(current) != expected.Hash)) {
		return ErrDataConflict
	}

	temp, err := os.CreateTemp(dir, "doitdoit-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()

	if _, err := temp.Write(bytes); err != nil {
		temp.Close()
		os.Remove(tempPath)
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		os.Remove(tempPath)
		return err
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		return err
	}

	// Restrict permissions to the owner for privacy
	if err := os.Chmod(tempPath, 0600); err != nil {
		os.Remove(tempPath)
		return err
	}
	if currentExists {
		if err := writeBackup(path+".bak", current); err != nil {
			os.Remove(tempPath)
			return fmt.Errorf("writing backup: %w", err)
		}
	}

	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return err
	}

	return nil
}

func writeBackup(path string, contents []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "doitdoit-backup-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(contents); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, 0600); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
