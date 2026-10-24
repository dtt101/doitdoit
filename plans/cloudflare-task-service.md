# Cloudflare task service, MCP, and Go TUI

Status as of 2026-10-24: the expanded Rust service and migration 0004 are deployed.
The Go TUI/CLI service mode and explicit import/recovery tooling are implemented
in the working tree. Local checks and the independent D1 restore drill pass.
The extended production smoke passes, including desktop refresh and revocation.
ChatGPT and TUI edits pass in both directions by user report. A Gemini consent
navigation fix is deployed; user confirmed Gemini sign-in and task listing.
Broader TUI/platform and assistant checks remain open; real-data import and
cutover have not occurred.

Architecture accepted: 2026-10-19. Work branch: `feat/cloudflare-task-service`.
Implementation checkpoint: expanded service operations, Go TUI/CLI integration,
explicit import/recovery tooling and service-mode review fixes. The earlier
consent callback fix is recorded in `5ed3f22`.

## Stage overview

| Stage | Status | Remaining acceptance gate |
| --- | --- | --- |
| 1. Contract and infrastructure spike | Contracts settled; Codex verified | ChatGPT/Claude and natural client refresh checks |
| 2. Complete service operations | Implemented, locally tested and deployed; hosted smoke passed | Further client acceptance tracked below |
| 3. Go client and TUI integration | Implemented; hosted desktop login/refresh verified | Hands-on TUI/macOS verification |
| 4. Import, recovery and web transition | Tooling and fixture tests implemented | Explicit reviewed real-data import stays separate |
| 5. Operational handoff and cutover readiness | Backup, restore and code rollback verified | Off-machine backup operation and remaining client checks |
| Later: plugin packaging | Deferred | Direct MCP remains the current route |

Stages retain the detailed acceptance gates below. Initial production deployment
was deliberately brought forward from stage 5 for hosted testing. Deployment
does not mean the remaining stages are complete.

## Completed and verified

- [x] Rust/WASM Worker with D1, HTTP task API and authenticated stateless MCP.
  Application behavior, OAuth and MCP are all Rust; Node is development/test tooling.
- [x] Create/get/list/update, title/notes, explicit date set/clear and explicit
  completion status; no rollover writes in the service.
- [x] Per-task revisions, visible stale-write conflicts, identical retry receipts
  and atomic task/history/receipt writes. Local tests cover races and rollback.
- [x] GitHub owner allowlist, public-client registration, S256 PKCE,
  browser-bound consent, resource binding, rotating refresh credentials and revocation.
- [x] Public auth rate limits and hourly expired-state cleanup; spent credentials
  remain available for replay detection while their grant is live.
- [x] Local GitHub smoke workflow using the standalone Go HTTP client.
- [x] Production Worker, D1 migrations 0001–0003, private OAuth secrets and
  direct Codex MCP connection on the user-confirmed Workers Free plan.
- [x] Live health/discovery, anonymous API/MCP rejection and foreign-Origin rejection.
- [x] Production MCP creation, date update, identical retry, stale-date conflict,
  completion and final read-back. The newer date survived both conflict and completion.
- [x] Production OAuth refresh rotation, wrong-resource rejection, unchanged scopes,
  renewed MCP access and revocation of both access generations and the latest refresh.
- [x] Consent callback redirect fix, reproduced in an isolated browser, regression
  tested and deployed. The policy permits GitHub and the selected registered
  callback origin so an existing GitHub session can return directly to the client.
- [x] Expanded service: delete/restore/history, paged filter/search, neighbor reorder,
  compaction, fixed-cursor board/export, changes and HTTP-only atomic staged import.
- [x] Migration 0004 upgrade test preserves exact old tasks, events and receipts;
  rollback tests and competing-gap reorder tests pass.
- [x] Go desktop PKCE login, private rotating credentials with cross-process refresh
  locking, explicit backend selection, HTTP task operations and service TUI mode.
- [x] Overdue presentation preserves dates; revision-guarded undo, retained uncertain
  commands, invalid-draft protection, conflict review and deleted-task restoration.
- [x] Strict frozen-source import planning, stable ID mappings, preserved recovery
  artifacts, resumable staging/publication and field-level export verification.
- [x] Full production SQL backup restored into a separate unbound D1 database;
  every row in all 10 tables matched. Temporary database deleted after verification.
