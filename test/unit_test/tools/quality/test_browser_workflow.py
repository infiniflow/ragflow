"""Execute the actual workflow identity/gate scripts with disposable artifacts."""

import hashlib
import ast
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import xml.etree.ElementTree as ET

import yaml

from tools.quality.browser_inventory import expected_inventory, verify_junit


ROOT = Path(__file__).resolve().parents[4]
WORKFLOW = yaml.safe_load((ROOT / ".github/workflows/browser-regression.yml").read_text())


def step(job, name):
    return next(item for item in WORKFLOW["jobs"][job]["steps"] if item.get("name") == name)


class BrowserWorkflowTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        (self.root / "web/dist").mkdir(parents=True)
        (self.root / "web/dist/index.html").write_text("<html>fixture</html>")
        (self.root / "web/pnpm-lock.yaml").write_text("fixture-lock")
        (self.root / "browser-build-parameters.json").write_text(json.dumps({"minify": "terser", "sourcemap": True}))
        self.mandatory, self.inventory_digest = expected_inventory()
        self.env = {**os.environ, "PYTHONPATH": str(ROOT), "GITHUB_SHA": "a" * 40, "GITHUB_OUTPUT": str(self.root / "output")}
        self.execute("frontend", "Record frontend artifact identity")
        self.digest = hashlib.sha256((self.root / "browser-build.json").read_bytes()).hexdigest()
        self.env.update(EXPECTED_MANIFEST_SHA256=self.digest, MANIFEST_SHA256=self.digest)
        self.env["NEEDS_JSON"] = json.dumps({"frontend": {"result": "success"}, "browser": {"result": "success"}})
        for browser in ("chromium", "firefox", "webkit"):
            directory = self.root / "browser-results" / ("browser-result-" + browser)
            directory.mkdir(parents=True)
            (directory / (browser + ".json")).write_text(
                json.dumps(
                    {
                        "browser": browser,
                        "sha": self.env["GITHUB_SHA"],
                        "manifest_sha256": self.digest,
                        "result": "success",
                        "tests": len(self.mandatory),
                        "inventory_sha256": self.inventory_digest,
                    }
                )
            )

    def execute(self, job, name, success=True):
        command = step(job, name)["run"]
        self.assertTrue(command.startswith("python3 - <<'PY'\n"))
        script = command.split("\n", 1)[1].rsplit("\nPY", 1)[0]
        result = subprocess.run([sys.executable, "-c", script], cwd=self.root, env=self.env, capture_output=True, text=True)
        self.assertEqual(result.returncode == 0, success, result.stderr)

    def verify(self, success=True):
        self.execute("browser", "Verify frontend artifact", success)

    def gate(self, success=True):
        self.execute("browser-gate", "Require every browser and matching artifact identity", success)

    def test_single_build_exact_browser_matrix_and_fail_closed_wiring(self):
        jobs = WORKFLOW["jobs"]
        builds = [item for job in jobs.values() for item in job["steps"] if item.get("run") == "pnpm build"]
        self.assertEqual(len(builds), 1)
        self.assertNotIn("strategy", jobs["frontend"])
        self.assertEqual(jobs["browser"]["strategy"]["matrix"]["browser"], ["chromium", "firefox", "webkit"])
        self.assertFalse(jobs["browser"]["strategy"]["fail-fast"])
        self.assertEqual(jobs["browser"]["needs"], "frontend")
        self.assertEqual(jobs["browser-gate"]["needs"], ["frontend", "browser"])
        self.assertEqual(jobs["browser-gate"]["if"], "always()")
        self.assertEqual(step("frontend", "Install frontend dependencies")["run"], "pnpm install --frozen-lockfile")
        self.assertEqual(step("frontend", "Frontend unit tests")["run"], "pnpm test --runInBand")
        self.assertNotIn("if", step("frontend", "Frontend unit tests"))
        frontend_uv = next(item for item in jobs["frontend"]["steps"] if item.get("uses") == "astral-sh/setup-uv@v6")
        browser_uv = next(item for item in jobs["browser"]["steps"] if item.get("uses") == "astral-sh/setup-uv@v6")
        self.assertIs(frontend_uv["with"]["enable-cache"], False)
        self.assertNotIn("enable-cache", browser_uv["with"])
        self.assertEqual(step("browser", "Run isolated browser journeys")["run"], ".venv/bin/python test/run_browser_regression.py --browser ${{ matrix.browser }}")
        self.assertEqual(step("browser", "Upload browser failure evidence")["if"], "failure()")
        self.assertEqual(step("frontend", "Upload frontend build")["with"]["if-no-files-found"], "error")
        self.assertEqual(step("browser", "Upload successful browser receipt")["with"]["if-no-files-found"], "error")
        self.assertNotIn("merge-multiple", step("browser-gate", "Download browser receipts")["with"])
        self.assertEqual(jobs["browser-gate"]["steps"][0]["uses"], "actions/checkout@v6")

    def junit(self, nodes=None):
        nodes = sorted(self.mandatory) if nodes is None else nodes
        root = ET.Element("testsuites")
        suite = ET.SubElement(root, "testsuite", tests=str(len(nodes)), failures="0", errors="0", skipped="0")
        for node in nodes:
            path, name = node.split("::", 1)
            ET.SubElement(suite, "testcase", classname=path.removesuffix(".py").replace("/", "."), name=name)
        return ET.ElementTree(root)

    def test_expected_inventory_is_independent_and_covers_current_six_suites(self):
        self.assertEqual(len(self.mandatory), 52)
        parsed = ast.parse((ROOT / "test/run_browser_regression.py").read_text())
        suites = next(ast.literal_eval(node.value) for node in parsed.body if isinstance(node, ast.Assign) and any(isinstance(target, ast.Name) and target.id == "SUITES" for target in node.targets))
        self.assertEqual({node.split("::", 1)[0] for node in self.mandatory}, set(suites))
        self.assertEqual(len(suites), 6)

    def test_actual_artifact_verifier_and_gate_accept_complete_evidence(self):
        self.verify()
        self.gate()

    def test_missing_manifest_fails(self):
        (self.root / "browser-build.json").unlink()
        self.verify(False)

    def test_changed_artifact_content_fails(self):
        (self.root / "web/dist/index.html").write_text("different")
        self.verify(False)

    def test_extra_artifact_file_fails(self):
        (self.root / "web/dist/extra.js").write_text("unexpected")
        self.verify(False)

    def test_missing_artifact_file_fails(self):
        (self.root / "web/dist/index.html").unlink()
        self.verify(False)

    def test_changed_lock_fails(self):
        (self.root / "web/pnpm-lock.yaml").write_text("other lock")
        self.verify(False)

    def test_source_sha_mismatch_fails(self):
        self.env["GITHUB_SHA"] = "b" * 40
        self.verify(False)
        self.gate(False)

    def test_manifest_digest_mismatch_fails(self):
        self.env["EXPECTED_MANIFEST_SHA256"] = "b" * 64
        self.verify(False)

    def test_missing_and_invalid_manifest_digest_fail_gate(self):
        for digest in ("", "invalid"):
            with self.subTest(digest=digest):
                self.env["MANIFEST_SHA256"] = digest
                self.gate(False)

    def test_missing_browser_artifact_fails(self):
        directory = self.root / "browser-results/browser-result-webkit"
        (directory / "webkit.json").unlink()
        directory.rmdir()
        self.gate(False)

    def test_missing_browser_receipt_fails(self):
        (self.root / "browser-results/browser-result-webkit/webkit.json").unlink()
        self.gate(False)

    def test_extra_browser_artifact_fails(self):
        (self.root / "browser-results/browser-result-unexpected").mkdir()
        self.gate(False)

    def test_wrong_browser_digest_or_result_fails(self):
        path = self.root / "browser-results/browser-result-webkit/webkit.json"
        original = json.loads(path.read_text())
        for key, value in (
            ("browser", "chromium"),
            ("manifest_sha256", "b" * 64),
            ("inventory_sha256", "b" * 64),
            ("result", "skipped"),
            ("tests", 0),
            ("tests", 1),
            ("tests", len(self.mandatory) - 1),
            ("tests", True),
        ):
            with self.subTest(key=key):
                path.write_text(json.dumps({**original, key: value}))
                self.gate(False)

    def test_receipt_rejects_empty_skipped_failed_missing_test_report(self):
        self.env["BROWSER"] = "chromium"
        self.execute("browser", "Record successful browser", False)
        for xml in ("<testsuites/>", "<testsuites><testcase><skipped/></testcase></testsuites>", "<testsuites><testcase><failure/></testcase></testsuites>"):
            with self.subTest(xml=xml):
                (self.root / "browser-junit.xml").write_text(xml)
                self.execute("browser", "Record successful browser", False)
        self.junit().write(self.root / "browser-junit.xml")
        self.execute("browser", "Record successful browser")

    def test_receipt_rejects_reduced_extra_duplicate_skipped_error_and_wrong_identity(self):
        self.env["BROWSER"] = "chromium"
        for change in ("one", "missing", "extra", "duplicate", "wrong", "skipped", "failure", "error", "suite-error", "root-error", "wrong-count"):
            with self.subTest(change=change):
                tree = self.junit()
                suite = tree.getroot().find("testsuite")
                case = suite.find("testcase")
                if change == "one":
                    tree = self.junit([sorted(self.mandatory)[0]])
                elif change == "missing":
                    suite.remove(case)
                    suite.set("tests", str(len(self.mandatory) - 1))
                elif change in {"extra", "duplicate"}:
                    clone = ET.fromstring(ET.tostring(case))
                    if change == "extra":
                        clone.set("name", "test_unexpected")
                    suite.append(clone)
                    suite.set("tests", "52")
                elif change == "wrong":
                    case.set("name", "test_renamed_without_review")
                elif change == "suite-error":
                    suite.set("errors", "1")
                elif change == "root-error":
                    tree.getroot().set("failures", "1")
                elif change == "wrong-count":
                    suite.set("tests", "999")
                else:
                    ET.SubElement(case, change)
                tree.write(self.root / "browser-junit.xml")
                self.execute("browser", "Record successful browser", False)

    def test_actual_helper_accepts_all_cases_and_rejects_reduced_report(self):
        report = self.root / "report.xml"
        self.junit().write(report)
        self.assertEqual(verify_junit(report), {"tests": 52, "inventory_sha256": self.inventory_digest})
        self.junit([sorted(self.mandatory)[0]]).write(report)
        with self.assertRaisesRegex(ValueError, "inventory mismatch"):
            verify_junit(report)

    def test_invalid_inventory_schema_count_and_duplicates_fail(self):
        from tools.quality.browser_inventory import INVENTORY

        original = json.loads(INVENTORY.read_bytes())
        path = self.root / "inventory.json"
        for key, value in (("schema", 2), ("schema", True), ("expected_count", 1), ("nodeids", []), ("nodeids", sorted(self.mandatory)[:-1] + [sorted(self.mandatory)[0]])):
            with self.subTest(key=key):
                path.write_text(json.dumps({**original, key: value}))
                with self.assertRaises(ValueError):
                    expected_inventory(path)

    def test_skipped_cancelled_failed_and_missing_jobs_fail(self):
        for job in ("frontend", "browser"):
            for result in ("skipped", "cancelled", "failure", None):
                with self.subTest(job=job, result=result):
                    needs = {"frontend": {"result": "success"}, "browser": {"result": "success"}}
                    if result is None:
                        del needs[job]
                    else:
                        needs[job]["result"] = result
                    self.env["NEEDS_JSON"] = json.dumps(needs)
                    self.gate(False)


if __name__ == "__main__":
    unittest.main()
