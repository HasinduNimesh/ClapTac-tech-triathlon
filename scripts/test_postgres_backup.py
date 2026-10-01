import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "scripts" / "create-postgres-backup.sh"


class PostgresBackupCommandTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        (self.bin / "pg_dump").write_text(
            "#!/bin/sh\n"
            "[ \"${MOCK_DUMP_FAIL:-}\" != 1 ] || exit 17\n"
            "for arg in \"$@\"; do case \"$arg\" in --file=*) file=${arg#--file=};; esac; done\n"
            "printf 'custom-format-test-archive' > \"$file\"\n"
        )
        (self.bin / "pg_restore").write_text(
            "#!/bin/sh\n"
            "[ \"${MOCK_RESTORE_FAIL:-}\" != 1 ] || exit 18\n"
            "[ \"$1\" = --list ] && test -s \"$2\"\n"
        )
        for executable in self.bin.iterdir():
            executable.chmod(0o755)
        self.backup_dir = self.root / "persistent-backups"
        self.env = {
            **os.environ,
            "PATH": f"{self.bin}:{os.environ['PATH']}",
            "SOURCE_DATABASE_URL": "postgres://example.invalid/test",
            "WAYPOINT_BACKUP_DIR": str(self.backup_dir),
        }

    def tearDown(self):
        self.temp.cleanup()

    def run_backup(self, **overrides):
        return subprocess.run(
            [str(SCRIPT)], env={**self.env, **overrides}, text=True,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
        )

    def test_creates_restricted_validated_archive_and_matching_checksum(self):
        result = self.run_backup()

        self.assertEqual(result.returncode, 0, result.stderr)
        archives = list(self.backup_dir.glob("waypoint-postgres-*.dump"))
        self.assertEqual(len(archives), 1)
        archive = archives[0]
        self.assertEqual(archive.read_bytes(), b"custom-format-test-archive")
        self.assertEqual(archive.stat().st_mode & 0o777, 0o600)
        checksum_file = Path(f"{archive}.sha256")
        expected = hashlib.sha256(archive.read_bytes()).hexdigest()
        self.assertEqual(checksum_file.read_text(), f"{expected}  {archive.name}\n")
        self.assertIn("POSTGRES_BACKUP=PASS", result.stdout)

    def test_failed_dump_does_not_publish_partial_archive(self):
        result = self.run_backup(MOCK_DUMP_FAIL="1")

        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(list(self.backup_dir.glob("waypoint-postgres-*.dump")), [])
        self.assertEqual(list(self.backup_dir.iterdir()), [])

    def test_unrestorable_dump_does_not_publish_archive_or_checksum(self):
        result = self.run_backup(MOCK_RESTORE_FAIL="1")

        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(list(self.backup_dir.glob("waypoint-postgres-*.dump")), [])
        self.assertEqual(list(self.backup_dir.iterdir()), [])


if __name__ == "__main__":
    unittest.main()
