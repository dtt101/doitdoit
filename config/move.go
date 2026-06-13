package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrOldNotRemoved indicates the data was moved to the destination
// successfully but the original file could not be removed. The move is
// effectively complete — callers should treat this as a warning rather than a
// failure, leaving the old file behind for manual cleanup.
var ErrOldNotRemoved = errors.New("storage moved but old file could not be removed")

// SamePath reports whether two paths refer to the same location after
// resolving to absolute form.
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

// MoveStorage moves the storage file from oldPath to newPath, creating the
// destination directory if needed. It first attempts an atomic rename (valid
// on the same filesystem) and falls back to a copy when the rename fails, for
// example across filesystems. On success the old file no longer exists.
func MoveStorage(oldPath, newPath string) error {
	newDir := filepath.Dir(newPath)
	if err := os.MkdirAll(newDir, 0755); err != nil {
		return fmt.Errorf("creating directory for new path: %w", err)
	}

	// Fast path: atomic rename on the same filesystem.
	if err := os.Rename(oldPath, newPath); err == nil {
		return nil
	}

	// Fallback: copy to the destination, then remove the original.
	if err := copyFile(oldPath, newPath); err != nil {
		return err
	}
	// The data is safely at the destination; failing to remove the original is
	// non-fatal. Signal it distinctly so callers can warn but still proceed.
	if err := os.Remove(oldPath); err != nil {
		return fmt.Errorf("%w: %v", ErrOldNotRemoved, err)
	}
	return nil
}

// copyFile copies src to dst durably: it writes to a temp file in the
// destination directory, fsyncs, restricts permissions to the owner, and
// atomically renames into place.
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
