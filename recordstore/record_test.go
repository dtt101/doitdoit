package recordstore

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestCanonicalAndSharedRecords(t *testing.T) {
	raw, err := os.ReadFile("../docs/storage/fixtures/scenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Raw     map[string]string `json:"raw"`
		Records map[string]any    `json:"records"`
		Vectors []struct {
			Value json.RawMessage `json:"value"`
			ASCII string          `json:"ascii"`
		} `json:"canonical_vectors"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, vector := range fixture.Vectors {
		got, err := Canonical(vector.Value)
		if err != nil || string(got) != vector.ASCII {
			t.Fatalf("canonical=%s want=%s err=%v", got, vector.ASCII, err)
		}
	}
	expanded := map[string]Record{}
	var expand func(string) Record
	var walk func(any) any
	walk = func(value any) any {
		switch v := value.(type) {
		case string:
			if strings.HasPrefix(v, "$raw:") {
				return digest([]byte(fixture.Raw[strings.TrimPrefix(v, "$raw:")]))
			}
			if strings.HasPrefix(v, "$") {
				return expand(v[1:]).ID
			}
			return v
		case []any:
			out := make([]any, len(v))
			for i, item := range v {
				out[i] = walk(item)
			}
			return out
		case map[string]any:
			out := map[string]any{}
			for k, item := range v {
				out[k] = walk(item)
			}
			return out
		default:
			return value
		}
	}
	expand = func(name string) Record {
		if record, ok := expanded[name]; ok {
			return record
		}
		body := walk(fixture.Records[name]).(map[string]any)
		for _, key := range []string{"parents", "resolves"} {
			if list, ok := body[key].([]any); ok {
				sort.Slice(list, func(i, j int) bool { return list[i].(string) < list[j].(string) })
			}
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		record, err := Parse(encoded)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		expanded[name] = record
		return record
	}
	for name := range fixture.Records {
		expand(name)
	}
	if expanded["base"].ID != expanded["same"].ID || expanded["base"].ID == expanded["other"].ID {
		t.Fatal("import identity changed")
	}
}

func TestRejectAmbiguousJSON(t *testing.T) {
	for _, raw := range []string{
		`{"a":1,"a":2}`, `{"a":1,"\u0061":2}`, `"\ud800"`, `"\udc00"`,
		`"\ud800\u0041"`, `1.1`, `9007199254740992`, `9007199254740991.1`,
		`1e1000000000`, `null true`, "\xef\xbb\xbf{}", "\"\xff\"",
	} {
		if _, err := Canonical([]byte(raw)); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	for raw, want := range map[string]string{`-0`: "0", `1e2`: "100", `"\ud83d\ude00"`: `"\ud83d\ude00"`} {
		got, err := Canonical([]byte(raw))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %s %v", raw, got, err)
		}
	}
}

func TestRejectInvalidRecords(t *testing.T) {
	for _, body := range []string{
		`{"kind":"import","schema":2,"snapshot":{}}`,
		`{"kind":"import","schema":1,"snapshot":{},"unknown":1}`,
		`{"kind":"import","schema":1,"snapshot":{"2026-02-30":[]}}`,
		`{"kind":"import","schema":1,"snapshot":{"Future":[]}}`,
		`{"kind":"import","schema":1,"snapshot":null}`,
		`{"kind":"activate","schema":1,"import":"../file","backup":"missing"}`,
		`{"kind":"change","schema":1}`, `null`, `[]`,
	} {
		if _, err := Parse([]byte(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestChangeValidation(t *testing.T) {
	valid := `{"schema":1,"kind":"change","parents":["` + strings.Repeat("1", 64) + `"],"origin":{"device":"` + strings.Repeat("2", 32) + `","nonce":"` + strings.Repeat("3", 32) + `","at":"2026-09-07T23:30:00.000Z","day":"2026-09-08","offset_minutes":60},"action":"edit","task_ids":["a"],"buckets":[{"bucket":"Future","before":[{"id":"a","title":"before","completed":false,"created_at":"2026-09-07T10:00:00Z","due_date":""}],"after":[{"id":"a","title":"after","completed":false,"created_at":"2026-09-07T10:00:00Z","due_date":""}]}]}`
	if _, err := Parse([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"missing parents":                 func(v map[string]any) { v["parents"] = []any{} },
		"duplicate parents":               func(v map[string]any) { v["parents"] = []any{strings.Repeat("1", 64), strings.Repeat("1", 64)} },
		"unknown action":                  func(v map[string]any) { v["action"] = "rename" },
		"unordered task IDs":              func(v map[string]any) { v["task_ids"] = []any{"b", "a"} },
		"null task ID":                    func(v map[string]any) { v["task_ids"] = []any{nil} },
		"unexpected resolution":           func(v map[string]any) { v["resolves"] = v["parents"] },
		"undo without reference":          func(v map[string]any) { v["action"] = "undo" },
		"resolution without alternatives": func(v map[string]any) { v["action"] = "resolve" },
		"origin date mismatch":            func(v map[string]any) { v["origin"].(map[string]any)["day"] = "2026-09-07" },
		"invalid origin offset":           func(v map[string]any) { v["origin"].(map[string]any)["offset_minutes"] = 841 },
		"unknown origin field":            func(v map[string]any) { v["origin"].(map[string]any)["extra"] = true },
		"duplicate buckets":               func(v map[string]any) { b := v["buckets"].([]any)[0]; v["buckets"] = []any{b, b} },
		"missing before":                  func(v map[string]any) { delete(v["buckets"].([]any)[0].(map[string]any), "before") },
		"null after":                      func(v map[string]any) { v["buckets"].([]any)[0].(map[string]any)["after"] = nil },
		"invalid due date":                func(v map[string]any) { changeTask(v)["due_date"] = "2026-02-30" },
		"invalid timestamp":               func(v map[string]any) { changeTask(v)["created_at"] = "yesterday" },
		"invalid title":                   func(v map[string]any) { changeTask(v)["title"] = nil },
		"invalid completion":              func(v map[string]any) { changeTask(v)["completed"] = "false" },
		"duplicate tasks": func(v map[string]any) {
			b := v["buckets"].([]any)[0].(map[string]any)
			a := b["after"].([]any)[0]
			b["after"] = []any{a, a}
		},
	} {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal([]byte(valid), &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			body, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Parse(body); err == nil {
				t.Fatalf("accepted invalid change: %s", body)
			}
		})
	}
}

func changeTask(value map[string]any) map[string]any {
	return value["buckets"].([]any)[0].(map[string]any)["after"].([]any)[0].(map[string]any)
}

func TestRejectOversizedRecordBeforeQueueWrites(t *testing.T) {
	s := newStore(t)
	if _, err := s.Queue([]byte(strings.Repeat(" ", MaxBytes+1))); err == nil {
		t.Fatal("oversized input accepted")
	}
	if _, err := os.Stat(s.Pending); !os.IsNotExist(err) {
		t.Fatal("oversized input touched pending storage")
	}
}
