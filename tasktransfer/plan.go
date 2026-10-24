// Package tasktransfer plans explicit imports without loading user configuration,
// rolling dates forward, pruning history or modifying the source.
package tasktransfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/dtt101/doitdoit/taskclient"
	"github.com/dtt101/doitdoit/taskstore"
)

type Mapping struct {
	Source string `json:"source"`
	OldID  string `json:"old_id"`
	ID     string `json:"id"`
}
type Report struct {
	SourceSHA256   string    `json:"source_sha256"`
	Format         string    `json:"format"`
	TaskCount      int       `json:"task_count"`
	CompletedCount int       `json:"completed_count"`
	Mappings       []Mapping `json:"mappings"`
	Issues         []string  `json:"issues"`
	RecoveryPaths  []string  `json:"recovery_paths,omitempty"`
}
type Plan struct {
	Report   Report            `json:"report"`
	Tasks    []taskclient.Task `json:"tasks"`
	Timezone string            `json:"timezone,omitempty"`
}

var uuid = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validDate(s string) bool {
	d, e := time.Parse("2006-01-02", s)
	return e == nil && d.Format("2006-01-02") == s
}
func stableID(seed string) string {
	b := sha256.Sum256([]byte(seed))
	b[6] = b[6]&15 | 80
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func fingerprint(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

// DecodeStrict rejects duplicate keys and unknown typed fields before migration.
func DecodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 64 {
			return fmt.Errorf("JSON nesting exceeds 64 levels")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		seen := map[string]bool{}
		for decoder.More() {
			if delimiter == '{' {
				key, e := decoder.Token()
				if e != nil {
					return e
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return fmt.Errorf("duplicate JSON key")
				}
				seen[name] = true
			}
			if err := walk(depth + 1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
func Build(data []byte) (Plan, error) {
	p := Plan{Report: Report{SourceSHA256: fingerprint(data), Mappings: []Mapping{}, Issues: []string{}}, Tasks: []taskclient.Task{}}
	if len(data) > 64<<20 {
		return p, fmt.Errorf("source exceeds 64 MiB")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return p, fmt.Errorf("source must be a JSON object")
	}
	if _, ok := root["schema_version"]; ok {
		var exported taskclient.Export
		if err := DecodeStrict(data, &exported); err != nil {
			return p, err
		}
		if exported.SchemaVersion != 1 {
			return p, fmt.Errorf("unsupported export version")
		}
		p.Report.Format = "service-export-v1"
		p.Timezone = exported.Timezone
		p.Tasks = exported.Tasks
		if _, err := time.LoadLocation(p.Timezone); err != nil {
			p.Report.Issues = append(p.Report.Issues, "Export has an unknown owner timezone")
		}
	} else {
		source, err := taskstore.DecodeReviewData(data)
		if err != nil {
			return p, err
		}
		p.Report.Format = "legacy-json"
		ids := map[string]bool{}
		buckets := source.SortedKeys()
		positions := map[string]int{}
		for _, bucket := range buckets {
			if bucket != "Future" && !validDate(bucket) {
				p.Report.Issues = append(p.Report.Issues, fmt.Sprintf("Invalid date bucket %q", bucket))
				continue
			}
			for index, old := range source[bucket] {
				location := fmt.Sprintf("%s[%d]", bucket, index)
				normalized := strings.ToLower(old.ID)
				if old.ID != "" && ids[normalized] {
					p.Report.Issues = append(p.Report.Issues, "Duplicate legacy ID at "+location)
				}
				ids[normalized] = true
				id := old.ID
				if !uuid.MatchString(id) {
					id = stableID(p.Report.SourceSHA256 + "/" + location + "/" + old.ID)
				}
				if id != old.ID {
					p.Report.Mappings = append(p.Report.Mappings, Mapping{location, old.ID, id})
				}
				var date *string
				if bucket != "Future" {
					v := bucket
					date = &v
					if old.DueDate != "" && old.DueDate != bucket {
						p.Report.Issues = append(p.Report.Issues, "Bucket/due_date disagreement at "+location)
					}
				} else if old.DueDate != "" {
					if !validDate(old.DueDate) {
						p.Report.Issues = append(p.Report.Issues, "Invalid Future due_date at "+location)
					} else {
						v := old.DueDate
						date = &v
					}
				}
				status := "open"
				if old.Completed {
					status = "completed"
				}
				group := status + "/"
				if date != nil {
					group += *date
				}
				positions[group]++
				if old.CreatedAt.IsZero() {
					p.Report.Issues = append(p.Report.Issues, "Missing/unknown creation time at "+location)
				}
				p.Tasks = append(p.Tasks, taskclient.Task{ID: id, Title: old.Title, Notes: old.Notes, Status: status, ScheduledDate: date, Position: fmt.Sprintf("%016xi", uint64(positions[group])*1048576), Revision: 1, CreatedAt: old.CreatedAt, UpdatedAt: old.CreatedAt})
			}
		}
	}
	seen := map[string]bool{}
	for i, t := range p.Tasks {
		if !uuid.MatchString(t.ID) || seen[strings.ToLower(t.ID)] {
			p.Report.Issues = append(p.Report.Issues, fmt.Sprintf("Invalid/duplicate task ID at task %d", i))
		}
		seen[strings.ToLower(t.ID)] = true
		if strings.TrimSpace(t.Title) == "" || len(utf16.Encode([]rune(t.Title))) > 500 || len(utf16.Encode([]rune(t.Notes))) > 32000 || (t.Status != "open" && t.Status != "completed") || t.Revision < 1 || t.Revision > 9_007_199_254_740_991 || t.CreatedAt.IsZero() || t.UpdatedAt.IsZero() || !validPosition(t.Position) || (t.Status == "open" && t.CompletedAt != nil) || (t.ScheduledDate != nil && !validDate(*t.ScheduledDate)) {
			p.Report.Issues = append(p.Report.Issues, fmt.Sprintf("Invalid task fields at task %d", i))
		}
		if t.Status == "completed" {
			p.Report.CompletedCount++
		}
	}
	p.Report.TaskCount = len(p.Tasks)
	if len(p.Tasks) > 10000 {
		p.Report.Issues = append(p.Report.Issues, "Import exceeds 10,000 tasks")
	}
	return p, nil
}

func validPosition(position string) bool {
	if len(position) == 0 || len(position) > 256 {
		return false
	}
	for _, b := range []byte(position) {
		if !(b >= 'a' && b <= 'z') && !(b >= '0' && b <= '9') && b != '-' {
			return false
		}
	}
	return true
}

// Verify compares every portable task field; server acknowledgement alone is not verification.
func Verify(expected, actual []taskclient.Task) error {
	if len(expected) != len(actual) {
		return fmt.Errorf("task count differs: expected %d, got %d", len(expected), len(actual))
	}
	a := append([]taskclient.Task(nil), expected...)
	b := append([]taskclient.Task(nil), actual...)
	sort.Slice(a, func(i, j int) bool { return a[i].ID < a[j].ID })
	sort.Slice(b, func(i, j int) bool { return b[i].ID < b[j].ID })
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	if !bytes.Equal(left, right) {
		return fmt.Errorf("export differs from the planned fields, dates, ordering or timestamps")
	}
	return nil
}
