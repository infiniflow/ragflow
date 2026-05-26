import subprocess
from pathlib import Path
import unittest


class ProcessUtilsTest(unittest.TestCase):
    def test_run_forever_restarts_after_failure(self):
        bash = Path(r"C:\Program Files\Git\bin\bash.exe")
        self.assertTrue(bash.exists(), "Git Bash is required for this test")

        cmd = [
            str(bash),
            "-lc",
            (
                "RUN_FOREVER_MAX_RESTARTS=2; "
                "source docker/process_utils.sh; "
                "run_forever demo bash -lc 'echo cycle; exit 1'"
            ),
        ]

        result = subprocess.run(cmd, capture_output=True, text=True, timeout=10)

        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.count("Starting demo..."), 2)
        self.assertEqual(result.stdout.count("demo exited with code 1"), 2)


if __name__ == "__main__":
    unittest.main()
