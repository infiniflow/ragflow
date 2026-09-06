"""Import/metric and real Git CLI probes; application code is never imported."""

import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import yaml


ROOT = Path(__file__).resolve().parents[4]
sys.path.insert(0, str(ROOT / "tools/quality"))
SPEC = importlib.util.spec_from_file_location("python_observer", ROOT / "tools/quality/inspect_python.py")
observer = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(observer)


class ImportAnalysisTests(unittest.TestCase):
    def analyze(self, sources, selected=("owned/domain.py",)):
        owners = {path: {"origin": "extension" if path.startswith("owned/") else "upstream", "owner": "fixture"} for path in sources}
        return observer.analyze(sources, set(selected), owners)

    def test_imported_symbol_is_not_invented_module(self):
        report = self.analyze({"owned/domain.py": "from owned.dto import Value", "owned/dto.py": "from dataclasses import dataclass\nclass Value: pass"})
        self.assertEqual(report["analysis_status"], "OBSERVED")
        self.assertEqual(report["policy_status"], "NOT_EVALUATED")
        self.assertFalse(any(edge["target"].endswith(".Value") for edge in report["edges"]))

    def test_relative_imports_and_namespace_packages(self):
        report = self.analyze({"owned/domain.py": "from . import dto\nfrom .dto import Value", "owned/dto.py": "class Value: pass", "owned/__init__.py": "from . import dto"})
        targets = {edge["target"] for edge in report["edges"]}
        self.assertTrue({"owned", "owned.dto"} <= targets)
        self.assertFalse(report["incomplete_reasons"])

    def test_relative_import_above_root_is_incomplete(self):
        report = self.analyze({"owned/domain.py": "from ...outside import Value"})
        self.assertEqual(report["incomplete_reasons"][0]["kind"], "invalid_relative_import")

    def test_implicit_parent_reveals_bootstrap_without_executing_it(self):
        report = self.analyze(
            {"api/__init__.py": "", "api/apps/__init__.py": "import peewee\nraise RuntimeError('must never execute')", "api/apps/owned/domain.py": "value = 1"}, ("api/apps/owned/domain.py",)
        )
        bootstrap = next(path for path in report["coupling_paths"] if path["category"] == "bootstrap")
        orm = next(path for path in report["coupling_paths"] if path["target"] == "peewee")
        self.assertEqual(bootstrap["chain"][0]["kind"], "parent_init")
        self.assertEqual([edge["target"] for edge in orm["chain"]], ["api.apps", "peewee"])

    def test_helper_reveals_transitive_orm_and_upstream_origin(self):
        report = self.analyze({"owned/domain.py": "from .helper import run", "owned/helper.py": "from api.db.db_models import Model", "api/db/db_models.py": "import peewee"})
        orm = next(path for path in report["coupling_paths"] if path["target"] == "peewee")
        self.assertEqual([edge["target"] for edge in orm["chain"]], ["owned.helper", "api.db.db_models", "peewee"])
        upstream = next(node for node in report["nodes"] if node["module"] == "api.db.db_models")
        self.assertEqual(upstream["origin"], "upstream")

    def test_literal_dynamic_aliases_are_resolved(self):
        for source in ("import importlib as il\nil.import_module('peewee')", "from importlib import import_module as load\nload('peewee')", "__import__('peewee')"):
            with self.subTest(source=source):
                report = self.analyze({"owned/domain.py": source})
                self.assertTrue(any(edge["target"] == "peewee" and edge["kind"] == "literal_dynamic_import" for edge in report["edges"]))

    def test_computed_shadowed_and_escaped_import_callables_are_unknown(self):
        for source in (
            "from importlib import import_module as load\nload(config['module'])",
            "from importlib import import_module as load\nload = other\nload('peewee')",
            "import importlib as il\nil = other\nil.import_module('peewee')",
            "loader = __import__\nloader(target)",
            "loader: object = __import__\nloader(target)",
            "(loader := __import__)(target)",
            "from importlib import import_module as load\nfrom other import load\nload('peewee')",
            "from importlib import import_module as load\ndef helper():\n from external import factory as load\nload('peewee')",
        ):
            with self.subTest(source=source):
                report = self.analyze({"owned/domain.py": source})
                self.assertEqual(report["analysis_status"], "INCOMPLETE")

    def test_eager_annotations_and_deferred_annotations_are_distinct(self):
        source = "def use(value: __import__('peewee')) -> __import__('requests'):\n pass\n"
        eager = self.analyze({"owned/domain.py": source})
        deferred = self.analyze({"owned/domain.py": "from __future__ import annotations\n" + source})
        self.assertEqual({item["target"] for item in eager["coupling_paths"]}, {"peewee", "requests"})
        self.assertFalse(deferred["coupling_paths"])
        self.assertTrue(all(edge["phase"] == "module" for edge in eager["edges"]))

    def test_relative_builtin_import_is_not_claimed_absolute(self):
        report = self.analyze({"owned/domain.py": "__import__('child', level=1)"})
        self.assertEqual(report["analysis_status"], "INCOMPLETE")
        self.assertFalse(any(edge["target"] == "child" for edge in report["edges"]))

    def test_lambda_import_is_deferred(self):
        report = self.analyze({"owned/domain.py": "loader = lambda: __import__('peewee')"})
        self.assertEqual(next(edge for edge in report["edges"] if edge["target"] == "peewee")["phase"], "call")

    def test_type_only_and_conditional_and_deferred_imports_are_distinct(self):
        source = "from typing import TYPE_CHECKING\nif TYPE_CHECKING:\n import peewee\nif enabled:\n import requests\ndef use():\n import redis\n"
        report = self.analyze({"owned/domain.py": source})
        self.assertFalse(any(path["target"] == "peewee" for path in report["coupling_paths"]))
        requests = next(edge for edge in report["edges"] if edge["target"] == "requests")
        redis = next(edge for edge in report["edges"] if edge["target"] == "redis")
        self.assertTrue(requests["conditional"])
        self.assertEqual(redis["phase"], "call")

    def test_both_supported_fallbacks_remain_visible(self):
        report = self.analyze({"owned/domain.py": "try:\n import requests\nexcept ImportError:\n import httpx\n"})
        self.assertEqual({edge["target"] for edge in report["edges"] if edge["conditional"]}, {"requests", "httpx"})

    def test_missing_local_and_external_imports_are_distinct(self):
        report = self.analyze({"owned/domain.py": "import owned.missing\nimport external_dependency"})
        self.assertEqual([item["detail"] for item in report["incomplete_reasons"]], ["owned.missing"])
        self.assertEqual(next(edge for edge in report["edges"] if edge["target"] == "external_dependency")["resolution"], "external")

    def test_dynamic_exports_star_and_syntax_errors_are_not_silenced(self):
        for source in ("from owned.dto import *", "def __getattr__(name):\n return load(name)", "this is invalid python!"):
            with self.subTest(source=source):
                report = self.analyze({"owned/domain.py": source, "owned/dto.py": ""})
                self.assertEqual(report["analysis_status"], "INCOMPLETE")

    def test_instance_getattr_is_not_mistaken_for_module_exports(self):
        report = self.analyze({"owned/domain.py": "class Value:\n def __getattr__(self, name):\n  return 1\n"})
        self.assertEqual(report["analysis_status"], "OBSERVED")
        self.assertFalse(report["incomplete_reasons"])

    def test_ambiguous_module_rejected_and_registration_cycle_terminates(self):
        with self.assertRaisesRegex(ValueError, "Ambiguous module"):
            self.analyze({"owned/domain.py": "", "owned.py": "", "owned/__init__.py": ""})
        report = self.analyze({"owned/domain.py": "value = 1", "owned/__init__.py": "from . import domain"})
        self.assertEqual(report["analysis_status"], "OBSERVED")
        self.assertEqual(report["dead_code_status"], "NOT_ANALYZED")

    def test_nested_functions_do_not_hide_or_double_count_complexity(self):
        report = self.analyze({"owned/domain.py": "def outer(items):\n def inner(items):\n  for a in items:\n   for b in items:\n    if a and b:\n     yield a\n return inner(items)\n"})
        functions = {item["symbol"]: item for item in report["functions"]}
        self.assertEqual(functions["outer"]["branch_points"], 0)
        self.assertEqual(functions["outer.inner"]["branch_points"], 4)
        self.assertEqual(functions["outer.inner"]["max_loop_nesting"], 2)
        self.assertIn("max_loop_nesting", functions["outer.inner"]["signals"])

    def test_comprehension_generators_count_as_nested_loops(self):
        report = self.analyze({"owned/domain.py": "def pairs(xs, ys):\n return [(x,y) for x in xs for y in ys if y]\n"})
        self.assertEqual(report["functions"][0]["max_loop_nesting"], 2)
        self.assertEqual(report["functions"][0]["branch_points"], 3)


class CommandTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Analysis fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        self.write("api/__init__.py", "")
        self.git("add", "api")
        self.git("commit", "-qm", "upstream")
        self.base = {"commit": self.git("rev-parse", "HEAD"), "tree": self.git("rev-parse", "HEAD^{tree}")}
        self.write(".gitignore", "output/\n")
        self.write("api/owned.py", "import json\n")
        self.write("tools/quality/upstream-base.json", json.dumps(self.base))
        self.mapping = {
            "modules": [
                {
                    "id": "fixture",
                    "paths": [
                        ".gitignore",
                        "api/owned.py",
                        "tools/quality/upstream-base.json",
                        "tools/quality/module-map.yaml",
                        "tools/quality/file-inventory.json",
                        "tools/quality/core-changes.yaml",
                    ],
                }
            ]
        }
        self.write("tools/quality/module-map.yaml", yaml.safe_dump(self.mapping))

    def write(self, name, text):
        target = self.root / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text, encoding="utf-8")

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root).decode().strip()

    def run_cli(self, *extra):
        return subprocess.run(
            [sys.executable, "-B", str(ROOT / "tools/quality/inspect_python.py"), "--root", str(self.root), "--module", "fixture", "--output", str(self.root / "output/report.json"), *extra],
            capture_output=True,
            text=True,
            timeout=25,
        )

    def test_real_cli_records_identity_without_writing_sources(self):
        status = self.git("status", "--porcelain")
        result = self.run_cli()
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads((self.root / "output/report.json").read_text())
        self.assertEqual(report["input"]["upstream_base"], self.base["commit"])
        self.assertEqual(report["scope"]["selected_paths"], ["api/owned.py"])
        self.assertEqual(self.git("status", "--porcelain"), status)

    def test_incomplete_analysis_writes_report_but_returns_nonzero(self):
        self.write("api/owned.py", "import api.missing")
        result = self.run_cli()
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertEqual(json.loads((self.root / "output/report.json").read_text())["analysis_status"], "INCOMPLETE")

    def test_unknown_owner_or_source_output_is_rejected(self):
        for args in (("--module", "typo"), ("--output", str(self.root / "api/owned.py"))):
            with self.subTest(args=args):
                result = self.run_cli(*args)
                self.assertEqual(result.returncode, 2)
                self.assertIn("INCOMPLETE", result.stderr)
        self.assertEqual((self.root / "api/owned.py").read_text(), "import json\n")

    def test_wrong_upstream_tree_or_unclassified_source_fails_closed(self):
        self.write("api/new.py", "")
        self.assertEqual(self.run_cli().returncode, 2)
        self.mapping["modules"][0]["paths"].append("api/new.py")
        self.write("tools/quality/module-map.yaml", yaml.safe_dump(self.mapping))
        self.base["tree"] = "0" * 40
        self.write("tools/quality/upstream-base.json", json.dumps(self.base))
        result = self.run_cli()
        self.assertEqual(result.returncode, 2)
        self.assertIn("Upstream tree differs", result.stderr)

    def test_concurrent_source_edit_does_not_produce_report(self):
        original = observer.analyze

        def edit_during_analysis(*args):
            result = original(*args)
            self.write("api/owned.py", "import peewee")
            return result

        with patch.object(observer, "analyze", side_effect=edit_during_analysis):
            code = observer.main(["--root", str(self.root), "--module", "fixture", "--output", str(self.root / "output/report.json")])
        self.assertEqual(code, 2)
        self.assertFalse((self.root / "output/report.json").exists())


if __name__ == "__main__":
    unittest.main()