- [x] Production code rollback to `26470207-c46f-4293-a5f0-64fbcf5230a3`
  against migration 0004: health and existing authenticated MCP read passed with
  the canonical task unchanged. Latest Worker restored at 100% afterward.
- [x] `make backup-production` creates unique private SQL/log files and keeps
  signed export URLs out of terminal output. Post-migration backup command verified.
- [x] Latest local verification: 10 Rust tests, 44 Worker integration tests,
  production dry-run, full Go tests/vet/race and 48 static web tests passed.
- [x] Desktop cross-builds pass for macOS amd64/arm64 and Linux arm64;
  Linux amd64 tests/builds run locally. Cross-builds do not replace hands-on UI checks.
- [x] Extended production smoke passed on 2026-10-22: GitHub desktop login,
  credential refresh/rotation/private persistence, reorder, soft delete/restore,
  history, change feed, discovery of all 11 MCP tools, MCP history read-back,
  completion with dates preserved, portable export field equivalence and revoked
  access rejection. Both disposable test tasks were completed; the temporary
  login was revoked and no smoke credentials were retained.

The earlier refresh/revocation check tested the server through a separate read-only
OAuth client. Automatic refresh by Codex itself has not yet been verified.
ChatGPT's later idle-session check is recorded below; Claude remains unverified.

## Next work, in order

### ChatGPT acceptance progress — 2026-10-24

- User added the hosted MCP URL as a custom server in ChatGPT and reported
  successful listing of the three completed service test tasks.
- User reported creation and read-back of `[ChatGPT test] shared task`, open,
  scheduled `2026-11-01`, with note `Created through ChatGPT.` Independent MCP
  read-back confirmed those fields at revision 1 (task
  `c7ae9078-6997-4fb9-9e9e-37bbbe66331b`).
- No sign-in prompt appeared. A fresh anonymous production `tools/list` request
  returned HTTP 401 with the OAuth discovery challenge. Source requires a valid,
  unexpired, unrevoked owner access token. Existing authorization reuse is
  possible, but the origin of ChatGPT's authorization has not been established.
- User confirmed the service TUI displayed the same test task. ChatGPT then
  completed it and reported read-back with the original date and note preserved;
  user confirmed completion appeared in the TUI after manual refresh (`r`).
  This verifies the ChatGPT-to-TUI direction through user-observed behavior.
- User reopened the test task in the TUI, then ChatGPT fetched it and reported
  status `open`. User-observed edits now pass in both directions.
- User reported a successful ChatGPT read at 10:55 and confirmed the idle-session
  check complete after being asked to wait at least 16 minutes without reconnecting.
  Session continuity passes by user report; the refresh exchange was not traced.
- User deferred Claude and selected Gemini next. Setup instructions now live in
  `service/README.md`. Gemini sign-in/listing subsequently passed; Google's current
  web-app instructions restrict custom apps to eligible US personal accounts.
  Real-data import and default-backend cutover have not occurred.

### Gemini consent fix — 2026-10-24

- User reached the Google consent page but approval stalled in both the Gemini
  webview and a normal browser. Browser Console confirmed a `form-action` CSP
  violation. The policy included GitHub and Google's registered callback origin;
  the specific later redirect causing rejection was not captured.
- Consent POST now ends with HTTP 200 HTML, immediate meta navigation and a
  matching fallback link. This separates subsequent navigation from the form's
  redirect chain. Both approve and deny use this boundary; the form policy is
  now strictly `form-action 'self'`. Exact callback registration, cookie binding,
  one-time consent, PKCE and owner checks remain enforced.
- Rust formatting/Clippy and 10 unit tests passed; WASM build, all 45 Worker
  integration tests and production deployment dry-run passed. Integration tests
  inspect HTTP/HTML behavior, not browser navigation. Live disposable registration
  checks verified both continuation responses, single-use rejection, health and
  anonymous MCP rejection; an existing authenticated MCP read also passed.
- Wrangler backup failed with Cloudflare authentication error 10000. The connected
  Cloudflare API exported the full SQL successfully to private ignored
  `service/.backups/20261024T100919Z-pre-gemini-consent.sql` (62,219 bytes) before
  deployment. The later Wrangler retry and restore verification are recorded below.
