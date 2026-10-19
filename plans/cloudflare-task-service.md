# Cloudflare task service, MCP, and Go TUI

Status: accepted architecture; implementation planned.
Decision date: 2026-10-19.

## Direction

Build our own task manager in this repository. Keep the Go Bubble Tea TUI and
CLI, introduce a TypeScript Cloudflare Worker, and make Cloudflare D1 the
authoritative database. The Worker exposes an HTTP task API and an authenticated
remote MCP endpoint backed by the same task operations.

This plan supersedes the future architecture in
[major-version-storage.md](major-version-storage.md). That document continues to
describe the safety requirements of the existing JSON implementation until the
explicit cutover. This is a new storage decision, not a revival of the abandoned
immutable-record backend or the per-task JSON proposal.

Writing this plan does not authorize provisioning, deployment, import of real
tasks, or a release. Implement and verify locally first; make any later deployment
or migration concrete and reviewable.

## Goals and boundaries

- Own the task model, service, TUI, and MCP tools.
- Use one hosting provider, Workers plus D1, without maintaining a server machine.
- Keep macOS/Linux amd64/arm64 releases, existing themes, Omarchy detection and
  opt-in live updates, keyboard navigation, notes, scheduling, and ordering.
- Preserve explicit scheduled dates. Calculate overdue status for display.
- Prevent unrelated tasks from conflicting and prevent stale writes from
  silently overwriting dates, notes, completion, or deletion.
- Provide recoverable deletion, task history, safe retries, and portable exports.
- Start with one owner, online writes, and a small relational schema.
- Do not add a separate AI model, agent runtime, vector database, queue, Durable
  Object, or second database unless implementation evidence requires one.
- Defer offline editing/sync, collaboration, recurring tasks, attachments,
  projects, labels, and a separate deadline field. MCP is an interface to the
  service; it is not the storage or synchronization mechanism.

## Architecture

```text
Local Go TUI / CLI ---- HTTPS task API ---+
                                        +-- Task operations -- D1
Assistant MCP clients -- HTTPS /mcp -----+
```

Both adapters call the same application operations. Transport handlers contain
authentication, schema decoding, and response mapping, not separate task rules.
Only the Worker accesses D1; clients receive no Cloudflare management credentials
or database access tokens.

Suggested repository layout:

- service/: TypeScript Worker, HTTP and MCP adapters, task operations, SQL
  migrations, Wrangler configuration, package lockfile, and backend tests.
- api/: versioned schemas, documented errors, and shared contract fixtures.
- taskclient/: Go HTTP client, task/revision types, and contract tests.
- model/: existing Bubble Tea UI adapted to task operations and server reads.
- taskstore/: legacy JSON implementation plus explicit import/export conversion.

Do not force the new provider through Store.Load/Save: that interface represents
whole-file replacement. Introduce a task-oriented client boundary and retain the
old interface for legacy operation/import until cutover. Do not make the TUI call
MCP internally; the HTTP API is its direct application interface.

## Task semantics

A task has a stable ID, title, plain-text notes, completion status, optional
scheduled date, ordering key, revision, and creation/update/completion/deletion
timestamps. IDs are server-generated collision-resistant identifiers.
Server timestamps record accepted changes, not which competing edit should win.

Dates are local-calendar YYYY-MM-DD values; timestamps are UTC instants. Store
an owner timezone, initially configured explicitly. Clients and MCP tools use
that timezone for Today and date filters; test midnight and DST boundaries.
Undated tasks have a null scheduled date, not a special database bucket.

Unfinished tasks with a date before Today appear in an Overdue section while
retaining their original date. Navigation, startup, midnight, and reads must not
reschedule tasks. Completing or reopening a task does not change its date.
Scheduling accepts explicit valid past dates as well as today/future dates;
remove the existing clamp to today in the new path.

Completion grouping and date grouping are read/view concerns. Reordering changes
an explicit ordering key, with a stable task-ID tie-breaker. Establish the exact
ordering scope (scheduled date plus completion section) in the contract. A
reorder uses before/after neighbour IDs; the server calculates positions. Cover
concurrent reorder and position compaction without renumbering the whole board
for ordinary edits. Do not use floating-point positions without precision tests.

Retention defaults to forever. If enabled later, only completed tasks qualify,
based on known completion time, followed by a documented recovery period before
permanent purge. Never delete unfinished overdue tasks. Imported tasks with
unknown completion time do not qualify for automatic age-based deletion.

## Database and write guarantees

Initial logical tables:

- owners: allowlisted identity, timezone, and account settings.
- tasks: owner, task fields, revision, and timestamps.
- task_events: ordered cursor, task ID, operation ID, actor/client, prior/new
  revision, and before/after values needed for history and undo.
- operation_receipts: owner/client operation ID, canonical request hash, result,
  and acknowledgement metadata.
- import_runs: source fingerprint, mapping/report, progress, and verification.

