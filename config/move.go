package config

import "github.com/dtt101/doitdoit/taskstore"

// Preserve the configuration command's public errors and API.
var ErrOldNotRemoved = taskstore.ErrOldNotRemoved
var ErrDestinationExists = taskstore.ErrDestinationExists

func SamePath(a, b string) bool                 { return taskstore.SamePath(a, b) }
func MoveStorage(oldPath, newPath string) error { return taskstore.NewJSON(oldPath).Move(newPath) }