- Deployed version `55534ca4-2d1d-4b50-8c18-83a4de53207c`. User confirmed a fresh
  Gemini connection succeeded and listed tasks after deployment. This verifies
  sign-in and reads by user report.
- Gemini created and read back `[Gemini test] shared task`, open, scheduled
  `2026-11-02`, with note `Created through Gemini.` User confirmed it appeared
  correctly in the service TUI. User then completed the task in the TUI; Gemini
  fetched it as completed with the same date and note. This verifies edits in
  both directions by user report. Gemini completion writes remain unverified.
- Following the instruction to wait at least 16 minutes without reconnecting,
  user reported Gemini fetched the task as completed with its original date
  and note. Idle-session continuity passes by user report; the token refresh
  exchange and elapsed interval were not independently traced.

### Backup command verification — 2026-10-24

- User chose to retain the existing cf/Wrangler tool split after checking cf's
  missing D1 SQL export support. No dependency or local database changes were made.
- Retrying `make backup-production` with the existing Wrangler login succeeded;
  no new login, token copying or credential changes were needed. Earlier error
  10000 did not recur; its underlying cause remains unconfirmed.
- Private ignored backup: `service/.backups/20261024T181216Z-yoZh9G.sql`,
  76,122 bytes, mode 0600 under a 0700 directory. SQL restored to an isolated
  in-memory SQLite database with 13 tables; integrity check passed and no foreign
  key violations were found. No production restore was performed.
- Added backup authentication troubleshooting to `service/OPERATIONS.md`.
  Independent/off-machine retention and scheduling activation remain outstanding.

### Encrypted scheduled backup preparation — 2026-10-24

- User authorized automation only if it incurs no cost. Verified the repository
  is public and currently has no Actions artifacts. Standard public Linux runners
  are free; artifact storage uses a shared allowance, so small files alone cannot
  establish zero cost. Billing API access did not establish a spending hard stop.
- Added `.github/workflows/backup.yml`: daily 03:43 UTC/manual export, in-memory
  SQLite validation, GPG encryption, encrypted-only artifact upload, seven-day
  daily/35-day Sunday retention and always-run removal of runner copies. The job
  rejects forks/private repositories and uses read-only repository permissions.
- Raw/encrypted files are capped at 1 MiB. Repository artifacts plus a reserved
  2 MiB upload allowance must fit within 16 MiB. No dependency cache is created.
  Both `BACKUP_ENABLED` and `BACKUP_PAID_USAGE_BLOCKED` default to off. Do not
  activate unless paid Actions/artifact storage is blocked at account level.
- Generated public recipient `service/backup-recipient.asc` with pinned fingerprint
  `76004AFB9D8EB8FE0AC750097AA20096FA5B2F0B`. Private key is only in ignored
  `service/.backups/recovery/doitdoit-backup-private-key.asc` (0600), without a
  passphrase. User must preserve a protected independent copy before activation;
  no private key is to be uploaded to GitHub.
- Five isolated tests cover invalid/oversized/partial exports, foreign-key
  violations, forbidden disk attachment, real encryption/decryption, wrong-key
  rejection and refusing output replacement. Added `make test-backup` to CI.
  Actual production SQL also validated and encrypted/decrypted byte-for-byte.
- Setup/recovery and the public-schedule inactivity limitation are documented in
  `service/OPERATIONS.md`. Still needed: confirmed spending hard stop, recovery
  key custody, dedicated `CLOUDFLARE_BACKUP_API_TOKEN` secret, reviewed publication
  to the default branch, activation and a successful hosted backup/restore drill.
  No paid usage, activation gates or hosted schedule were enabled by preparing
  these files. User subsequently created `CLOUDFLARE_BACKUP_API_TOKEN` in GitHub;
  its name/presence was verified without reading the value. Its export permission
  still needs a hosted test. No activation variables are set yet.
- User confirmed the private recovery key has a secure independent copy and
  supplied GitHub billing status showing an account-wide Actions budget of $0
  with usage blocked (also Packages at $0/blocked). These satisfy the activation
  prerequisites by user report. User explicitly authorized merging the full
  Cloudflare service branch so scheduled tooling can run from `main`.
- PR #33 was opened for that merge. The initial CI vulnerability scan found
  standard-library issues in the older Go toolchain; `go.mod` and `mise.toml`
  now both require Go 1.27.3, the patch identified by govulncheck. Application
  dependency versions are unchanged. Patched-toolchain checks precede merge.

