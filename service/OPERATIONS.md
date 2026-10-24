# Service operations

Commands below run from `service/` unless stated otherwise. Use local development
or the existing production environment; never bind development to production D1.

## Production reference

| Resource | Value |
| --- | --- |
| Account | `91d8cf0b8c805dac586f9ab55d6d6212` |
| Worker | `doitdoit-service` |
| Origin | `https://doitdoit-service.davidt-91d.workers.dev` |
| D1 | `doitdoit-prod` / `43659423-7b12-42ac-8f1e-fc97f3ed0fea` |
| Region / timezone | Western Europe / `Europe/London` |
| Current recorded Worker version | `55534ca4-2d1d-4b50-8c18-83a4de53207c` |
| Previous version | `344b0f95-3840-4252-836b-5b80796b47de` (Gemini consent issue) |
| Applied migrations | `0001`–`0004` |

Read replication is disabled. Hourly auth cleanup runs at minute 17. Verify the
live version before rollback; merging Git does not deploy the Worker.

## Deploy

Keep `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` and `GITHUB_OWNER_ID` in ignored
`.env.production` with permissions `0600`. The production GitHub OAuth callback
is `https://doitdoit-service.davidt-91d.workers.dev/callback`.

```sh
make check
make test-backup
make bundle-production
make backup-production
make migrate-production
make deploy
```

Deployment uses `cf` authentication and uploads the production secrets file.
Retain the pre-migration SQL backup. After deploy, check `/health`, OAuth
sign-in and authenticated reads/writes. The extended hosted smoke creates and
completes two labelled test tasks and revokes its temporary login:

```sh
go run ../devtools/service-smoke --url https://doitdoit-service.davidt-91d.workers.dev --extended
```

## Backups

| Export | Contains | Use |
| --- | --- | --- |
| `doitdoit service export --out NEW_FILE` | Task state, including deleted tasks | Portable task recovery/import |
| `make backup-production` | Full D1 SQL, including history, receipts and OAuth state | Database recovery |

Manual SQL backups and logs are owner-only files under ignored `.backups/`.
Set `DOITDOIT_BACKUP_DIR` to use another private destination. Files are never
replaced or pruned. Export may briefly block D1 queries. Logs can contain signed
download URLs; keep SQL, logs and credentials out of Git and chat.

### Automated encrypted exports

The [backup workflow](../.github/workflows/backup.yml) is active on `main`.

- Schedule: **03:43 UTC daily**; **Run workflow** also supports manual exports.
- Validation: restore SQL into temporary SQLite, check tables, integrity and
  foreign keys, then encrypt with GnuPG. Only the encrypted artifact is uploaded.
- Retention: seven days for daily copies; 35 days for Sunday copies.
- Limits: 1 MiB per raw/encrypted export; repository artifacts plus 2 MiB reserved
  upload space must fit within 16 MiB. Exceeding a limit fails the job.
- Credentials: repository secret `CLOUDFLARE_BACKUP_API_TOKEN`, scoped to this
  Cloudflare account with **D1 Edit**, without Worker/DNS permissions.
- Gates: repository variables `BACKUP_ENABLED=true` and
  `BACKUP_PAID_USAGE_BLOCKED=true`. Set `BACKUP_ENABLED=false` to pause jobs.

The account has a user-confirmed **$0 Actions budget with usage blocked**.
Keep that hard stop: public standard runners are free, but artifact storage
shares an account allowance. Size caps alone do not guarantee zero charges.
Do not activate on a private repository or enable paid usage.
[GitHub billing controls](https://docs.github.com/en/billing/how-tos/set-up-budgets).

Enable Actions failure notifications and periodically check successful runs.
Public-repository schedules may stop after 60 days without repository activity;
re-enable if necessary. A skipped/disabled job is not a backup.

[First hosted backup](https://github.com/dtt101/doitdoit/actions/runs/40125151080)
and downloaded-artifact recovery passed on 2026-10-24. Sunday retention still
needs its first Sunday run. Keep manual pre-migration exports separately.

### Recovery key

- Private key: `.backups/recovery/doitdoit-backup-private-key.asc`, ignored and
  owner-only, without a passphrase. The owner has a protected independent copy.
- Public key: `backup-recipient.asc`; pinned fingerprint
  `76004AFB9D8EB8FE0AC750097AA20096FA5B2F0B`.
- Never upload the private key to GitHub. Keep old private keys when rotating
  recipients, and update/test the pinned fingerprint in `scripts/prepare-backup.py`.

Download an encrypted artifact from Actions and decrypt to a new private file:

```sh
umask 077
gpg --import /path/to/offline/doitdoit-backup-private-key.asc
gpg --output recovered-new.sql --decrypt downloaded.sql.gpg
```

### Backup failures

For manual error 10000, inspect the private log and retry once. If it persists:

```sh
npm exec -- wrangler login
make backup-production
```

`cf` and Wrangler keep separate logins. For hosted authentication failures,
replace the GitHub backup token and verify its account/D1 permissions. Do not
paste tokens or complete export logs into issues or chat.

D1's [Free Time Travel recovery window](https://developers.cloudflare.com/d1/platform/limits/)
is seven days. It supplements independent backups; a production rewind also
rewinds task edits and OAuth/receipt state.

## Restore

Test in an isolated database, never by overwriting production:

1. Decrypt an artifact and restore it into temporary SQLite. Check integrity,
   foreign keys and task fields. Remove temporary plaintext afterward.
2. For a cloud restore drill, create an unbound temporary D1 database with read
   replication disabled. Import using `npm exec -- wrangler d1 execute TEMP_ID
   --remote --file PRIVATE_BACKUP --yes` with the production account selected.
3. Compare every restored table with the local SQLite restore. Delete the
   temporary database after verification; retain the verified backup.

For actual recovery, preserve the damaged database until verification completes.
Revoke restored OAuth grants and require fresh client login before serving a
replacement database. Never expose restored credentials through a test Worker.

## Code rollback

Check `npm exec -- cf workers deployments create --help`. Preview a reviewed
existing version with `--dry-run`, then deploy it at 100% using
`--strategy percentage --versions '[{"version_id":"VERSION_UUID","percentage":100}]'`.
Do not bypass binding/secret compatibility checks. Verify health and authenticated
operations. Code rollback does not rewind D1 or reverse migrations; older Worker
versions may not support newer client operations.

## Failed or conflicting task writes

Preserve the retained command and retry the identical operation ID after an
uncertain outcome. A timeout does not establish failure. For HTTP 409, review the
current task before submitting a new operation ID/revision. In the TUI, `d` opens
retained-command review; a second `d` archives it privately without cancelling a
possible server write. Lost refresh-rotation responses require login again.
History and receipts are retained indefinitely; do not prune without a new contract.

Monitor Worker errors/CPU and D1 usage in the Cloudflare dashboard. Free quotas
can interrupt service; do not enable a paid plan automatically.

## Cutover

Real-task import and selecting the service as default require an explicit decision:

1. Freeze the source JSON and dry-run the import.
2. Review field mappings, invalid/ambiguous data and all recovery material.
3. Apply the import and verify every field, count and order through export.
4. Select the service with `doitdoit service use`.

Keep the original JSON, `.bak` and recovery files. After service edits, the old
JSON is stale; returning to it requires a reviewed current service export. The
Dropbox web companion remains a separate JSON client.
