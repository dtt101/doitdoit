package model

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
)

// Keep the entire suite away from the developer's real configuration and recovery directories.
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

func newReloadTestModel(t *testing.T) Model {
	t.Helper()
	m := Model{
		Data:        TodoData{},
		VisibleDays: 3,
		State:       Browsing,
		TextInput:   textinput.New(),
		todayKey:    time.Now().Format(dateLayout),
	}
	m.configureTextInput("New task...")
	m.updateDateKeys()
	return m
}

func dayKey(offset int) string {
	return startOfDay(time.Now()).AddDate(0, 0, offset).Format(dateLayout)
}

func assertTaskOrder(t *testing.T, tasks []Task, want []string) {
	t.Helper()
	if len(tasks) != len(want) {
		t.Fatalf("got %d tasks, want %d: %v", len(tasks), len(want), tasks)
	}
	for i, id := range want {
		if tasks[i].ID != id {
			t.Fatalf("task %d = %q, want %q; all tasks: %v", i, tasks[i].ID, id, tasks)
		}
	}
}

func sameJSON(a, b TodoData) bool {
	aj, errA := json.Marshal(a)
	bj, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(aj, bj)
}
