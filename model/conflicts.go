package model

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/dtt101/doitdoit/taskstore"
)

type conflictReview struct {
	comparison                       taskstore.Review
	file                             taskstore.Snapshot
	source                           *taskstore.ReviewFile
	currentBytes                     []byte
	index, offset                    int
	preview                          taskstore.Data
	previewing, original, discarding bool
	fullPreview, moreHelp            bool
	message                          string
}

func (m *Model) startRecovery(snapshot taskstore.Snapshot) error {
	pending, err := taskstore.ReadRecovery(m.FilePath)
	if err != nil {
		return err
	}
	if pending != nil {
		m.pending = pending
		m.Data = TodoData(taskstore.Clone(pending.Draft))
		m.baseline = taskstore.Clone(pending.Base)
		m.saveErr = taskstore.ErrDataConflict
		m.openConflictReview()
		return nil
	}
	m.scanConflicts(snapshot)
	if len(m.conflictPaths) > 0 {
		m.openConflictReview()
	}
	return nil
}

// Scanning is intentionally nonrecursive. A malformed source is reported and
// retained; it is never interpreted as a redundant empty backup.
func (m *Model) scanConflicts(snapshot taskstore.Snapshot) {
	paths, err := taskstore.ConflictPaths(m.FilePath)
	if err != nil {
		m.conflictNotice = err.Error()
		return
	}
	m.conflictPaths = nil
	var diagnostics []string
	for _, path := range paths {
		source, err := taskstore.ReadReviewFile(path)
		if err != nil {
			diagnostics = append(diagnostics, err.Error())
			continue
		}
		// Use the strict decoder for both sides before deleting or saving.
		current, err := taskstore.ReadReviewFile(m.FilePath)
		if err != nil {
			diagnostics = append(diagnostics, err.Error())
			break
		}
		if current.Snapshot.Revision != snapshot.Revision {
			diagnostics = append(diagnostics, "Task file changed; press r to rescan")
			break
		}
		if taskstore.Compare(current.Snapshot.Data, source.Snapshot.Data, nil).Redundant() {
			if err := taskstore.RemoveReviewedCopy(source, m.dataStore(), snapshot.Revision); err != nil {
				diagnostics = append(diagnostics, "Cleanup failed: "+err.Error())
				m.conflictPaths = append(m.conflictPaths, path)
			}
		} else {
			m.conflictPaths = append(m.conflictPaths, path)
		}
	}
	m.conflictNotice = strings.Join(diagnostics, "; ")
}

func (m *Model) retainDraft() {
	m.recoveryExport = ""
	file, err := m.dataStore().Load()
	if err != nil {
		file = taskstore.Snapshot{Revision: taskstore.Revision{Hash: m.dataHash, Exists: m.dataExists}}
	}
	m.pending = &taskstore.Recovery{StorePath: m.FilePath, Base: taskstore.Clone(m.baseline), Draft: taskstore.Clone(taskstore.Data(m.Data)), File: taskstore.Clone(file.Data), Revision: file.Revision}
	_, m.recoveryErr = taskstore.WriteRecovery(m.FilePath, *m.pending, true)
	// Notes are already copied into Data before persist; pause their editor too.
	m.State = Browsing
}

func (m *Model) openConflictReview() {
	if m.pending != nil {
		old, oldErr := taskstore.RecoveryDirectory(m.pending.StorePath)
		current, currentErr := taskstore.RecoveryDirectory(m.FilePath)
		if oldErr != nil || currentErr != nil || old != current {
			m.Err = fmt.Errorf("pending recovery belongs to a different task file; reopen %s", m.pending.StorePath)
			return
		}
	}
	file, err := m.dataStore().Load()
	if err != nil {
		m.Err = err
		return
	}
	var raw []byte
	if file.Revision.Exists {
		strict, err := taskstore.ReadReviewFile(m.FilePath)
		if err != nil {
			m.Err = err
			return
		}
		if strict.Snapshot.Revision != file.Revision {
			m.Err = taskstore.ErrDataConflict
			return
		}
		raw = strict.Bytes
	}
	r := &conflictReview{file: file, currentBytes: raw}
	if m.pending != nil {
		r.comparison = taskstore.Compare(file.Data, m.pending.Draft, m.pending.Base)
		if m.pending.Revision == file.Revision && len(m.pending.Choices) == len(r.comparison.Differences) {
			for i, choice := range m.pending.Choices {
				d := r.comparison.Differences[i]
				d.Choice = choice.Choice
				if taskstore.SameDifference(d, choice) {
					r.comparison.Differences[i].Choice = choice.Choice
				}
			}
		}
	} else {
		m.scanConflicts(file)
		if len(m.conflictPaths) == 0 {
			if m.conflictNotice == "" {
				m.feedback = "Your tasks are up to date."
			}
			return
		}
		source, err := taskstore.ReadReviewFile(m.conflictPaths[0])
		if err != nil {
			m.Err = err
			return
		}
		r.source = &source
		r.comparison = taskstore.Compare(file.Data, source.Snapshot.Data, nil)
	}
	m.review = r
	m.State = Browsing
	m.ShowHelp = false
}

