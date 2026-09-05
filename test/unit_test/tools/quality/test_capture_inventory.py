"""Isolated Git fixtures for the T0 inventory capture, without application imports."""

import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


SCRIPT = Path(__file__).resolve().parents[4] / "tools/quality/capture_inventory.py"
SPEC = importlib.util.spec_from_file_location("t0_capture", SCRIPT)
capture_module = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(capture_module)


class CaptureInventoryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Inventory Fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        self.git("config", "core.autocrlf", "false")
        (self.repo / "core.py").write_text("base = 1\n", encoding="utf-8")
        self.git("add", "core.py")
        self.git("commit", "-qm", "base")
        self.base = {"commit": self.git("rev-parse", "HEAD"), "tree": self.git("rev-parse", "HEAD^{tree}")}

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.repo, check=True, capture_output=True, text=True).stdout.strip()

    def snapshot(self, *paths):
        mapping = {"modules": [{"id": "fixture", "paths": sorted(set(paths) | capture_module.GENERATED)}]}
        return capture_module.capture(self.repo, self.base, mapping)

    def test_staged_unstaged_and_untracked_have_distinct_fingerprints(self):
        (self.repo / "core.py").write_text("base = 2\n", encoding="utf-8")
        self.git("add", "core.py")
        (self.repo / "core.py").write_text("base = 3\n", encoding="utf-8")
        (self.repo / "new space.py").write_text("extension = True\n", encoding="utf-8")
        records = {r["path"]: r for r in self.snapshot("core.py", "new space.py")["records"]}
        self.assertEqual(records["core.py"]["origin"], "core_change")
        self.assertEqual(records["core.py"]["staged_change"], "M")
        self.assertEqual(records["core.py"]["unstaged_change"], "M")
        self.assertNotEqual(records["core.py"]["base_entry"]["oid"], records["core.py"]["index_entry"]["oid"])
        self.assertTrue(records["new space.py"]["untracked"])
        self.assertEqual(records["new space.py"]["origin"], "extension")

    def test_rename_keeps_deleted_and_added_paths(self):
        self.git("mv", "core.py", "renamed.py")
        records = {r["path"]: r for r in self.snapshot("core.py", "renamed.py")["records"]}
        self.assertEqual(records["core.py"]["staged_change"], "D")
        self.assertFalse(records["core.py"]["working_file"]["present"])
        self.assertEqual(records["renamed.py"]["staged_change"], "A")

    def test_tracked_file_is_included_even_under_new_ignore(self):
        (self.repo / ".gitignore").write_text("core.py\nlocal-secret.env\n", encoding="utf-8")
        (self.repo / "core.py").write_text("base = 5\n", encoding="utf-8")
        (self.repo / "local-secret.env").write_text("secret\n", encoding="utf-8")
        snapshot = self.snapshot(".gitignore", "core.py")
        self.assertNotIn("local-secret.env", [r["path"] for r in snapshot["records"]])
        self.assertIn("core.py", [r["path"] for r in snapshot["records"]])

    def test_unknown_path_fails_instead_of_assigning_upstream(self):
        (self.repo / "unknown.py").write_text("unknown = 1\n", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "Unclassified paths"):
            self.snapshot()

    def test_changed_upstream_tree_fails(self):
        self.base["tree"] = "0" * 40
        with self.assertRaisesRegex(ValueError, "Upstream tree differs"):
            self.snapshot()

    def test_concurrent_content_edit_with_same_status_fails(self):
        target = self.repo / "core.py"
        target.write_text("base = 2\n", encoding="utf-8")
        original = capture_module.fingerprint
        modified = False

        def changing_fingerprint(repo, name):
            nonlocal modified
            result = original(repo, name)
            if name == "core.py" and not modified:
                target.write_text("base = 3\n", encoding="utf-8")
                modified = True
            return result

        with patch.object(capture_module, "fingerprint", side_effect=changing_fingerprint):
            with self.assertRaisesRegex(ValueError, "Concurrent edit"):
                self.snapshot("core.py")

    def test_concurrent_index_edit_with_same_status_fails(self):
        target = self.repo / "core.py"
        target.write_text("base = 2\n", encoding="utf-8")
        self.git("add", "core.py")
        target.write_text("base = 3\n", encoding="utf-8")
        original = capture_module.fingerprint
        modified = False

        def changing_index(repo, name):
            nonlocal modified
            result = original(repo, name)
            if name == "core.py" and not modified:
                target.write_text("base = 4\n", encoding="utf-8")
                self.git("add", "core.py")
                target.write_text("base = 3\n", encoding="utf-8")
                modified = True
            return result

        with patch.object(capture_module, "fingerprint", side_effect=changing_index):
            with self.assertRaisesRegex(ValueError, "Index contents changed"):
                self.snapshot("core.py")

    def test_capture_is_read_only(self):
        (self.repo / "core.py").write_text("base = 4\n", encoding="utf-8")
        before = self.git("status", "--porcelain"), (self.repo / "core.py").read_bytes()
        first, second = self.snapshot("core.py"), self.snapshot("core.py")
        self.assertEqual(first["snapshot_sha256"], second["snapshot_sha256"])
        self.assertEqual(before, (self.git("status", "--porcelain"), (self.repo / "core.py").read_bytes()))


if __name__ == "__main__":
    unittest.main()