1. Exercise hands-on service TUI workflows on supported desktop platforms and
   intended ChatGPT/Claude clients, including natural access-token expiry.
2. Operate the documented independent/off-machine backup retention policy.
3. Review the dry-run of the actual source only when separately requested, then
   obtain explicit authorization for real-data publication and backend cutover.

Plugin packaging remains deferred. These outstanding gates are not implicit
permission to import real tasks, change the selected backend, release or push.

## Production reference and handoff

- Account: `91d8cf0b8c805dac586f9ab55d6d6212`.
- Worker: `doitdoit-service`; origin:
  `https://doitdoit-service.davidt-91d.workers.dev`.
- MCP: `https://doitdoit-service.davidt-91d.workers.dev/mcp`.
- Current recorded production version: `55534ca4-2d1d-4b50-8c18-83a4de53207c`,
  serving 100% of traffic. Initial deployment was on 2026-10-20.
- D1: `doitdoit-prod` / `43659423-7b12-42ac-8f1e-fc97f3ed0fea`, Western Europe,
  read replication disabled. Auth cleanup runs hourly at minute 17.
- User chose one production environment and local development; do not add staging.
  Workers Free was confirmed by the user; no paid subscription was enabled.
- `service/cloudflare.config.ts` owns development/production configuration.
  Make targets use the preferred `cf` CLI, with Wrangler underneath for Rust
  builds. Local migrations retain Wrangler because cf rejects the existing
  nil local database ID; this preserves the local database.
- Production credentials are in ignored `service/.env.production` with `0600`
  permissions and deployed as Worker secrets. The separate localhost OAuth app
  uses ignored `.dev.vars`. Never put credentials in Git or chat.
- `make smoke` defaults to localhost. `go run ./devtools/service-smoke --url ORIGIN
  --extended` checks the expanded hosted operation set with a temporary private
  desktop login, which it revokes and removes.
- Production migrations 0001–0004 are applied. Pre-0004 full SQL backup is in
  ignored private `service/.backups/pre-0004.sql`. Restore verification compared
  all 10 tables with a local SQLite restore. Temporary recovery DB
  `a08642d9-6158-41fe-a11c-ffb6daa25770` was deleted after verification.
- Previous Worker `344b0f95-3840-4252-836b-5b80796b47de` is the immediate rollback
  target (it has the Gemini consent issue). The earlier restore drill used
  `26470207-c46f-4293-a5f0-64fbcf5230a3`.
  [Operational procedures](../service/OPERATIONS.md) distinguish task export,
  full D1 backup, code rollback and real-data cutover.
- Disposable production MCP test task `cd65eb40-27d4-419c-a2a7-dc4540d6bf02`
  remains completed: created with `2020-02-29` at revision 1, rescheduled to
  `2030-01-02` at revision 2, identical retry unchanged, stale revision rejected,
  completed at revision 3 with the date preserved.
- The active backend remains JSON. Service mode is opt-in; no real-task import
  or backend switch has occurred.
  Import, cutover, releases, tags, pushes and plugin publication remain separate
  actions requiring the relevant explicit instruction.

This is the restart point for a new chat. Read the branch's Git log, working-tree
diff and implementation alongside this plan. Preserve existing JSON safety until
an explicitly authorized cutover; distinguish verified behavior from open gates.

## Direction

Build our own task manager in this repository. Keep the Go Bubble Tea TUI and
CLI, introduce a Rust Cloudflare Worker, and make Cloudflare D1 the
authoritative database. Following the language decision on 2026-10-19, Rust owns
task validation, HTTP, MCP, OAuth endpoints, and D1 operations. There is no
TypeScript application layer or runtime npm dependency. Node remains Cloudflare
development/test tooling only. The Worker exposes an HTTP task API and an authenticated
remote MCP endpoint backed by the same task operations.

This plan supersedes the future architecture in
[major-version-storage.md](major-version-storage.md). That document continues to
describe the safety requirements of the existing JSON implementation until the
explicit cutover. This is a new storage decision, not a revival of the abandoned
immutable-record backend or the per-task JSON proposal.

The user's subsequent instruction authorizes the initial service deployment
directly to production. Keep the account, resources and configuration concrete
and reviewable. Import of real tasks, TUI cutover, releases and release tags remain
separate actions; they are not implied by deploying the service.

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

