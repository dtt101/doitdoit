package recordstore

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
)

type replayFixture struct {
	Raw       map[string]string `json:"raw"`
	Records   map[string]any    `json:"records"`
	Scenarios []struct {
		Name        string         `json:"name"`
		Delivery    []string       `json:"delivery"`
		WithholdRaw []string       `json:"withhold_raw"`
		Expected    map[string]any `json:"expected"`
		Then        *struct {
			Delivery []string       `json:"delivery"`
			Raw      []string       `json:"raw"`
			Expected map[string]any `json:"expected"`
		} `json:"then"`
	} `json:"scenarios"`
}

func loadReplayFixture(t *testing.T) (replayFixture, map[string]Record) {
	t.Helper()
	raw, err := os.ReadFile("../docs/storage/fixtures/scenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	var f replayFixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	expanded := map[string]Record{}
	var expand func(string) Record
	var walk func(any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case string:
			if strings.HasPrefix(x, "$raw:") {
				return digest([]byte(f.Raw[x[5:]]))
			}
			if strings.HasPrefix(x, "$") {
				return expand(x[1:]).ID
			}
			return x
		case []any:
			out := []any{}
			for _, a := range x {
				out = append(out, walk(a))
			}
			return out
		case map[string]any:
			out := map[string]any{}
			for k, a := range x {
				out[k] = walk(a)
			}
			return out
		}
		return v
	}
	expand = func(name string) Record {
		if r, ok := expanded[name]; ok {
			return r
		}
		body := walk(f.Records[name]).(map[string]any)
		for _, k := range []string{"parents", "resolves"} {
			if list, ok := body[k].([]any); ok {
				sort.Slice(list, func(i, j int) bool { return list[i].(string) < list[j].(string) })
			}
		}
		raw, _ := json.Marshal(body)
		r, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		expanded[name] = r
		return r
	}
	for _, name := range keys(f.Records) {
		expand(name)
	}
	return f, expanded
}
func checkExpected(t *testing.T, got any, expected any, records map[string]Record, key string) {
	t.Helper()
	if key == "snapshot" || key == "common" {
		if !equal(got, expected) {
			t.Fatalf("%s: got %v want %v", key, got, expected)
		}
		return
	}
	switch want := expected.(type) {
	case map[string]any:
		actual, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("%s: got %v want object", key, got)
		}
		for k, v := range want {
			checkExpected(t, actual[k], v, records, k)
		}
	case []any:
		actual, ok := got.([]any)
		if !ok || len(actual) != len(want) {
			t.Fatalf("%s: got %v want %v", key, got, want)
		}
		if key == "pending" || key == "alternatives" || key == "heads" {
			ids := []string{}
			for _, v := range want {
				ids = append(ids, records[v.(string)].ID)
			}
			sort.Strings(ids)
			raw, _ := json.Marshal(actual)
			target, _ := json.Marshal(ids)
			if string(raw) != string(target) {
				t.Fatalf("%s: got %s want %s", key, raw, target)
			}
			return
		}
		if key == "candidates" {
			want = append([]any{}, want...)
			sort.Slice(want, func(i, j int) bool {
				return records[want[i].(map[string]any)["tip"].(string)[1:]].ID < records[want[j].(map[string]any)["tip"].(string)[1:]].ID
			})
		}
		for i, v := range want {
			checkExpected(t, actual[i], v, records, key)
		}
	case string:
		if strings.HasPrefix(want, "$") {
			want = records[want[1:]].ID
		}
		if got != want {
			t.Fatalf("%s: got %v want %v", key, got, want)
		}
	default:
		if !equal(got, want) {
			t.Fatalf("%s: got %v want %v", key, got, want)
		}
	}
}
func TestReplaySharedFixtures(t *testing.T) {
	f, records := loadReplayFixture(t)
	for _, scenario := range f.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			backups := map[string][]byte{}
			for name, raw := range f.Raw {
				withhold := false
				for _, n := range scenario.WithholdRaw {
					withhold = withhold || name == n
				}
				if !withhold {
					backups[digest([]byte(raw))] = []byte(raw)
				}
			}
			delivered := []Record{}
			for _, name := range scenario.Delivery {
				delivered = append(delivered, records[name])
			}
			verify := func(expected map[string]any) {
				t.Helper()
				base := Replay(delivered, backups)
				raw, _ := json.Marshal(base)
				var actual any
				_ = json.Unmarshal(raw, &actual)
				checkExpected(t, actual, expected, records, "")
				if _, specified := expected["issues"]; !specified && len(base.Issues) > 0 {
					t.Fatalf("unexpected issues: %+v", base.Issues)
				}
				random := rand.New(rand.NewSource(42))
				for i := 0; i < 12; i++ {
					shuffled := append(append([]Record{}, delivered...), delivered...)
					random.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
					if result := Replay(shuffled, backups); !equal(base, result) {
						t.Fatalf("delivery order changed replay\n%+v\n%+v", base, result)
					}
				}
			}
			verify(scenario.Expected)
			if scenario.Then != nil {
				for _, name := range scenario.Then.Delivery {
					delivered = append(delivered, records[name])
				}
				for _, name := range scenario.Then.Raw {
					raw := f.Raw[name]
					backups[digest([]byte(raw))] = []byte(raw)
				}
				verify(scenario.Then.Expected)
			}
		})
	}
}

