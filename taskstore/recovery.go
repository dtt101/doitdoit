package taskstore

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Recovery is local recovery material, never part of the synced task schema.
// Bytes preserves exact source contents even after the reviewed copy is removed.
type Recovery struct {
	StorePath         string
	Base, Draft, File Data
	Revision          Revision
	Source            *ReviewFile  `json:",omitempty"`
	CurrentBytes      []byte       `json:",omitempty"`
	Choices           []Difference `json:",omitempty"`
}

func RecoveryDirectory(path string) (string, error) {
	identity, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(identity); err == nil {
		identity = resolved
	} else if resolved, err := filepath.EvalSymlinks(filepath.Dir(identity)); err == nil {
		identity = filepath.Join(resolved, filepath.Base(identity))
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "doitdoit", "recovery", fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))), nil
}

func WriteRecovery(path string, recovery Recovery, pending bool) (string, error) {
	dir, err := RecoveryDirectory(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	contents, err := json.MarshalIndent(recovery, "", "  ")
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("review-%x.json", sha256.Sum256(contents))
	if pending {
		name = "pending.json"
	}
	destination := filepath.Join(dir, name)
	if err := writeBackup(destination, contents); err != nil {
		return "", err
	}
	return destination, nil
}

func ReadRecovery(path string) (*Recovery, error) {
	dir, err := RecoveryDirectory(path)
	if err != nil {
		return nil, err
	}
	contents, err := os.ReadFile(filepath.Join(dir, "pending.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recovery Recovery
	if err := json.Unmarshal(contents, &recovery); err != nil {
		return nil, fmt.Errorf("reading pending recovery: %w", err)
	}
	other, err := RecoveryDirectory(recovery.StorePath)
	if err != nil || other != dir || recovery.Draft == nil {
		return nil, fmt.Errorf("invalid recovery or mismatched task file")
	}
	return &recovery, nil
}

func ClearRecovery(path string) error {
	dir, err := RecoveryDirectory(path)
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, "pending.json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
