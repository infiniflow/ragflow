"""Shadow verifier policy and failure evidence, independent of application imports."""

import importlib.util
import copy
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import Mock, patch


ROOT = Path(__file__).resolve().parents[4]
with patch.object(sys, "path", [str(ROOT / "tools/quality"), *sys.path]):
    SPEC = importlib.util.spec_from_file_location("shadow_verify", ROOT / "tools/quality/verify.py")
    verify = importlib.util.module_from_spec(SPEC)
    SPEC.loader.exec_module(verify)

CHECKS = [
    {"id": "python", "owners": ["python"]},
    {"id": "web", "owners": ["web"]},
    {"id": "go", "owners": ["go"]},
]


class VerifyTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.output = self.root / "evidence"
        self.output.mkdir()

    def check(self, **kwargs):
        return {"id": "fixture", "command": ["{python}", "-c", "pass"], "timeout": 3, "result": "junit", **kwargs}

    def parse(self, text, kind="junit"):
        path = self.root / "result"
        path.write_text(text, encoding="utf-8")
        return verify.parse_result(kind, path)[0]

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.root), *args], stderr=subprocess.STDOUT).decode().strip()

    def test_git_inventory_includes_deleted_renamed_staged_unstaged_and_untracked(self):
        self.assertIsNotNone(shutil.which("git"))
        self.git("init", "-q")
        self.git("config", "user.name", "Fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        for name in ("deleted.py", "renamed old.py", "staged.py", "unstaged.py"):
            (self.root / name).write_text("old\n")
        self.git("add", ".")
        self.git("commit", "-qm", "fixture")
        (self.root / "deleted.py").unlink()
        self.git("mv", "renamed old.py", "renamed new.py")
        (self.root / "staged.py").write_text("staged\n")
        self.git("add", "staged.py")
        (self.root / "unstaged.py").write_text("unstaged\n")
        (self.root / "untracked.py").write_text("new\n")
        self.assertEqual(verify.changed_paths(self.root, "HEAD"), ["deleted.py", "renamed new.py", "renamed old.py", "staged.py", "unstaged.py", "untracked.py"])

    def test_staged_change_cancelled_in_worktree_still_requires_verification(self):
        self.git("init", "-q")
        self.git("config", "user.name", "Fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        path = self.root / "owned.py"
        path.write_text("original\n")
        self.git("add", ".")
        self.git("commit", "-qm", "fixture")
        path.write_text("staged modification\n")
        self.git("add", "owned.py")
        path.write_text("original\n")
        self.assertEqual(self.git("diff", "--name-only", "HEAD"), "")
        self.assertEqual(verify.changed_paths(self.root, "HEAD"), ["owned.py"])

    def test_pytest_addopts_preserves_result_path_with_spaces_and_windows_separators(self):
        output = self.root / "result directory with spaces"
        output.mkdir()
        (self.root / "test_fixture.py").write_text("def test_keep(): assert True\ndef test_drop(): assert False\n")
        check = self.check(command=["{python}", "-m", "pytest", "-q", "test_fixture.py"], test_options=["--junitxml={result}", "-k", "test_keep"])
        with patch.dict(os.environ, {"PYTEST_ADDOPTS": "--invalid-inherited-option"}):
            result = verify.execute(check, self.root, output, False)
        self.assertEqual(result["status"], "PASS", (output / "fixture.log").read_text())
        self.assertEqual(result["reason"], "1 test cases")
        self.assertTrue((output / "fixture.result").is_file())
        self.assertEqual(list(self.root.glob("*.result")), [])

    def test_selector_known_owner_and_path_fallback(self):
        for paths, owners, expected in ((["api/new.py"], {}, ["python"]), (["web/new.ts"], {}, ["web"]), (["internal/new.go"], {}, ["go"]), (["owned/new.py"], {"owned/new.py": "python"}, ["python"])):
            with self.subTest(paths=paths):
                self.assertEqual(verify.select(paths, CHECKS, "quick", owners)[0], expected)

    def test_inherited_pytest_filter_cannot_hide_failure_without_test_options(self):
        (self.root / "test_fixture.py").write_text("def test_keep(): assert True\ndef test_hidden(): assert False\n")
        check = self.check(command=["{python}", "-m", "pytest", "-q", "test_fixture.py", "--junitxml={result}"], timeout=30)
        with patch.dict(os.environ, {"PYTEST_ADDOPTS": "-k test_keep"}):
            result = verify.execute(check, self.root, self.output, False)
        self.assertEqual(result["status"], "FAIL")
        xml = (self.output / "fixture.result").read_text()
        self.assertIn('name="test_hidden"', xml)
        self.assertIn('name="test_keep"', xml)

    def test_real_frontend_owner_is_narrow_but_shared_transport_stays_full(self):
        checks = json.loads((ROOT / "tools/quality/checks.yaml").read_bytes())["checks"]
        records = json.loads((ROOT / "tools/quality/file-inventory.json").read_bytes())["records"]
        owners = {record["path"]: record["module"] for record in records}
        self.assertEqual(owners["web/src/pages/home/banner.tsx"], "frontend-foundation")
        self.assertEqual(verify.select(["web/src/pages/home/banner.tsx"], checks, "quick", owners)[0], ["frontend-types", "frontend-tests"])
        for path in ("web/src/utils/request.ts", "web/src/services/user-service.ts", "web/src/routes.tsx"):
            self.assertEqual(verify.select([path], checks, "quick", owners)[0], [check["id"] for check in checks])

    def test_unit_coalescing_requires_exact_supported_commands_and_options(self):
        checks = json.loads((ROOT / "tools/quality/checks.yaml").read_bytes())["checks"]
        selected = [check["id"] for check in checks]
        self.assertEqual(verify.coverage_providers(checks, selected), {"process-contracts": "python-unit"})
        self.assertEqual(verify.coverage_providers(checks, ["process-contracts"]), {})
        for index, key, value in (
            (0, "command", ["{python}", "-m", "pytest", "test/unit_test/nonexistent.py", "-q", "--junitxml={result}"]),
            (0, "test_options", ["-m", "special"]),
            (1, "command", ["{python}", "run_tests.py", "-i", "-k", "subset"]),
            (1, "test_options", ["--junitxml={result}", "--ignore=test/unit_test/tools"]),
            (0, "env", ["SPECIAL_PROFILE"]),
        ):
            with self.subTest(key=key, value=value):
                altered = copy.deepcopy(checks)
                altered[index][key] = value
                self.assertEqual(verify.coverage_providers(altered, selected), {})
        import run_tests

        command = run_tests.TestRunner().build_pytest_command()
        self.assertEqual(command[:3], [sys.executable, "-m", "pytest"])
        self.assertEqual(Path(command[3]).resolve(), (ROOT / "test/unit_test").resolve())
        self.assertNotIn("--ignore", command)
        self.assertNotIn("-k", command)

    def test_full_run_executes_unit_once_and_propagates_provider_outcome(self):
        original = json.loads((ROOT / "tools/quality/checks.yaml").read_bytes())
        quality = self.root / "tools/quality"
        quality.mkdir(parents=True)
        config = {**original, "checks": original["checks"][:2]}
        (quality / "checks.yaml").write_text(json.dumps(config))
        (quality / "file-inventory.json").write_text(json.dumps({"records": []}))
        for status in ("PASS", "FAIL", "INCOMPLETE", "CANCELLED"):
            destination = self.root / status
            evidence = {"id": "python-unit", "status": status, "seconds": 1, "log": "same-run-unit.log"}
            with (
                patch.object(verify, "ROOT", self.root),
                patch.object(verify, "changed_paths", return_value=[]),
                patch.object(verify, "execute", return_value=evidence) as execute,
                patch.object(sys, "argv", ["verify.py", "run", "--mode", "full", "--output", str(destination)]),
                patch("builtins.print"),
            ):
                self.assertEqual(verify.main(), 2)
                self.assertEqual(execute.call_count, 1)
                self.assertEqual(execute.call_args.args[0]["id"], "python-unit")
            report = json.loads((destination / "report.json").read_text())
            self.assertEqual(report["execution"], ["python-unit"])
            self.assertEqual(report["covered_by"], {"process-contracts": "python-unit"})
            self.assertEqual({entry["status"] for entry in report["results"]}, {status})
            self.assertEqual(report["release_status"], "INCOMPLETE")

    def test_selector_unknown_shared_control_config_and_lock_changes_are_full(self):
        paths = [
            "unknown.bin",
            "pyproject.toml",
            "uv.lock",
            "go.mod",
            "Dockerfile",
            "build.sh",
            "lefthook.yml",
            ".dockerignore",
            "web/pnpm-lock.yaml",
            "web/package-lock.json",
            "api/config.py",
            "tools/new.py",
            ".github/workflows/x.yml",
            "test/new.py",
            "docker/x.yml",
            "deployment/x.sh",
            "ragflow_deps/x.py",
        ]
        for path in paths:
            with self.subTest(path=path):
                self.assertEqual(verify.select([path], CHECKS, "quick", {})[0], ["python", "web", "go"])
        self.assertEqual(verify.select(["api/deleted.py"], CHECKS, "quick", {"api/deleted.py": "unmapped-module"})[0], ["python", "web", "go"])

    def test_selector_rename_considers_old_and_new_owners(self):
        self.assertEqual(verify.select(["api/deleted.py", "web/new.ts"], CHECKS, "quick", {})[0], ["python", "web"])

    def test_candidate_and_full_modes_never_use_quick_selection(self):
        for mode in ("candidate", "full"):
            self.assertEqual(verify.select([], CHECKS, mode, {})[0], ["python", "web", "go"])

    def test_junit_missing_empty_skipped_failure_error_malformed_and_pass(self):
        self.assertEqual(verify.parse_result("junit", self.root / "missing")[0], "INCOMPLETE")
        for xml, status in (
            ("", "INCOMPLETE"),
            ("<testsuite/>", "INCOMPLETE"),
            ("<testsuite><testcase><skipped/></testcase></testsuite>", "INCOMPLETE"),
            ("<testsuite><testcase><failure/></testcase></testsuite>", "FAIL"),
            ("<testsuite><testcase><error/></testcase></testsuite>", "FAIL"),
            ("<testsuites><testsuite><testcase name='ok'/></testsuite></testsuites>", "PASS"),
        ):
            with self.subTest(xml=xml):
                self.assertEqual(self.parse(xml), status)
        self.assertEqual(verify.parse_result("exit", self.root / "missing")[0], "INCOMPLETE")

    def test_jest_missing_empty_skipped_failure_counts_and_pass(self):
        complete = {"success": True, "numTotalTests": 2, "numPassedTests": 2}
        for data, status in (
            ({}, "INCOMPLETE"),
            (complete, "PASS"),
            ({**complete, "success": False}, "FAIL"),
            ({**complete, "numFailedTests": 1}, "FAIL"),
            ({**complete, "numFailedTestSuites": 1}, "FAIL"),
            ({**complete, "numPendingTests": 1}, "INCOMPLETE"),
            ({**complete, "numTodoTests": 1}, "INCOMPLETE"),
            ({**complete, "numPendingTestSuites": 1}, "INCOMPLETE"),
            ({**complete, "numPassedTests": 1}, "INCOMPLETE"),
            ({**complete, "numPassedTests": 0, "numTotalTests": 0}, "INCOMPLETE"),
        ):
            with self.subTest(data=data):
                self.assertEqual(self.parse(json.dumps(data), "jest"), status)
        self.assertEqual(self.parse("not json", "jest"), "INCOMPLETE")

    def test_missing_prerequisites_and_stale_evidence_never_start_process(self):
        cases = [{"platform": "not-a-real-platform"}, {"isolated": True}, {"env": ["RAGFLOW_UNIT_ABSENT_PREREQUISITE"]}, {"files": ["absent-file"]}, {"tools": ["ragflow-nonexistent-tool-fixture"]}]
        with patch.dict(os.environ, {}, clear=True):
            for missing in cases:
                with self.subTest(missing=missing), patch.object(verify.subprocess, "Popen") as process:
                    result = verify.execute(self.check(**missing), self.root, self.output, False)
                    self.assertEqual(result["status"], "INCOMPLETE")
                    process.assert_not_called()
        for suffix in ("result", "log"):
            path = self.output / ("fixture." + suffix)
            path.write_text("stale")
            with patch.object(verify.subprocess, "Popen") as process:
                result = verify.execute(self.check(), self.root, self.output, False)
                self.assertEqual(result["status"], "INCOMPLETE")
                self.assertIn("stale", result["reason"])
                process.assert_not_called()
            path.unlink()

    def test_nonzero_exit_is_failure_even_with_passing_result(self):
        result = verify.execute(self.check(command=["{python}", "-c", "raise SystemExit(7)"]), self.root, self.output, False)
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["exit_code"], 7)

    def test_jest_rejects_wrong_json_shapes_and_noninteger_counts(self):
        for value in (None, [], True, "result"):
            with self.subTest(value=value):
                self.assertEqual(self.parse(json.dumps(value), "jest"), "INCOMPLETE")
        for key in ("numTotalTests", "numPassedTests", "numFailedTests", "numFailedTestSuites", "numPendingTests", "numTodoTests", "numPendingTestSuites"):
            for value in (True, "1", -1, 1.5, None):
                with self.subTest(key=key, value=value):
                    data = {"success": True, "numTotalTests": 1, "numPassedTests": 1, key: value}
                    self.assertEqual(self.parse(json.dumps(data), "jest"), "INCOMPLETE")

    def test_junit_rejects_invalid_root_and_summary_failures_skips(self):
        self.assertEqual(self.parse("<notjunit><testcase/></notjunit>"), "INCOMPLETE")
        for tag in ("testsuite", "testsuites"):
            for attribute, status in (("failures", "FAIL"), ("errors", "FAIL"), ("skipped", "INCOMPLETE")):
                with self.subTest(tag=tag, attribute=attribute):
                    content = "<testcase/>" if tag == "testsuite" else "<testsuite><testcase/></testsuite>"
                    self.assertEqual(self.parse(f'<{tag} {attribute}="1">{content}</{tag}>'), status)
        self.assertEqual(self.parse('<testsuite failures="invalid"><testcase/></testsuite>'), "INCOMPLETE")

    def test_invalid_working_directory_records_incomplete_instead_of_crashing(self):
        result = verify.execute(self.check(cwd="absent-directory"), self.root, self.output, False)
        self.assertEqual(result["status"], "INCOMPLETE")
        self.assertIn("could not start", result["reason"])

    def test_cancelled_and_deadline_request_tree_cleanup(self):
        for exception, status in ((KeyboardInterrupt(), "CANCELLED"), (subprocess.TimeoutExpired("fixture", 1), "INCOMPLETE")):
            with self.subTest(status=status):
                process = Mock()
                process.wait.side_effect = exception
                with patch.object(verify.subprocess, "Popen", return_value=process), patch.object(verify, "stop_tree") as stop:
                    result = verify.execute(self.check(), self.root, self.output, False)
                    self.assertEqual(result["status"], status)
                    stop.assert_called_once_with(process)
                (self.output / "fixture.log").unlink()

    def test_actual_deadline_terminates_child_before_it_can_write(self):
        child = "import time,pathlib; time.sleep(2); pathlib.Path('child-survived').write_text('bad')"
        parent = "import subprocess,sys,time; subprocess.Popen([sys.executable,'-c'," + repr(child) + "]); time.sleep(30)"
        result = verify.execute(self.check(command=["{python}", "-c", parent], timeout=0.5), self.root, self.output, False)
        self.assertEqual(result["status"], "INCOMPLETE")
        self.assertIn("process tree terminated", result["reason"])
        time.sleep(2.1)
        self.assertFalse((self.root / "child-survived").exists())

    def test_shadow_run_cannot_return_release_pass_even_when_all_selected_tests_pass(self):
        quality = self.root / "tools/quality"
        quality.mkdir(parents=True)
        check = self.check(command=["{python}", "-c", "import pathlib,sys; pathlib.Path(sys.argv[1]).write_text('<testsuite><testcase/></testsuite>')", "{result}"], owners=["python"])
        config = {"schema": 1, "rollout": "shadow", "checks": [check], "required_external_evidence": ["canonical CI parity"]}
        (quality / "checks.yaml").write_text(json.dumps(config))
        (quality / "file-inventory.json").write_text(json.dumps({"files": [], "records": []}))
        destination = self.root / "shadow-run"
        with (
            patch.object(verify, "ROOT", self.root),
            patch.object(verify, "changed_paths", return_value=["api/new.py"]),
            patch.object(sys, "argv", ["verify.py", "run", "--mode", "full", "--output", str(destination)]),
            patch("builtins.print"),
        ):
            self.assertEqual(verify.main(), 2)
        report = json.loads((destination / "report.json").read_text())
        self.assertEqual(report["release_status"], "INCOMPLETE")
        self.assertEqual(report["results"][0]["status"], "PASS")
        self.assertEqual(report["required_external_evidence"], ["canonical CI parity"])


if __name__ == "__main__":
    unittest.main()
