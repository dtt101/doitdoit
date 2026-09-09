# Stage 3: immutable publication and durable pending edits

Status: implemented as an inactive foundation. The TUI, CLI, and web application
still use the legacy JSON adapter. This stage does not migrate task files, create
sidecars at startup, replay operations, or change same-bucket conflict behavior.
Linux and macOS are supported, with Omarchy integration preserved.

## Local record store

`recordstore.Store` takes two absolute paths: `Root` for the synced sidecar and
`Pending` for a device-local queue belonging to that store. The caller supplies
these paths; the package never discovers or accesses the user's configuration.
The paths must not overlap, including through existing ancestor symlink aliases.
Store roots, pending directories, and record entries must not themselves be
symlinks. Normal ancestor aliases, such as macOS `/var`, are supported.

| API | Contract |
| --- | --- |
| `Canonical` | Reject ambiguous JSON and produce ADR 0001 ASCII bytes |
| `Parse` | Validate a standalone protocol-1 body and return its SHA-256 ID and canonical bytes |
| `Queue` | Validate and persist a private local pending record before returning its ID |
| `ScanPending` | Return valid local pending records and individual recovery issues after restart |
| `Scan` | Return valid published records and individual recovery issues |
| `Flush` | Publish pending records, verify and sync each publication, then remove its pending copy |
| `Backup` | Preserve exact valid JSON bytes under `legacy/<raw-sha256>.json`; task-level import validation comes in stage 5 |

Publication writes a temporary `0600` file in a sibling `.staging` directory,
flushes it, and hard-links it into the destination under its content-derived ID.
The create-only link cannot overwrite another writer's record. An existing ID is
accepted only when its bytes match exactly. The actual destination file is also
flushed, including on retries, and the directory chain is synced before success.
New directories use `0700`; successful publication restores `0600` on a matching
existing file. Staging files are outside discovery namespaces and are never
interpreted as records. Crash leftovers are retained; automated cleanup is deferred.

There is no shared mutable queue or process-local lock. Multiple CLI/TUI processes
can use the same pending directory and synced store safely through independent
immutable files. Discovery tolerates pending entries removed by another successful
publisher. A crash after publication but before acknowledgement leaves a retry of
the same ID, which verifies the existing record instead of creating a duplicate.

A failed `Queue` must not be announced as saved. Failure can leave a complete but
not yet confirmed durable record: retain the original operation body/nonce and
retry it rather than inventing a second operation. `Flush` errors leave unpublished
or unconfirmed entries queued; earlier successfully published entries may already
be acknowledged. Removal is synced too; an interrupted removal can cause a safe
retry. Local publication does not confirm that Dropbox or another folder provider
has uploaded the record.

Malformed, unsupported, noncanonical, mismatched-ID, unexpected-name, symlink,
and special-file entries are reported and preserved. Scans also return valid
history alongside those issues. Flush stops when its initial discovery sees an
issue in either namespace. External folder changes can arrive after a scan; this
is not a distributed lock or a guarantee of a complete remote view. Concurrent
replacement of store directories or manual deletion of immutable history is
outside the publication protocol.

Filesystems must support hard links and file/directory sync. Unsupported operations,
permission errors, and full disks produce errors rather than a weaker durability
fallback. Guarantees depend on the filesystem and hardware honoring sync requests.
Backups of the whole store remain necessary for folder deletion or device loss.

## Validation boundary

Standalone validation covers known schemas/kinds/fields, required field types,
actual calendar dates and timestamps, Unicode scalar strings, sorted unique IDs,
unique task IDs within snapshots/batches, and resolve/undo field requirements.
Origin local day must agree with its timestamp and UTC offset. Record bodies use
content-derived identities; directory order and modification times have no role.

Input and canonical output are limited to 16 MiB. JSON numbers must be exact safe
integers; lexical numbers longer than 128 bytes or exponents outside -1000 through
1000 are rejected before expensive conversion. These resource limits are local
validation policy and must be aligned with the production JavaScript validator
before integration. A large legacy import must fail visibly rather than truncate;
large-history migration and performance acceptance remain later stages.

Graph-dependent checks belong to stage 4: causal closure, missing dependencies,
action intent, `before` projections, ordering semantics, activation/backup matching,
and conflict resolution. Scanning a record does not declare it safe to apply.
Stage 5 supplies canonical legacy normalization, ID repair, automatic discovery,
activation, and late legacy observation. This library is deliberately not an
implementation of `taskstore.Store` until replay and migration exist.

## Browser outbox

`web/outbox.js` exports `create({ storage, account, path, validate })`. Integration
supplies browser `localStorage`, a stable provider account/store scope, and an
async validator for `{ id, body }` envelopes. `body` is canonical JSON text; the
validator must verify its protocol and exact hash, throwing or returning `false`
on failure. The detached envelope is frozen during validation. Each record has
its own storage key; tabs never replace one shared queue array.

`enqueue` returns an ID only after synchronous storage write and readback succeed.
Quota, permission, read, and write errors propagate; callers must retain the edit
and show a failure instead of claiming it was saved. `list` returns records and
per-entry issues without removing damaged data. `flush(publish)` calls a publisher
that must resolve only after a verified create-only upload (including exact ID/body
matching on retries). A failed upload keeps that and later entries pending. Data
changed during upload remains available for recovery instead of being deleted.

Scope isolation prevents one account/path from flushing another's queue. The
integration must supply the resolved store identity consistently; this module does
not discover Dropbox accounts or normalize their paths. The production protocol
validator/replayer and Dropbox publisher arrive in stages 4 and 7. The current
web page does not load the outbox or change its existing storage behavior.

Browser persistence follows `localStorage` guarantees, not an OS-level fsync
contract. Clearing site data, browser eviction, private-session teardown, or losing
the browser profile can erase edits that have not uploaded. There is no recovery
from this queue after its backing browser storage is gone. This limit must remain
visible when the outbox is integrated into the UI.

## Verification and handoff

Behavioral tests cover:

- Shared protocol examples, canonical byte vectors, equivalent import IDs,
  ambiguous JSON, invalid records, and size limits.
- Partial writes, simulated disk-full/permission failures, file and directory sync
  failures, ancestor durability on retry, immutable collisions, private permissions,
  malformed records, staging leftovers, and aliased paths.
- Subprocess interruption after publication, restart, duplicate delivery, and six
  simultaneous writers sharing one queue; this exercises the storage boundary,
  with actual CLI/TUI integration intentionally deferred to stage 6.
- Browser queue reopening, concurrent tab instances, duplicate envelopes, quota
  and storage failures, interruption before acknowledgement, partial upload failure,
  corrupt evidence, detached validation, and account/path isolation. These tests
  use a storage adapter, not a claim of hardware power-loss or real-browser testing.

Passed locally on Linux with Go 1.27.1: `go test -count=1 ./...`, `go vet ./...`,
`go test -race ./...`,
`node --test web/*.test.js`, JavaScript syntax checks, licence inventory,
GoReleaser validation, and Linux/macOS cross-compilation for amd64 and arm64.
The concurrent subprocess/restart test also passed 20 consecutive runs.
Native macOS execution is covered by the CI matrix and still needs that remote
run. A live Omarchy theme-switch smoke test remains a release check.

Next: stage 4 deterministic replay and retained conflict state in Go and plain
JavaScript, using shuffled/duplicated shared fixtures. No storage activation or
release tag is part of this stage.
