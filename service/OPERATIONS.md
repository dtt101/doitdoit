# Production operations and recovery

Use the account, Worker, D1 IDs and latest deployed version recorded in
[the plan](../plans/cloudflare-task-service.md). Local development and production
are the only environments. Never point a development Worker at production D1.

## Deploy

Run `make check` and `make bundle-production`, save a private full SQL backup,
then `make migrate-production` followed by `make deploy`. Migration 0004 rebuilds
receipt/event tables while preserving their rows and foreign keys; the upgrade
integration test covers a populated 0001–0003 database. It also keeps the old
create/update command shape working for code rollback. Old UUID positions are
compacted on the next ordering operation, with history and revision changes.

Deployment credentials belong to the official `cf` login or a narrowly scoped
Cloudflare API token in CI. Application OAuth credentials are separate. Production
GitHub secrets live in ignored `.env.production` (`0600`); localhost secrets live
in `.dev.vars`. Do not put either file, access tokens, signed export URLs, SQL
backups or database query results in Git or chat. There is no automatic deploy
workflow that needs a repository secret today. CI tests with mocked identities.

## Two different exports

`doitdoit service export --out NEW_FILE` writes a private, consistent portable
task snapshot including deleted tasks. It preserves dates, notes, status, rank,
revision and timestamps. It is useful for task recovery and explicit import to
another empty account/database. It does not contain history, operation receipts
or OAuth grants, and is not a full database backup.

For full recovery, use a D1 SQL export. The current `cf` beta has no equivalent
export command, so the installed Wrangler tooling remains the fallback:

Run `make backup-production`. It writes a new timestamped `0600` SQL file and
private log in ignored `.backups/` (`0700`). Set `DOITDOIT_BACKUP_DIR` to an
appropriate private independent-storage destination if desired. Existing
backups are never overwritten or pruned. The command checks for a nonempty
export; a restore drill is still needed to prove recovery.

Export may briefly interrupt D1 queries. Its log can include a signed download
URL. Keep backup files/logs private; `.backups/` is ignored. Keep at least the
last seven daily exports and four weekly exports in independent private storage,
plus a pre-migration export. This is the retention policy to operate before
cutover; automated off-machine backup scheduling is not configured by this PR.
Do not delete the only verified backup after a deployment.

[D1 Time Travel](https://developers.cloudflare.com/d1/reference/time-travel/) is
always enabled, but the [Free-plan recovery window is seven days](https://developers.cloudflare.com/d1/platform/limits/).
It supplements an independent export. Restoring production rewinds accepted
task edits and OAuth/receipt state; it needs an explicit incident decision and
clients must reconnect safely. Never use it as a routine code rollback.

## Restore drill

Create a temporary D1 database with read replication disabled, without connecting
any Worker. Import the full SQL with `wrangler d1 execute TEMP_ID --remote
--file PRIVATE_BACKUP --yes`. Query every restored table into a private file and
compare all values to a local SQLite restore of the same export; check foreign
keys, task dates/notes/order, history and receipts. Remove the temporary database
after verification. Record only counts, timestamps and IDs in the plan, never
private task values or credential hashes.

Restored OAuth grants must never be exposed through a new Worker. For an actual
recovery into a replacement database, revoke restored grants and require fresh
client login before serving it. Keep the damaged database for review until the
recovery is verified. Source JSON and retained local commands remain untouched.

## Code rollback

Inspect `cf workers deployments create --help`, then preview and deploy a
reviewed existing version at 100% using `--strategy percentage --versions
'[{"version_id":"VERSION_UUID","percentage":100}]'`. Use `--dry-run` first.
Do not bypass secret/binding compatibility checks. Verify health, discovery,
existing-task reads and authenticated operations, then restore the intended
current version at 100%. A code rollback does not rewind D1 or apply down migrations.
Clients calling newer operations need the new Worker restored.

## Usage and incident checks

On [Workers Free](https://developers.cloudflare.com/workers/platform/pricing/),
quota exhaustion can interrupt service; this project never enables a paid plan
automatically. Review Workers requests/CPU/errors and D1 Metrics → Row Metrics
and storage in the Cloudflare dashboard. [D1 pricing](https://developers.cloudflare.com/d1/platform/pricing/)
currently lists 5 million reads/day and 100,000 writes/day on Free. Index writes
and history count too. The TUI applies paged change events every 15 seconds while
idle and separately after a task acknowledgement. Reconciliation does not delay
the acknowledgement or fetch a full board on each write. Startup, manual
refresh, view changes and invalid-cursor recovery use snapshots. Replayed events
cannot replace a newer acknowledged revision. It does not continuously rewrite data.

For an uncertain write, keep its private command and retry the exact operation
ID. In the TUI, `d` opens a retained-command review and a second `d` archives it
privately and refreshes. Archiving does not cancel a server write or establish
its outcome; check the service before recreating any uncertain task. Conflicted
updates cannot be applied to soft-deleted tasks; archive the edit and use an
explicit restore from Deleted if needed. A 409 requires reviewing the current revision; never guess a new revision
or operation ID merely because a request timed out. A lost refresh rotation
response requires login again; the desktop intentionally will not replay the
possibly spent refresh credential. History and retry receipts are retained
indefinitely. Do not prune them without a new retry/cursor retention contract.

## Cutover boundary

Real-data import and `service use` require a separate explicit cutover decision.
Dry-run first, review all mapping/issues/recovery material, publish atomically,
then verify every field and count from export. The original JSON and its `.bak`
remain unchanged. After service edits, switching to the old JSON loses sight of
those edits; recovery requires a reviewed current export. The static Dropbox
companion continues to edit only its old JSON and is not a service client.
