# Major-version storage, sync, and history plan

Status: stage 1 specified in [ADR 0001](../docs/storage/0001-immutable-storage.md);
stage 2 implemented in [storage boundaries](../docs/storage/0002-storage-boundaries.md);
stage 3 implemented as an [inactive foundation](../docs/storage/0003-immutable-record-storage.md).
Stage 3 merged in PR #24 (`10809d9`). Review follow-ups are recorded below;
the four live web fixes are implemented, and three foundation fixes remain open.
Stages 4–11 remain planned;
this document does not itself authorize implementation.

## Review follow-ups

Reviewed local commit: `58bb03d` (`feat: add immutable storage foundation for
Linux and macOS`), subsequently merged in PR #24. Targeted reproductions exposed
seven gaps despite the existing Go/web suites passing. The four web findings below
are now addressed; the three Go foundation findings remain open. This list records work to do;
adding it does not activate storage or authorize publishing a release.

Prioritize the live web data-loss issues, then finish the stage 3 corrections
before stage 4 integration. The web issues predate this commit. `outbox.js` is
intentionally inactive, so it does not protect the current JSON save flow. Fix the
live JSON client without waiting for immutable transport or silently expanding
stage 3 into migration. Preserve Linux/macOS support and Omarchy coverage.

### Live web app — repair before immutable storage integration

- [x] **P1 — Reject stale reload results.** `web/app.js`, `reload`: a download
  started while clean replaces `state.data` and clears `dirty` even when an edit
  occurred during the request. Track mutation/request generations and discard
  stale responses without replacing local edits or their revision context.
  **Acceptance:** defer a download, edit locally, then deliver the old response;
  the edit remains dirty and recoverable. Cover overlapping reload responses too.

- [x] **P1 — Acknowledge only the version actually uploaded.** `web/app.js`,
  `doSave`: an older upload unconditionally clears `dirty` after a newer edit.
  Capture the uploaded snapshot and mutation generation; retain dirty state and
  schedule the newer version when an edit occurred in flight.
  **Acceptance:** upload edit A, make edit B before A completes, then finish A;
  B remains unsaved until its own successful upload, and focus/periodic reload
  cannot erase B. Cover upload failures and concurrent maintenance saves.

- [x] **P1 — Keep Dropbox revision protection on every write.** `web/sync.js`,
  `downloadOnce`/`uploadOnce`: every download HTTP 409 becomes an empty store,
  missing revision metadata is accepted, and a null revision selects unconditional
  `overwrite`. Recognize only an explicit path-not-found response as absence;
  validate successful download/upload revision metadata; use create-only mode
  for a confirmed new file and revision-checked updates for existing files.
  **Acceptance:** other 409 errors and missing/malformed metadata fail without
  clearing state; a file created by another client between not-found and upload
  causes a visible conflict, never an overwrite. Update the existing test that
  currently treats any 409 as a missing file.

- [x] **P2 — Create recovery data before destructive reload.** `web/app.js`,
  reload menu/recovery handling: the confirmation promises a recovery copy,
  but snapshots are currently written only on conflict. After an ordinary network
  failure, forced reload discards edits without creating that copy. Persist the
  current unsaved snapshot before replacement and handle storage failures visibly.
  **Acceptance:** after a failed upload, forced reload preserves the exact local
  edit in downloadable recovery; quota/write failure blocks replacement instead
  of claiming recovery succeeded. Cover dirty-state handling on disconnect and
  authentication failure as well.

Implemented with app-level asynchronous orchestration tests in `web/app.test.js`
and transport regression tests in `web/sync.test.js`. Domain and transport unit
tests alone did not catch the original races. Use fake requests and isolated browser storage,
never real Dropbox credentials or task files. Keep future immutable transport's
matching requirements in stage 7; these repairs protect the current application.

### Stage 3 — correct the committed foundation before integration

