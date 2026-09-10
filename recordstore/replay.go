package recordstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Task and Snapshot preserve wire values, including timestamp spelling and order.
type Task struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
	CreatedAt string `json:"created_at"`
	DueDate   string `json:"due_date"`
}
type Snapshot map[string][]Task
type replacement struct {
	Bucket string `json:"bucket"`
	Before []Task `json:"before"`
	After  []Task `json:"after"`
}
type replayRecord struct {
	Kind      string        `json:"kind"`
	Snapshot  Snapshot      `json:"snapshot"`
	Import    string        `json:"import"`
	Backup    string        `json:"backup"`
	Parents   []string      `json:"parents"`
	Action    string        `json:"action"`
	TaskIDs   []string      `json:"task_ids"`
	Buckets   []replacement `json:"buckets"`
	Resolves  []string      `json:"resolves"`
	UndoOf    string        `json:"undo_of"`
	ancestors map[string]bool
}
type Candidate struct {
	Tip      string   `json:"tip"`
	Snapshot Snapshot `json:"snapshot"`
}
type Conflict struct {
	Buckets      []string    `json:"buckets"`
	Alternatives []string    `json:"alternatives"`
	Common       Snapshot    `json:"common"`
	Candidates   []Candidate `json:"candidates"`
}
type ReplayIssue struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

