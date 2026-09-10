# Simple file storage and visible conflict resolution

Status: accepted direction, replacing the immutable-storage roadmap on 2026-09-10.
The existing JSON file remains authoritative. No storage migration is planned.
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

When another version prevents a save, retain the user's local edits and show a
persistent message: “Changes need review — your edits have not been saved.” Offer
“Review changes”. Never replace the draft with the incoming file automatically.
Pause automatic writes while review is pending; allow navigation and explicit
recovery export. The first version can pause editing during review to keep the
interaction predictable.

The review compares three snapshots: the version originally loaded, the local
draft, and the latest observed file. Label the two current alternatives “Your
changes” and “File version”; do not infer an author, device, or which is newer
from timestamps. Show the original value on demand.

- Group differences by date bucket and show task titles, completion state, date,
  and ordering changes. Mark additions and removals explicitly.
- Match tasks by existing stable IDs across buckets so moves can be explained.
  Missing or duplicate IDs fall back to a whole-bucket comparison; do not guess
  identity from titles or silently repair IDs as part of review.
- Highlight competing changes to the same task and delete/edit cases. Also show
  separate-task changes within a conflicted bucket, explaining that both copies
  changed that list. Displaying task differences does not change the merge policy.
- For the first release, let users choose “Use your list” or “Use file list” for
  each conflicted bucket, with an exact preview. Review connected source and
  destination buckets together for cross-bucket moves; block any result that
  duplicates a task ID or silently loses a moved task.
- Offer recovery export before discarding alternatives. Cancellation retains the
  draft and conflict state. Do not introduce automatic field merging or a “keep
  both” action that can silently duplicate tasks.

Once choices are complete, show the resulting task lists and a single “Save
resolved changes” action. Preserve independent bucket changes in that result.
Save against the revision actually reviewed. If the file changes again, retain
the draft and choices as recovery material, refresh the comparison, and require
review of affected choices. Never force an unconditional overwrite or silently
apply a decision to different input.

Keep this interaction equivalent in TUI and web, using their existing controls.
The CLI should fail clearly on unresolved conflict, preserve any prepared unsaved
change before exit, and give a concrete recovery path; it must not open an
unexpected interactive resolver or report success for an unsaved task.

## Minimal implementation

Use a pure comparison function alongside the existing `taskstore.Merge` boundary.
Its output contains affected buckets, task differences, and the three snapshots
needed for review. Keep UI state outside the JSON schema. Resolution produces a
normal candidate snapshot which passes existing validation and conditional save.
No event replay, action-intent validator, or cross-device conflict metadata is
needed. Use small shared examples for equivalent Go/web comparison behavior.

Persist a recoverable draft before a destructive action or before reporting that
unsaved work is recoverable across restart. Scope recovery to the exact store
(and Dropbox account for web); retain the baseline, local draft, and relevant
revision context. Reuse the existing web recovery mechanism where practical.
Desktop recovery must use atomic writes and `0600` permissions in an application
local location. Fail visibly if recovery storage fails; retain the in-memory draft
and block its replacement. Clear recovery only after confirmed save or explicit
discard. Do not build a general queue, history browser, or background retry engine.

## Delivery plan

1. **Establish the JSON baseline (cleanup implemented in this checkout).** Remove
   the inactive backend while retaining storage boundaries, live safety fixes,
   and UI changes. Run the existing tests and verify runtime references continue
   to use JSON. Existing task files require no migration.
2. **Describe conflicts and preserve drafts.** Add pure conflict comparison and
   the minimal scoped recovery snapshot. Keep the existing merge behavior. Test
   same-task edits, different tasks in one bucket, delete/edit, moves, ordering,
   ambiguous IDs, recovery failure, and restart with an unsaved draft.
3. **Ship conflict review in TUI and web.** Add visible markers, comparison,
   bucket choices, preview, cancellation, export, and revision-checked resolution.
   Include CLI conflict recovery. Test a second external edit during review and
   during save, failed saves, and switching files/accounts with pending recovery.

Each PR must deliver a small reviewable behavior. Do not grow these steps into a
new storage protocol. Task-level manual selection or narrower automatic merging
can be proposed later with concrete user examples and a separate policy review.
Cross-device history, analytics, and immutable snapshots are outside this plan.

## Verification

Use temporary files, isolated HOME, and fake Dropbox/browser storage; never test
against real task files or credentials. Run Go tests, vet, and race checks for
persistence/reload changes, plus web tests for web changes. CI must pass on Linux
and macOS, preserving isolated Omarchy coverage. Verify that existing JSON fixtures
round-trip unchanged and a conflict never silently overwrites either observed
alternative. Exercise the complete review/save interaction, not just comparison
helpers. Record known sync limits in the user documentation.