Index common owner/status/date/order queries and event cursors. Task history
contains private task data and receives the same access controls as tasks.
Deletion sets deleted_at and advances the revision; restoring is an explicit
operation. Retain history/receipts under a documented policy, never an implicit
cleanup that permits an expired request to create duplicates.

Each mutation must:

1. Authenticate the owner and validate typed, bounded inputs.
2. Check an expected task revision, or create-only identity for creation.
3. Atomically commit the task change, history event, and operation receipt.
4. Return the canonical task and revision after the successful commit.

A repeated operation ID with the same payload returns the original result.
Reuse with a different payload fails. Define the receipt lifetime and retry
window before permitting receipt cleanup.

Different tasks can be updated independently. For v1, any stale revision on the
same task returns a conflict with the current task and attempted change. Do not
auto-select by timestamps or silently retry against the newer revision. A
follow-up resolution uses a new operation ID and the freshly reviewed revision.
Field-aware merging can be considered separately after these guarantees hold.

D1-specific feasibility is an early gate: use supported prepared statements,
transactional batches, and/or SQL triggers/constraints. A conditional UPDATE
affecting zero rows does not itself abort a batch. Prove that stale writes cannot
leave history or successful receipts, and injected failures cannot leave partial
state. Do not assume an interactive BEGIN/read/write/COMMIT connection is
available across Worker calls. Select and document the SQL pattern after the
local concurrency prototype passes.

Keep initial reads on the primary path; defer read replicas. Board reads and
paged change feeds must have an explicit consistency contract: a snapshot and
cursor cannot skip intervening accepted changes. Clients must tolerate replayed
events and detect a cursor too old to resume, then refresh safely.

## HTTP API and MCP

Version the API under /v1 and expose /mcp separately. Start with:

- List/filter/search tasks; get one task.
- Create task; edit title/notes; set completion explicitly.
- Reschedule or clear schedule; reorder.
- Delete, restore, and read task history.
- Fetch board/change feed and export owner data.

Keep import as an explicit CLI/administrative workflow initially. Do not expose
arbitrary SQL, deployment administration, or bulk destructive operations to MCP.

MCP tools use stable task IDs, explicit fields, expected revisions, and operation
IDs for writes. Avoid toggle operations: retries must not reverse their own
effects. Return structured results and structured conflict details, not just
prose. MCP tools and HTTP routes share validation and authorization tests.
No assistant has to run on the server.

## Authentication

Authentication is part of the first vertical slice, not a later addition.
Use established OAuth/OIDC libraries and a supported identity provider instead
of implementing cryptography. Start with one allowlisted owner identity;
authentication alone must not permit other accounts to access tasks.

The first spike selects a concrete identity provider and OAuth integration,
then proves remote MCP login and tool calls with the intended assistant clients.
Validate required metadata/discovery, client registration, PKCE, redirect URI
validation, issuer/audience checks, expiry, refresh, scopes, and revocation.
A browser-protected dashboard or Cloudflare Access login is not, by itself,
proof that MCP client authorization works.

Use browser authorization with PKCE and a loopback callback for the desktop CLI
where supported by the selected integration. Persist local credentials with
owner-only permissions; never log tokens. Clearly document any initial manual
credential flow and do not make it the unexamined permanent design.

Keep Cloudflare deployment credentials in CI/secrets, distinct from application
credentials. Test unauthenticated requests, wrong owners, expired tokens, and
scope failures through both API and MCP. No public task endpoint or anonymous
production mutation route.

## TUI and CLI behaviour

Reuse the board, notes editor, keyboard workflows, themes, and platform support.
Replace mutate-snapshot/persist orchestration with task commands and explicit
acknowledgements. The service owns canonical task rules; Go owns presentation,
input, and navigation.

Perform network I/O through Bubble Tea commands. Do not block the update loop.
Track request identity and task revisions so stale responses/reloads cannot
replace newer edits. Refresh on demand and with modest change-feed polling;
defer a persistent push channel until needed.

Display disconnected, saving, saved, and failed states honestly. V1 does not
accept new offline mutations. If connectivity fails during an in-flight edit,
preserve the attempted command/draft locally in private recovery storage and
retry the same operation ID after reconnecting to establish its outcome.
A timeout is not proof that the write failed. Never call a change saved before
acknowledgement or discard notes on a failed request.

Undo sends an inverse operation guarded by the revision produced by the original
change. Do not restore an entire old board and overwrite intervening edits.

## Import and cutover

No automatic migration and no dual writes.

1. Freeze a source snapshot explicitly and preserve exact JSON bytes and its
   fingerprint. Read using strict decoding; reject unknown data rather than
   silently dropping it.
2. Dry-run an import and report counts, invalid dates, ambiguous/missing IDs,
   completed tasks, ordering, and pending recovery material.
3. Preserve title, notes, completion, creation time, and dates without rollover,
   pruning, or completion-time invention. Dated buckets map to scheduled dates;
   Future plus a valid due_date maps to that date. A bucket/due_date disagreement
   is reported for explicit mapping. Do not silently drop invalid dates.
