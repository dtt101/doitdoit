import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    "prepare_backup", Path(__file__).with_name("prepare-backup.py"))
backup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backup)


class BackupTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.source = self.root / "source.sql"
        self.sql = "\n".join(f"CREATE TABLE {table} (id TEXT PRIMARY KEY);"
                             for table in sorted(backup.REQUIRED_TABLES))
        self.source.write_text(self.sql)

    def test_valid_export_restores(self):
        backup.validate(self.source)

    def test_rejects_empty_oversized_partial_and_corrupt_exports(self):
        for sql in ["", " " * (backup.MAX_BYTES + 1),
                    "CREATE TABLE tasks(id TEXT);", self.sql + "INVALID SQL;"]:
            with self.subTest(length=len(sql)):
                self.source.write_text(sql)
                with self.assertRaises(Exception):
                    backup.validate(self.source)

    def test_rejects_foreign_key_violations(self):
        self.source.write_text(self.sql + """
            CREATE TABLE child (owner TEXT REFERENCES owners(id));
            INSERT INTO child VALUES ('missing');
        """)
        with self.assertRaises(ValueError):
            backup.validate(self.source)

    def test_export_cannot_attach_a_file(self):
        target = self.root / "must-not-exist.sqlite"
        self.source.write_text(self.sql + f"ATTACH DATABASE '{target}' AS other;")
        with self.assertRaises(Exception):
            backup.validate(self.source)
        self.assertFalse(target.exists())

    def test_round_trip_encryption_and_refuse_overwrite(self):
        home = self.root / "keys"
        home.mkdir(mode=0o700)
        gpg = ["gpg", "--homedir", str(home), "--batch", "--no-tty",
               "--pinentry-mode", "loopback", "--passphrase", ""]
        try:
            subprocess.run(gpg + ["--quick-generate-key", "isolated backup test",
                           "rsa2048", "encr", "0"], check=True, capture_output=True)
            listed = subprocess.run(gpg + ["--with-colons", "--list-keys"],
                                    check=True, capture_output=True, text=True)
            fingerprint = next(line.split(":")[9] for line in listed.stdout.splitlines()
                               if line.startswith("fpr:"))
            key = self.root / "public.asc"
            with key.open("wb") as out:
                subprocess.run(gpg + ["--armor", "--export", fingerprint],
                               check=True, stdout=out, stderr=subprocess.DEVNULL)
            original = backup.RECIPIENT
            self.addCleanup(setattr, backup, "RECIPIENT", original)
            backup.RECIPIENT = fingerprint
            output = self.root / "encrypted.sql.gpg"
            backup.prepare(self.source, key, output)
            restored = subprocess.run(gpg + ["--decrypt", str(output)], check=True,
                                      capture_output=True).stdout
            self.assertEqual(restored, self.source.read_bytes())
            first = output.read_bytes()
            with self.assertRaises(ValueError):
                backup.prepare(self.source, key, output)
            self.assertEqual(output.read_bytes(), first)
            backup.RECIPIENT = "0" * 40
            failed = self.root / "wrong-key.sql.gpg"
            with self.assertRaises(subprocess.CalledProcessError):
                backup.prepare(self.source, key, failed)
            self.assertFalse(failed.exists())
        finally:
            subprocess.run(["gpgconf", "--homedir", str(home), "--kill", "gpg-agent"],
                           capture_output=True)


if __name__ == "__main__":
    unittest.main()
