"""Conservative DEAD-01 scan/plan tests."""

import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import yaml


ROOT = Path(__file__).resolve().parents[4]
sys.path.insert(0, str(ROOT / "tools/quality"))
SPEC = importlib.util.spec_from_file_location("dead_code_checker", ROOT / "tools/quality/check_python_dead_code.py")
checker = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(checker)


PROFILE = {"id": "owned", "owner_module": "owned"}


class DeadCodeScanTests(unittest.TestCase):
    def analyze(self, sources):
        return checker.analyze_profile(sources, {path for path in sources if path.startswith("owned/")}, PROFILE)

    def test_static_import_preserves_exact_symbol(self):
        result = self.analyze({"owned/domain.py": "def active(): return 1\n", "api/consumer.py": "from owned.domain import active\n"})
        self.assertEqual(result["symbols"][0]["classification"], "PRESERVE")
        self.assertFalse(result["static_candidates"])

    def test_unreferenced_private_helper_is_candidate_but_never_patch(self):
        result = self.analyze({"owned/domain.py": "def _unused(): return 1\n"})
        self.assertEqual(result["static_candidates"][0]["symbol"], "_unused")
        self.assertEqual(result["plan"]["action"], "NO_AUTOMATIC_PATCH")
        self.assertEqual(result["dead_code_status"], "NOT_CONFIRMED")

    def test_unreachable_private_group_remains_candidates(self):
        result = self.analyze({"owned/domain.py": "def _a(): return _b()\ndef _b(): return _a()\n"})
        self.assertEqual({item["symbol"] for item in result["static_candidates"]}, {"_a", "_b"})

    def test_unconsumed_public_symbol_requires_review(self):
        result = self.analyze({"owned/domain.py": "def public_api(): return 1\n"})
        self.assertEqual(result["symbols"][0]["classification"], "REVIEW_REQUIRED")

    def test_unknown_dynamic_loading_is_incomplete(self):
        result = self.analyze({"owned/domain.py": "def _candidate(): return 1\n__import__(target)\n"})
        self.assertEqual(result["analysis_status"], "INCOMPLETE")
        self.assertEqual(result["symbols"][0]["classification"], "INCOMPLETE")

    def test_private_explicit_export_requires_review(self):
        result = self.analyze({"owned/domain.py": '__all__ = ["_exported"]\ndef _exported(): return 1\n'})
        self.assertEqual(result["symbols"][0]["classification"], "REVIEW_REQUIRED")
        self.assertEqual(result["symbols"][0]["evidence"][0]["kind"], "explicit_export")

    def test_private_type_only_consumer_requires_review(self):
        result = self.analyze(
            {
                "owned/domain.py": "def _typed(): return 1\n",
                "api/consumer.py": "from typing import TYPE_CHECKING\nif TYPE_CHECKING:\n from owned.domain import _typed\n",
            }
        )
        self.assertEqual(result["symbols"][0]["classification"], "REVIEW_REQUIRED")
        self.assertEqual(result["symbols"][0]["evidence"][0]["kind"], "type_only_import")

    def test_star_consumer_is_incomplete_and_never_a_candidate(self):
        result = self.analyze({"owned/domain.py": "def _hidden(): return 1\n", "api/consumer.py": "from owned.domain import *\n"})
        self.assertEqual(result["analysis_status"], "INCOMPLETE")
        self.assertEqual(result["symbols"][0]["classification"], "REVIEW_REQUIRED")
        self.assertFalse(result["static_candidates"])

    def test_candidate_fingerprint_ignores_display_line_changes(self):
        first = self.analyze({"owned/domain.py": "def _unused(): return 1\n"})["static_candidates"][0]["fingerprint"]
        second = self.analyze({"owned/domain.py": "\n\ndef _unused(): return 1\n"})["static_candidates"][0]["fingerprint"]
        self.assertEqual(first, second)

    def test_policy_rejects_unsafe_prefix(self):
        policy = {
            "schema_version": 1,
            "profiles": [{"id": "owned", "owner_module": "owned", "source_roots": ["owned"], "path_prefixes": ["../owned"]}],
        }
        with tempfile.TemporaryDirectory() as temp:
            policy_path = Path(temp) / "policy.yaml"
            policy_path.write_text(yaml.safe_dump(policy), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "Unsafe path prefix"):
                checker.load_policy(policy_path, {"owned"})


class CommandTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Dead-code fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        self.write("api/__init__.py", "")
        self.git("add", "api")
        self.git("commit", "-qm", "upstream")
        base = {"commit": self.git("rev-parse", "HEAD"), "tree": self.git("rev-parse", "HEAD^{tree}")}
        self.write(".gitignore", "output/\n")
        self.write("api/consumer.py", "from business_documents.domain import active\n")
        self.write("business_documents/__init__.py", "")
        self.write("business_documents/domain.py", "def active(): return 1\ndef _unused(): return 2\n")
        self.write("tools/quality/upstream-base.json", json.dumps(base))
        self.write(
            "tools/quality/python-dead-code.yaml",
            yaml.safe_dump(
                {
                    "schema_version": 1,
                    "profiles": [
                        {
                            "id": "domain",
                            "owner_module": "business-documents",
                            "source_roots": ["api", "business_documents"],
                            "path_prefixes": ["business_documents/"],
                        }
                    ],
                }
            ),
        )
        mapped = [
            ".gitignore",
            "api/consumer.py",
            "business_documents/__init__.py",
            "business_documents/domain.py",
            "tools/quality/core-changes.yaml",
            "tools/quality/file-inventory.json",
            "tools/quality/upstream-base.json",
            "tools/quality/module-map.yaml",
            "tools/quality/python-dead-code.yaml",
        ]
        self.write("tools/quality/module-map.yaml", yaml.safe_dump({"modules": [{"id": "business-documents", "paths": mapped}]}))

    def write(self, name, content):
        target = self.root / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root).decode().strip()

    def run_cli(self, output="output/report.json", *extra):
        return subprocess.run(
            [
                sys.executable,
                "-B",
                str(ROOT / "tools/quality/check_python_dead_code.py"),
                "--root",
                str(self.root),
                "--output",
                str(self.root / output),
                *extra,
            ],
            capture_output=True,
            text=True,
            timeout=25,
        )

    def test_real_cli_is_report_only_and_records_candidate(self):
        status = self.git("status", "--porcelain")
        result = self.run_cli()
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads((self.root / "output/report.json").read_text(encoding="utf-8"))
        self.assertEqual(report["analysis_status"], "OBSERVED")
        self.assertEqual(report["profiles"][0]["static_candidates"][0]["symbol"], "_unused")
        self.assertFalse(report["plan"]["apply_supported"])
        self.assertEqual(self.git("status", "--porcelain"), status)

    def test_cli_unknown_profile_and_source_output_are_rejected(self):
        for output, args in (("output/report.json", ("--profile", "typo")), ("business_documents/domain.py", ())):
            with self.subTest(output=output, args=args):
                result = self.run_cli(output, *args)
                self.assertEqual(result.returncode, 2)
                self.assertIn("INCOMPLETE", result.stderr)
        self.assertIn("def active", (self.root / "business_documents/domain.py").read_text(encoding="utf-8"))

    def test_cli_dynamic_owner_writes_incomplete_report(self):
        self.write("business_documents/domain.py", "def _candidate(): return 1\n__import__(target)\n")
        result = self.run_cli()
        self.assertEqual(result.returncode, 2, result.stderr)
        report = json.loads((self.root / "output/report.json").read_text(encoding="utf-8"))
        self.assertEqual(report["analysis_status"], "INCOMPLETE")
        self.assertEqual(report["profiles"][0]["symbols"][0]["classification"], "INCOMPLETE")


if __name__ == "__main__":
    unittest.main()
