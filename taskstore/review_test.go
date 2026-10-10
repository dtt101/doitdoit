package taskstore

import (
	"reflect"
	"testing"
)

func chooseAll(r *Review, choice Choice) {
	for i := range r.Differences {
		r.Differences[i].Choice = choice
	}
}

func TestReviewRedundantSubsetAndDifferences(t *testing.T) {
	current := Data{"Future": {{ID: "a", Title: "first"}, {ID: "b", Title: "new"}}}
	for _, alternative := range []Data{{"Future": {{ID: "a", Title: "first"}}}, Clone(current), {}} {
		if !Compare(current, alternative, nil).Redundant() {
			t.Fatal("matching subset should be redundant")
		}
	}
	for _, alternative := range []Data{
		{"Future": {{ID: "a", Title: "first", Completed: true}}},
		{"Future": {{ID: "a", Title: "first", Notes: "notes"}}},
		{"Future": {{ID: "b", Title: "new"}, {ID: "a", Title: "first"}}},
		{"Future": {{ID: "c", Title: "missing"}}},
		{"Future": {{Title: "ambiguous"}}},
	} {
		if Compare(current, alternative, nil).Redundant() {
			t.Fatalf("lost difference: %v", alternative)
		}
	}
}

func TestReviewIndividualChoicesAndMove(t *testing.T) {
	current := Data{"Future": {{ID: "a", Title: "current", Notes: "keep"}, {ID: "b", Title: "delete"}, {ID: "d", Title: "stay"}}}
	alternative := Data{"2030-01-01": {{ID: "a", Title: "moved", DueDate: "2030-01-01"}, {ID: "c", Title: "import"}}, "Future": {{ID: "d", Title: "stay"}}}
	r := Compare(current, alternative, nil)
	if _, err := r.Resolve(); err == nil {
		t.Fatal("unresolved choices saved")
	}
	chooseAll(&r, UseAlternative)
	result, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, alternative) {
		t.Fatalf("move/import/delete: %#v", result)
	}
	if len(current["Future"]) != 3 || current["Future"][0].Title != "current" {
		t.Fatal("mutated comparison input")
	}
	chooseAll(&r, UseCurrent)
	result, err = r.Resolve()
	if err != nil || !reflect.DeepEqual(result, current) {
		t.Fatalf("keep current: %v %v", result, err)
	}
}

func TestReviewThreeWayPreservesUnopposedTasksButRequiresCompetingChoice(t *testing.T) {
	base := Data{"Future": {{ID: "a", Title: "original"}, {ID: "b", Title: "second"}}}
	file, draft := Clone(base), Clone(base)
	file["Future"][0].Notes = "external"
	draft["Future"][0].Notes = "local"
	draft["Future"][1].Title = "local edit"
	file["Future"] = append(file["Future"], Task{ID: "c", Title: "external addition"})
	r := Compare(file, draft, base)
	for i, d := range r.Differences {
		if d.ID == "a" {
			if d.Choice != Undecided {
				t.Fatal("competing edit was selected")
			}
			r.Differences[i].Choice = UseAlternative
		}
	}
	result, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if len(result["Future"]) != 3 || result["Future"][0].Notes != "local" || result["Future"][1].Title != "local edit" || result["Future"][2].ID != "c" {
		t.Fatalf("lost independent edit: %v", result)
	}
	if _, err := Merge(base, draft, file); err == nil {
		t.Fatal("automatic merge policy changed")
	}
}

func TestReviewOrderingAndAmbiguousConnectedLists(t *testing.T) {
	current := Data{"Future": {{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	alternative := Data{"Future": {{ID: "b"}, {ID: "a"}}}
	r := Compare(current, alternative, nil)
	chooseAll(&r, UseCurrent)
	for i, d := range r.Differences {
		if d.Kind == "order" {
			r.Differences[i].Choice = UseAlternative
		}
	}
	result, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if result["Future"][0].ID != "b" || result["Future"][2].ID != "c" {
		t.Fatal(result)
	}
	current = Data{"Future": {{ID: "a"}, {ID: "a"}, {ID: "b"}}}
	alternative = Data{"2030-01-01": {{ID: "a"}, {ID: "b"}}}
	r = Compare(current, alternative, nil)
	if len(r.Differences) != 1 || r.Differences[0].Kind != "lists" || len(r.Differences[0].Buckets) != 2 {
		t.Fatalf("move split across ambiguous lists: %+v", r.Differences)
	}
	chooseAll(&r, UseAlternative)
	result, err = r.Resolve()
	if err != nil || !reflect.DeepEqual(result, alternative) {
		t.Fatalf("fallback: %v %v", result, err)
	}
}

func TestRecoveryKeepsCurrentAndExcludesCompletedWork(t *testing.T) {
	current := Data{"Future": {
		{ID: "done", Title: "Finished", Completed: true},
		{ID: "open", Title: "Current title"},
		{ID: "new", Title: "Current only"},
	}}
	copy := Data{"Future": {
		{ID: "done", Title: "Finished"},
		{ID: "open", Title: "Old title"},
		{ID: "deleted", Title: "Deleted task"},
		{ID: "old-done", Title: "Old completed", Completed: true},
	}}
	r := CompareRecovery(current, copy)
	if len(r.Differences) != 3 {
		t.Fatalf("expected only unfinished task decisions: %+v", r.Differences)
	}
	for _, d := range r.Differences {
		if d.ID == "done" || d.ID == "old-done" || d.Choice != UseCurrent {
			t.Fatalf("offered completed work or defaulted to restoring a copy: %+v", d)
		}
	}
	result, err := r.Resolve()
	if err != nil || !reflect.DeepEqual(result, current) {
		t.Fatalf("default resurrected deleted work: %v %v", result, err)
	}
	chooseAll(&r, UseAlternative)
	result, err = r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	indexed := taskIndex(result)
	if !indexed["done"].Task.Completed || indexed["old-done"] != nil || indexed["deleted"] == nil || indexed["open"].Task.Title != "Old title" {
		t.Fatalf("explicit restore changed completed work or lost selected work: %v", result)
	}
}

func TestRecoverySkipsListAndOrderChoicesContainingCompletedWork(t *testing.T) {
	for _, current := range []Data{
		{"Future": {{ID: "done", Completed: true}, {ID: "open"}}},
		{"Future": {{ID: "done", Completed: true}, {ID: "open"}, {ID: "open"}}},
	} {
		copy := Data{"Future": {{ID: "open"}, {ID: "done"}}}
		r := CompareRecovery(current, copy)
		if len(r.Differences) != 0 {
			t.Fatalf("whole-list decision could change completed work: %+v", r.Differences)
		}
		result, err := r.Resolve()
		if err != nil || !reflect.DeepEqual(result, current) {
			t.Fatalf("excluded decisions changed current tasks: %v %v", result, err)
		}
	}
}