- [ ] **P2 — Strict creation timestamp validation.** `recordstore/record.go`,
  `tasks`: `time.Parse(time.RFC3339Nano, ...)` accepts malformed timestamps such
  as offsets `+24:00` and `+01:60`, a one-digit hour, or a comma fractional
  separator. The reviewed JavaScript date parser rejects these same inputs.
  Add explicit format/range validation while preserving valid legacy timestamp
  spelling, precision, and offsets.
  **Acceptance:** shared fixtures reject these malformed values and accept valid
  legacy values consistently; `Parse`/`Queue` never acknowledge invalid records.
  Reuse these fixtures in stage 4's production JavaScript validator.

- [ ] **P2 — Bound directory durability checks to a safe owned boundary.**
  `recordstore/store.go`, `syncDirectories`: syncing every ancestor up to `/`
  makes a save fail below a traversable but unreadable directory even when the
  pending directory supports writing and syncing. Establish a durable store-owned
  boundary with process-safe initialization; do not merely ignore sync failures
  or reintroduce the interrupted-directory-creation race fixed in stage 3.
  **Acceptance:** a usable store below a traverse-only ancestor accepts and
  persists edits; directory-creation interruption, concurrent initialization,
  retry, and genuine file/directory sync failures still preserve acknowledged
  operations. Run native Linux/macOS checks.

- [ ] **P2 — Recover pending edits independently of synced-root health.**
  `recordstore/store.go`, `ScanPending`: validation of both paths returns early
  when the synced root is damaged, hiding intact acknowledged pending records.
  Return valid local pending records alongside the root issue while keeping
  publication blocked until its destination is safe.
  **Acceptance:** queue an edit, replace the synced root with a regular file,
  then restart; pending recovery still returns the exact edit and reports the
  root error. The damaged root and local recovery data remain untouched.

### Verification and handoff for these fixes

Use focused reproductions as regression tests, then run the existing Go suite,
vet, race suite, and web suite. Run release inventory/GoReleaser checks when their
files change. Keep fixes reviewable, record which checkboxes are completed, and
leave stage 4–9 runtime activation deferred. Web verification uses deferred fake
requests and an isolated DOM/storage adapter running the real app and transport.
Real Dropbox and browser power-loss testing are not claimed.

## Outcome

Keep doitdoit local-first, free to distribute, and usable without an application
backend. Replace competing whole-file saves with immutable changes transported
through user-owned storage. Build reliable recovery and history into that model.

Ship activation in the next major release (determine the actual version from
release tags when preparing it). Storage schema versions are separate from the
application release version. Do not create or push a release tag without an
explicit request.

An ordinary upgrade must be invisible: no migration wizard, new path to choose,
manual export/import, repeated setup, or task workflow changes. Existing storage
configuration, themes, retention choices, task IDs, dates, order, and completion
state carry forward automatically. The web client discovers the same storage
using its existing Dropbox path and authorization where permissions allow it.
Surface actionable errors and genuine conflicts; invisibility must never mean
silently dropping edits or claiming migration/sync succeeded prematurely.

## Architecture and constraints

- Desktop support is Linux and macOS only (`amd64` and `arm64`), with Omarchy
  a first-class Linux platform. Preserve theme detection, opt-in live updates,
  and managed hook safety. Do not implement Windows storage or release fallbacks.

- Retain the configured JSON path as the discovery anchor. A deterministic
  sibling directory, `<configured-path>.store/`, holds versioned immutable records.
  ADR 0001 specifies naming and Dropbox discovery.
- Each operation or atomic operation batch has a globally unique ID, task IDs,
  schema version, operation payload, causal predecessors, and origin metadata.
  Timestamps support history; they do not decide causality or conflict winners.
  Device identities live locally, never in shared machine configuration.
- Publish complete records atomically under unique names. Never have multiple
  clients append to a shared log. Deduplicate by record ID/content, validate
  records, and wait for missing dependencies before applying dependent changes.
  Directory arrival order and modification times are not an ordering protocol.
- Implement equivalent deterministic replay in Go and plain JavaScript, with
  shared fixtures. A local cache is disposable and rebuildable. Start without
  SQLite or additional runtime dependencies; revisit only with measured need.
