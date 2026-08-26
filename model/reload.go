package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The data file can also be written by the web app, so the TUI polls it and
// reloads external changes in the background. Polling mtime+size is preferred
// over inotify-style watching because saves (ours and Dropbox's) are atomic
// renames, which replace the inode a watcher would hold.
const reloadInterval = 5 * time.Second

type reloadTickMsg time.Time

// dataFileCheckedMsg carries the result of a background file check. A nil
// data with a nil err means the file was unchanged.
type dataFileCheckedMsg struct {
	data    TodoData
	modTime time.Time
	size    int64
	hash    [sha256.Size]byte
	hashed  bool
	err     error
}

// reloadTick schedules the next background check of the data file.
func reloadTick() tea.Cmd {
	return tea.Tick(reloadInterval, func(t time.Time) tea.Msg {
		return reloadTickMsg(t)
	})
}

// checkDataFile hashes and parses the data file off the update loop. Content
// hashing catches sync tools that preserve both modification time and size.
func checkDataFile(path string, lastHash [sha256.Size]byte, lastExists bool) tea.Cmd {
	return func() tea.Msg {
		contents, err := os.ReadFile(path)
		if err != nil {
			// Missing or unreadable (e.g. mid-sync); try again next tick.
			return dataFileCheckedMsg{}
		}
		fi, err := os.Stat(path)
		if err != nil {
			return dataFileCheckedMsg{}
		}
		hash := sha256.Sum256(contents)
		if lastExists && hash == lastHash {
			return dataFileCheckedMsg{}
		}
		var data TodoData
		err = json.Unmarshal(contents, &data)
		return dataFileCheckedMsg{data: data, modTime: fi.ModTime(), size: fi.Size(), hash: hash, hashed: true, err: err}
	}
}

func (m Model) handleReloadTick() (tea.Model, tea.Cmd) {
	// Never reload under the user's feet: skip while typing a task or date,
	// or while a move is pending.
	if m.State != Browsing {
		return m, reloadTick()
	}
	return m, checkDataFile(m.FilePath, m.dataHash, m.dataExists)
}

func (m Model) handleDataFileChecked(msg dataFileCheckedMsg) (tea.Model, tea.Cmd) {
	// Unchanged, stale (we persisted while the check was in flight), or a
	// transient read/parse failure: keep current state and check again later.
	if msg.data == nil || msg.err != nil || !msg.modTime.After(m.dataModTime) {
		return m, reloadTick()
	}

	m.dataModTime = msg.modTime
	m.dataSize = msg.size
	if msg.hashed {
		m.dataHash = msg.hash
		m.dataExists = true
	}

	if !sameJSON(m.Data, msg.data) {
		m.applyReloadedData(msg.data)
	}
	return m, reloadTick()
}

// applyReloadedData swaps in externally-changed data while keeping the cursor
// on the same task where possible. Rollover/prune/distribution run in memory
// only — persisting here would bump the file's mtime and ping-pong writes
// with the web app; changes reach disk on the user's next edit.
func (m *Model) applyReloadedData(data TodoData) {
	focusedKey := m.getCurrentKey()
	focusedID := ""
	if tasks := m.Data[focusedKey]; m.RowIdx >= 0 && m.RowIdx < len(tasks) {
		focusedID = tasks[m.RowIdx].ID
	}

	m.Data = data
	m.Data.rollOverIncompleteTasks()
	m.Data.pruneOldTasks(m.RetentionDays)
	m.Data.distributeFutureTasksThrough(m.lastVisibleDate())
	m.clearMoveUndo()

	if focusedID != "" {
		for i, task := range m.Data[focusedKey] {
			if task.ID == focusedID {
				m.RowIdx = i
				break
			}
		}
	}
	m.clampRow()
}

// sameJSON reports whether two data sets marshal identically. json.Marshal
// sorts map keys, so this is a cheap deterministic deep-equal — the same
// diff gate the web app uses before re-rendering.
func sameJSON(a, b TodoData) bool {
	aj, errA := json.Marshal(a)
	bj, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(aj, bj)
}
