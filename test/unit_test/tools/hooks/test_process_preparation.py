"""Isolated contract tests; run with unittest (no live pytest bootstrap)."""

import hashlib
import io
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch

from ragflow_deps import prepare_native
from tools.hooks import prepare_web, web_check


class HookPreparationTests(unittest.TestCase):
    def test_changed_paths(self):
        self.assertFalse(prepare_web.needs_web(b"api/web.py\0docs/web.md\0"))
        self.assertTrue(prepare_web.needs_web(b"api/a.py\0web/src/with space.tsx\0"))
        self.assertTrue(prepare_web.needs_web(b"web/package-lock.json\0"))

    def test_prepare_installs_once_and_releases_lock_on_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch.object(prepare_web.shutil, "which", return_value="npm"), patch.object(prepare_web.tempfile, "gettempdir", return_value=directory):
                with patch.object(prepare_web.subprocess, "run", return_value=subprocess.CompletedProcess([], 12)) as run:
                    self.assertEqual(prepare_web.prepare(root), 12)
                    self.assertEqual(run.call_count, 1)
                    self.assertEqual(run.call_args.kwargs["env"]["LEFTHOOK"], "0")
                self.assertEqual(list(root.glob("*.lock")), [])
                key = hashlib.sha256(os.fsencode(str(root.resolve()))).hexdigest()[:24]
                lock = root / f"ragflow-web-prepare-{key}.lock"
                lock.write_text("busy")
                with patch.object(prepare_web.subprocess, "run") as run:
                    with self.assertRaisesRegex(RuntimeError, "Another preparation"):
                        prepare_web.prepare(root)
                    run.assert_not_called()
                    self.assertEqual(lock.read_text(), "busy")

    def test_check_only_has_no_fix_and_preserves_spaced_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for tool, relative in web_check.TOOLS.items():
                entry = root / "web/node_modules" / relative
                entry.parent.mkdir(parents=True, exist_ok=True)
                entry.touch()
                with (
                    patch.dict(os.environ, LEFTHOOK_CHECK_ONLY="1"),
                    patch.object(web_check.shutil, "which", return_value="node"),
                    patch.object(web_check.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)) as run,
                ):
                    self.assertEqual(web_check.run(tool, ["web/src/a b.ts"], root), 0)
                    command = run.call_args.args[0]
                    self.assertNotIn("--fix", command)
                    self.assertNotIn("--write", command)
                    self.assertEqual(command[-1], str(Path("src/a b.ts")))
                    self.assertNotIn("npx", command)

    def test_missing_dependency_fails_without_installing(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(web_check.subprocess, "run") as run:
            with self.assertRaisesRegex(RuntimeError, "prepare_web.py"):
                web_check.run("eslint", ["web/a.ts"], Path(directory))
            run.assert_not_called()


class NativePreparationTests(unittest.TestCase):
    def fixture(self, root, member="lib/a.a"):
        archive = root / "native.tar.gz"
        content = b"verified library"
        with tarfile.open(archive, "w:gz") as bundle:
            info = tarfile.TarInfo(member)
            info.size = len(content)
            bundle.addfile(info, io.BytesIO(content))
        return {"archive": archive.name, "version": "1", "sha256": prepare_native.digest(archive), "files": {"lib/a.a": hashlib.sha256(content).hexdigest()}}

    def test_prepare_reuse_and_corruption(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            dep = self.fixture(root)
            cache = root / "cache"
            prepare_native.prepare_one("test", dep, root, cache)
            library = cache / "test/lib/a.a"
            before = library.stat().st_mtime_ns
            with patch.object(prepare_native.tarfile, "open", side_effect=AssertionError("must not extract cache")):
                prepare_native.prepare_one("test", dep, root, cache)
            self.assertEqual(before, library.stat().st_mtime_ns)
            library.write_bytes(b"bad")
            with self.assertRaisesRegex(ValueError, "corrupt"):
                prepare_native.prepare_one("test", dep, root, cache)
            self.assertEqual(library.read_bytes(), b"bad")

    def test_archive_mismatch_and_traversal_do_not_publish(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            dep = self.fixture(root)
            dep["sha256"] = "0" * 64
            with self.assertRaisesRegex(ValueError, "archive"):
                prepare_native.prepare_one("test", dep, root, root / "cache")
            dep = self.fixture(root, "../../escape")
            with self.assertRaises(tarfile.FilterError):
                prepare_native.prepare_one("test", dep, root, root / "cache")
            self.assertFalse((root / "cache/test").exists())
            self.assertFalse((root / "escape").exists())

    def test_go_module_version_mismatch(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            base = root / "ragflow_deps"
            base.mkdir()
            (base / "native-deps.json").write_bytes((prepare_native.BASE / "native-deps.json").read_bytes())
            (root / "go.mod").write_text("module example\n")
            with patch.object(prepare_native, "BASE", base), self.assertRaisesRegex(ValueError, "go.mod"):
                prepare_native.dependencies()

    def test_download_checks_before_publish_and_does_not_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            dep = {"archive": "download.tar.gz", "url": "https://example.invalid/native", "sha256": hashlib.sha256(b"valid").hexdigest()}
            with patch.object(prepare_native.urllib.request, "urlopen", return_value=io.BytesIO(b"corrupt")):
                with self.assertRaisesRegex(ValueError, "checksum mismatch"):
                    prepare_native.download(dep, root)
            self.assertEqual(list(root.iterdir()), [])
            with patch.object(prepare_native.urllib.request, "urlopen", return_value=io.BytesIO(b"valid")):
                prepare_native.download(dep, root)
            with patch.object(prepare_native.urllib.request, "urlopen") as network:
                prepare_native.download(dep, root)
                network.assert_not_called()
            target = root / dep["archive"]
            target.write_bytes(b"user data")
            with self.assertRaisesRegex(ValueError, "refusing overwrite"):
                prepare_native.download(dep, root)
            self.assertEqual(target.read_bytes(), b"user data")


if __name__ == "__main__":
    unittest.main()