// View is disposable. Snapshot is nil without activation or with any conflict.
// Common contains the verified common projection plus independent changes.
// Pending and Issues prevent callers from treating this as a complete store.
type View struct {
	Snapshot         Snapshot      `json:"snapshot"`
	Common           Snapshot      `json:"common"`
	Conflicts        []Conflict    `json:"conflicts"`
	Pending          []string      `json:"pending"`
	Issues           []ReplayIssue `json:"issues"`
	Heads            []string      `json:"heads"`
	UniqueImports    int           `json:"unique_imports"`
	CompletionEvents int           `json:"completion_events"`
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}
func equal(a, b any) bool { x, _ := json.Marshal(a); y, _ := json.Marshal(b); return bytes.Equal(x, y) }
func array(s Snapshot, k string) []Task {
	if s[k] == nil {
		return []Task{}
	}
	return s[k]
}
func copySnapshot(s Snapshot) Snapshot {
	out := Snapshot{}
	for k, v := range s {
		out[k] = append([]Task{}, v...)
	}
	return out
}
func setBucket(s Snapshot, k string, v []Task) {
	if len(v) == 0 {
		delete(s, k)
	} else {
		s[k] = append([]Task{}, v...)
	}
}
func selectBuckets(s Snapshot, ks []string) Snapshot {
	if s == nil {
		return nil
	}
	out := Snapshot{}
	for _, k := range ks {
		setBucket(out, k, s[k])
	}
	return out
}
func writes(r *replayRecord) Snapshot {
	if r.Kind == "import" {
		return r.Snapshot
	}
	s := Snapshot{}
	for _, b := range r.Buckets {
		s[b.Bucket] = b.After
	}
	return s
}
func intersects(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
func covers(have, want []string) bool { return len(union(have, want)) == len(have) }
func union(a, b []string) []string {
	m := map[string]bool{}
	for _, k := range append(append([]string{}, a...), b...) {
		m[k] = true
	}
	return keys(m)
}
func maximal(ids []string, nodes map[string]*replayRecord) []string {
	out := []string{}
	for _, id := range ids {
		dominated := false
		for _, other := range ids {
			if nodes[other].ancestors[id] {
				dominated = true
				break
			}
		}
		if !dominated {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// NormalizeLegacy is pure backup verification, not a migration or file writer.
// Identity repair hashes the original parsed object before defaults/repair.
func NormalizeLegacy(raw []byte) (Snapshot, error) {
	canonical, err := Canonical(raw)
	if err != nil {
		return nil, err
	}
	var original map[string][]map[string]json.RawMessage
	if err = json.Unmarshal(canonical, &original); err != nil || original == nil {
		return nil, fmt.Errorf("invalid legacy snapshot")
	}
	counts := map[string]int{}
	for k, list := range original {
		if !bucket(k) || list == nil {
			return nil, fmt.Errorf("invalid legacy bucket")
		}
		for _, t := range list {
			if err := fields(t, "title completed created_at", "id due_date"); err != nil {
				return nil, err
			}
			if id, ok := t["id"]; ok {
				if len(id) == 0 || id[0] != '"' {
					return nil, fmt.Errorf("invalid legacy ID")
				}
				counts[str(id)]++
			}
		}
	}
	result := Snapshot{}
	hash := digest(canonical)
	for _, k := range keys(original) {
		for i, t := range original[k] {
			id := str(t["id"])
			if id == "" || counts[id] > 1 {
				id = fmt.Sprintf("repair:%s:%s:%d", hash, k, i)
			}
			t["id"], _ = json.Marshal(id)
			if _, ok := t["due_date"]; !ok {
				t["due_date"] = json.RawMessage(`""`)
			}
			b, _ := json.Marshal(t)
			var task Task
			if err := json.Unmarshal(b, &task); err != nil {
				return nil, err
			}
			result[k] = append(result[k], task)
			// Reuse standalone task type, timestamp, and required-field validation.
			if err := tasks(append(append([]byte{'['}, b...), ']'), map[string]bool{}, false); err != nil {
				return nil, err
			}
		}
	}
	if err := validSnapshot(result); err != nil {
		return nil, err
	}
	return result, nil
}
func validSnapshot(s Snapshot) error {
	ids := map[string]bool{}
	for _, k := range keys(s) {
		done := false
		for _, t := range s[k] {
			if ids[t.ID] {
				return fmt.Errorf("duplicate task ID")
			}
			ids[t.ID] = true
			if done && !t.Completed {
				return fmt.Errorf("active task after completed task")
			}
			done = done || t.Completed
		}
	}
	return nil
}

// Limits bound this reference implementation before stage 9 performance work.
const MaxReplayRecords = 512
const MaxReplayViews = 4096

type replayLimit struct{}

func limitedView(records []Record) View {
	ids := map[string]bool{}
	for _, r := range records {
		ids[r.ID] = true
	}
	return View{Common: Snapshot{}, Conflicts: []Conflict{}, Pending: keys(ids), Issues: []ReplayIssue{{ID: "", Code: "replay-limit"}}, Heads: []string{}}
}

// Replay rebuilds a view from exact canonical envelopes and raw backup bytes.
// Invalid envelopes are retained by the caller and reported, never repaired here.
// No caches, files, configuration, or runtime adapters are accessed.
func Replay(records []Record, backups map[string][]byte) (result View) {
	// A limit is an explicit incomplete result, never a partial successful replay.
	defer func() {
		if failure := recover(); failure != nil {
			if _, ok := failure.(replayLimit); ok {
				result = limitedView(records)
			} else {
				panic(failure)
			}
		}
	}()
	unique := map[string]bool{}
	for _, r := range records {
		unique[r.ID] = true
	}
	if len(unique) > MaxReplayRecords {
		return limitedView(records)
	}
	p := projector{cache: map[string]View{}}
	nodes := map[string]*replayRecord{}
	bad := map[string]string{}
	for _, envelope := range records {
		r, err := Parse(envelope.Body)
		if err != nil || r.ID != envelope.ID || !bytes.Equal(r.Body, envelope.Body) {
			bad[envelope.ID] = "invalid-record"
			continue
		}
		var n replayRecord
		_ = json.Unmarshal(r.Body, &n)
		n.ancestors = map[string]bool{}
		nodes[r.ID] = &n
	}
	for id := range bad {
		delete(nodes, id)
	}
	accepted := map[string]*replayRecord{}
	pending := map[string]bool{}
	for id := range nodes {
		pending[id] = true
	}
	for {
		progress := false
		for _, id := range keys(pending) {
			n := nodes[id]
			deps := n.Parents
			if n.Kind == "activate" {
				deps = []string{n.Import}
			}
			ready := true
			for _, dep := range deps {
				if accepted[dep] == nil {
					ready = false
				}
			}
			if !ready {
				continue
			}
			n.ancestors = map[string]bool{}
			for _, dep := range deps {
				n.ancestors[dep] = true
				for a := range accepted[dep].ancestors {
					n.ancestors[a] = true
				}
			}
			code := ""
			switch n.Kind {
			case "import":
				if validSnapshot(n.Snapshot) != nil {
					code = "invalid-order"
				}
			case "activate":
				if accepted[n.Import].Kind != "import" {
					code = "invalid-activation"
					break
				}
				raw, ok := backups[n.Backup]
				if !ok {
					continue
				}
				snapshot, err := NormalizeLegacy(raw)
				if digest(raw) != n.Backup || err != nil || !equal(snapshot, accepted[n.Import].Snapshot) {
					code = "invalid-backup"
				}
			case "change":
				closure := subset(accepted, n.ancestors)
				v := p.project(closure)
				if !hasActivation(closure) || !equal(maximal(n.Parents, accepted), n.Parents) {
					code = "invalid-parents"
				} else if err := validateChange(n, v, accepted); err != nil {
					code = "invalid-intent"
				}
			}
			delete(pending, id)
			progress = true
			if code != "" {
				bad[id] = code
			} else {
				accepted[id] = n
			}
		}
		if !progress {
			break
		}
	}
	v := p.project(accepted)
	v.Pending = keys(pending)
	v.Issues = []ReplayIssue{}
	for _, id := range keys(bad) {
		v.Issues = append(v.Issues, ReplayIssue{id, bad[id]})
	}
	v.Heads = maximal(keys(accepted), accepted)
	for _, n := range accepted {
		if n.Kind == "import" {
			v.UniqueImports++
		}
		if n.Action == "complete" {
			v.CompletionEvents++
		}
	}
	// Detach all returned task arrays from the input graph and one another.
	raw, _ := json.Marshal(v)
	var detached View
	_ = json.Unmarshal(raw, &detached)
	return detached
}
func subset(nodes map[string]*replayRecord, ids map[string]bool) map[string]*replayRecord {
	out := map[string]*replayRecord{}
	for id := range ids {
		if n := nodes[id]; n != nil {
			out[id] = n
		}
	}
	return out
}
func hasActivation(nodes map[string]*replayRecord) bool {
	for _, n := range nodes {
		if n.Kind == "activate" {
			return true
		}
	}
	return false
}

// A component connects incompatible concurrent atomic batches. Keep historical
// conflict edges until a validated resolution observes both ends; coincidentally
// equal later values must not silently resolve earlier competing intentions.
type component struct{ buckets, ids []string }
type projector struct {
	cache  map[string]View
	visits int
}

func (p *projector) project(nodes map[string]*replayRecord) View {
	cacheKey := strings.Join(keys(nodes), ",")
	if cached, ok := p.cache[cacheKey]; ok {
		return cached
	}
	p.visits++
	if p.visits > MaxReplayViews {
		panic(replayLimit{})
	}
	v := View{Common: Snapshot{}, Conflicts: []Conflict{}, Pending: []string{}, Issues: []ReplayIssue{}, Heads: []string{}}
	active := map[string]bool{}
	for _, n := range nodes {
		if n.Kind == "activate" {
			active[n.Import] = true
		}
	}
	events := map[string]*replayRecord{}
	for id, n := range nodes {
		if n.Kind == "change" || n.Kind == "import" && active[id] {
			events[id] = n
		}
	}
	ids := keys(events)
	comps := []component{}
	for i, a := range ids {
		for _, b := range ids[i+1:] {
			x, y := events[a], events[b]
			if x.ancestors[b] || y.ancestors[a] {
				continue
			}
			ks := conflictBuckets(x, y)
			if len(ks) == 0 {
				continue
			}
			resolved := false
			for _, r := range events {
				if r.Action == "resolve" && r.ancestors[a] && r.ancestors[b] && covers(keys(writes(r)), ks) {
					resolved = true
					break
				}
			}
			if !resolved {
				comps = append(comps, component{ks, []string{a, b}})
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for i := range comps {
			c := &comps[i]
			for _, id := range ids {
				n := events[id]
				if n.Kind != "change" || !intersects(keys(writes(n)), c.buckets) {
					continue
				}
				desc := false
				for _, tip := range c.ids {
					if n.ancestors[tip] {
						desc = true
					}
				}
				if desc {
					ks := union(c.buckets, keys(writes(n)))
					ns := union(c.ids, []string{id})
					if len(ks) != len(c.buckets) || len(ns) != len(c.ids) {
						c.buckets, c.ids = ks, ns
						changed = true
					}
				}
			}
		}
		for i := 0; i < len(comps); i++ {
			for j := i + 1; j < len(comps); {
				if intersects(comps[i].buckets, comps[j].buckets) {
					comps[i].buckets = union(comps[i].buckets, comps[j].buckets)
					comps[i].ids = union(comps[i].ids, comps[j].ids)
					comps = append(comps[:j], comps[j+1:]...)
					changed = true
				} else {
					j++
				}
			}
		}
	}
	// Outside components all maximal bucket writers agree or commute.
	allBuckets := map[string]bool{}
	for _, n := range events {
		for k := range writes(n) {
			allBuckets[k] = true
		}
	}
	for _, k := range keys(allBuckets) {
		writers := []string{}
		for _, id := range ids {
			if _, ok := writes(events[id])[k]; ok {
				writers = append(writers, id)
			}
		}
		tips := maximal(writers, nodes)
		if len(tips) > 0 {
			setBucket(v.Common, k, writes(events[tips[0]])[k])
		}
	}
	sort.Slice(comps, func(i, j int) bool { return less(comps[i].buckets[0], comps[j].buckets[0]) })
	for _, c := range comps {
		tips := maximal(c.ids, nodes)
		commonIDs := map[string]bool{}
		for a := range nodes[tips[0]].ancestors {
			commonIDs[a] = true
		}
		for _, id := range tips[1:] {
			for a := range commonIDs {
				if !nodes[id].ancestors[a] {
					delete(commonIDs, a)
				}
			}
		}
		commonNodes := subset(nodes, commonIDs)
		// Separate activation wrappers of the same import still share imported state.
		for id, n := range nodes {
			if n.Kind == "activate" && commonIDs[n.Import] {
				commonNodes[id] = n
			}
		}
		common := p.project(commonNodes)
		var baseline Snapshot
		if hasActivation(commonNodes) {
			baseline = selectBuckets(common.Common, c.buckets)
		}
		conflict := Conflict{Buckets: c.buckets, Alternatives: tips, Common: baseline, Candidates: []Candidate{}}
		for _, id := range tips {
			closure := subset(nodes, nodes[id].ancestors)
			closure[id] = nodes[id]
			if nodes[id].Kind == "import" {
				for aid, n := range nodes {
					if n.Kind == "activate" && n.Import == id {
						closure[aid] = n
					}
				}
			}
			candidate := p.project(closure)
			conflict.Candidates = append(conflict.Candidates, Candidate{id, selectBuckets(candidate.Common, c.buckets)})
		}
		for _, k := range c.buckets {
			setBucket(v.Common, k, baseline[k])
		}
		v.Conflicts = append(v.Conflicts, conflict)
	}
	if hasActivation(nodes) && len(v.Conflicts) == 0 {
		v.Snapshot = copySnapshot(v.Common)
	}
	p.cache[cacheKey] = v
	return v
}
func conflictBuckets(a, b *replayRecord) []string {
	x, y := writes(a), writes(b)
	ks := map[string]bool{}
	for k, left := range x {
		if right, ok := y[k]; ok && !equal(left, right) {
			ks[k] = true
		}
	}
	// An identity cannot independently occupy two buckets, even for disjoint writes.
	for k, left := range x {
		for l, right := range y {
			if k == l {
				continue
			}
			for _, t := range left {
				for _, u := range right {
					if t.ID == u.ID {
						ks[k] = true
						ks[l] = true
					}
				}
			}
		}
	}
	if a.Kind == "change" && b.Kind == "change" && intersects(keys(x), keys(y)) && !equal(a.Buckets, b.Buckets) {
		for k := range x {
			ks[k] = true
		}
		for k := range y {
			ks[k] = true
		}
	}
	if len(ks) > 0 {
		if a.Kind == "change" {
			for k := range x {
				ks[k] = true
			}
		}
		if b.Kind == "change" {
			for k := range y {
				ks[k] = true
			}
		}
	}
	return keys(ks)
}