func (m *Model) reviewRecovery() taskstore.Recovery {
	r := m.review
	recovery := taskstore.Recovery{StorePath: m.FilePath, Base: taskstore.Clone(r.comparison.Base), Draft: taskstore.Clone(r.comparison.Alternative), File: taskstore.Clone(r.file.Data), Revision: r.file.Revision, Source: r.source, CurrentBytes: r.currentBytes, Choices: r.comparison.Differences}
	return recovery
}

func (m *Model) exportReview() {
	path, err := taskstore.WriteRecovery(m.FilePath, m.reviewRecovery(), false)
	if err != nil {
		m.review.message = "Export failed: " + err.Error()
		return
	}
	m.review.message = "Recovery exported: " + path
	if m.pending != nil {
		m.recoveryExport = path
	}
}

func (m *Model) preserveChoices() {
	if m.pending == nil {
		return
	}
	recovery := m.reviewRecovery()
	m.pending = &recovery
	_, m.recoveryErr = taskstore.WriteRecovery(m.FilePath, recovery, true)
}

func (m *Model) previewResolution() {
	r := m.review
	candidate, err := r.comparison.Resolve()
	if err != nil {
		r.message = err.Error()
		return
	}
	// Retain the original lists for the preview; only this candidate undergoes
	// normal lifecycle transformations. Nothing is written here.
	if !sameJSON(TodoData(candidate), TodoData(r.file.Data)) {
		candidate.MigrateDatedFutureTasks()
		candidate.RollOverIncompleteTasks()
		if m.RetentionDays > 0 {
			candidate.PruneOldTasks(m.RetentionDays)
		}
		candidate.GroupTasksByCompletion()
	}
	r.preview = candidate
	r.previewing, r.offset = true, 0
	r.message = ""
	m.preserveChoices()
}

func (m *Model) refreshChangedReview() {
	// Save old decisions as recovery material before replacing their context.
	if _, err := taskstore.WriteRecovery(m.FilePath, m.reviewRecovery(), false); err != nil {
		m.review.message = "Cannot preserve review: " + err.Error()
		return
	}
	source := m.review.source
	m.review = nil
	m.openConflictReview()
	if m.review != nil {
		for i := range m.review.comparison.Differences {
			m.review.comparison.Differences[i].Choice = taskstore.Undecided
		}
		m.review.message = "Your tasks changed again. Review the updated choices before saving."
	} else if source != nil && m.conflictNotice == "" {
		m.conflictNotice = "The tasks available to review changed. Press r to check again."
	}
}

func (m *Model) applyResolution() {
	r := m.review
	if !r.previewing {
		return
	}
	current, err := m.dataStore().Load()
	if err != nil {
		r.message = err.Error()
		return
	}
	if current.Revision != r.file.Revision {
		m.refreshChangedReview()
		return
	}
	if r.source != nil {
		if err := taskstore.VerifyReviewFile(*r.source); err != nil {
			if errors.Is(err, taskstore.ErrDataConflict) || os.IsNotExist(err) {
				m.refreshChangedReview()
			} else {
				r.message = err.Error()
			}
			return
		}
	}
	if _, err := taskstore.WriteRecovery(m.FilePath, m.reviewRecovery(), false); err != nil {
		r.message = "Cannot preserve alternatives: " + err.Error()
		return
	}
	revision := r.file.Revision
	if !sameJSON(TodoData(r.preview), TodoData(current.Data)) {
		revision, err = m.dataStore().Save(r.preview, &r.file.Revision)
		if err != nil {
			if errors.Is(err, taskstore.ErrDataConflict) {
				m.refreshChangedReview()
			} else {
				r.message = "Save failed: " + err.Error()
			}
			return
		}
	}
	// A successful save may have raced with a later external write. Keep the
	// reviewed copy and pending draft unless the saved revision is still current.
	observed, err := m.dataStore().Load()
	if err != nil || observed.Revision != revision {
		r.message = "Your tasks changed again after saving. Your choices are protected; press Esc, then r to review."
		return
	}
	m.Data = TodoData(taskstore.Clone(r.preview))
	m.recordRevision(revision)
	m.clearMoveUndo()
	m.Err, m.saveErr = nil, nil
	m.conflictNotice = ""
	if r.source != nil {
		if err := taskstore.RemoveReviewedCopy(*r.source, m.dataStore(), revision); err != nil {
			m.conflictNotice = "Saved; conflict cleanup needs retry: " + err.Error()
		}
	}
	if m.pending != nil {
		if err := taskstore.ClearRecovery(m.FilePath); err != nil {
			r.file = observed
			r.message = "Saved; recovery cleanup failed. Enter retries cleanup: " + err.Error()
			return
		}
		m.pending, m.recoveryErr = nil, nil
		m.recoveryExport = ""
	}
	m.review = nil
	m.updateDateKeys()
	m.clampRow()
	m.feedback = "Your tasks are up to date."
	// Fresh comparison for each remaining source. Do not overwrite cleanup errors.
	if m.conflictNotice == "" {
		m.openConflictReview()
		if m.review != nil {
			m.review.message = "Saved. Review the remaining tasks."
		}
	}
}