- Preserve independent changes and expose competing intentions. Deletion is an
  explicit record, undo creates a compensating operation, and resolution refers
  to the conflicting versions. Never silently use last-write-wins for task text.
- Keep atomic replacement, local `0600` files, backups, external-change checks,
  and Linux/macOS filesystem durability. Immutable history supplements
  backups; it is not a substitute for recovering an accidentally deleted folder.
- The web companion remains static and self-contained; OAuth credentials remain
  browser-local. No hosted coordination service is introduced.
- Sync remains eventual. Immutable records reduce overwrite loss but cannot
  force Dropbox/Drive to deliver promptly. Generic folder mode can report local
  durability and observed changes, not remote upload confirmation.

## Migration contract: release-blocking requirements

1. Read and validate legacy bytes before any maintenance or rollover. Preserve
   an exact durable recovery copy before activating new storage. Invalid or
   inaccessible input must not become an empty task collection.
2. Import through an idempotent, restartable protocol with content-derived
   snapshot/import identities. Equivalent snapshots imported on two offline
   devices must not duplicate tasks or history. Define canonicalization identically
   in Go and JavaScript, including dates and legacy ID repair.
3. Support different snapshots migrating independently. Never choose the first
   device as an implicitly authoritative winner. Preserve both snapshots and
   reconcile using known baselines; ambiguous differences become visible
   conflicts. Absence from a stale snapshot is not proof of deletion.
4. Publish and verify all required import records before an activation record.
   A synced activation record may arrive before its dependencies: readers must
   wait safely, retain pending edits, and resume without re-importing stale data.
   Cache or configuration updates cannot be the sole evidence of completion.
5. Preserve the legacy JSON as a compatibility input during rollout. New storage
   becomes authoritative; do not continuously overwrite that input with a
   generated projection. Observe later legacy writes and preserve them as
   immutable snapshots. Reconcile deltas against a known imported baseline;
   request resolution when the baseline or intent is ambiguous. Test conflict
   copies as well as changes to the original path, without importing unrelated
   files by filename guesswork.
6. Be explicit about the limit: an unmodified old client cannot understand new
   records, and folder sync can hide or overwrite edits before any new client
   observes them. Promise zero-setup migration for upgraded clients, not seamless
   indefinite bidirectional operation with old binaries. Release notes instruct
   users to update their devices; the application requires no migration choices.
   Do not claim guaranteed recovery of unobserved old-client writes.
7. JSON export remains available in the existing date-bucket format, including
   `Future`, at an explicit destination. It is not a second writable authority.
   Provide recovery/downgrade export containing the latest state; restoring the
   original backup alone would discard post-migration changes.
8. Imported completion times and prior activity remain unknown. Do not fabricate
   historical events from current state or count imports as newly completed work.

## PR stages

Each stage is independently reviewable and includes its own behavioral tests.
PRs 1–8 can land behind an internal development gate with legacy storage still
the public default. Do not expose a migration toggle as a normal user workflow.
PR 9 activates the complete path only for the major release. PRs 10–11 can ship
as later minor releases without delaying the storage reliability improvement.

### PR 1 — Specify the storage and compatibility protocol

Delivered: [ADR](../docs/storage/0001-immutable-storage.md),
[record schema](../docs/storage/record.schema.json), and
[shared examples](../docs/storage/fixtures/README.md). Runtime activation is deferred.
The protocol conservatively imports unequal late legacy snapshots as alternatives:
remembering a prior local hash does not prove a remote writer's baseline.

Deliver an architecture decision and versioned record schema covering task
identity, imports, causality, atomic batches, order, moves, delete/edit conflicts,
resolution, unknown schema handling, discovery, and activation. Define the
legacy-import bridge and cache boundary above precisely enough for two separate
implementations. Include a compatibility matrix for old/new TUI and web clients.

Acceptance: shared example fixtures cover concurrent imports of equal and unequal
snapshots, missing dependencies, and late legacy writes. Resolve the inability
to guarantee mixed-version sync explicitly before coding around it.

