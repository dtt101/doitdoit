package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrOldNotRemoved means the data reached the destination but the original
// file could not be removed; callers should treat this as a warning.
var ErrOldNotRemoved = errors.New("storage moved but old file could not be removed")

// SamePath reports whether two paths resolve to the same absolute location.
func SamePath(a, b string) bool {
	absA, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return absA == absB
}

// MoveStorage moves the storage file from oldPath to newPath, trying an atomic
// rename first and falling back to a copy across filesystems. It returns
// ErrOldNotRemoved if the data is moved but the original cannot be deleted.
func MoveStorage(oldPath, newPath string) error {
	newDir := filepath.Dir(newPath)
	if err := os.MkdirAll(newDir, 0755); err != nil {
		return fmt.Errorf("creating directory for new path: %w", err)
	}

	if err := os.Rename(oldPath, newPath); err == nil {
		return nil
	}

	if err := copyFile(oldPath, newPath); err != nil {
		return err
	}
	if err := os.Remove(oldPath); err != nil {
		return fmt.Errorf("%w: %v", ErrOldNotRemoved, err)
	}
	return nil
}

// copyFile durably copies src to dst via a temp file, fsync, 0600, and rename.
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening current storage file: %w", err)
	}
	defer source.Close()

	temp, err := os.CreateTemp(filepath.Dir(dst), "doitdoit-move-*")
	if err != nil {
		return fmt.Errorf("creating temp file in destination: %w", err)
	}
	tempPath := temp.Name()

	if _, err := io.Copy(temp, source); err != nil {
		temp.Close()
		os.Remove(tempPath)
		return fmt.Errorf("copying data: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		os.Remove(tempPath)
		return fmt.Errorf("flushing data: %w", err)
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Chmod(tempPath, 0600); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := os.Rename(tempPath, dst); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("moving temp file into place: %w", err)
	}
	return nil
}
