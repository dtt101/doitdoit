# Stage 5: restartable migration and legacy observation

Status: implemented as an inactive desktop migration library. Normal TUI, CLI,
and web startup still use the legacy JSON adapter. No migration toggle, setup
prompt, runtime activation, or provider upload is added by this stage.

## Discovery and API

`config.Config.RecordMigration(localParent)` uses the existing `StoragePath`,
including its existing tilde/relative-path expansion, and returns a
`recordstore.Migrator`. It does not load or save configuration, select retention,
or run migration. Direct library callers can use
`recordstore.NewMigrator(absoluteAnchor, localParent)`.

The synced directory is `<configured-anchor>.store`. Parent symlink aliases resolve
consistently for local scope identity; a legacy-file symlink remains the configured
discovery anchor and is followed only for reading. Migration never replaces the
symlink or its target. Dangling links, nonregular files, invalid bytes, and permission
errors cannot become a fresh empty store.

`localParent` must already exist durably on the local device, outside the synced
store. Integration supplies this internal directory; users are not asked to choose
another path. Migration creates and syncs its own scope below that boundary:

```
<localParent>/<sha256(resolved-anchor)>/
  migration/legacy/<raw-hash>.json    exact durable observed snapshots
  witness/records/<activation>.json  evidence of previously verified activation
  pending/<record-hash>.json         existing durable edit queue for stage 6
```

`RecoveryDirectory()` exposes the scope for future recovery tooling. New files are
`0600` and owned directories are `0700`. Directory sync stops at fixed durable
parents and rechecks their entries after interrupted/concurrent initialization.
Existing legacy `.bak` files, configuration, themes, retention choices (including
undecided retention), task IDs, date buckets, completion state, timestamp spelling,
and task order are untouched. No rollover or retention runs before importing.

`Migrator.Run(selectedCopies...)` performs one observation/reconciliation pass.
It returns the replay view, still-pending local records, explicit identity-repair
metadata, and a status:

| Status | Meaning |
| --- | --- |
| ready | Verified activation and complete available replay, with no conflicts |
| conflict | Migration records are verified; retained alternatives need resolution |
| waiting | Observed dependencies are unavailable; keep the last verified view and retry after arrival |
| recovery with an error | Validation, I/O, lost activation evidence, or another failure prevented success |

The durable pending queue and replay's `View.Pending` have different meanings:
queued edits may be fully replayable locally while still unpublished. Migration
reads and returns those edits, rescans them before its final view, and never flushes
or removes them. `ErrLegacyChanged` asks the caller to retry observation because
source bytes/existence changed during the pass; it does not authorize discarding
local data or replacing the legacy file. Results are snapshots of observed local
state, not remote-sync confirmation or a filesystem lock.

## Durable sequence and restart

1. Inspect synced records/backups, local recovery data, activation witnesses, and
   pending edits. Report unknown entries, malformed records, corrupt backup hashes,
   and unsafe paths without removing evidence. A missing dependency does not mean
   legacy storage is authoritative again.
2. Reconstruct any interrupted plans from the exact local journal backups. If an
   observed activation/change still lacks dependencies after accounting for these
   known plans, return `waiting` before importing the current legacy input.
3. Read and validate the legacy anchor and any explicitly selected copies. Only
   explicit absence of the anchor on a fresh, unwitnessed store permits `{}`.
   An existing incomplete store is not a fresh-install shortcut. Preflight the
   combined replay, including the existing replay limits, before new writes.
4. Persist every observed raw snapshot in the device-local journal before any
   shared publication. This journal is recovery data, not a disposable cache.
5. Publish the exact raw backup in `legacy/`, then the content-derived import in
   `records/`. Read back both exact byte sequences before publishing activation.
   Publication uses the existing immutable hard-link protocol and durability checks;
   generic pending `Flush` is not used to order migration dependencies.
6. Rescan and replay the shared store with local pending edits. Only after complete
   valid replay, capture recovery bytes and sync prerequisites for all observed
   activations, including those received from another device, then persist their
   witnesses. Recheck source bytes/existence before success.

A crash can occur at every boundary. Retrying reconstructs identical import and
activation IDs, verifies existing publications, and syncs their actual inodes and
directories. An error never acknowledges completion. An interrupted local journal
is retained even if the legacy file changes before restart; both observed snapshots
are eventually published as alternatives. Temporary files stay outside discovery
namespaces and remain available as evidence; no cleanup or history purge is added.

A witness is never the sole evidence of completion. Every witnessed activation,
its import, and its raw backup must still exist and validate in synced storage.
A missing sidecar, lost activation, missing import, or missing backup causes a
recovery error instead of silently recreating an old store. Pending edits remain
recoverable alongside a damaged synced root. Automatic restoration from witnesses
is deliberately not attempted; explicit recovery/export belongs to stage 6.

## Offline migration and late legacy writes

Equivalent offline snapshots share import identity; differently formatted originals
retain separate exact backups/activations without duplicating tasks or completion
history. Unequal snapshots remain immutable alternatives under stage 4 replay.
Neither the first device nor the newest timestamp wins. An activation delivered
before its import or backup remains pending until its dependencies arrive.

Every later observed legacy snapshot follows the same durable sequence. The
original JSON remains a compatibility input and is never overwritten with the new
projection. The current normalizer preserves valid IDs and repairs every missing,
empty, or duplicated occurrence using ADR 0001's deterministic rule. `Repairs`
reports the import, bucket, index, original ID, and replacement ID so stage 6 can
surface identity ambiguity. Original bytes remain recoverable.

A remembered local hash does not prove which baseline an old client edited.
Protocol 1 therefore retains unequal late snapshots as alternatives and never
infers deletion from their omissions. Existing newer completions and local pending
edits cannot be overwritten by an old-client snapshot. Conflict-copy paths must
be explicitly selected by the caller or associated by a provider; no adjacent
filenames are guessed or imported automatically.

The same pure `PrepareMigration` / `web/record-migration.js.prepare` implementation
contract produces matching import/activation hashes, exact raw backup identities,
and repair metadata in Go and JavaScript. The JavaScript helper has no persistence
or transport. Durable browser migration and Dropbox publication remain stage 7;
it is not loaded by the current page or service worker.

## Verification and remaining gates

Tests use temporary stores, isolated configuration/HOME, and fake folder delivery.
They cover every sequence checkpoint, targeted file/directory sync failures at
journal/backup/import/activation/witness publication, genuine subprocess interruption,
concurrent process initialization, repeated launches, two offline migrations,
out-of-order dependency delivery, selected conflict copies, malformed/permission
failures, changed source bytes, stable repair metadata, pending edits, missing
sidecars, legacy symlinks, and traverse-only ancestors. A Node bridge compares the
complete Go/JavaScript migration plans against the shared legacy-validation vectors.

Passed locally on Linux with Go 1.27.1: `go test -count=1 ./...`, `go vet ./...`,
`go test -race ./...`, and `node --test web/*.test.js`. Recordstore test binaries
cross-compile for macOS amd64 and arm64; native macOS execution remains the existing
CI matrix. Cross-compilation is not native filesystem verification. Runtime integration (stage 6), browser/provider transport (stage 7),
history/retention policy (stage 8), and large-history performance/default activation
(stage 9) remain deferred. Stage 4 replay limits remain explicit and can block a
large migration without truncating it.

The journal and witnesses are device-local recovery material, not a complete backup
of all later immutable history. Folder deletion and device loss still require store
backups. No implementation can recover old-client writes overwritten before they
are observed durably, or prove remote sync completion from local filesystem state.