Both Rust transports call the same Rust task operations. Transport handlers contain
authentication, schema decoding, and response mapping, not separate task rules.
Only the Worker accesses D1; clients receive no Cloudflare management credentials
or database access tokens.

Suggested repository layout:

- service/: Rust Worker crate and Cargo lockfile, SQL migrations, Rust OAuth/MCP
  endpoints, cf configuration and Wrangler build settings, development-tool npm lockfile,
  native Rust contract tests and local WASM/D1 integration tests.
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
an explicit ordering key, with a stable task-ID tie-breaker. The ordering scope is scheduled date plus completion status, as documented in
the API contract. A
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

## Later plugin packaging

Package the existing remote MCP connection as a doitdoit plugin after the direct
MCP workflows are established. Include discoverable metadata, connection and
login instructions, and optional skills explaining explicit dates, task IDs,
expected revisions, retry operation IDs and conflict resolution.

Keep the Rust Worker, D1 and OAuth service as the shared backend. Direct MCP
connections remain supported, and the planned Go TUI integration uses the HTTP API.
Check each target client's current packaging requirements when implementing;
do not assume one package works unchanged in Codex, ChatGPT and Claude.

This is deferred distribution work, not the current implementation milestone.
No plugin installation or publication is part of the current scope.

## Authentication

Authentication is part of the first vertical slice, not a later addition.
Use GitHub as the upstream identity provider and maintained Rust cryptographic
primitives. The deployed Rust service implements a narrow public-client OAuth
server with S256 PKCE, exact registered redirects, browser-bound consent/state,
opaque hashed credentials, resource binding, expiry, rotating refresh tokens,
and revocation in D1. It does not implement a general OAuth/OIDC platform or
Client ID Metadata Documents; clients use dynamic registration. Initial auth
regression review, production rate limits and expired-state cleanup are in place.
This does not constitute an independent security audit. Complete remaining hosted
client and refresh checks before real-data cutover. Start with one allowlisted owner identity;
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

### 1. Contract and infrastructure spike — mostly complete

Completed: Rust Worker/D1 skeleton and migrations, initial API contract and
fixtures, standalone Go HTTP client, authenticated Codex MCP access, real local
and production GitHub login, and the D1 atomic revision/history/receipt pattern.
Two writers cannot silently overwrite the same revision; tested acknowledged
mutations have matching history and retry receipts. Production checks cover
explicit dates, identical retries, stale conflicts, completion, refresh and revocation.

Contract decisions and remaining client gates:

- [x] Establish ordering scope, representation, neighbor conflicts and compaction rules.
- [x] Specify consistent board snapshot/change-feed cursors and stale-cursor recovery.
- [ ] Verify intended ChatGPT/Claude client interoperability.
- [ ] Verify client-managed refresh across natural expiry and revoked-login behavior.

Gate: the selected contracts are documented and the intended client flows are
verified. Local tests use fixtures; authorized hosted tests use disposable tasks.

### 2. Complete service operations — implemented and deployed

Completed foundation: create/get/list/update with explicit completion/scheduling,
owner checks, bounded input/output, revisions, history writes and idempotency.
User-facing history, soft delete/restore and the remaining operation set are
implemented in the shared Rust layer and deployed.

Implemented slices (local integration coverage plus the extended production smoke):

- [x] Revision-guarded soft delete/restore and owner-scoped task history reads.
- [x] Filtering/search with bounded pagination.
- [x] Reorder and position compaction using the stage 1 contract.
- [x] Consistent board reads, paged change feed and stale-cursor recovery.
- [x] Portable owner export including the metadata needed for recovery.
- [x] HTTP/MCP parity and stale, unauthorized, retried and concurrent behavior tests
  for each new operation.

Gate: the complete operation set shares one task layer and its success, conflict,
retry, isolation and rollback behavior is covered by contract/integration tests.

### 3. Go client and TUI integration — implemented locally

The Go HTTP client, desktop login and service TUI mode are implemented. JSON
remains the default. The TUI applies paged change events every 15 seconds while
idle and separately after canonical task acknowledgements. Writes release the
saving state before reconciliation; routine writes/polls do not fetch the full
board. Event revisions prevent replayed events from replacing newer task
acknowledgements, and compaction events update affected tasks. Startup, manual
refresh, view changes and invalid-cursor recovery use consistent board/export
snapshots. Background reconciliation cannot replace an active editor.

