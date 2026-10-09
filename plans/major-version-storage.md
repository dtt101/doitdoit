# Simple file storage and visible conflict resolution

Status: accepted direction, replacing the immutable-storage roadmap on 2026-09-10.
The existing JSON file remains authoritative. No backend migration is planned.
On load, legacy dated tasks in Future are normalized into their date buckets
before rollover and retention. Future is reserved for undated tasks; calendar
navigation and layout never change task storage.
This document replaces the former eleven-stage plan; its stages 6–11 are cancelled.

## Goal

Keep doitdoit a task manager backed by one readable, user-owned JSON file. Retain
the storage abstraction and the safety improvements already made. Focus the next
work on showing users which tasks differ when a save conflicts, preserving their
work, and helping them choose the result.

The current TUI, CLI, and static web application still use JSON storage. The
immutable publication, replay, and migration libraries have not been activated.
Restoring this direction requires no conversion of users' task data.

## Keep

- `taskstore.Store`, the JSON adapter, task lifecycle code independent of Bubble
  Tea, and the existing CLI/TUI/configuration storage boundaries.
- The existing JSON fields, task IDs, local-calendar date buckets, `Future`,
  configured paths, and normal file backup and scripting workflows.
- Atomic replacement, owner-only permissions, `.bak` creation, loaded-data/revision
  pairing, external-change checks, and safe storage moves.
- Conservative three-way merging: independent buckets can merge; differing edits
  to the same bucket remain visible conflicts. Identical results can converge.
- Live web fixes for stale reloads, edits made during uploads, Dropbox revision
  protection, and recovery before destructive reload. Keep their regression tests.
- Existing retention, rollover, ordering, undo, themes, Omarchy integration, and
  Linux/macOS support. The web companion stays static and self-contained.

## Retire the experimental backend

The unused record-store package, migration adapter, browser protocol/outbox
modules, and their dedicated tests, schema, fixtures, and design documents have
been removed from this checkout. Git history preserves the abandoned design.
The live JSON fixes, UI changes, storage abstraction, and platform improvements
are retained. Stage 5 PR #28 has been closed. Its migration implementation is abandoned.

No user files or recovery data are removed. Do not activate migration or create
record-store sidecars. Any future storage-format change needs a new decision.

## Storage contract and limits

There is one authoritative task JSON file, with the existing backup behavior.
There is no operation log, causal graph, background migration, distributed history,
new server, or replacement database. Local recovery copies are unsaved drafts,
not a second synced authority.

Desktop saves compare the loaded revision before atomic replacement. Dropbox web
saves use revision-checked updates and create-only writes for confirmed absence.
These protections must remain. Generic folder sync still has a check-to-replace
race and can hide simultaneous offline writes before the app observes them.
A conflict interface can resolve versions the app has seen; it cannot recover an
unobserved version or guarantee that a sync provider has uploaded a local save.
Document this limit without claiming lossless multi-device synchronization.

## Conflict experience

The terminal implementation extends the original whole-list proposal with
explicit task selection and discovery of Dropbox conflict copies. The JSON
schema and conservative automatic `Merge` policy remain unchanged. Web review
and CLI recovery integration are deferred; their current behavior is unchanged.

### Discovered copies

Scan only Dropbox-named conflict copies adjacent to the configured task file,
including conflicted `.bak` files. Preserve ordinary backups and historical
snapshots. Compare raw snapshots before startup maintenance writes. Silently
remove a copy only if every task matches the current file in content, bucket,
and shared relative order; extra current tasks are not evidence of deletion.
Malformed data, unknown fields, and ambiguous IDs never qualify for automatic
cleanup. Keep excluded completed differences in their source copies; report
source-read failures.

Keep the ordinary review task-focused: missing tasks, specific changed fields, and plain task actions. Do not show filenames, copy counts, storage terminology, or unchanged creation timestamps. Preview changes first, with the resulting unfinished list available on demand for saved copies. “Save & finish” updates the authoritative task list and removes resolved copies in the background. Keep export and advanced controls under `?`.

Discovered copies are optional recovery material: display a notice at startup
and during background discovery without opening review or blocking normal
rollover and retention; `r` opens review or rescans. Process files in filename order
and recalculate after each resolution. Show unfinished additions, changed tasks, current-only
tasks, and ordering. Exclude tasks completed on either side, including old
unfinished versions of tasks now completed. Exclude whole-list/order decisions
containing completed tasks. Preserve copies with only excluded differences
without prompting. Keep full snapshots and exact bytes for recovery exports.
Creation timestamps do not identify the latest edit.

Users explicitly restore/skip tasks from saved copies, select complete task
versions, and keep/delete current-only tasks. Default every discovered-copy
decision to keeping current. Label copied tasks as recovery material, not missing
work: there is no baseline to establish whether an absent task was deleted.
Moves are one task decision across buckets. Ordering is a separate list choice, with retained
current-only tasks appended after the selected alternative order. Missing or
duplicate IDs require whole-list choices for connected buckets; no title matching,
ID repair, automatic field merge, or newly introduced duplicate ID is allowed.

`K` keeps current for all differences. `p` previews exact resulting lists and
normal rollover/retention effects; Enter applies the result. Choosing an unchanged
result does not rewrite the main file or its regular backup. Esc defers review.
Before deleting a nonredundant reviewed source, preserve exact source bytes and
the pre-resolution file in local recovery storage. Failed saves retain the source;
cleanup failures remain visible and retryable. Check both reviewed inputs before
saving or deletion. Changed inputs require refreshed review, never force-save.

### Failed saves

Retain the loaded baseline independently of undo. After a failed save, persist a
local draft with its baseline and revision context, pause editing and automatic
writes, and offer review of “Your changes” and “File version”. Navigation and
export remain available. Unopposed task changes are proposed in the explicit
review, without broadening the existing automatic bucket-merge policy. Show the
baseline on demand. Cancellation keeps the draft and conflict warning.

Pending recovery is scoped to the resolved task-file path and restored before
startup maintenance. Atomic owner-only recovery files live under
`os.UserConfigDir()/doitdoit/recovery/`; they are not synced authorities. Preserve
old choices as recovery material when inputs change. Clear pending recovery only
after a confirmed save or explicit discard. Block ordinary quit if recovery
cannot be persisted; keep the in-memory draft and offer export or explicit
discard. Explicit exports and resolved alternatives remain available until the
user removes them. Do not build a history browser or retry queue.

### Implementation boundary

Pure `taskstore.Compare` / `Review.Resolve` helpers describe and resolve two- or
three-snapshot reviews. File discovery, strict recovery decoding, guarded cleanup,
and local recovery persistence remain independent of Bubble Tea. The model owns
review UI state, preview, lifecycle effects, and conditional application through
`taskstore.Store`. The normal task schema and storage API remain compatible.

## Verification

Use temporary files, isolated HOME, and fake Dropbox/browser storage; never test
against real task files or credentials. Run Go tests, vet, and race checks for
persistence/reload changes, plus web tests for web changes. CI must pass on Linux
and macOS, preserving isolated Omarchy coverage. Verify that existing JSON fixtures
round-trip unchanged and a conflict never silently overwrites either observed
alternative. Exercise the complete review/save interaction, not just comparison
helpers. Record known sync limits in the user documentation.
