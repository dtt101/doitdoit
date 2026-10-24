package tasktransfer

import (
	"encoding/json"
	"github.com/dtt101/doitdoit/taskclient"
	"github.com/dtt101/doitdoit/taskstore"
	"strings"
	"testing"
	"time"
)

func TestLegacyPlanPreservesPastDatesNotesAndUnknownCompletionTime(t *testing.T) {
	data := taskstore.Data{"2020-02-29": {{ID: "123", Title: "Keep", Notes: "  ☕\n", CreatedAt: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC), Completed: true}}, "Future": {{ID: "456", Title: "Dated", DueDate: "2030-01-02", CreatedAt: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)}}}
	raw, _ := json.Marshal(data)
	a, err := Build(raw)
	if err != nil || len(a.Report.Issues) > 0 {
		t.Fatalf("plan failed: %v %v", err, a.Report.Issues)
	}
	b, _ := Build(raw)
	if a.Tasks[0].ID != b.Tasks[0].ID {
		t.Fatal("mapping unstable")
	}
	if *a.Tasks[0].ScheduledDate != "2020-02-29" || a.Tasks[0].Notes != "  ☕\n" || a.Tasks[0].CompletedAt != nil {
		t.Fatal("legacy fields invented or changed")
	}
	if *a.Tasks[1].ScheduledDate != "2030-01-02" {
		t.Fatal("Future date lost")
	}
}

func TestPortableImportDryRunMatchesServicePositionAndCompletionValidation(t *testing.T) {
	now := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	base := taskclient.Task{ID: "11111111-1111-4111-8111-111111111111", Title: "Keep", Notes: "  note\n", Status: "open", Position: "a-123z", Revision: 1, CreatedAt: now, UpdatedAt: now}
	cases := []struct {
		name   string
		change func(*taskclient.Task)
		valid  bool
	}{
		{"valid", func(*taskclient.Task) {}, true},
		{"max-position", func(t *taskclient.Task) { t.Position = strings.Repeat("a", 256) }, true},
		{"long-position", func(t *taskclient.Task) { t.Position = strings.Repeat("a", 257) }, false},
		{"uppercase", func(t *taskclient.Task) { t.Position = "A" }, false},
		{"non-ascii", func(t *taskclient.Task) { t.Position = "é" }, false},
		{"punctuation", func(t *taskclient.Task) { t.Position = "a_b" }, false},
		{"open-with-completion", func(t *taskclient.Task) { t.CompletedAt = &now }, false},
		{"completed-with-unknown-time", func(t *taskclient.Task) { t.Status = "completed" }, true},
		{"unsafe-revision", func(t *taskclient.Task) { t.Revision = 9_007_199_254_740_992 }, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			task := base
			test.change(&task)
			raw, _ := json.Marshal(taskclient.Export{SchemaVersion: 1, Board: taskclient.Board{Timezone: "Europe/London", Tasks: []taskclient.Task{task}}})
			plan, err := Build(raw)
			if err != nil || (len(plan.Report.Issues) == 0) != test.valid {
				t.Fatalf("validation mismatch: %v %v", err, plan.Report.Issues)
			}
		})
	}
}
func TestPlanRefusesUnknownDuplicateAndConflictingSourceData(t *testing.T) {
	for _, raw := range []string{`{"Future":[],"Future":[]}`, `{"Future":[{"id":"x","title":"T","secret":1}]}`, `{"Future":[null]}`} {
		if _, err := Build([]byte(raw)); err == nil {
			t.Fatal("ambiguous JSON accepted")
		}
	}
	raw := []byte(`{"2020-02-29":[{"id":"same","title":"One","created_at":"2019-01-01T00:00:00Z","completed":false,"due_date":"2030-01-02"},{"id":"same","title":"Two","created_at":"2019-01-01T00:00:00Z","completed":false}]}`)
	p, err := Build(raw)
	if err != nil || len(p.Report.Issues) < 2 {
		t.Fatal("date/ID conflicts not reported")
	}
}
