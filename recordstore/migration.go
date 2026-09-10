package recordstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var ErrLegacyChanged = errors.New("legacy input changed during migration; retry to preserve the newer snapshot")
var ErrMigrationRecovery = errors.New("migration requires recovery")

// IdentityRepair makes ambiguous legacy identities visible without inventing a
// title-based match between snapshots. Originals remain in exact raw backups.
type IdentityRepair struct {
	Import     string `json:"import"`
	Bucket     string `json:"bucket"`
	Index      int    `json:"index"`
	OriginalID string `json:"original_id"`
	ID         string `json:"id"`
}
type MigrationPlan struct {
	Raw        []byte
	BackupID   string
	Import     Record
	Activation Record
	Repairs    []IdentityRepair
}

// PrepareMigration is pure. It validates before any maintenance and derives all
// identities from original bytes and normalized content. No nonce or clock is used.
func PrepareMigration(raw []byte) (MigrationPlan, error) {
	snapshot, err := NormalizeLegacy(raw)
	if err != nil {
		return MigrationPlan{}, err
	}
	encoded, _ := json.Marshal(map[string]any{"schema": 1, "kind": "import", "snapshot": snapshot})
	imported, err := Parse(encoded)
	if err != nil {
		return MigrationPlan{}, err
	}
	backup := digest(raw)
	encoded, _ = json.Marshal(map[string]any{"schema": 1, "kind": "activate", "import": imported.ID, "backup": backup})
	activation, err := Parse(encoded)
	if err != nil {
		return MigrationPlan{}, err
	}
	var original map[string][]struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &original)
	repairs := []IdentityRepair{}
	for _, bucket := range keys(snapshot) {
		for i, task := range snapshot[bucket] {
			if old := original[bucket][i].ID; old != task.ID {
				repairs = append(repairs, IdentityRepair{imported.ID, bucket, i, old, task.ID})
			}
		}
	}
	return MigrationPlan{append([]byte{}, raw...), backup, imported, activation, repairs}, nil
}

// Migrator is inactive: only explicit library callers run it. Anchor is the
// existing configured JSON path. LocalParent is an existing durable device-local
// directory; its child scope is derived from the resolved anchor, never synced.
// No constructor reads task content, creates directories, or changes config.
type Migrator struct {
	Anchor      string
	LocalParent string
	Store       Store
	scope       string
	// Instance-local fault checkpoints, also used by subprocess crash tests.
	step func(string) error
}

type MigrationResult struct {
	Status  string // ready, conflict, waiting; errors leave status recovery
	View    View
	Pending []Record // durable edits, never flushed or deleted by migration
	Repairs []IdentityRepair
}

func NewMigrator(anchor, localParent string) (Migrator, error) {
	if !filepath.IsAbs(anchor) || !filepath.IsAbs(localParent) {
		return Migrator{}, fmt.Errorf("migration paths must be absolute")
	}
	// Resolve normal parent aliases without following a legacy-file symlink.
	parent, err := filepath.EvalSymlinks(filepath.Dir(anchor))
	if err != nil {
		return Migrator{}, err
	}
	local, err := filepath.EvalSymlinks(localParent)
	if err != nil {
		return Migrator{}, err
	}
	anchor = filepath.Join(parent, filepath.Base(anchor))
	scope := filepath.Join(local, digest([]byte(anchor)))
	s := Store{Root: anchor + ".store", Pending: filepath.Join(scope, "pending")}
	// The complete local scope, not only pending, must be separate from sync.
	root, err := resolvePath(s.Root)
	if err != nil {
		return Migrator{}, err
	}
	if err := separatePaths(root, local); err != nil {
		return Migrator{}, err
	}
	if anchor == local || anchor == scope {
		return Migrator{}, fmt.Errorf("legacy and local paths overlap")
	}
	return Migrator{Anchor: anchor, LocalParent: local, Store: s, scope: scope}, nil
}
func (m Migrator) checkpoint(name string) error {
	if m.step != nil {
		return m.step(name)
	}
	return nil
}
func (m Migrator) localStore(name string) Store {
	s := m.Store
	s.Root = filepath.Join(m.scope, name)
	return s
}
func (m Migrator) ensureScope() error {
	if err := ensureDirectory(m.scope); err != nil {
		return err
	}
	flush := m.Store.syncDir
	if flush == nil {
		flush = syncDirectory
	}
	// Always include the explicit parent, including after interrupted mkdir.
	for _, path := range []string{m.scope, m.LocalParent} {
		if err := flush(path); err != nil {
			return err
		}
	}
	return m.checkpoint("scope")
}

