# Stage 2: legacy storage boundaries

Status: implemented, with the existing JSON adapter as the only runtime format.
The immutable backend experiment has been removed. JSON remains authoritative;
no sidecars or migration are required.

## Go boundary

`taskstore` has no Bubble Tea, clipboard, configuration, or theme dependencies.
It owns task data and lifecycle operations, conservative bucket merging, capture,
and the `Store` interface. `JSONStore` implements the existing JSON serialization,
content checks, atomic replacement, `0600` files, backups, and no-replace moves.

`Load` returns task data paired with its revision. `Save` takes that expected
revision and returns the revision actually saved. A nil expectation is retained
only for the existing unconditional `TodoData.Save` compatibility API. Snapshot
mtime/size metadata supports the existing reload policy; it never authorizes a save.
`LoadStore` performs startup maintenance above the adapter and saves only when dirty.

The TUI owns selection, presentation, input validation, scheduling target selection,
feedback, and undo navigation. It passes data operations to `taskstore` and all
I/O through its store. Existing `model.Task`, `TodoData`, `Load`, and `CaptureTask`
entry points remain compatibility wrappers. The CLI now calls `taskstore` directly.
Configuration commands retain their public move functions and sentinel errors.

## Write/read inventory

| Entry point | Boundary | Existing policy retained |
| --- | --- | --- |
| TUI startup | `LoadStore` → `Store.Load/Save` | Rollover, explicit retention, completion grouping; distribution stays in memory |
| CLI add | `Capture` → `LoadStore`, `Store.Save` | Validate input, insert before completed, compare loaded revision |
| TUI task edits, undo, midnight maintenance | `Model.persist` → `Store.Save` | Expected revision, error feedback, one conservative merge retry |
| Conflict retry | `Store.Load`, `Merge`, `Store.Save` | Separate buckets merge; competing same-bucket edits remain conflicts |
| Background reload | `checkStore` → `Store.Load` | Pause while typing; transient failures ignored; stale result gate and in-memory maintenance retained |
| Legacy `TodoData.Save/SaveIfUnchanged` | `JSONStore.Save` | Existing serialization and conditional/unconditional API behavior |
| `config move` | `JSONStore.Move` | Exclusive hard-link publication, cross-filesystem copy fallback, no destination overwrite |
| Web load/focus reload | `taskStore.load` | Existing Dropbox download and auth refresh |
| Web edits and load maintenance | `taskStore.save` | Existing revision upload, debounce, errors, and recovery snapshots |

Configuration/theme files and OAuth token storage are not task-file adapters.
First-run path selection still checks whether the configured file exists and keeps
its current prompts. The revised [storage plan](../../plans/major-version-storage.md)
retains this boundary and JSON format; automatic migration has been cancelled.

## Snapshot consistency

Extraction exposed separate reads of data and its comparison hash in capture,
startup tracking, and conflict retry. These now use one paired snapshot. A follow-up
read after saving may refresh matching metadata, but cannot replace the revision
with an external edit's hash. An intervening same-bucket edit therefore remains a
visible conflict instead of being accidentally authorized for overwrite.

The JSON adapter still has the existing narrow check-to-rename race and eventual
folder sync limitations. This stage does not promise cross-device atomic writes.

## Web boundary

`Sync.createJSONStore` exposes `load`, `save`, and detached `recovery` snapshots.
The application supplies authentication callbacks and retains browser/UI state.
Transport, revision handling, and authentication retries use the existing code.
Task insert/edit/toggle/delete/move operations now live in `domain.js`; rendering,
dialogs, drag cancellation, debounce, and maintenance timing stay in `app.js`.
There are no new scripts, dependencies, OAuth permissions, or storage keys.

This is an extraction, not an alignment of historical client differences. Existing
web empty-bucket cleanup and flat-Future drag rules remain; Go retains its current
empty buckets and presentation-constrained reordering. Existing lifecycle tests
continue to protect shared rollover, retention, and distribution behavior.

## Verification

Existing lifecycle/persistence/UI tests remain unchanged. The existing move tests
move to `taskstore` with only their package declaration changed. Added tests inject
intervening writes around capture, maintenance, startup, and saving; exercise the
adapter's revision/backup behavior; and check web auth refresh, conflicts, recovery,
and extracted operations. All stores in tests are temporary.

Required checks: Go suite, vet, race suite, web suite, JavaScript syntax check,
and release inventory tests. No major release or storage activation is needed to
ship this extraction on its own.