Service-mode review fixes cover single-column Overdue/Deleted geometry, fixed
day navigation, Today-based relative moves from Overdue, selection reset on
return to Today, and Add from Overdue visibly switching to Today before entry.
Help, empty states, status/feedback and wordmark click bounds match service mode.
Stuck drafts can be reviewed and archived privately without sending a mutation;
archiving an uncertain command does not cancel any possible server write.
Conflict application is offered only for updates to active tasks.

CLI capture writes its private draft before any network request. Explicit dates
and undated capture skip timezone reads. Relative dates use a one-task board
metadata page and the original capture instant in the owner timezone; the
resolved absolute date is persisted before creation and retained across retries.
Import dry-runs check rank characters/length, safe revisions and completion
consistency with the Rust validator before attempting staging.

- [x] Persistent login/refresh, private credential storage and explicit backend selection.
- [x] Task commands and canonical acknowledgements through Bubble Tea commands.
- [x] Overdue view with no rollover writes or date changes during reads/midnight.
- [x] Notes, reorder, completion, delete/restore, revision-guarded undo and refresh.
- [x] Disconnected/saving/saved/failed feedback, stale-response protection,
  private retained drafts and safe retries after an uncertain write outcome.

Gate: complete workflows work on macOS/Linux, including delayed responses,
network loss, conflicts and retained drafts. Keep the legacy JSON backend safe.

### 4. Import, recovery and web transition — tooling implemented

- [x] Explicit frozen source snapshot and strict dry-run with mapping/conflict report.
- [x] Resumable import with preserved fields, dates, ordering and stable ID mapping.
- [x] Field/count/order/export verification and retained recovery material.
- [x] Document the Dropbox companion boundary and rollback after service edits.

Gate: representative legacy fixtures migrate without lost notes or dates;
repeated/interrupted import creates no duplicates and preserves the source.
Executing an import of real tasks remains separately authorized.

### 5. Operational handoff and cutover readiness — partly complete

Completed: local/production configurations, production Worker and D1, private
secrets, migration/deployment commands, health/discovery, query-redacted sampled
traces, auth/body limits and hourly auth cleanup. Initial deployment was brought
forward for stage 1 hosted testing; production contains disposable test data.

- [x] Document deployment credentials and operational recovery procedures.
- [x] Confirm D1 Free Time Travel (7 days), document independent export retention
  and dashboard usage/cost visibility. Off-machine backup scheduling remains operational work.
- [x] Restore into a separate recovery database and compare exported data.
- [x] Verify production code rollback against migration 0004 and restore the latest
  Worker. Code rollback did not rewind database contents or migrations.
- [ ] Complete remaining real-client, concurrency and retry-recovery acceptance checks.
- [ ] Obtain explicit authorization for real-data import and client cutover.

Gate for real-data cutover: authentication and intended client interoperability,
complete service/TUI workflows, export/restore and rollback are demonstrated.
No release tags or pushes are implied by this plan.

### Later: plugin packaging — deferred

After direct MCP workflows are established, prepare client-specific metadata,
connection/login instructions and optional task-management skills. The same Rust
service and OAuth backend remain authoritative. No plugin implementation or
publication is scheduled ahead of the current MCP work.

## Verification and repository updates

Backend tests run locally against the Workers/D1 test environment, not a
developer's database. Include concurrent writers, stale dates, duplicate retries,
failure injection between mutation steps, deletes versus edits, reorder races,
authentication, history, and consistent change-feed pagination.

Rust checks are cargo fmt --check, cargo clippy --locked --target
wasm32-unknown-unknown -- -D warnings, and cargo test --locked in service/.
Build the pinned worker-build WASM artifact before local integration tests.

Go checks remain go test -count=1 ./..., go vet ./..., and go test -race ./...
for persistence/reload changes. Preserve isolated HOME/config and Omarchy tests,
Linux/macOS coverage, and supported release architectures. Existing CI already
runs Rust formatting/lints, migrations and local Worker tests, including the
0004 upgrade/ordering/import suites, without replacing the Go and web gates.

Maintain shared contract fixtures for date/null semantics, Unicode/notes,
ordering, revisions, and error decoding across Rust and Go. Test that
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
