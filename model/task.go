package model

import (
	"crypto/sha256"
	"time"

	"github.com/dtt101/doitdoit/taskstore"
)

const dateLayout = taskstore.DateLayout

var ErrDataConflict = taskstore.ErrDataConflict

type Task = taskstore.Task

// TodoData retains the model API while task behavior lives independently of Bubble Tea.
type TodoData taskstore.Data

func startOfDay(t time.Time) time.Time      { return taskstore.StartOfDay(t) }
func parseDate(s string) (time.Time, error) { return taskstore.ParseDate(s) }
func loadRaw(path string) (TodoData, error) {
	data, _, _, err := loadRawState(path)
	return data, err
}
func loadRawState(path string) (TodoData, [sha256.Size]byte, bool, error) {
	snapshot, err := taskstore.NewJSON(path).Load()
	return TodoData(snapshot.Data), snapshot.Revision.Hash, snapshot.Revision.Exists, err
}
func Load(path string, retentionDays int) (TodoData, error) {
	data, _, err := loadWithRollover(path, retentionDays)
	return data, err
}
func loadWithRollover(path string, retentionDays int) (TodoData, int, error) {
	snapshot, count, err := taskstore.LoadStore(taskstore.NewJSON(path), retentionDays)
	return TodoData(snapshot.Data), count, err
}
func (d TodoData) carryForwardCount() int         { return taskstore.Data(d).CarryForwardCount() }
func (d TodoData) groupTasksByCompletion() bool   { return taskstore.Data(d).GroupTasksByCompletion() }
func (d TodoData) rollOverIncompleteTasks() bool  { return taskstore.Data(d).RollOverIncompleteTasks() }
func (d TodoData) pruneOldTasks(days int) bool    { return taskstore.Data(d).PruneOldTasks(days) }
func (d TodoData) DistributeFutureTasks(days int) { taskstore.Data(d).DistributeFutureTasks(days) }
func (d TodoData) distributeFutureTasksThrough(date time.Time) bool {
	return taskstore.Data(d).DistributeFutureTasksThrough(date)
}
func (d TodoData) SortedKeys() []string { return taskstore.Data(d).SortedKeys() }
func (d TodoData) Save(path string) error {
	_, err := taskstore.NewJSON(path).Save(taskstore.Data(d), nil)
	return err
}
func (d TodoData) SaveIfUnchanged(path string, hash *[sha256.Size]byte, exists bool) error {
	var expected *taskstore.Revision
	if hash != nil {
		expected = &taskstore.Revision{Hash: *hash, Exists: exists}
	}
	_, err := taskstore.NewJSON(path).Save(taskstore.Data(d), expected)
	return err
}