// RecoveryDirectory contains durable migration backups and activation witnesses.
func (m Migrator) RecoveryDirectory() string { return m.scope }
func (m Migrator) checkScope() error {
	if m.scope == "" || m.Store.Root != m.Anchor+".store" || m.Store.Pending != filepath.Join(m.scope, "pending") {
		return fmt.Errorf("invalid migration layout")
	}
	info, err := os.Lstat(m.scope)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("local migration scope is not a plain directory")
	}
	entries, err := os.ReadDir(m.scope)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "pending", "migration", "witness", ".staging":
		default:
			return fmt.Errorf("unexpected local migration entry: %s", entry.Name())
		}
	}
	return nil
}

func recovery(err error) error { return fmt.Errorf("%w: %w", ErrMigrationRecovery, err) }
func archiveError(a Archive) error {
	if len(a.Issues) > 0 {
		return fmt.Errorf("%s: %w", a.Issues[0].Path, a.Issues[0].Err)
	}
	return nil
}
func recordsByID(records []Record) map[string]Record {
	out := map[string]Record{}
	for _, r := range records {
		out[r.ID] = r
	}
	return out
}
func recordKind(r Record) replayRecord { var n replayRecord; _ = json.Unmarshal(r.Body, &n); return n }
func migrationView(a Archive, pending []Record) View {
	return Replay(append(append([]Record{}, a.Records...), pending...), a.Backups)
}
func plannedView(synced Archive, pending []Record, plans []MigrationPlan) View {
	prospective := synced
	prospective.Records = append([]Record{}, synced.Records...)
	prospective.Backups = map[string][]byte{}
	for id, raw := range synced.Backups {
		prospective.Backups[id] = raw
	}
	for _, p := range plans {
		prospective.Records = append(prospective.Records, p.Import, p.Activation)
		prospective.Backups[p.BackupID] = p.Raw
	}
	return migrationView(prospective, pending)
}

// Legacy input may itself be a configured symlink. Resolve it only for reading;
// the discovery anchor and symlink stay unchanged. A dangling link is damage,
// never the explicit absence that permits an initial empty import.
func readLegacy(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, fmt.Errorf("resolve legacy symlink %s: %v", path, err)
		}
		raw, err := readPlain(target)
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("legacy symlink target disappeared: %s", path)
		}
		return raw, err
	}
	raw, err := readPlain(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrLegacyChanged
	}
	return raw, err
}
func completeView(v View) bool { return len(v.Pending) == 0 && len(v.Issues) == 0 }

