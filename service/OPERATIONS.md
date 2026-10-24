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
cutover. The encrypted GitHub Actions workflow is prepared below; activation is
separate from adding its files.
Do not delete the only verified backup after a deployment.

### Automated encrypted exports

`.github/workflows/backup.yml` runs at 03:43 UTC daily and supports **Run workflow**
for an on-demand backup. It uses standard `ubuntu-latest` runners in this public
repository, installs the pinned tooling with `npm ci`, and runs the same SQL
export. It does not build/deploy the Worker or change task data.

Before upload, `scripts/prepare-backup.py` restores SQL into in-memory SQLite,
checks the expected service tables, integrity and foreign keys, and encrypts it
with GnuPG. Only `.sql.gpg` is uploaded; raw SQL, private export logs and local
keyrings are removed at the end of the runner job. Failed validation/encryption
blocks upload. Run `make test-backup` for isolated tests, including encryption
round-trip and wrong-key rejection.

Daily artifacts expire after seven days; Sunday artifacts after 35 days. The
workflow limits both raw and encrypted exports to 1 MiB and checks that the
repository's current artifacts plus a reserved upload allowance fit within
16 MiB. It fails rather than deleting existing backups or raising these limits.
These caps cover this workflow, not shared usage in other repositories.

**Zero-cost activation requirements:** standard hosted runners are free for
public repositories, but artifact storage shares the account's included allowance
with other Actions/Packages usage. In GitHub **Settings → Billing & licensing →
Budgets and alerts**, verify a hard stop on paid Actions/artifact storage usage;
an email-only budget alert is insufficient. If the account cannot enforce this,
leave the workflow disabled. No paid plan or larger runner is required or enabled.
See [Actions billing](https://docs.github.com/en/billing/concepts/product-billing/github-actions)
and [spending controls](https://docs.github.com/en/billing/how-tos/set-up-budgets).

Before activating:

1. Preserve the private recovery key from
   `service/.backups/recovery/doitdoit-backup-private-key.asc` in protected offline
   storage or a password manager. It is owner-only, ignored by Git, and currently
   has no passphrase. Do not upload it to GitHub, put it in chat, or rely on this
   machine as the only copy. The public recipient in `service/backup-recipient.asc`
   has fingerprint `76004AFB9D8EB8FE0AC750097AA20096FA5B2F0B`; the encryptor pins
   that fingerprint. Keep old private keys when rotating recipients.
2. Add repository secret `CLOUDFLARE_BACKUP_API_TOKEN` for the existing Cloudflare
   account with only the D1 permission required by SQL export (D1 Edit). No Worker
   deployment or GitHub OAuth application secrets belong in this job. Scope the
   token to the account; do not claim database-only restriction where the API
   token permissions do not support it. Test the actual export before relying on it.
3. After verifying paid usage is blocked, set repository variable
   `BACKUP_PAID_USAGE_BLOCKED=true`. After securing the recovery key and installing
   the token, set `BACKUP_ENABLED=true`. Both gates default to off. The workflow
   also rejects forks/private repositories and uses a read-only GitHub token.
4. Publish the reviewed workflow to the default branch, run it manually, download
   its encrypted artifact and perform a restore drill before calling automation
   operational. No workflow push, secret provisioning, billing changes or gate
   activation is implied by the local files alone.

For local decryption into a new private file:

```sh
umask 077
gpg --import /path/to/offline/doitdoit-backup-private-key.asc
gpg --output recovered-new.sql --decrypt downloaded.sql.gpg
```

Never use a production restore as the test. Follow the isolated restore procedure
below, and revoke restored OAuth grants before any replacement database is served.
Enable GitHub Actions failure notifications for your account. Scheduled jobs can
be delayed, and public-repository schedules can be disabled after 60 days without
activity; periodically check successful runs and re-enable if necessary. See
[scheduled workflow controls](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/disable-and-enable-workflows).
Manual pre-migration exports remain separate and are never pruned by this workflow.

### Backup authentication failures

`cf` and Wrangler keep separate OAuth logins. A successful `cf` deployment does
not verify Wrangler's access for the SQL export. If backup fails, inspect the
private log locally; it may contain signed download URLs, so do not paste the
whole log into chat or an issue. Cloudflare error 10000 means the API rejected
authentication; it does not establish that the database or export script is broken.

Retry `make backup-production` once. If authentication still fails, run
`npm exec -- wrangler login` from `service/`, complete Cloudflare sign-in, then
retry the backup. Never paste credentials into chat. For unattended backups, use
a narrowly scoped account API token supplied privately through
`CLOUDFLARE_API_TOKEN` instead of depending on interactive sign-in.

On 2026-10-24, an earlier error 10000 did not recur on retry with the saved
Wrangler login. The resulting private SQL backup restored successfully into an
isolated in-memory SQLite database, with integrity and foreign-key checks passing.
No credential changes were needed; the original authentication failure's cause
was not established. Keep independent backup retention as a separate operational
requirement.

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
