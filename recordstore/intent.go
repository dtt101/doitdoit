package recordstore

import "fmt"

type located struct {
	task   Task
	bucket string
}

func locations(s Snapshot) map[string]located {
	out := map[string]located{}
	for k, list := range s {
		for _, t := range list {
			out[t.ID] = located{t, k}
		}
	}
	return out
}

// Index shifts caused by inserting/removing/changing a task are not edits to its
// neighbours. Only inversions among otherwise unchanged tasks count as reorder.
func affected(before, after Snapshot) []string {
	a, b := locations(before), locations(after)
	changed := map[string]bool{}
	for id, x := range a {
		if y, ok := b[id]; !ok || x != y {
			changed[id] = true
		}
	}
	for id := range b {
		if _, ok := a[id]; !ok {
			changed[id] = true
		}
	}
	orderChanged := map[string]bool{}
	for k, list := range before {
		pos := map[string]int{}
		for i, t := range after[k] {
			pos[t.ID] = i
		}
		for i, x := range list {
			if changed[x.ID] {
				continue
			}
			for _, y := range list[i+1:] {
				if !changed[y.ID] && pos[x.ID] > pos[y.ID] {
					orderChanged[x.ID] = true
					orderChanged[y.ID] = true
				}
			}
		}
	}
	for id := range orderChanged {
		changed[id] = true
	}
	return keys(changed)
}
func validateChange(n *replayRecord, v View, nodes map[string]*replayRecord) error {
	fail := func() error { return fmt.Errorf("action does not match causal bucket replacements") }
	before := copySnapshot(v.Common)
	after := copySnapshot(before)
	touched := []string{}
	for _, b := range n.Buckets {
		touched = append(touched, b.Bucket)
		setBucket(after, b.Bucket, b.After)
	}
	if validSnapshot(after) != nil {
		return fail()
	}
	if n.Action == "resolve" {
		selected := []Conflict{}
		wantBuckets, wantTips := []string{}, []string{}
		for _, c := range v.Conflicts {
			if intersects(c.Buckets, touched) {
				selected = append(selected, c)
				wantBuckets = union(wantBuckets, c.Buckets)
				wantTips = union(wantTips, c.Alternatives)
			}
		}
		if len(selected) == 0 || !equal(touched, wantBuckets) || !equal(n.Resolves, wantTips) {
			return fail()
		}
		changed := []string{}
		for _, c := range selected {
			for _, candidate := range c.Candidates {
				changed = union(changed, affected(candidate.Snapshot, selectBuckets(after, c.Buckets)))
			}
		}
		if !equal(changed, n.TaskIDs) {
			return fail()
		}
		return nil
	}
	for _, c := range v.Conflicts {
		if intersects(c.Buckets, touched) {
			return fail()
		}
	}
	for _, b := range n.Buckets {
		if !equal(b.Before, array(before, b.Bucket)) || equal(b.Before, b.After) {
			return fail()
		}
	}
	changed := affected(before, after)
	if len(changed) == 0 || !equal(changed, n.TaskIDs) {
		return fail()
	}
	if n.Action == "undo" {
		original := nodes[n.UndoOf]
		if original == nil || !n.ancestors[n.UndoOf] || original.Kind != "change" || original.Action == "resolve" || len(original.Buckets) != len(n.Buckets) {
			return fail()
		}
		for i, b := range n.Buckets {
			o := original.Buckets[i]
			if b.Bucket != o.Bucket || !equal(b.Before, o.After) || !equal(b.After, o.Before) {
				return fail()
			}
		}
		return nil
	}
	a, b := locations(before), locations(after)
	for _, id := range union(keys(a), keys(b)) {
		x, xok := a[id]
		y, yok := b[id]
		if xok && yok && x == y {
			continue
		}
		switch n.Action {
		case "create":
			if xok || !yok || !newTaskID(id) {
				return fail()
			}
		case "delete":
			if !xok || yok {
				return fail()
			}
		case "edit":
			if !xok || !yok || x.bucket != y.bucket || x.task.Title == y.task.Title {
				return fail()
			}
			x.task.Title = y.task.Title
			if x != y {
				return fail()
			}
		case "complete", "reopen":
			if !xok || !yok || x.bucket != y.bucket || x.task.Completed == y.task.Completed || y.task.Completed != (n.Action == "complete") {
				return fail()
			}
			x.task.Completed = y.task.Completed
			if x != y {
				return fail()
			}
		case "move":
			if !xok || !yok || (x.bucket == y.bucket && x.task.DueDate == y.task.DueDate) {
				return fail()
			}
			x.bucket = y.bucket
			x.task.DueDate = y.task.DueDate
			if x != y {
				return fail()
			}
		case "maintenance":
			if !xok {
				return fail()
			}
			if yok {
				x.bucket = y.bucket
				x.task.DueDate = y.task.DueDate
				if x != y {
					return fail()
				}
			}
		case "reorder":
			return fail()
		default:
			return fail()
		}
	}
	// Non-reorder commands preserve the relative order of untouched neighbours.
	if n.Action != "reorder" && n.Action != "maintenance" {
		for _, id := range changed {
			x, xok := a[id]
			y, yok := b[id]
			if xok && yok && x == y {
				return fail()
			}
		}
	}
	return nil
}
func newTaskID(id string) bool {
	return len(id) == 37 && id[:5] == "task:" && hex32.MatchString(id[5:])
}
