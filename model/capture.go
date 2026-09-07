package model

import (
	"github.com/dtt101/doitdoit/taskstore"
	"time"
)

func CaptureTask(path, title, when string, retentionDays int) (Task, string, error) {
	return taskstore.CaptureTask(path, title, when, retentionDays)
}
func captureTarget(when string, now time.Time) (string, bool, error) {
	return taskstore.CaptureTarget(when, now)
}
