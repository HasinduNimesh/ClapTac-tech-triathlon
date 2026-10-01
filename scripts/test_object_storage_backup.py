import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "scripts" / "create-object-storage-backup.sh"


class ObjectStorageBackupWrapperTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        (self.bin / "date").write_text("#!/bin/sh\nprintf '20260930t180000z\\n'\n")
        (self.bin / "python3").write_text(
            "#!/bin/sh\n"
            "printf '%s\\n' \"$RESTORE_OBJECT_BUCKET\" > \"$CAPTURED_BUCKET\"\n"
            "printf 'MOCK_COPY=PASS\\n'\n"
        )
        for executable in self.bin.iterdir():
            executable.chmod(0o755)
        self.capture = self.root / "bucket-name.txt"
        self.env = {
            **os.environ,
            "PATH": f"{self.bin}:{os.environ['PATH']}",
            "CAPTURED_BUCKET": str(self.capture),
            "SOURCE_OBJECT_ENDPOINT": "https://source.example",
            "SOURCE_OBJECT_BUCKET": "source-proof",
            "SOURCE_OBJECT_ACCESS_KEY": "source-key",
            "SOURCE_OBJECT_SECRET_KEY": "source-secret",
            "RESTORE_OBJECT_ENDPOINT": "https://backup.example",
            "RESTORE_OBJECT_BUCKET": "must-be-replaced",
            "RESTORE_OBJECT_ACCESS_KEY": "backup-key",
            "RESTORE_OBJECT_SECRET_KEY": "backup-secret",
        }

    def tearDown(self):
        self.temp.cleanup()

    def run_wrapper(self, **overrides):
        return subprocess.run(
            [str(SCRIPT)], env={**self.env, **overrides}, text=True,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
        )

    def test_uses_unique_timestamped_bucket_and_overrides_fixed_target_name(self):
        result = self.run_wrapper(WAYPOINT_BACKUP_BUCKET_PREFIX="waypoint-proof")

        self.assertEqual(result.returncode, 0, result.stderr)
        name = self.capture.read_text().strip()
        self.assertRegex(name, re.compile(r"^waypoint-proof-20260930t180000z-\d+$"))
        self.assertIn(f"OBJECT_BACKUP_BUCKET={name}", result.stdout)

    def test_rejects_unsafe_or_overlong_prefix_without_invoking_copy(self):
        for prefix in ("../waypoint", "Uppercase", "ab", "x" * 50):
            with self.subTest(prefix=prefix):
                self.capture.unlink(missing_ok=True)
                result = self.run_wrapper(WAYPOINT_BACKUP_BUCKET_PREFIX=prefix)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.capture.exists())

    def test_requires_target_credentials_before_invoking_copy(self):
        result = self.run_wrapper(RESTORE_OBJECT_SECRET_KEY="")

        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.capture.exists())


if __name__ == "__main__":
    unittest.main()