func TestSharedValidationAndLegacyNormalization(t *testing.T) {
	raw, err := os.ReadFile("../docs/storage/fixtures/validation.json")
	if err != nil {
		t.Fatal(err)
	}
	var f map[string][]struct {
		Raw       string   `json:"raw"`
		Valid     bool     `json:"valid"`
		Canonical string   `json:"canonical"`
		Snapshot  Snapshot `json:"snapshot"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	for kind, cases := range f {
		for _, tc := range cases {
			t.Run(kind+"/"+tc.Raw, func(t *testing.T) {
				switch kind {
				case "legacy":
					got, err := NormalizeLegacy([]byte(tc.Raw))
					if (err == nil) != tc.Valid || tc.Valid && !equal(got, tc.Snapshot) {
						t.Fatalf("normalize=%v err=%v want=%v", got, err, tc.Snapshot)
					}
				case "canonical":
					got, err := Canonical([]byte(tc.Raw))
					if (err == nil) != tc.Valid || tc.Valid && string(got) != tc.Canonical {
						t.Fatalf("canonical=%s err=%v", got, err)
					}
				case "records":
					_, err := Parse([]byte(tc.Raw))
					if (err == nil) != tc.Valid {
						t.Fatalf("parse=%v", err)
					}
				}
			})
		}
	}
}
func TestReplayRejectsCorruptionAndKeepsDependentsPending(t *testing.T) {
	_, records := loadReplayFixture(t)
	r := records["base"]
	r.Body = append(append([]byte{}, r.Body...), '\n')
	got := Replay([]Record{records["base"], r, records["active"], records["complete"]}, nil)
	if got.Snapshot != nil || len(got.Issues) != 1 || got.Issues[0].ID != r.ID || len(got.Pending) != 2 {
		t.Fatalf("corruption=%+v", got)
	}
}
func TestReplayLimitsAreExplicitAndDuplicatesDoNotUseRecordBudget(t *testing.T) {
	records := []Record{}
	for i := 0; i <= MaxReplayRecords; i++ {
		r, err := Parse(body(i))
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, r)
	}
	got := Replay(records, nil)
	if got.Snapshot != nil || len(got.Issues) != 1 || got.Issues[0].Code != "replay-limit" || len(got.Pending) != len(records) {
		t.Fatalf("limit=%+v", got)
	}
	for i := range records {
		records[i] = records[0]
	}
	if got := Replay(records, nil); len(got.Issues) != 0 || got.UniqueImports != 1 {
		t.Fatalf("duplicates=%+v", got)
	}
	nested := strings.Repeat("[", 256) + "0" + strings.Repeat("]", 256)
	if _, err := Canonical([]byte(nested)); err == nil {
		t.Fatal("nesting limit ignored")
	}
	// Exercise the projection budget without allocating adversarial histories.
	defer func() {
		if _, ok := recover().(replayLimit); !ok {
			t.Fatal("projection budget did not stop replay")
		}
	}()
	p := projector{cache: map[string]View{}, visits: MaxReplayViews}
	p.project(map[string]*replayRecord{})
}

// Compare the complete result, not just selected fixture assertions: canonical
// hashes, common state, every candidate, issues, pending IDs, heads, and counts.
func TestReplayMatchesJavaScript(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is needed for cross-client conformance; both clients also run their own fixture suite")
	}
	f, records := loadReplayFixture(t)
	type envelope struct {
		ID   string `json:"id"`
		Body string `json:"body"`
	}
	type input struct {
		Records []envelope        `json:"records"`
		Backups map[string]string `json:"backups"`
	}
	inputs := []input{}
	expected := []View{}
	for _, scenario := range f.Scenarios {
		delivered := []Record{}
		backup := map[string][]byte{}
		for name, raw := range f.Raw {
			withheld := false
			for _, w := range scenario.WithholdRaw {
				withheld = withheld || w == name
			}
			if !withheld {
				backup[digest([]byte(raw))] = []byte(raw)
			}
		}
		add := func(names []string) {
			for _, name := range names {
				delivered = append(delivered, records[name])
			}
			in := input{Records: []envelope{}, Backups: map[string]string{}}
			for _, r := range delivered {
				in.Records = append(in.Records, envelope{r.ID, string(r.Body)})
			}
			for id, raw := range backup {
				in.Backups[id] = string(raw)
			}
			inputs = append(inputs, in)
			expected = append(expected, Replay(delivered, backup))
		}
		add(scenario.Delivery)
		if scenario.Then != nil {
			for _, name := range scenario.Then.Raw {
				raw := f.Raw[name]
				backup[digest([]byte(raw))] = []byte(raw)
			}
			add(scenario.Then.Delivery)
		}
	}
	encoded, _ := json.Marshal(inputs)
	cmd := exec.Command("node", "testdata/replay.js")
	cmd.Stdin = bytes.NewReader(encoded)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("JavaScript replay: %v %s", err, out)
	}
	var actual []View
	if err := json.Unmarshal(out, &actual); err != nil {
		t.Fatal(err)
	}
	if len(actual) != len(expected) {
		t.Fatal("missing conformance results")
	}
	for i, want := range expected {
		if !equal(actual[i], want) {
			gotJSON, _ := json.Marshal(actual[i])
			wantJSON, _ := json.Marshal(want)
			t.Fatalf("case %d cross-client mismatch\nGo: %s\nJS: %s", i, wantJSON, gotJSON)
		}
	}
}

func TestActivationVerifiesRawHashAndNormalizedContent(t *testing.T) {
	f, records := loadReplayFixture(t)
	view := Replay([]Record{records["base"], records["active"]}, map[string][]byte{digest([]byte(f.Raw["original"])): []byte(f.Raw["reformatted"])})
	if view.Snapshot != nil || len(view.Issues) != 1 || view.Issues[0].ID != records["active"].ID || view.Issues[0].Code != "invalid-backup" {
		t.Fatalf("backup hash mismatch=%+v", view)
	}
}