func (m Model) handleReviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	r := m.review
	key := msg.String()
	if r.discarding {
		switch key {
		case "esc", "n":
			r.discarding = false
		case "y":
			latest, err := m.dataStore().Load()
			if err != nil {
				r.message = "Cannot load file version: " + err.Error()
				return m, nil
			}
			if err := taskstore.ClearRecovery(m.FilePath); err != nil {
				m.conflictNotice = "Draft discarded; old recovery could not be removed: " + err.Error()
			}
			m.pending, m.recoveryErr, m.saveErr, m.Err = nil, nil, nil, nil
			m.recoveryExport = ""
			m.Data = TodoData(taskstore.Clone(latest.Data))
			m.recordRevision(latest.Revision)
			m.review = nil
			m.clearMoveUndo()
		}
		return m, nil
	}
	switch key {
	case "q", "ctrl+c":
		m.preserveChoices()
		if m.pending != nil && m.recoveryErr != nil && m.recoveryExport == "" {
			r.message = "Recovery failed; export (x) or explicitly discard (D) before quitting."
			return m, nil
		}
		return m, tea.Quit
	case "esc":
		if r.previewing {
			r.previewing = false
			r.offset = 0
		} else {
			m.preserveChoices()
			m.review = nil
		}
	case "x":
		m.exportReview()
	case "D":
		if m.pending != nil {
			r.discarding = true
		}
	case "?":
		r.moreHelp = !r.moreHelp
	case "v":
		if r.previewing {
			r.fullPreview = !r.fullPreview
			r.offset = 0
		}
	case "b":
		r.original = !r.original
		r.offset = 0
	case "pgdown":
		r.offset += max(1, m.height/2)
	case "pgup":
		r.offset = max(0, r.offset-max(1, m.height/2))
	case "down", "j":
		if r.previewing {
			r.offset++
		} else {
			r.index = min(max(0, len(r.comparison.Differences)-1), r.index+1)
			r.offset = 0
		}
	case "up", "k":
		if r.previewing {
			r.offset = max(0, r.offset-1)
		} else {
			r.index = max(0, r.index-1)
			r.offset = 0
		}
	case "1", "2", "enter":
		if r.previewing {
			if key == "enter" {
				m.applyResolution()
			}
			break
		}
		if len(r.comparison.Differences) > 0 {
			d := &r.comparison.Differences[r.index]
			if key == "1" {
				d.Choice = taskstore.UseCurrent
			} else if key == "2" {
				d.Choice = taskstore.UseAlternative
			} else {
				d.Choice = taskstore.Choice(int(d.Choice)%2 + 1)
			}
			m.preserveChoices()
		}
	case "K":
		if !r.previewing {
			for i := range r.comparison.Differences {
				r.comparison.Differences[i].Choice = taskstore.UseCurrent
			}
			m.preserveChoices()
		}
	case "p":
		m.previewResolution()
	}
	if m.review != nil {
		_, maximum := m.renderConflictPanel()
		m.review.offset = min(m.review.offset, maximum)
	}
	return m, nil
}

func (m Model) pendingKeyAllowed(key string) bool {
	switch key {
	case "q", "ctrl+c", "r", "?", "right", "left", "up", "down", "h", "j", "k", "l", "f", "t", "T", "c", "pgup", "pgdown", "y":
		return true
	}
	return false
}

func (m Model) conflictStatus() string {
	if m.pending != nil {
		if m.recoveryErr != nil {
			return "Changes need review — recovery failed: " + m.recoveryErr.Error() + ". Press r."
		}
		return "Your changes need review before they can be saved. Press r."
	}
	if m.conflictNotice != "" {
		return m.conflictNotice
	}
	if len(m.conflictPaths) > 0 {
		return "Some tasks need your attention. Press r to review."
	}
	return ""
}
