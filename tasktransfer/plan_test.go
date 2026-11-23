package tasktransfer

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dtt101/doitdoit/taskclient"
)

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

func TestPlanRejectsLegacyUnknownDuplicateAndConflictingSourceData(t *testing.T) {
	for _, raw := range []string{
		`{"Future":[]}`,
		`{"schema_version":1,"schema_version":1,"timezone":"Europe/London","tasks":[]}`,
		`{"schema_version":1,"timezone":"Europe/London","tasks":[],"secret":1}`,
		`{"schema_version":2,"timezone":"Europe/London","tasks":[]}`,
	} {
		if _, err := Build([]byte(raw)); err == nil {
			t.Fatalf("expected error for %s", raw)
		}
	}
	now := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	dup := taskclient.Task{ID: "11111111-1111-4111-8111-111111111111", Title: "One", Status: "open", Position: "a", Revision: 1, CreatedAt: now, UpdatedAt: now}
	raw, _ := json.Marshal(taskclient.Export{SchemaVersion: 1, Board: taskclient.Board{Timezone: "Europe/London", Tasks: []taskclient.Task{dup, dup}}})
	p, err := Build(raw)
	if err != nil || len(p.Report.Issues) == 0 {
		t.Fatalf("duplicate task ID not reported: %v %v", err, p.Report.Issues)
	}
}

func TestDecodeStrictRejectsCaseFoldedDuplicateKeysAndInvalidTimezones(t *testing.T) {
	if _, err := Build([]byte(`{"schema_version":1,"timezone":"Europe/London","Timezone":"UTC","tasks":[]}`)); err == nil {
		t.Fatal("case-folded duplicate key accepted")
	}
	for _, tz := range []string{"", "Local"} {
		raw, _ := json.Marshal(taskclient.Export{SchemaVersion: 1, Board: taskclient.Board{Timezone: tz, Tasks: []taskclient.Task{}}})
		p, err := Build(raw)
		if err != nil || len(p.Report.Issues) == 0 {
			t.Fatalf("expected timezone issue for %q: err=%v issues=%v", tz, err, p.Report.Issues)
		}
	}
}

func TestVerifyTreatsEquivalentTimestampsWithDifferentOffsetsAsEqual(t *testing.T) {
	plusTwo := time.FixedZone("UTC+2", 2*3600)
	t1 := time.Date(2026, 1, 2, 12, 0, 0, 0, plusTwo)
	t2 := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	expected := []taskclient.Task{{ID: "1", Title: "A", Status: "open", Position: "a", Revision: 1, CreatedAt: t1, UpdatedAt: t1}}
	actual := []taskclient.Task{{ID: "1", Title: "A", Status: "open", Position: "a", Revision: 1, CreatedAt: t2, UpdatedAt: t2}}
	if err := Verify(expected, actual); err != nil {
		t.Fatalf("equivalent instants rejected: %v", err)
	}
}