### PR 2 — Extract storage boundaries without changing behavior

Delivered: `taskstore.Store` and its legacy JSON adapter; task operations independent
of Bubble Tea; TUI/CLI/reload/maintenance/move integration; and the web JSON store
boundary. [Write-path inventory and verification](../docs/storage/0002-storage-boundaries.md).
Paired snapshots retain the revision of data actually loaded/saved through an
intervening write. No new storage format or migration is activated.

Separate task operations and storage from Bubble Tea state. Route startup, CLI
capture, reload, maintenance, and configuration moves through a common boundary.
Keep the existing JSON adapter and current same-bucket conflict behavior intact.
Introduce an equivalent boundary in the web client.

Acceptance: existing lifecycle and persistence tests pass unchanged; every write
path is accounted for and uses the existing safeguards.

### PR 3 — Add immutable record storage and durable pending edits

Delivered: standalone `recordstore` validation, immutable local publication, durable
pending inspection/retry, exact-byte backups, and an account/store-scoped browser
outbox. [Contracts, verification, and limitations](../docs/storage/0003-immutable-record-storage.md).
These components remain inactive; replay, migration, and application wiring are
later stages. The existing JSON adapter remains the public default.

Implement local publication, validation, discovery, retry, and record deduplication.
Serialize or safely coordinate multiple processes on one machine. Ignore incomplete
temporary writes; preserve malformed records and report errors without discarding
valid history. Persist pending edits before acknowledging success. Add the web's
durable browser outbox with explicit quota/write failure handling.

Acceptance: interrupted writes, disk full, permission failures, duplicate delivery,
restart/retry, and simultaneous CLI/TUI writes do not lose acknowledged operations.
Cleared browser storage remains a documented limit for edits not yet uploaded.

### PR 4 — Implement deterministic replay and conflict state

Implement create, edit, completion/reopen, move, ordering, delete, and resolution
in both languages. Retain unresolved alternatives as data. Preserve existing
same-bucket conflict rules initially; finer automatic merges are PR 10's explicit
policy change. Model multi-task commands as atomic batches. Rebuild disposable
caches from complete records and retain pending dependencies.

Acceptance: both implementations produce the same state and conflict set for
shared fixtures under shuffled and duplicated delivery, including concurrent
ordering, delete/edit, missing parents, and device clock skew.

### PR 5 — Implement automatic migration and late legacy reconciliation

Build the restartable import, recovery copy, activation, and legacy observation
protocol from PR 1. Wire discovery to the existing configured path. Preserve all
configuration and data semantics, including undecided retention. Handle duplicate
or missing legacy task IDs deterministically and visibly preserve ambiguous data.

Acceptance: fault injection at every migration boundary, two offline migrations,
unequal initial snapshots, late old-client edits, missing sidecars, and repeat
launches satisfy the migration contract. No successful migration needs a prompt.

### PR 6 — Integrate TUI, CLI, reload, and storage management

Connect the new adapter behind the internal gate. Keep existing controls and
normal display stable. Make reload reconcile incoming operations without losing
typing or pending mutations. Add actionable conflict recovery using retained
versions. Extend `config move` to move/verify the entire store and recovery data,
with destination collision protection and recoverable interruption handling.
Provide explicit JSON export and cache rebuild/recovery commands.

Acceptance: add/edit/complete/move/delete/undo survive restart and external arrival;
move and export preserve the latest state. Normal upgrade startup has no extra
setup. No path accidentally falls back to writing only the old snapshot.

### PR 7 — Integrate static web and Dropbox transport

Use immutable create-only uploads with verified ID/content matches on retries;
list records with pagination and reconcile downloads/outbox items. Distinguish a
missing file from authorization, malformed response, and other API errors.
Complete browser migration from the configured legacy path. Handle stale cached
web code, incompatible schemas, expired sessions, and offline restart. Scope local
state to the account/store so switching accounts cannot replay another outbox.

