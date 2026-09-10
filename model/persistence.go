package model

import (
	"errors"
	"time"

	"github.com/dtt101/doitdoit/taskstore"
)

// dataStore also supports existing callers that construct a Model literal or
// change its public FilePath. Only reuse an adapter for the path it was bound to.
func (m Model) dataStore() taskstore.Store {
	if m.store != nil && m.FilePath == m.storePath {
		return m.store
	}
	return taskstore.NewJSON(m.FilePath)
}

func (m *Model) trackFileState() {
	snapshot, err := m.dataStore().Load()
	if err != nil {
		return
	}
	m.dataModTime, m.dataSize = snapshot.ModTime, snapshot.Size
	m.dataHash, m.dataExists = snapshot.Revision.Hash, snapshot.Revision.Exists
}

// Keep the revision returned by the operation, not an unrelated later read.
// Only use a follow-up observation for the legacy reload metadata.
func (m *Model) recordRevision(revision taskstore.Revision) {
	m.dataHash, m.dataExists = revision.Hash, revision.Exists
	m.dataModTime, m.dataSize = time.Time{}, 0
	if snapshot, err := m.dataStore().Load(); err == nil && snapshot.Revision == revision {
		m.dataModTime, m.dataSize = snapshot.ModTime, snapshot.Size
	}
}

func (m *Model) persist() {
	revision, err := m.dataStore().Save(taskstore.Data(m.Data), &taskstore.Revision{Hash: m.dataHash, Exists: m.dataExists})
	if errors.Is(err, ErrDataConflict) && m.saveErr == nil {
		// Once a save fails, undo may contain unsaved edits rather than the
		// loaded baseline. It is no longer safe to use it for a merge retry.
		err = m.retryPersistOnFreshData()
	} else if err == nil {
		m.recordRevision(revision)
	}
	m.Err = err
	m.saveErr = err
}

// Retry one conservative three-way merge. Competing edits in the same bucket
// remain visible conflicts; undo keeps the external changes that were merged.
func (m *Model) retryPersistOnFreshData() error {
	if m.moveUndo == nil {
		return ErrDataConflict
	}
	snapshot, err := m.dataStore().Load()
	if err != nil {
		return err
	}
	if !snapshot.Revision.Exists {
		return ErrDataConflict
	}
	remote := TodoData(snapshot.Data)
	merged, err := mergeTodoData(m.moveUndo.Data, m.Data, remote)
	if err != nil {
		return err
	}
	revision, err := m.dataStore().Save(taskstore.Data(merged), &snapshot.Revision)
	if err != nil {
		return err
	}
	m.moveUndo.Data = cloneTodoData(remote)
	m.Data = merged
	m.recordRevision(revision)
	return nil
}

func mergeTodoData(base, local, remote TodoData) (TodoData, error) {
	merged, err := taskstore.Merge(taskstore.Data(base), taskstore.Data(local), taskstore.Data(remote))
	return TodoData(merged), err
}
