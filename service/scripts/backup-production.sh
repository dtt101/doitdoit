#!/bin/sh
set -eu
umask 077

# cf beta has no SQL-export equivalent. Keep its Wrangler fallback isolated here.
# The log may contain a signed download URL; never print it to a shared console.
backup_dir=${DOITDOIT_BACKUP_DIR:-.backups}
mkdir -p "$backup_dir"
chmod 700 "$backup_dir"
backup=$(mktemp "$backup_dir/$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX.sql")
log="${backup%.sql}.log"
if ! npm exec -- wrangler d1 export 43659423-7b12-42ac-8f1e-fc97f3ed0fea \
    --remote --output "$backup" > "$log" 2>&1; then
    printf 'Backup failed. Inspect the private log: %s\n' "$log" >&2
    exit 1
fi
if [ ! -s "$backup" ]; then
    printf 'Backup is empty; inspect the private log: %s\n' "$log" >&2
    exit 1
fi
printf 'Private full SQL backup saved: %s\n' "$backup"
