package config

import (
	"fmt"
	"path/filepath"

	"github.com/dtt101/doitdoit/recordstore"
)

// RecordMigration discovers the inactive migration library from existing config.
// It does not load/save config, choose retention, or run migration. Runtime callers
// are intentionally deferred to stage 6; no user-visible migration toggle exists.
func (c *Config) RecordMigration(localParent string) (recordstore.Migrator, error) {
	if c == nil || c.StoragePath == "" {
		return recordstore.Migrator{}, fmt.Errorf("storage path is not configured")
	}
	path, err := ExpandPath(c.StoragePath)
	if err != nil {
		return recordstore.Migrator{}, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return recordstore.Migrator{}, err
	}
	return recordstore.NewMigrator(path, localParent)
}