Acceptance: browser/TUI integration scenarios preserve edits through disconnection,
token expiry, retry, duplicate uploads, and partial downloads. Existing authorization
works where its scopes permit; any required scope change is identified before
release. Web assets and service-worker updates cannot mix incompatible versions.

### PR 8 — Add trustworthy history and preserve retention semantics

Expose a basic task history/recovery view using recorded actions. Record completion
and reopening times plus the local calendar context needed for daily reporting.
Exclude imports, retries, and automatic maintenance from user activity counts.
Apply existing retention choices to task visibility and define historical data
retention separately: do not silently turn a pruning preference into indefinite
retention of deleted task contents.

Acceptance: existing retention behavior remains consistent across clients; unknown
imported history stays unknown. Define and test an explicit history purge path
and its stale-device behavior. If safe physical purge cannot fit this PR, settle
and communicate that limitation before activation; do not imply hiding is erasure.
Defer automatic distributed log compaction until it has a proven protocol.

### PR 9 — Validate and activate the next major release

Run the migration/sync matrix end to end against representative legacy fixtures,
including large histories and different local time zones. Exercise recovery after
lost caches, missing records, and partial folder restoration. Set a startup/replay
performance budget from measured fixtures and meet it before activation.

Enable new storage by default together with the compatible web release. Coordinate
web deployment so it does not silently migrate production stores before the major
CLI release is available. Update README storage, backup, downgrade, retention,
compatibility, and release documentation. Keep GoReleaser/workflows consistent if
release behavior changes. No new application version file or packaging system.

Acceptance: zero-setup upgrade and fresh install pass on supported platforms;
recovery is demonstrated; known mixed-version limits are documented. Release
preparation is separate from authorization to create/push an annotated tag.

### PR 10 — Narrow conflicts to actual competing task changes

Explicitly revise the documented same-bucket conflict policy and corresponding
project guidance. Merge independent task changes and independent fields only when
the protocol proves compatibility. Continue surfacing competing titles, delete/edit,
and ambiguous ordering. Provide a task-focused resolution view in both clients.

Acceptance: concurrent edits to separate tasks in Today combine; genuine competing
intentions retain both versions until resolved. Resolution converges on every client.

### PR 11 — Add stats and reduce maintenance writes

Build completion trends, completion latency, and rescheduling counts from real
events, with defined reopen/recomplete counting and calendar rules. Separate user
rescheduling from automatic rollover. Evaluate derived rollover as a separate
behavior change within this PR only if small; otherwise split it into another PR.
Preserve original scheduling history and keep CLI/web lifecycle behavior aligned.

Acceptance: totals remain identical under replay/duplicate delivery, imports do
not inflate activity, and time-zone changes have documented behavior. Derived
rollover, if included, preserves existing Today/Future presentation and ordering.

## Verification and handoff gates

- For Go changes: format with `gofmt`, run `go test -count=1 ./...` and
  `go vet ./...`; persistence, reload, concurrency, and release changes also run
  `go test -race ./...`. Use the pinned mise toolchain. CI and release gates run
  on Linux and macOS;
  include the isolated Omarchy integration tests and a live theme-switch smoke
  test before release.
- Run `node --test web/*.test.js` for web changes and shared protocol fixtures.
  Add behavior-focused cross-client/fault scenarios, not only unit replay tests.
- Tests use temporary stores and isolated HOME; never access the real task file
  or `~/.doitdoit_config.json`. Keep generated binaries/caches/dist out of changes.
- Update third-party notices and run release inventory checks if dependencies,
  embedded themes, or distributed files change. Run `go mod tidy` only when
  imports/dependencies require it.
- Each PR records its delivered behavior, validation, compatibility impact, and
  remaining limitations. No stage removes recovery material as incidental cleanup.

## Deferred work

Automatic distributed compaction, additional direct provider integrations, a hosted
service, CRDT library adoption, SQLite analytics caches, and indefinite old-client
bidirectional compatibility are outside the initial major release. Optimize file
counts only after measuring them; safe immutable batching can precede compaction.