// Run discovers <configured-path>.store and automatically observes the legacy
// anchor. Extra paths are explicitly selected/provider-associated conflict copies;
// no nearby filenames are guessed. Missing extra paths are errors, not empty data.
func (m Migrator) Run(selectedCopies ...string) (result MigrationResult, err error) {
	result.Status = "recovery"
	result.Repairs = []IdentityRepair{}
	if err := m.checkScope(); err != nil {
		return result, recovery(err)
	}
	// Return intact pending edits even when synced storage is damaged.
	pending := m.Store.ScanPending()
	result.Pending = pending.Records
	synced := m.Store.Inspect()
	result.View = migrationView(synced, pending.Records)
	if err := archiveError(synced); err != nil {
		return result, recovery(err)
	}
	if len(pending.Issues) > 0 {
		return result, recovery(pending.Issues[0].Err)
	}
	journal, witness := m.localStore("migration").Inspect(), m.localStore("witness").Inspect()
	for _, a := range []Archive{journal, witness} {
		if err := archiveError(a); err != nil {
			return result, recovery(err)
		}
	}
	// A witness is durable evidence of prior activation, never proof that a
	// missing sidecar is a fresh store or that a partial restore is complete.
	available := recordsByID(synced.Records)
	for _, r := range witness.Records {
		n := recordKind(r)
		stored, ok := available[r.ID]
		if n.Kind != "activate" || !ok || !bytes.Equal(stored.Body, r.Body) {
			return result, recovery(fmt.Errorf("previous activation %s is missing or damaged", r.ID))
		}
		if !completeView(Replay([]Record{r, available[n.Import]}, synced.Backups)) {
			return result, recovery(fmt.Errorf("previous activation %s is incomplete", r.ID))
		}
	}
	if len(witness.Backups) > 0 || len(journal.Records) > 0 {
		return result, recovery(fmt.Errorf("unexpected migration journal contents"))
	}
	if len(result.View.Issues) > 0 {
		return result, recovery(fmt.Errorf("invalid replay: %s", result.View.Issues[0].Code))
	}
	// Every durable journal backup is an observed snapshot, even if the process
	// stopped before publishing any shared record. Reconstruct plans on restart.
	plans := []MigrationPlan{}
	for _, id := range keys(journal.Backups) {
		plan, e := PrepareMigration(journal.Backups[id])
		if e != nil {
			return result, recovery(e)
		}
		plans = append(plans, plan)
	}
	future := plannedView(synced, pending.Records, plans)
	if len(future.Issues) > 0 {
		return result, recovery(fmt.Errorf("journal replay: %s", future.Issues[0].Code))
	}
	if len(future.Pending) > 0 || future.Snapshot == nil && len(future.Conflicts) == 0 && (synced.Exists || len(pending.Records) > 0) && len(plans) == 0 {
		result.Status = "waiting"
		return result, nil
	}
	// Read and validate every selected input before any new write. For a first
	// install only, explicit ENOENT at the anchor is the canonical empty import.
	type observation struct {
		path   string
		raw    []byte
		exists bool
	}
	observations := []observation{}
	for i, path := range append([]string{m.Anchor}, selectedCopies...) {
		if !filepath.IsAbs(path) {
			return result, fmt.Errorf("selected legacy path must be absolute")
		}
		raw, e := readLegacy(path)
		exists := true
		if errors.Is(e, os.ErrNotExist) && i == 0 {
			exists = false
			e = nil
			if synced.Exists || len(plans) > 0 || len(pending.Records) > 0 {
				observations = append(observations, observation{path, nil, false})
				continue
			}
			raw = []byte("{}")
		}
		if e != nil {
			return result, fmt.Errorf("read legacy %s: %w", path, e)
		}
		plan, e := PrepareMigration(raw)
		if e != nil {
			return result, fmt.Errorf("validate legacy %s: %w", path, e)
		}
		observations = append(observations, observation{path, raw, exists})
		plans = append(plans, plan)
	}
	future = plannedView(synced, pending.Records, plans)
	if !completeView(future) {
		return result, recovery(fmt.Errorf("observed snapshots exceed replay limits or have invalid dependencies"))
	}
	if err := m.checkpoint("read"); err != nil {
		return result, err
	}
	if err := m.ensureScope(); err != nil {
		return result, err
	}
	// Preserve all observations locally before publishing any of them, so an
	// interrupted run never depends on the current (possibly changed) legacy file.
	for _, p := range plans {
		if _, err := m.localStore("migration").Backup(p.Raw); err != nil {
			return result, err
		}
		if err := m.checkpoint("journal"); err != nil {
			return result, err
		}
	}
	for _, p := range plans {
		if err := m.publishMigration(p); err != nil {
			return result, err
		}
	}
	synced = m.Store.Inspect()
	pending = m.Store.ScanPending()
	result.Pending = pending.Records
	result.View = migrationView(synced, pending.Records)
	if len(pending.Issues) > 0 {
		return result, recovery(pending.Issues[0].Err)
	}
	if err := archiveError(synced); err != nil {
		return result, recovery(err)
	}
	if len(result.View.Issues) > 0 {
		return result, recovery(fmt.Errorf("invalid final replay: %s", result.View.Issues[0].Code))
	}
	if len(result.View.Pending) > 0 {
		result.Status = "waiting"
		return result, nil
	}
	if err := m.checkpoint("replay"); err != nil {
		return result, err
	}
	// Persist witnesses for every observed complete activation, including those
	// received from another device. They never replace shared activation evidence.
	for _, r := range synced.Records {
		if n := recordKind(r); n.Kind == "activate" {
			// An incoming activation may have arrived after our initial scan. Preserve
			// its raw recovery bytes locally and sync its actual shared prerequisites
			// before recording a durable witness; readback alone is not durability.
			plan, e := PrepareMigration(synced.Backups[n.Backup])
			if e != nil || plan.Activation.ID != r.ID {
				return result, recovery(fmt.Errorf("activation changed before witness: %v", e))
			}
			if _, err := m.localStore("migration").Backup(plan.Raw); err != nil {
				return result, err
			}
			if err := m.checkpoint("journal"); err != nil {
				return result, err
			}
			if err := m.publishMigration(plan); err != nil {
				return result, err
			}
			if err := m.localStore("witness").put(filepath.Join(m.scope, "witness", "records"), r.ID, r.Body); err != nil {
				return result, err
			}
		}
	}
	if err := m.checkpoint("witness"); err != nil {
		return result, err
	}
	seen := map[string]bool{}
	for _, id := range keys(synced.Backups) {
		p, e := PrepareMigration(synced.Backups[id])
		if e != nil {
			return result, recovery(e)
		}
		for _, repair := range p.Repairs {
			key := repair.Import + ":" + repair.ID
			if !seen[key] {
				result.Repairs = append(result.Repairs, repair)
				seen[key] = true
			}
		}
	}
	sort.Slice(result.Repairs, func(i, j int) bool {
		a, b := result.Repairs[i], result.Repairs[j]
		if a.Import != b.Import {
			return a.Import < b.Import
		}
		if a.Bucket != b.Bucket {
			return less(a.Bucket, b.Bucket)
		}
		return a.Index < b.Index
	})
	// Observe source changes without ever replacing the legacy compatibility input.
	for _, o := range observations {
		raw, e := readLegacy(o.path)
		exists := true
		if errors.Is(e, os.ErrNotExist) {
			exists = false
			e = nil
		}
		if e != nil {
			return result, e
		}
		if exists != o.exists || exists && !bytes.Equal(raw, o.raw) {
			return result, ErrLegacyChanged
		}
	}
	if err := m.checkpoint("finished"); err != nil {
		return result, err
	}
	result.Status = "ready"
	if len(result.View.Conflicts) > 0 {
		result.Status = "conflict"
	}
	return result, nil
}

