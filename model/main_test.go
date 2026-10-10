package model

import (
	"os"
	"testing"
)

// All model tests may now encounter persistent draft recovery. Keep the entire
// suite away from the developer's real configuration and recovery directories.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "doitdoit-model-home-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", dir); err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_CONFIG_HOME", dir); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
