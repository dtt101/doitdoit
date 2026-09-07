package taskstore

import (
	"crypto/sha256"
	"errors"
	"time"
)

var ErrDataConflict = errors.New("task file changed externally; reload before trying again")

// Revision identifies the exact bytes loaded, including absence of the file.
type Revision struct {
	Hash   [sha256.Size]byte
	Exists bool
}

// Snapshot couples parsed data to its revision. Metadata is used by the legacy
// reload policy only; it never replaces the content check before saving.
type Snapshot struct {
	Data     Data
	Revision Revision
	ModTime  time.Time
	Size     int64
}

// Store is the boundary shared by startup, capture, reload, and persistence.
// Nil expected revisions are reserved for the existing unconditional Save API.
// Implementations must not apply rollover, retention, or UI transformations.
type Store interface {
	Load() (Snapshot, error)
	Save(Data, *Revision) (Revision, error)
	Move(destination string) error
}

// LoadStore applies the existing startup maintenance and saves only when dirty.
// The returned revision describes the data read or saved, never a later reread.
func LoadStore(store Store, retentionDays int) (Snapshot, int, error) {
	snapshot, err := store.Load()
	if err != nil {
		return Snapshot{}, 0, err
	}
	data := snapshot.Data
	count := data.CarryForwardCount()
	dirty := data.RollOverIncompleteTasks()
	if retentionDays > 0 && data.PruneOldTasks(retentionDays) {
		dirty = true
	}
	if data.GroupTasksByCompletion() {
		dirty = true
	}
	if dirty {
		revision, err := store.Save(data, &snapshot.Revision)
		if err != nil {
			return Snapshot{}, 0, err
		}
		snapshot.Revision = revision
	}
	return snapshot, count, nil
}
