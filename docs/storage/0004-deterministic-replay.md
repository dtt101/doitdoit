# Stage 4: deterministic replay and retained conflicts

Status: implemented as an inactive library. The TUI, CLI, and static page continue
to use the legacy JSON adapter. No migration, startup discovery, file writer,
Dropbox transport, or application controls are changed by this stage.

## API and rebuild boundary

Go exposes `recordstore.Replay(records, backups) View`. JavaScript exposes
`DoitdoitRecordProtocol` from `web/record-protocol.js` and
`DoitdoitRecordReplay.replay(records, backups)` from `web/record-replay.js`.
The JavaScript APIs also support CommonJS for Node tests. Browser hashing uses
Web Crypto; neither module is loaded by the current page or service worker.

Inputs are exact canonical record envelopes and a map from raw-backup hash to raw
bytes (UTF-8 strings in JavaScript). Replay checks both canonical bytes and hashes,
deduplicates records, validates dependency closures, and verifies activation backup
content. A malformed duplicate poisons its ID even if another envelope with that
ID is valid. The caller retains the original evidence and durable pending queue.
Replay never deletes or repairs input. Returned arrays/maps are detached; JavaScript
also detaches input before awaiting hashing.

| View field | Meaning |
| --- | --- |
| `snapshot` | Complete conflict-free projection of the available valid branches; null without activation or with any conflict |
| `common` | Common verified state plus independent disjoint changes |
| `conflicts` | Connected buckets, maximal competing IDs, common baseline, and exact per-tip component snapshots |
| `pending` | Observed records blocked on absent or invalid dependencies, including missing activation backups |
| `issues` | Record IDs and stable failure codes; publication must stop while issues remain |
| `heads` | Maximal accepted causal record IDs, including activation wrappers |
| `unique_imports` | Accepted distinct import IDs, including imports not yet activated |
| `completion_events` | Accepted explicit complete records, never imports, retries, maintenance, or inferred past activity |

A non-null snapshot does **not** imply sync is complete: independent complete
branches can be shown while `pending` or `issues` are nonempty. Integration must
retain its last verified view and durable pending records, surface incomplete
state, and block publication on invalid/unsupported input. It must never turn a
null snapshot into an empty writable task collection. Rebuild is pure: its only
cache is a per-call memo of causal projections. No persisted cache or application
fallback path is introduced.

## Causality and conflict projection

Replay validates parents before children. Parents must be an antichain of maximal
observed causal IDs; a change must have an activation in its ancestor closure.
Unknown or invalid records cannot satisfy dependencies. Missing records and
backups remain pending and replay resumes when the caller rebuilds with them.
Content-addressed IDs and explicit ancestry determine the result; clocks,
filenames, delivery order, and modification times do not.

Concurrent changes commute only for disjoint touched buckets or exactly identical
full replacement batches. Different tasks edited in the same bucket still
conflict. A multi-bucket move conflicts as a whole. Conflicts also connect buckets
when concurrent projections put one task identity in different places.

A conflict retains historical competing intentions until an explicit resolution;
two branches later reaching identical values do not silently resolve an earlier
conflict. Descendants touching a component extend that branch's alternative.
Disjoint descendants remain independently visible even if they observed a conflict.
Import candidates omit absent buckets rather than treating absence as deletion.
Different imports have no fabricated common baseline; equal imported identities
can share a baseline across separate activation wrappers.

Each candidate snapshot contains just its component's buckets, preserving the
branch's full atomic result. Other independent buckets are in `common`. The
component's `common` is null when there is no common activated import. These are
recovery alternatives, not a flattened list ordered by record hash.

A resolution must name all competing maximal tips in the selected components,
include them causally, and replace every connected bucket. It may resolve one
component while leaving another observed component unresolved. A previously
unseen branch, or a conflicting concurrent resolution, can reopen a conflict.

## Intent validation

Before arrays must exactly match the parent's common projection, and ordinary
changes cannot touch an unresolved component. Validate a batch everywhere or
apply it nowhere. Global IDs remain unique, and active tasks precede completed
tasks. Ordinary replacements must actually change their buckets.

`task_ids` is the exact affected set: additions, removals, field or bucket changes,
plus inversions among otherwise unchanged neighbours. Inserting, removing, moving,
or completing one task does not count every neighbour's shifted index as an edit.
Resolve uses the union of differences from all candidate snapshots to the result.
Sets use unsigned UTF-16 ordering in both implementations.

| Action | Allowed delta |
| --- | --- |
| create | Add new `task:<32 lowercase hex>` identities; retain existing tasks and their relative order |
| edit | Change titles only, preserving identity, creation timestamp, completion, due date, and bucket |
| complete / reopen | Toggle completion in the named direction; preserve other fields and unaffected relative order |
| move | Change bucket and/or due date, preserving all other fields |
| reorder | Change order only; preserve all task fields and bucket memberships |
| delete | Remove tasks only |
| maintenance | Move, reschedule, reorder, or prune existing tasks; never create, rename, or toggle completion |
| undo | Apply the exact inverse arrays of an ancestor change against its still-current after state |
| resolve | Replace whole selected conflict components and name their exact competing tips |

Undo of a resolution has no single before state and is rejected; a new explicit
resolution is required. Undo of an ordinary change or a previous undo is an
additional record, never deletion of history. Stage 8 must define statistics for
multi-task complete batches and concurrent repeated intentions before exposing
activity analytics; the current count is a fixture-level record count.

## Activation verification and limits

`NormalizeLegacy` / `normalizeLegacy` is a pure normalizer used to verify raw
activation backups. It rejects ambiguous JSON, unknown task fields, invalid dates,
wrong types, and invalid task ordering. It defaults missing due dates, omits empty
buckets, preserves valid original timestamp spelling, and repairs every occurrence
of missing/empty/duplicate IDs using ADR 0001's original canonical snapshot hash.
Golden shared vectors pin repair IDs. It does not discover files, write backups,
observe legacy changes, or implement a restartable migration; those remain stage 5.

Both implementations enforce 16 MiB input/canonical record limits, 256 JSON nesting
levels, safe integers, and the same numeric token/exponent limits. This initial
replayer additionally allows at most 512 distinct observed record IDs and 4096
uncached causal projections per rebuild. Duplicate delivery does not consume the
record-ID budget. Exceeding a replay limit returns a null snapshot, all observed
IDs pending, and `replay-limit`; callers must retain their previous verified view.
These conservative bounds make incomplete work explicit. Stage 9 must measure
large histories and revise the implementation/budgets before runtime activation;
this is not a production performance acceptance claim.

## Verification

Shared scenarios cover shuffled and duplicated delivery, equal/unequal imports,
missing imports/backups, late legacy alternatives, all action types, ordering,
atomic moves, delete/edit conflicts, branch descendants, partial resolutions,
reopened conflicts, disjoint edits, invalid action intent, and clock skew.
Shared validation vectors cover canonicalization, timestamps, backup normalization,
and exact ID repair. Go also invokes a small Node test bridge, when Node is
available, to compare the **entire** result across languages. Both languages run
independent fixture assertions even without that bridge.

Passed locally on Linux with Go 1.27.1: `go test -count=1 ./...`, `go vet ./...`,
`go test -race ./...`, and `node --test web/*.test.js`. Recordstore test binaries
also cross-compile for macOS amd64 and arm64. Native macOS execution remains the
existing CI matrix;
Linux and macOS runtime activation, migration fault injection, provider integration,
large-history performance, and the live Omarchy theme-switch check remain later
stage/release gates.
