package taskstore

import (
	"fmt"
	"reflect"
	"sort"
)

// Choice always selects a complete task (including its location), or a complete
// connected group of lists when identity is ambiguous. It never merges fields.
type Choice int

const (
	Undecided Choice = iota
	UseCurrent
	UseAlternative
)

type LocatedTask struct {
	Bucket string
	Task   Task
}

type Difference struct {
	Kind                           string // add, change, remove, order, lists
	ID                             string
	Buckets                        []string
	Current, Alternative, Original *LocatedTask
	Choice                         Choice
}

type Review struct {
	Current, Alternative, Base Data
	Differences                []Difference
}

func taskIndex(data Data) map[string]*LocatedTask {
	r := make(map[string]*LocatedTask)
	for b, tasks := range data {
		for _, t := range tasks {
			r[t.ID] = &LocatedTask{b, t}
		}
	}
	return r
}

func sameTask(a, b *LocatedTask) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Bucket == b.Bucket && equalTask(a.Task, b.Task)
}

func equalTask(a, b Task) bool {
	return a.ID == b.ID && a.Title == b.Title && a.Notes == b.Notes && a.Completed == b.Completed && a.DueDate == b.DueDate && a.CreatedAt.Equal(b.CreatedAt)
}

// SameDifference ignores the choice and time.Time's process-local internals, so
// recovery choices survive a JSON round trip only for identical alternatives.
func SameDifference(a, b Difference) bool {
	return a.Kind == b.Kind && a.ID == b.ID && reflect.DeepEqual(a.Buckets, b.Buckets) && sameTask(a.Current, b.Current) && sameTask(a.Alternative, b.Alternative) && sameTask(a.Original, b.Original)
}