4. Preserve valid unique legacy IDs or record a stable old-to-new mapping.
   Require reviewed handling of ambiguous IDs; never deduplicate by title.
5. Import with resumable operation IDs and an import-run marker. Large imports
   can use bounded batches, remain unpublished until verification, and must not
   expose a partial board as a completed migration.
6. Verify field-level data, task counts, order, and portable export. Pending
   drafts/conflict copies are preserved and explicitly reviewed, never assumed
   to be current or silently merged.
7. Explicitly switch the client configuration to the service, keeping the
   original JSON and recovery files untouched.

Rollback before service edits can select the original backend. After service
edits, rollback requires a fresh reviewed export; the original JSON is stale.
During transition, clients must clearly identify which backend they are using.

The experimental Dropbox web app stays separate until deliberately adapted.
Do not let it continue writing to what users believe is the cloud authority.
A later static web client can call the same task API; no D1 secret enters it.

## Delivery milestones and acceptance gates

### 1. Contract and infrastructure spike

Add the service skeleton, local Wrangler/D1 environment, initial SQL migrations,
API schemas, and a Go client proof of concept. Prove authenticated MCP access
and the D1 atomic revision/history/receipt pattern. Document selected auth
provider, credential flow, ordering representation, and cursor consistency.
Use fixtures only. Gate: two writers cannot silently overwrite the same revision;
an acknowledged mutation always has matching history and retry receipt.

### 2. Complete service operations

Implement task operations, history, deletion/restore, ordering, filtering,
change feeds, and export. Add owner isolation, limits, useful errors, and
idempotency. Gate: API and MCP contract tests exercise the same successful,
stale, unauthorized, and retried actions.

### 3. Go client and TUI integration

Add explicit backend selection and service login/configuration. Adapt ordinary
actions, notes, undo, reload, and save feedback. Add the Overdue view and remove
rollover writes from the service path. Gate: complete create/edit/reschedule/
complete/delete/restore/undo workflows, including delayed responses, network
loss, and retained drafts, work on supported desktop platforms.

### 4. Import, recovery, and web transition documentation

Implement dry-run, resumable import, verification/export, and explicit cutover.
Document the old web companion boundary and rollback procedure.
Gate: representative legacy fixtures migrate without lost notes or dates;
repeated/interrupted import creates no duplicates; the source remains intact.

### 5. Deployment and operational handoff

Prepare isolated development/staging/production configurations, least-privilege
deployment credentials, schema migration instructions, health checks, bounded
request logs without task contents/tokens, rate/body limits, and cost visibility.
Prefer stateless MCP handlers; persistent session state cannot live only in a
Worker instance.

Configure D1 recovery and independent exports with documented retention. Test a
restore into a separate database and export comparison. Separate backups from
same-database task history. Use additive schema changes and compatible Worker
rollouts; rolling back code must not assume SQL migrations can be reversed.

Gate: demonstrate staging authentication, real client interoperability,
concurrency, retry recovery, export/restore, and deploy rollback. Then obtain
authorization for production deployment and real-data cutover. No release tags
or pushes are implied by this plan.

## Verification and repository updates

Backend tests run locally against the Workers/D1 test environment, not a
developer's database. Include concurrent writers, stale dates, duplicate retries,
failure injection between mutation steps, deletes versus edits, reorder races,
authentication, history, and consistent change-feed pagination.

Go checks remain go test -count=1 ./..., go vet ./..., and go test -race ./...
for persistence/reload changes. Preserve isolated HOME/config and Omarchy tests,
Linux/macOS coverage, and supported release architectures. Add service lint,
typecheck, migrations, and tests to CI without replacing existing gates.

Maintain shared contract fixtures for date/null semantics, Unicode/notes,
ordering, revisions, and error decoding across TypeScript and Go. Test that
opening the UI and crossing midnight generate no task writes and that incomplete
overdue tasks survive retention.

As implementation lands, update AGENTS.md, README, CLI help/configuration,
THIRD_PARTY_NOTICES.md, service setup docs, and CI/release documentation. Keep
legacy safety guarantees until that path is retired deliberately. Do not edit
generated binaries, dist/, or .cache/. The existing static web app retains its
no-build/no-remote-code rules until an explicit replacement decision.

## Primary references

Checked when writing this plan; pin SDK versions and recheck interfaces during
the first spike.

- [Cloudflare remote MCP servers](https://developers.cloudflare.com/agents/guides/remote-mcp-server/)
- [MCP authorization on Cloudflare](https://developers.cloudflare.com/agents/model-context-protocol/authorization/)
- [D1 prepared statements and transactional batches](https://developers.cloudflare.com/d1/worker-api/d1-database/)
- [D1 Time Travel and backups](https://developers.cloudflare.com/d1/reference/time-travel/)
- [Workers pricing](https://developers.cloudflare.com/workers/platform/pricing/)
- [D1 pricing](https://developers.cloudflare.com/d1/platform/pricing/)
