#!/usr/bin/env python3
"""Validate and encrypt one full D1 export; never print SQL or private logs."""
import argparse
import os
from pathlib import Path
import sqlite3
import subprocess
import tempfile

MAX_BYTES = 1024 * 1024
RECIPIENT = "76004AFB9D8EB8FE0AC750097AA20096FA5B2F0B"
REQUIRED_TABLES = {
    "owners", "tasks", "task_events", "operation_receipts", "oauth_clients",
    "oauth_grants", "oauth_credentials", "oauth_pending", "import_runs",
}


def validate(source):
    if not 0 < source.stat().st_size <= MAX_BYTES:
        raise ValueError("SQL export is empty or exceeds the 1 MiB backup limit")
    with sqlite3.connect(":memory:") as db:
        # An export must not access disk, load extensions, or change this policy.
        db.set_authorizer(lambda action, *_: sqlite3.SQLITE_DENY
                          if action in (sqlite3.SQLITE_ATTACH, sqlite3.SQLITE_DETACH)
                          else sqlite3.SQLITE_OK)
        db.executescript(source.read_text())
        tables = {row[0] for row in db.execute(
            "SELECT name FROM sqlite_master WHERE type='table'")}
        if not REQUIRED_TABLES <= tables:
            raise ValueError("Export is missing required service tables")
        if db.execute("PRAGMA integrity_check").fetchall() != [("ok",)]:
            raise ValueError("Restored backup failed integrity check")
        if db.execute("PRAGMA foreign_key_check").fetchall():
            raise ValueError("Restored backup has foreign-key violations")


def prepare(source, public_key, output):
    validate(source)
    if output.exists():
        raise ValueError("Refusing to replace an existing encrypted backup")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="doitdoit-backup-gpg-") as home:
        os.chmod(home, 0o700)
        gpg = ["gpg", "--homedir", home, "--batch", "--no-tty"]
        subprocess.run(gpg + ["--import", str(public_key)], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        # A full fingerprint pins the recipient; a changed key fails closed.
        with output.open("xb") as encrypted:
            try:
                subprocess.run(gpg + ["--trust-model", "always", "--recipient",
                               RECIPIENT, "--output", "-", "--encrypt", str(source)], check=True,
                               stdout=encrypted, stderr=subprocess.DEVNULL)
            except Exception:
                output.unlink(missing_ok=True)
                raise
    if not 0 < output.stat().st_size <= MAX_BYTES:
        output.unlink()
        raise ValueError("Encrypted backup exceeds the 1 MiB limit")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path)
    parser.add_argument("public_key", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    os.umask(0o077)
    try:
        prepare(args.source, args.public_key, args.output)
    except Exception:
        # SQLite errors may embed task text; subprocess errors may embed paths.
        raise SystemExit("Backup validation/encryption failed; nothing may be uploaded")
    print("SQL restore verified; encrypted backup ready")
