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

// ErrDestinationExists means the requested destination is already occupied by
// a file, directory, or symlink. MoveStorage never overwrites it.
var ErrDestinationExists = errors.New("storage destination already exists")

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

// MoveStorage moves the storage file from oldPath to newPath, using an atomic
// no-replace hard link on one filesystem and falling back to a no-replace copy
// across filesystems. It returns ErrOldNotRemoved if the data reaches the
// destination but the original cannot be deleted.
func MoveStorage(oldPath, newPath string) error {
	return moveStorage(oldPath, newPath, os.Link)
}

func moveStorage(oldPath, newPath string, link func(string, string) error) error {
	if err := requireMissingDestination(newPath); err != nil {
		return err
	}
	newDir := filepath.Dir(newPath)
	if err := os.MkdirAll(newDir, 0755); err != nil {
		return fmt.Errorf("creating directory for new path: %w", err)
	}

	// A hard link provides same-filesystem move semantics with an atomic
	// create-if-absent destination. Unlike os.Rename, it cannot overwrite.
	if err := link(oldPath, newPath); err == nil {
		if err := os.Remove(oldPath); err != nil {
			return fmt.Errorf("%w: %v", ErrOldNotRemoved, err)
		}
		return nil
	} else if destinationExists(newPath) {
		return fmt.Errorf("%w: %s", ErrDestinationExists, newPath)
	}

	if err := copyFile(oldPath, newPath); err != nil {
		return err
	}
	if err := os.Remove(oldPath); err != nil {
		return fmt.Errorf("%w: %v", ErrOldNotRemoved, err)
	}
	return nil
}

func destinationExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func requireMissingDestination(path string) error {
	_, err := os.Lstat(path)
	if err == nil {
		return fmt.Errorf("%w: %s", ErrDestinationExists, path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking destination: %w", err)
	}
	return nil
}

// copyFile durably copies src to dst via a temp file, fsync, 0600, and an
// atomic hard-link publication that fails if dst appeared concurrently.
func copyFile(src, dst string) error {
	if err := requireMissingDestination(dst); err != nil {
		return err
	}
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
	if err := os.Link(tempPath, dst); err != nil {
		os.Remove(tempPath)
		if destinationExists(dst) {
			return fmt.Errorf("%w: %s", ErrDestinationExists, dst)
		}
		return fmt.Errorf("publishing copied file: %w", err)
	}
	if err := os.Remove(tempPath); err != nil {
		return fmt.Errorf("removing temporary link after copy: %w", err)
	}
	return nil
}