// Compare accepts a nil baseline for an independently discovered conflict copy.
// With a baseline, unopposed changes are proposed, but resolution still requires
// the caller's explicit preview/save action. Merge's policy is unchanged.
func Compare(current, alternative, base Data) Review {
	r := Review{Current: Clone(current), Alternative: Clone(alternative), Base: Clone(base)}
	a, b, original := taskIndex(current), taskIndex(alternative), taskIndex(base)
	// Connect buckets through IDs across all versions before falling back. This
	// prevents a list choice from severing a move or duplicating its destination.
	links := map[string]map[string]bool{}
	ambiguous := map[string]bool{}
	locations := map[string][]string{}
	for _, data := range []Data{current, alternative, base} {
		counts := map[string]int{}
		for bucket, tasks := range data {
			if links[bucket] == nil {
				links[bucket] = map[string]bool{}
			}
			for _, task := range tasks {
				counts[task.ID]++
				locations[task.ID] = append(locations[task.ID], bucket)
				if task.ID == "" {
					ambiguous[bucket] = true
				}
			}
		}
		for id, count := range counts {
			if count > 1 {
				for _, bucket := range locations[id] {
					ambiguous[bucket] = true
				}
			}
		}
	}
	for id, buckets := range locations {
		if id == "" {
			continue
		}
		for _, x := range buckets {
			for _, y := range buckets {
				links[x][y] = true
			}
		}
	}
	fallback := map[string]bool{}
	for _, start := range sortedKeys(links) {
		if fallback[start] || !ambiguous[start] {
			continue
		}
		group := map[string]bool{}
		queue := []string{start}
		for len(queue) > 0 {
			x := queue[0]
			queue = queue[1:]
			if group[x] {
				continue
			}
			group[x] = true
			fallback[x] = true
			for y := range links[x] {
				queue = append(queue, y)
			}
		}
		buckets := sortedKeys(group)
		if !sameLists(current, alternative, buckets) {
			r.Differences = append(r.Differences, Difference{Kind: "lists", Buckets: buckets})
		}
	}
	ids := map[string]bool{}
	for id := range a {
		ids[id] = true
	}
	for id := range b {
		ids[id] = true
	}
	for _, id := range sortedKeys(ids) {
		x, y := a[id], b[id]
		if (x != nil && fallback[x.Bucket]) || (y != nil && fallback[y.Bucket]) || sameTask(x, y) {
			continue
		}
		d := Difference{Kind: "change", ID: id, Current: x, Alternative: y, Original: original[id]}
		if x == nil {
			d.Kind = "add"
		}
		if y == nil {
			d.Kind = "remove"
			d.Choice = UseCurrent
		}
		if base != nil {
			if sameTask(y, original[id]) {
				d.Choice = UseCurrent
			} else if sameTask(x, original[id]) {
				d.Choice = UseAlternative
			}
			if !sameTask(x, original[id]) && !sameTask(y, original[id]) {
				d.Choice = Undecided
			}
		}
		r.Differences = append(r.Differences, d)
	}
	for _, bucket := range sortedKeys(links) {
		if fallback[bucket] || sameSharedOrder(current[bucket], alternative[bucket]) {
			continue
		}
		d := Difference{Kind: "order", Buckets: []string{bucket}}
		if base != nil {
			if sameSharedOrder(alternative[bucket], base[bucket]) {
				d.Choice = UseCurrent
			} else if sameSharedOrder(current[bucket], base[bucket]) {
				d.Choice = UseAlternative
			}
		}
		r.Differences = append(r.Differences, d)
	}
	rank := map[string]int{"add": 0, "change": 1, "remove": 2, "order": 3, "lists": 4}
	sort.SliceStable(r.Differences, func(i, j int) bool { return rank[r.Differences[i].Kind] < rank[r.Differences[j].Kind] })
	return r
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sameLists(a, b Data, buckets []string) bool {
	for _, bucket := range buckets {
		if len(a[bucket]) != len(b[bucket]) {
			return false
		}
		for i := range a[bucket] {
			if !equalTask(a[bucket][i], b[bucket][i]) {
				return false
			}
		}
	}
	return true
}

func sameSharedOrder(a, b []Task) bool {
	ids := map[string]int{}
	for _, t := range a {
		ids[t.ID] = 1
	}
	for _, t := range b {
		if ids[t.ID] == 1 {
			ids[t.ID] = 2
		}
	}
	x, y := []string{}, []string{}
	for _, t := range a {
		if ids[t.ID] == 2 {
			x = append(x, t.ID)
		}
	}
	for _, t := range b {
		if ids[t.ID] == 2 {
			y = append(y, t.ID)
		}
	}
	return reflect.DeepEqual(x, y)
}

// Redundant is deliberately asymmetric: new tasks in the current file are not
// evidence of deletions in an older copy. Ambiguous identity is never auto-cleaned.
func (r Review) Redundant() bool {
	for _, data := range []Data{r.Current, r.Alternative} {
		seen := map[string]bool{}
		for _, tasks := range data {
			for _, t := range tasks {
				if t.ID == "" || seen[t.ID] {
					return false
				}
				seen[t.ID] = true
			}
		}
	}
	for _, d := range r.Differences {
		if d.Kind != "remove" {
			return false
		}
	}
	return true
}

func (r Review) Resolve() (Data, error) {
	result := Clone(r.Current)
	selected := map[string]*LocatedTask{}
	changed := false
	for _, d := range r.Differences {
		if d.Choice != UseCurrent && d.Choice != UseAlternative {
			return nil, fmt.Errorf("choose an action for every difference")
		}
		if d.Choice != UseAlternative {
			continue
		}
		changed = true
		switch d.Kind {
		case "lists":
			for _, bucket := range d.Buckets {
				delete(result, bucket)
				if tasks, ok := r.Alternative[bucket]; ok {
					result[bucket] = append([]Task(nil), tasks...)
				}
			}
		case "add", "change", "remove":
			selected[d.ID] = d.Alternative
		}
	}
	if !changed {
		return result, nil
	}
	// Replace in place when the location is unchanged; append imports/moves in
	// alternative order, not map iteration order.
	for bucket, tasks := range result {
		out := make([]Task, 0, len(tasks))
		for _, t := range tasks {
			if v, ok := selected[t.ID]; ok {
				if v != nil && v.Bucket == bucket {
					out = append(out, v.Task)
					delete(selected, t.ID)
				}
			} else {
				out = append(out, t)
			}
		}
		result[bucket] = out
	}
	for _, bucket := range sortedKeys(r.Alternative) {
		for _, t := range r.Alternative[bucket] {
			if v, ok := selected[t.ID]; ok && v != nil {
				result[bucket] = append(result[bucket], t)
				delete(selected, t.ID)
			}
		}
	}
	for _, d := range r.Differences {
		if d.Kind != "order" || d.Choice != UseAlternative {
			continue
		}
		bucket := d.Buckets[0]
		tasks := result[bucket]
		ordered := make([]Task, 0, len(tasks))
		used := map[string]bool{}
		for _, t := range r.Alternative[bucket] {
			for _, v := range tasks {
				if t.ID == v.ID {
					ordered = append(ordered, v)
					used[v.ID] = true
				}
			}
		}
		for _, t := range tasks {
			if !used[t.ID] {
				ordered = append(ordered, t)
			}
		}
		result[bucket] = ordered
	}
	// Existing ambiguous lists can be retained, but importing a malformed copy
	// must not introduce or increase duplicate stable IDs in the current file.
	counts := func(data Data) map[string]int {
		c := map[string]int{}
		for _, tasks := range data {
			for _, t := range tasks {
				c[t.ID]++
			}
		}
		return c
	}
	a := counts(r.Current)
	for id, n := range counts(result) {
		if id != "" && n > max(1, a[id]) {
			return nil, fmt.Errorf("resolution duplicates task ID %q", id)
		}
	}
	result.GroupTasksByCompletion()
	return result, nil
}
