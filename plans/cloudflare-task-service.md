# Cloudflare task service plan

Updated 2026-10-24. Implementation is on `main` after
[PR #33](https://github.com/dtt101/doitdoit/pull/33). The Rust Worker and D1 schema
are deployed; TUI service mode and reviewed import/export tooling are available.
Encrypted off-machine backups are active and recovery-tested. The reviewed 69
real tasks are imported and field-verified. This computer now uses service mode;
backend selection remains explicit on other computers.

Use the [service guide](../service/README.md) for setup and clients,
[Operations](../service/OPERATIONS.md) for production IDs/deployment/recovery,
and the [API contract](../api/README.md) for wire behavior.

## Remaining work

The immediate goal is shared real tasks across the TUI and MCP on all computers.
The service-capable [v0.9.0 release](https://github.com/dtt101/doitdoit/releases/tag/v0.9.0)
is published. All four macOS/Linux archives passed checksum and build-metadata
checks; the published Linux binary starts and advertises service mode.

### Cutover checklist

1. Stop JSON-mode TUIs and Dropbox web edits on all computers; let file sync finish.
2. Dry-run the configured JSON source. Freeze its bytes and review task counts,
   field/ID/date issues, backups, conflict copies and retained drafts. Recheck the
   source fingerprint before applying if editing or sync has resumed.
3. After explicit approval, take a fresh service backup, import the reviewed source
   once, and verify every field, count and order through export. Keep all originals.
4. Publish a service-capable CLI release after separate release/tag authorization.
   Use the existing GitHub Releases/mise path for macOS and Linux.
5. On each computer, update with mise, run `doitdoit service login`, then
   `doitdoit service use`. Credentials and backend selection are per computer;
   never sync credentials between machines.
6. Verify a task change from each TUI appears in the other clients and MCP.
   Keep the original Dropbox JSON as a stale recovery source; the web companion uses the service.

Current step: import and `v0.9.0` publication complete; update/sign in/select the
service on each computer. Owner confirmed a fresh build works in service mode.
Default-backend changes remain per-computer explicit actions.

### Dry-run result — 2026-10-24

- Configured JSON: 69 tasks (24 open, 45 completed); no validation issues.
- All 69 legacy IDs have deterministic service-UUID mappings.
- Snapshot and report saved privately under the service configuration directory;
  frozen bytes match the source. Source and configuration are unchanged.
- Preserved one `.bak` and five saved review files; no pending recovery file.
  The backup's 66 IDs are present in the current source, with scheduling/status
  differences. Older saved reviews contain other task versions/IDs requiring
  review; do not restore or merge them automatically.
- Recovery review found 36 historical open title/note combinations unmatched by
  current ID or exact title; they may reflect deleted/renamed/completed tasks.
  No differing nonempty notes were found for matched tasks. Private candidate
  list: `service/.backups/import-review/recovery-review.md` (ignored, mode 0600).
  Owner selected current-source-only import; no candidates were restored.
- No service requests, import, rollover, pruning or backend change occurred.

### Import result — 2026-10-24

- Owner approved current 69 tasks only; all historical recovery candidates were
  excluded. Approved snapshot and original recovery-review report remain private.
- [Pre-import encrypted backup](https://github.com/dtt101/doitdoit/actions/runs/40126815027)
  passed. Import used the frozen reviewed source, not a newly read live file.
- All 69 tasks were published and field-verified through export. The five existing
  service test tasks were unchanged; total service task count is 74.
- [Post-import encrypted backup](https://github.com/dtt101/doitdoit/actions/runs/40126936482)
  passed SQL restore validation, encryption and independent artifact upload.
- Original JSON and configuration are unchanged. The private `verified.json`
  under the service import directory records the source fingerprint and run ID.
  Keep JSON-mode clients stopped until their explicit cutover; original files
  will become stale once real service edits begin.

### Follow-up checks

- Finish fresh/revoked ChatGPT login, Gemini completion writes and broader TUI
  conflict/retry checks; verify macOS hands-on. Claude remains deferred.
- Monitor daily backups, confirm Sunday retention, and repeat recovery drills.
  Keep the $0 paid-usage hard stop.
- Mobile companion now uses the shared service with GitHub OAuth, an exact browser
  origin allowlist, retained revision-guarded commands and original-date Overdue
  views. Verify owner sign-in and edits from the installed mobile app after rollout.

Releases, tags, real-data import, backend cutover and plugin publication remain
separate decisions. Do not infer authorization from completed infrastructure.

## Current verification

| Area | Result | Open checks |
| --- | --- | --- |
| API/MCP | Operations, conflicts, retries, ordering, history, deletion and staged import tested locally and by hosted smoke | Further real-client workflows |
| Desktop | Linux shared tasks and hosted OAuth verified; Linux/macOS CI passed | Hands-on macOS and broader network/conflict recovery |
| ChatGPT | Create/read/complete, edits both ways with TUI, idle-session read passed | Fresh login and client revocation |
| Gemini | Login/read/create, TUI completion read-back and idle-session read passed | Completion writes |
| Claude | Deferred | All client checks |
| Recovery | Independent D1 restore and code rollback drills passed; encrypted hosted artifact decrypted/restored | Ongoing schedule and periodic restore monitoring |

Assistant checks are user-observed. Idle reads passed after the requested wait;
the token-refresh exchange and elapsed interval were not independently traced.
Go 1.27.3 tests/vet/race and vulnerability scan, Rust/Worker tests, backup tests,
web tests and deployment dry-run passed before merge. CI is the ongoing gate.

## Architecture and scope

```text
Go TUI / CLI -------- HTTPS /v1 ---+
                                  +--- Rust task operations --- D1
Assistant clients --- HTTPS /mcp --+
```

Rust owns validation, task operations, OAuth, HTTP, MCP and D1 access. Go owns TUI
presentation and input; it calls HTTP directly, not MCP. Node is development
and Cloudflare tooling only. Keep the task-oriented service client separate from
the legacy whole-file `Store.Load/Save` interface.

Use one owner, online writes, Workers and D1. Production and local development
are the only environments. No additional server, AI runtime, vector database,
queue, Durable Object or database is needed. Read replication remains disabled.
Preserve Linux/macOS amd64/arm64 support, themes and Omarchy integration.

Deferred: offline editing/sync, collaboration, recurrence, attachments, projects,
labels, a separate deadline field and client-specific plugin packaging. Direct
MCP remains supported; any later plugin packages the same service.

This architecture supersedes the future backend proposals in
[major-version-storage.md](major-version-storage.md). That document's current
JSON safety guarantees still apply until explicit cutover. Do not revive the
abandoned immutable-record or per-task JSON backend.

## Task and data guarantees

- Scheduled dates are local-calendar `YYYY-MM-DD` in the owner's timezone;
  timestamps are UTC. Undated service tasks use `null`.
- Reads, startup, navigation and midnight do not reschedule tasks. Overdue is a
  view. Completion/reopening preserves dates; explicit past dates are valid.
- Ordering is within scheduled-date/status groups. Clients supply reviewed
  neighbors; the server calculates bounded string ranks without floating point.
  Compaction advances affected revisions and records history.
- Authenticate the allowlisted owner and validate bounded typed inputs for every
  operation. No client receives Cloudflare management credentials.
- Each mutation atomically commits task state, history and its operation receipt.
  Receipt-insert SQL triggers enforce the command; failures roll back all effects.
  A zero-row conditional update alone is not an atomicity guarantee in D1.
- Stale revisions return a visible conflict. Do not choose a winner by timestamp
  or retry automatically against a newer revision. Resolution uses a reviewed
  current revision and a new operation ID.
- Identical operation-ID retries return the original acknowledgement; different
  payloads fail. Receipts and history remain private and retained indefinitely.
- Snapshot pagination fixes an event cursor. Apply change-feed events by revision;
  replay cannot overwrite newer acknowledgements. Any future event pruning must
  define stale-cursor recovery first.
- Deletion is recoverable. Retention defaults to forever. Any future automatic
  purge must use known completion times and a recovery period; never purge
  unfinished overdue tasks or invent imported completion times.

Exact fields, bounds, pagination and errors are in the [API contract](../api/README.md).
MCP exposes task operations, not import, arbitrary SQL or service administration.

## Authentication

Allow only the configured numeric GitHub owner. Use public-client OAuth with
S256 PKCE, exact registered redirects, browser-bound single-use consent/state,
resource binding, scoped opaque credentials and rotating refresh tokens.
GitHub access tokens are used for identity only, never retained or forwarded.
Store credential digests in D1; keep desktop credentials owner-only.

Consent POST returns a continuation document before navigating to GitHub; keep
its form policy same-origin. A fresh connection attempt is required after an
expired/consumed consent page. Authentication, expiry, scopes, wrong-owner access
and revocation must be tested through HTTP and MCP. This is not an independent
security audit or a general OAuth identity platform.

## TUI and capture guarantees

Run network I/O through Bubble Tea commands. Show saving/saved/failed status only
from acknowledgements, track request identity/revisions, and never replace an
active editor with background reads. Poll changes every 15 seconds while idle;
use full snapshots for startup, manual refresh, view changes and invalid cursors.

Retain uncertain commands/drafts privately before network requests and retry
identical operation IDs. Archiving a draft does not cancel a possible write.
Undo is a revision-guarded inverse command, never whole-board replacement.
Relative capture dates use the original capture instant in the owner timezone;
persist the resolved date across retries. No new offline mutations are accepted.

## Import and cutover requirements

No automatic migration or dual writes. Follow the [operational cutover steps](../service/OPERATIONS.md#cutover).

- Freeze exact source bytes/fingerprint and decode strictly. Report unknown fields,
  invalid dates, ambiguous/missing IDs and bucket/due-date disagreements.
- Preserve title, notes, status, creation time, dates and order without rollover,
  pruning or invented completion times. `Future` plus valid `due_date` maps to that
  date. Preserve unique IDs or record stable mappings; never deduplicate by title.
- Keep `.bak`, conflict copies and retained drafts for explicit review.
- Stage resumable batches privately; publish the complete verified import
  atomically. Repeating/interrupted imports must not create duplicates.
- Verify every field, count and order through export before selecting the service.
  Keep original JSON and recovery files intact. After service edits, returning
  to JSON requires a fresh reviewed export.

The static web companion uses the shared service. Original Dropbox JSON is a
recovery source, not a second live backend for the web app.

## Change validation

Follow [AGENTS.md](../AGENTS.md) and CI. Tests must isolate HOME/config/task data.
Cover concurrent writes, stale dates, retries, rollback/failure injection,
delete/edit races, ordering, authentication, consistent reads, midnight/DST and
interrupted imports. Keep Go/Rust fixtures aligned. No task writes on reads or
midnight; incomplete overdue tasks survive retention.

Update client help and docs with behavior changes. Keep legacy atomic replacement,
permissions, backups and visible same-bucket conflicts intact until cutover.
