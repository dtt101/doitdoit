package recordstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Record struct {
	ID   string
	Body []byte
}

func digest(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }

// Parse validates a standalone protocol-1 record. Graph-dependent intent,
// activation/backup matching, and conflict validation are performed by Replay.
func Parse(body []byte) (Record, error) {
	canonical, err := Canonical(body)
	if err != nil {
		return Record{}, err
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &value); err != nil || value == nil {
		return Record{}, fmt.Errorf("record must be an object")
	}
	if string(value["schema"]) != "1" {
		return Record{}, fmt.Errorf("unsupported record schema")
	}
	var kind string
	if err := json.Unmarshal(value["kind"], &kind); err != nil {
		return Record{}, err
	}
	switch kind {
	case "import":
		if err := fields(value, "schema kind snapshot", ""); err != nil {
			return Record{}, err
		}
		var data map[string]json.RawMessage
		if err := json.Unmarshal(value["snapshot"], &data); err != nil || data == nil {
			return Record{}, fmt.Errorf("invalid snapshot")
		}
		seen := map[string]bool{}
		for key, raw := range data {
			if !bucket(key) {
				return Record{}, fmt.Errorf("invalid bucket")
			}
			if err := tasks(raw, seen, true); err != nil {
				return Record{}, err
			}
		}
	case "activate":
		if err := fields(value, "schema kind import backup", ""); err != nil {
			return Record{}, err
		}
		for _, key := range []string{"import", "backup"} {
			if !validID(str(value[key])) {
				return Record{}, fmt.Errorf("invalid %s reference", key)
			}
		}
	case "change":
		if err := fields(value, "schema kind parents origin action task_ids buckets", "resolves undo_of"); err != nil {
			return Record{}, err
		}
		if err := stringList(value["parents"], 1, validID); err != nil {
			return Record{}, err
		}
		if err := stringList(value["task_ids"], 0, func(s string) bool { return s != "" }); err != nil {
			return Record{}, err
		}
		action := str(value["action"])
		switch action {
		case "create", "edit", "complete", "reopen", "move", "reorder", "delete", "maintenance", "undo", "resolve":
		default:
			return Record{}, fmt.Errorf("unknown action")
		}
		if _, ok := value["resolves"]; ok != (action == "resolve") {
			return Record{}, fmt.Errorf("invalid resolves field")
		}
		if action == "resolve" {
			if err := stringList(value["resolves"], 2, validID); err != nil {
				return Record{}, err
			}
		}
		if _, ok := value["undo_of"]; ok != (action == "undo") {
			return Record{}, fmt.Errorf("invalid undo_of field")
		}
		if action == "undo" && !validID(str(value["undo_of"])) {
			return Record{}, fmt.Errorf("invalid undo reference")
		}
		if err := origin(value["origin"]); err != nil {
			return Record{}, err
		}
		var deltas []map[string]json.RawMessage
		if err := json.Unmarshal(value["buckets"], &deltas); err != nil || len(deltas) == 0 {
			return Record{}, fmt.Errorf("invalid bucket replacements")
		}
		keys := []string{}
		beforeIDs, afterIDs := map[string]bool{}, map[string]bool{}
		for _, delta := range deltas {
			required := "bucket after"
			if action != "resolve" {
				required += " before"
			}
			if err := fields(delta, required, ""); err != nil {
				return Record{}, err
			}
			key := str(delta["bucket"])
			if !bucket(key) {
				return Record{}, fmt.Errorf("invalid bucket")
			}
			keys = append(keys, key)
			if err := tasks(delta["after"], afterIDs, false); err != nil {
				return Record{}, err
			}
			if action != "resolve" {
				if err := tasks(delta["before"], beforeIDs, false); err != nil {
					return Record{}, err
				}
			}
		}
		if !ordered(keys) {
			return Record{}, fmt.Errorf("buckets must be sorted and unique")
		}
	default:
		return Record{}, fmt.Errorf("unsupported record kind")
	}
	return Record{ID: digest(canonical), Body: canonical}, nil
}

func fields(object map[string]json.RawMessage, required, optional string) error {
	allowed := map[string]bool{}
	for _, name := range strings.Fields(required) {
		if _, ok := object[name]; !ok {
			return fmt.Errorf("missing field %s", name)
		}
		allowed[name] = true
	}
	for _, name := range strings.Fields(optional) {
		allowed[name] = true
	}
	for name := range object {
		if !allowed[name] {
			return fmt.Errorf("unknown field %s", name)
		}
	}
	return nil
}

func str(raw json.RawMessage) string { var s string; _ = json.Unmarshal(raw, &s); return s }
func date(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Format("2006-01-02") == s
}
func bucket(s string) bool { return s == "Future" || date(s) }

func stringList(raw json.RawMessage, minimum int, valid func(string) bool) error {
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil || items == nil || len(items) < minimum || !ordered(items) {
		return fmt.Errorf("invalid sorted unique list")
	}
	for _, s := range items {
		if !valid(s) {
			return fmt.Errorf("invalid list entry")
		}
	}
	return nil
}

func tasks(raw json.RawMessage, seen map[string]bool, nonempty bool) error {
	var list []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil || list == nil || nonempty && len(list) == 0 {
		return fmt.Errorf("invalid task array")
	}
	for _, task := range list {
		if err := fields(task, "id title completed created_at due_date", ""); err != nil {
			return err
		}
		for _, key := range []string{"id", "title", "created_at", "due_date"} {
			if len(task[key]) == 0 || task[key][0] != '"' {
				return fmt.Errorf("invalid task string")
			}
		}
		id := str(task["id"])
		if id == "" || seen[id] {
			return fmt.Errorf("empty or duplicate task ID")
		}
		seen[id] = true
		if string(task["completed"]) != "true" && string(task["completed"]) != "false" {
			return fmt.Errorf("invalid completion state")
		}
		if !creationTimestamp(str(task["created_at"])) {
			return fmt.Errorf("invalid creation timestamp")
		}
		if due := str(task["due_date"]); due != "" && !date(due) {
			return fmt.Errorf("invalid due date")
		}
	}
	return nil
}

// Keep the original spelling for hashing; parsing alone accepts non-RFC3339
// forms (comma fractions, one-digit hours, and overflowing zone components).
var creationFormat = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](\.[0-9]+)?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$`)

func creationTimestamp(value string) bool {
	if !creationFormat.MatchString(value) {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

var hex32 = regexp.MustCompile(`^[a-f0-9]{32}$`)

func origin(raw json.RawMessage) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return err
	}
	if err := fields(object, "device nonce at day offset_minutes", ""); err != nil {
		return err
	}
	if !hex32.MatchString(str(object["device"])) || !hex32.MatchString(str(object["nonce"])) {
		return fmt.Errorf("invalid origin identity")
	}
	at := str(object["at"])
	t, err := time.Parse("2006-01-02T15:04:05.000Z", at)
	if err != nil || t.Format("2006-01-02T15:04:05.000Z") != at || !date(str(object["day"])) {
		return fmt.Errorf("invalid origin date")
	}
	var offset int
	if err := json.Unmarshal(object["offset_minutes"], &offset); err != nil || string(object["offset_minutes"]) == "null" || offset < -840 || offset > 840 {
		return fmt.Errorf("invalid UTC offset")
	}
	if t.Add(time.Duration(offset)*time.Minute).Format("2006-01-02") != str(object["day"]) {
		return fmt.Errorf("origin day does not match timestamp and UTC offset")
	}
	return nil
}