// publishMigration preserves dependency order; generic pending Flush deliberately
// has no migration semantics and must not publish these records by hash order.
func (m Migrator) publishMigration(p MigrationPlan) error {
	if _, err := m.Store.Backup(p.Raw); err != nil {
		return err
	}
	if err := m.checkpoint("backup"); err != nil {
		return err
	}
	if err := m.Store.put(filepath.Join(m.Store.Root, "records"), p.Import.ID, p.Import.Body); err != nil {
		return err
	}
	if err := m.checkpoint("import"); err != nil {
		return err
	}
	// Verify exact prerequisites immediately before publishing activation.
	backup, e := readPlain(filepath.Join(m.Store.Root, "legacy", p.BackupID+".json"))
	if e != nil || !bytes.Equal(backup, p.Raw) {
		return recovery(fmt.Errorf("backup changed before activation: %v", e))
	}
	imported, e := readPlain(filepath.Join(m.Store.Root, "records", p.Import.ID+".json"))
	if e != nil || !bytes.Equal(imported, p.Import.Body) {
		return recovery(fmt.Errorf("import changed before activation: %v", e))
	}
	if err := m.checkpoint("verified"); err != nil {
		return err
	}
	if err := m.Store.put(filepath.Join(m.Store.Root, "records"), p.Activation.ID, p.Activation.Body); err != nil {
		return err
	}
	if err := m.checkpoint("activation"); err != nil {
		return err
	}
	return nil
}
