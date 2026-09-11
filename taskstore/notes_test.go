package taskstore

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNotesJSONCompatibilityAndLifecycle(t *testing.T) {
	var legacy Task
	if err := json.Unmarshal([]byte(`{"id":"a","title":"old","completed":false}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Notes != "" {
		t.Fatal("legacy task has notes")
	}
	encoded, err := json.Marshal(legacy)
	if err != nil || strings.Contains(string(encoded), `"notes"`) {
		t.Fatalf("empty notes should be omitted: %s %v", encoded, err)
	}
	legacy.Notes = "Unicode café\nhttps://example.com"
	encoded, _ = json.Marshal(legacy)
	var decoded Task
	if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.Notes != legacy.Notes {
		t.Fatalf("round trip failed: %v", err)
	}
	yesterday := time.Now().AddDate(0, 0, -1).Format(DateLayout)
	today := time.Now().Format(DateLayout)
	data := Data{yesterday: {decoded}}
	data.RollOverIncompleteTasks()
	data.Move(today, 0, "Future", "")
	data.Toggle("Future", 0)
	data.Edit("Future", 0, "new title")
	if data["Future"][0].Notes != legacy.Notes {
		t.Fatal("task lifecycle lost notes")
	}
}
