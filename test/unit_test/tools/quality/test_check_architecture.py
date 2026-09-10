"""ARC-01/02/03 static, isolated-runtime, and real Git CLI probes."""

import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import yaml

ROOT = Path(__file__).resolve().parents[4]
sys.path.insert(0, str(ROOT / "tools/quality"))
SPEC = importlib.util.spec_from_file_location("architecture_checker", ROOT / "tools/quality/check_architecture.py")
checker = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(checker)


class BoundaryEvaluationTests(unittest.TestCase):
    boundary = {
        "id": "owned-domain",
        "owner_module": "owned",
        "rule_id": "ARC-01",
        "forbidden_markers": ["bootstrap", "orm", "http_framework", "network_client"],
    }

    def evaluate(self, sources, selected=("owned/domain.py",)):
        ownership = {path: {"origin": "extension", "owner": "owned"} for path in sources}
        return checker.evaluate_boundary(sources, set(selected), ownership, self.boundary)

    def test_pure_domain_and_separate_adapter_pass(self):
        result = self.evaluate(
            {
                "owned/domain.py": "from owned.dto import Value\n",
                "owned/dto.py": "class Value: pass\n",
                "owned/adapter.py": "from api.db.db_models import Model\n",
                "api/db/db_models.py": "import peewee\n",
            }
        )
        self.assertEqual(result["status"], "PASS")
        self.assertFalse(result["findings"])

    def test_direct_and_transitive_orm_dependencies_fail(self):
        for sources in (
            {"owned/domain.py": "import peewee\n"},
            {
                "owned/domain.py": "from owned.helper import load\n",
                "owned/helper.py": "from api.db.db_models import Model\n",
                "api/db/db_models.py": "import peewee\n",
            },
        ):
            with self.subTest(sources=sources):
                result = self.evaluate(sources)
                self.assertEqual(result["status"], "FAIL")
                self.assertTrue(any(item["rule_id"] == "ARC-01" for item in result["findings"]))

    def test_parent_package_bootstrap_fails_without_importing_it(self):
        result = self.evaluate(
            {
                "api/apps/__init__.py": "import peewee\nraise RuntimeError('must not execute')\n",
                "api/apps/owned/domain.py": "value = 1\n",
            },
            ("api/apps/owned/domain.py",),
        )
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual({item["category"] for item in result["findings"]}, {"bootstrap", "orm"})

    def test_unknown_dynamic_import_is_incomplete_even_with_known_finding(self):
        result = self.evaluate({"owned/domain.py": "import peewee\n__import__(target)\n"})
        self.assertEqual(result["status"], "INCOMPLETE")
        self.assertTrue(result["findings"])
        self.assertTrue(result["incomplete_reasons"])

    def test_fingerprint_ignores_display_line_changes(self):
        first = self.evaluate({"owned/domain.py": "import peewee\n"})["findings"][0]["fingerprint"]
        second = self.evaluate({"owned/domain.py": "\n\nimport peewee\n"})["findings"][0]["fingerprint"]
        self.assertEqual(first, second)

    def test_unknown_marker_is_configuration_error(self):
        boundary = {**self.boundary, "forbidden_markers": ["database-ish"]}
        with self.assertRaisesRegex(ValueError, "forbidden_markers"):
            checker.evaluate_boundary({"owned/domain.py": ""}, {"owned/domain.py"}, {}, boundary)


class ConnectionEvaluationTests(unittest.TestCase):
    connection = {
        "id": "owned-wiring",
        "owner_module": "owned",
        "rule_id": "ARC-02",
        "extension_path_prefixes": ["owned/"],
        "extension_module_prefixes": ["owned"],
        "allowed_forward_imports": [
            {"source": "owned.adapter", "target": "api.service", "symbols": ["fetch"]},
        ],
        "allowed_reverse_imports": [
            {"source": "api.service", "target": "owned.domain", "symbols": ["Value"]},
        ],
    }

    def evaluate(self, sources, connection=None):
        return checker.evaluate_connection(sources, connection or self.connection)

    def test_exact_forward_and_reverse_symbols_pass(self):
        result = self.evaluate(
            {
                "owned/domain.py": "Value = object()\n",
                "owned/adapter.py": "from api.service import fetch\n",
                "api/service.py": "from owned.domain import Value\ndef fetch(): pass\n",
            }
        )
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["scope"]["observed_imports"], 2)
        self.assertTrue(all(item["approved"] for item in result["observed_imports"]))

    def test_unknown_reverse_import_and_extra_symbol_fail(self):
        result = self.evaluate(
            {
                "owned/domain.py": "Value = Other = object()\n",
                "owned/adapter.py": "from api.service import fetch\n",
                "api/service.py": "from owned.domain import Value, Other\ndef fetch(): pass\n",
                "api/other.py": "from owned.domain import Value\n",
            }
        )
        self.assertEqual(result["status"], "FAIL")
        unapproved = [item for item in result["findings"] if item["kind"] == "unapproved_import"]
        self.assertEqual({(item["source"], item["symbol"]) for item in unapproved}, {("api.service", "Other"), ("api.other", "Value")})

    def test_owned_helper_cannot_hide_forward_core_import(self):
        connection = {**self.connection, "allowed_forward_imports": []}
        result = self.evaluate(
            {
                "owned/domain.py": "from owned.helper import load\n",
                "owned/helper.py": "from api.db import load\n",
                "api/db.py": "def load(): pass\n",
                "api/service.py": "from owned.domain import Value\n",
            },
            connection,
        )
        self.assertEqual(result["status"], "FAIL")
        finding = next(item for item in result["findings"] if item["kind"] == "unapproved_import")
        self.assertEqual((finding["source"], finding["target"], finding["symbol"]), ("owned.helper", "api.db", "load"))

    def test_stale_approval_fails(self):
        result = self.evaluate(
            {
                "owned/domain.py": "Value = object()\n",
                "owned/adapter.py": "from api.service import fetch\n",
                "api/service.py": "def fetch(): pass\n",
            }
        )
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(any(item["kind"] == "stale_approval" for item in result["findings"]))

    def test_dynamic_extension_import_is_incomplete_and_keeps_findings(self):
        connection = {**self.connection, "allowed_forward_imports": [], "allowed_reverse_imports": []}
        result = self.evaluate(
            {
                "owned/domain.py": "__import__(target)\nfrom api.db import Model\n",
                "api/db.py": "Model = object()\n",
            },
            connection,
        )
        self.assertEqual(result["status"], "INCOMPLETE")
        self.assertTrue(result["findings"])
        self.assertTrue(result["incomplete_reasons"])

    def test_explicit_module_access_can_be_approved_without_wildcard(self):
        connection = {
            **self.connection,
            "allowed_forward_imports": [],
            "allowed_reverse_imports": [
                {"source": "api.service", "target": "owned.domain", "symbols": [checker.MODULE_ACCESS]},
            ],
        }
        result = self.evaluate({"owned/domain.py": "Value = object()\n", "api/service.py": "import owned.domain\n"}, connection)
        self.assertEqual(result["status"], "PASS")

    def test_allowlist_rejects_wildcards_and_duplicates(self):
        with self.assertRaisesRegex(ValueError, "wildcard"):
            checker._normalized_allowed_imports(
                [{"source": "api.service", "target": "owned.domain", "symbols": ["*"]}],
                "owned-wiring",
                "allowed_reverse_imports",
            )


class CycleEvaluationTests(unittest.TestCase):
    cycle_check = {
        "id": "owned-cycles",
        "owner_module": "owned",
        "rule_id": "ARC-03",
    }

    def test_acyclic_owned_modules_pass(self):
        sources = {
            "owned/__init__.py": "",
            "owned/a.py": "from owned.b import Value\n",
            "owned/b.py": "Value = object()\n",
        }
        result = checker.evaluate_cycle_check(sources, set(sources), self.cycle_check)
        self.assertEqual(result["status"], "PASS")
        self.assertFalse(result["findings"])

    def test_explicit_owned_cycle_fails_but_parent_init_does_not(self):
        sources = {
            "owned/__init__.py": "from owned.a import A\n",
            "owned/a.py": "from owned.b import B\nA = object()\n",
            "owned/b.py": "from owned.a import A\nB = object()\n",
        }
        result = checker.evaluate_cycle_check(sources, set(sources), self.cycle_check)
        self.assertEqual(result["status"], "FAIL")
        finding = result["findings"][0]
        self.assertEqual(finding["modules"], ["owned.a", "owned.b"])
        self.assertTrue(all(edge["kind"] != "parent_init" for edge in finding["edges"]))

    def test_unknown_dynamic_loading_is_incomplete(self):
        sources = {"owned/a.py": "__import__(target)\n"}
        result = checker.evaluate_cycle_check(sources, set(sources), self.cycle_check)
        self.assertEqual(result["status"], "INCOMPLETE")
        self.assertTrue(result["incomplete_reasons"])
        with self.assertRaisesRegex(ValueError, "duplicate"):
            checker._normalized_allowed_imports(
                [
                    {"source": "api.service", "target": "owned.domain", "symbols": ["Value", "Other"]},
                    {"source": "api.service", "target": "owned.domain", "symbols": ["Value"]},
                ],
                "owned-wiring",
                "allowed_reverse_imports",
            )


class RuntimeProbeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def write(self, name, text):
        target = self.root / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text, encoding="utf-8")

    def import_probe(self, **overrides):
        probe = {
            "id": "owned-import",
            "owner_module": "owned",
            "profile": "fixture",
            "rule_id": "ARC-01",
            "kind": "isolated_import",
            "timeout_seconds": 10,
            "allow_output": False,
            "targets": [{"module": "owned.domain", "path": "owned/domain.py", "required_symbols": ["Value"]}],
            "forbidden_module_prefixes": ["unsafe_dependency"],
            **overrides,
        }
        return checker.evaluate_runtime_probe(self.root, probe, source_roots={"owned"})

    def test_clean_import_runs_in_fresh_process(self):
        self.write("owned/__init__.py", "")
        self.write("owned/domain.py", "Value = object()\n")
        result = self.import_probe()
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["observations"][0]["module"], "owned.domain")
        self.assertFalse(result["findings"])

    def test_nested_python_path_import_runs_in_fresh_process(self):
        self.write("services/fixture/src/owned/__init__.py", "")
        self.write("services/fixture/src/owned/domain.py", "Value = object()\n")
        result = checker.evaluate_runtime_probe(
            self.root,
            {
                "id": "owned-import",
                "owner_module": "owned",
                "profile": "fixture",
                "rule_id": "ARC-01",
                "kind": "isolated_import",
                "timeout_seconds": 10,
                "allow_output": False,
                "targets": [
                    {
                        "module": "owned.domain",
                        "path": "services/fixture/src/owned/domain.py",
                        "required_symbols": ["Value"],
                    }
                ],
                "forbidden_module_prefixes": [],
            },
            source_roots={"services"},
            python_paths=["services/fixture/src"],
        )
        self.assertEqual(result["status"], "PASS", result)
        self.assertEqual(result["scope"]["loaded_repo_modules"], 2)

    def test_parent_bootstrap_forbidden_module_is_runtime_failure(self):
        self.write("unsafe_dependency.py", "value = 1\n")
        self.write("owned/__init__.py", "import unsafe_dependency\n")
        self.write("owned/domain.py", "Value = object()\n")
        result = self.import_probe()
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(any(item["kind"] == "forbidden_runtime_module" for item in result["findings"]))

    def test_import_exception_is_incomplete_not_pass(self):
        self.write("owned/__init__.py", "")
        self.write("owned/domain.py", "raise RuntimeError('broken import')\n")
        result = self.import_probe()
        self.assertEqual(result["status"], "INCOMPLETE")
        self.assertTrue(result["incomplete_reasons"])

    def test_import_time_socket_effect_is_blocked_and_fails(self):
        self.write("owned/__init__.py", "")
        self.write(
            "owned/domain.py",
            "import socket\nsocket.create_connection(('127.0.0.1', 1))\nValue = object()\n",
        )
        result = self.import_probe()
        self.assertEqual(result["status"], "FAIL")
        self.assertTrue(any(item["kind"] == "blocked_import_side_effect" for item in result["findings"]))
        self.assertIn("socket.connect", result["audit_events"])

    def test_quart_registration_inventory_is_exact(self):
        self.write("api/__init__.py", "")
        self.write("api/apps/__init__.py", "raise RuntimeError('bootstrap must be stubbed')\n")
        self.write(
            "api/apps/restful_apis/fixture_api.py",
            "from api.apps import login_required\n@manager.route('/items', methods=['GET'])\n@login_required\nasync def list_items():\n    return {}\n",
        )
        probe = {
            "id": "fixture-registration",
            "owner_module": "owned",
            "profile": "fixture",
            "rule_id": "ARC-02",
            "kind": "ragflow_quart_blueprint",
            "timeout_seconds": 10,
            "allow_output": False,
            "target": {
                "module": "api.apps.restful_apis.fixture_api_runtime_probe",
                "path": "api/apps/restful_apis/fixture_api.py",
                "required_symbols": [],
            },
            "parent_stub": {"module": "api.apps", "path": "api/apps"},
            "blueprint_name": "fixture_api_runtime_probe",
            "url_prefix": "/api/v1",
            "expected_routes": [{"path": "/api/v1/items", "methods": ["GET"], "endpoint": "list_items"}],
            "forbidden_module_prefixes": [],
        }
        passed = checker.evaluate_runtime_probe(self.root, probe)
        self.assertEqual(passed["status"], "PASS", passed)
        probe["stub_modules"] = [{"module": "api.service", "symbols": {"Service": "class"}}]
        stale = checker.evaluate_runtime_probe(self.root, probe)
        self.assertEqual(stale["status"], "FAIL")
        self.assertTrue(any(item["kind"] == "stale_runtime_stub" for item in stale["findings"]))
        probe["stub_modules"] = []
        probe["expected_routes"] = [{"path": "/api/v1/missing", "methods": ["GET"], "endpoint": "list_items"}]
        failed = checker.evaluate_runtime_probe(self.root, probe)
        self.assertEqual(failed["status"], "FAIL")
        self.assertEqual({item["kind"] for item in failed["findings"]}, {"missing_runtime_route", "unexpected_runtime_route"})

    def test_required_pytest_contract_distinguishes_pass_fail_and_skip(self):
        probe = {
            "id": "fixture-contract",
            "owner_module": "owned",
            "profile": "fixture",
            "rule_id": "ARC-02",
            "kind": "pytest_contract",
            "timeout_seconds": 10,
            "test_ids": ["test_contract.py::test_contract"],
            "expected_tests": 1,
        }
        self.write("conftest.py", "raise RuntimeError('candidate conftest must not load')\n")
        with patch.dict(
            os.environ,
            {
                "GITHUB_ENV": "command-file",
                "ARCHITECTURE_EVIDENCE_DIR": "evidence",
                "API_TOKEN": "secret",
                "PYTHONPATH": "attacker",
                "SAFE_VALUE": "kept",
            },
            clear=True,
        ):
            environment = checker.sanitized_child_environment(PYTEST_DISABLE_PLUGIN_AUTOLOAD="1")
        self.assertEqual(environment, {"SAFE_VALUE": "kept", "PYTEST_DISABLE_PLUGIN_AUTOLOAD": "1"})
        cases = (
            ("def test_contract():\n    assert True\n", "PASS", set()),
            ("def test_contract():\n    assert False, 'broken contract'\n", "FAIL", {"contract_test_failure"}),
            ("import pytest\ndef test_contract():\n    pytest.skip('missing prerequisite')\n", "INCOMPLETE", set()),
        )
        for source, status, finding_kinds in cases:
            with self.subTest(status=status):
                self.write("test_contract.py", source)
                result = checker.evaluate_runtime_probe(self.root, probe)
                self.assertEqual(result["status"], status, result)
                self.assertEqual({item["kind"] for item in result["findings"]}, finding_kinds)
                self.assertEqual(result["scope"]["selected_paths"], ["test_contract.py"])
                self.assertFalse(result["observations"][0]["third_party_plugin_autoload"])
                self.assertFalse(result["observations"][0]["conftest_loading"])
                self.assertEqual(result["observations"][0]["explicit_plugins"], ["pytest_asyncio.plugin"])
                if status == "INCOMPLETE":
                    self.assertTrue(any(item["kind"] == "pytest_contract_skipped" for item in result["incomplete_reasons"]))

    def test_pytest_contract_supports_nested_python_path_and_working_directory(self):
        self.write("services/fixture/src/contract_package/__init__.py", "VALUE = 1\n")
        self.write(
            "services/fixture/tests/test_contract.py",
            "from pathlib import Path\nfrom contract_package import VALUE\n\ndef test_contract():\n    assert VALUE == 1\n    assert Path.cwd().name == 'fixture'\n",
        )
        probe = {
            "id": "nested-contract",
            "owner_module": "owned",
            "profile": "fixture",
            "rule_id": "ARC-02",
            "kind": "pytest_contract",
            "timeout_seconds": 10,
            "working_directory": "services/fixture",
            "test_ids": ["services/fixture/tests/test_contract.py::test_contract"],
            "expected_tests": 1,
        }
        result = checker.evaluate_runtime_probe(
            self.root,
            probe,
            python_paths=["services/fixture/src"],
        )
        self.assertEqual(result["status"], "PASS", result)


class CommandTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Architecture fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        self.write("README.md", "upstream\n")
        self.git("add", "README.md")
        self.git("commit", "-qm", "upstream")
        self.base = {"commit": self.git("rev-parse", "HEAD"), "tree": self.git("rev-parse", "HEAD^{tree}")}
        self.write(".gitignore", "output/\n")
        self.write("business_documents/__init__.py", "from business_documents.domain import value\n")
        self.write("business_documents/domain.py", "value = 1\n")
        self.write("tools/quality/upstream-base.json", json.dumps(self.base))
        self.policy = {
            "schema_version": 1,
            "profiles": [{"id": "fixture", "source_roots": ["business_documents"]}],
            "boundaries": [
                {
                    "id": "fixture-domain",
                    "owner_module": "fixture",
                    "profile": "fixture",
                    "rule_id": "ARC-01",
                    "path_prefixes": ["business_documents/"],
                    "forbidden_markers": ["orm", "bootstrap"],
                }
            ],
            "connections": [
                {
                    "id": "fixture-wiring",
                    "owner_module": "fixture",
                    "profile": "fixture",
                    "rule_id": "ARC-02",
                    "extension_path_prefixes": ["business_documents/"],
                    "extension_module_prefixes": ["business_documents"],
                    "allowed_forward_imports": [],
                    "allowed_reverse_imports": [],
                }
            ],
            "cycle_checks": [
                {
                    "id": "fixture-cycles",
                    "owner_module": "fixture",
                    "profile": "fixture",
                    "rule_id": "ARC-03",
                    "path_prefixes": ["business_documents/"],
                }
            ],
            "runtime_probes": [
                {
                    "id": "fixture-import",
                    "owner_module": "fixture",
                    "profile": "fixture",
                    "rule_id": "ARC-01",
                    "kind": "isolated_import",
                    "targets": [
                        {
                            "module": "business_documents.domain",
                            "path": "business_documents/domain.py",
                            "required_symbols": ["value"],
                        }
                    ],
                    "forbidden_module_prefixes": ["unsafe_dependency"],
                }
            ],
        }
        self.write("tools/quality/python-boundaries.yaml", yaml.safe_dump(self.policy))
        paths = [
            ".gitignore",
            "business_documents/__init__.py",
            "business_documents/domain.py",
            "tools/quality/core-changes.yaml",
            "tools/quality/file-inventory.json",
            "tools/quality/module-map.yaml",
            "tools/quality/python-boundaries.yaml",
            "tools/quality/upstream-base.json",
        ]
        mapping = {
            "modules": [
                {
                    "id": "fixture",
                    "owner": "fixture-owner",
                    "core_change_reason": "fixture",
                    "preserve": ["fixture"],
                    "tests": ["fixture"],
                    "paths": paths,
                }
            ]
        }
        self.write("tools/quality/module-map.yaml", yaml.safe_dump(mapping))

    def write(self, name, text):
        target = self.root / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text, encoding="utf-8")

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root).decode().strip()

    def run_cli(self, output="output/report.json", *extra):
        return subprocess.run(
            [
                sys.executable,
                "-B",
                str(ROOT / "tools/quality/check_architecture.py"),
                "--root",
                str(self.root),
                "--policy",
                str(self.root / "tools/quality/python-boundaries.yaml"),
                "--output",
                str(self.root / output),
                *extra,
            ],
            capture_output=True,
            text=True,
            timeout=25,
        )

    def test_real_cli_pass_is_report_only_and_preserves_tree(self):
        status = self.git("status", "--porcelain")
        result = self.run_cli()
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads((self.root / "output/report.json").read_text())
        self.assertEqual(report["policy_status"], "PASS")
        self.assertEqual(report["mode"], "T2_REPORT_ONLY")
        self.assertEqual(report["rules"], ["ARC-01", "ARC-02", "ARC-03"])
        self.assertEqual(len(report["connections"]), 1)
        self.assertEqual(len(report["cycle_checks"]), 1)
        self.assertEqual(len(report["runtime_probes"]), 1)
        self.assertEqual(report["runtime_probes"][0]["status"], "PASS")
        self.assertIsNone(report["input"]["pr_base_sha"])
        self.assertEqual(report["input"]["pr_base_status"], "NOT_PROVIDED_LOCAL_REPORT")
        self.assertEqual(self.git("status", "--porcelain"), status)

        with_base = self.run_cli("output/with-base.json", "--base-ref", "HEAD")
        self.assertEqual(with_base.returncode, 0, with_base.stderr)
        based_report = json.loads((self.root / "output/with-base.json").read_text())
        self.assertEqual(based_report["input"]["pr_base_sha"], self.base["commit"])
        self.assertEqual(based_report["input"]["pr_base_status"], "RESOLVED")

    def test_cli_reports_fail_and_incomplete_with_distinct_codes(self):
        self.write("business_documents/domain.py", "import peewee\nvalue = 1\n")
        failed = self.run_cli("output/fail.json")
        self.assertEqual(failed.returncode, 1, failed.stderr)
        self.assertEqual(json.loads((self.root / "output/fail.json").read_text())["policy_status"], "FAIL")

        self.write("business_documents/domain.py", "__import__(target)\n")
        incomplete = self.run_cli("output/incomplete.json")
        self.assertEqual(incomplete.returncode, 2, incomplete.stderr)
        self.assertEqual(json.loads((self.root / "output/incomplete.json").read_text())["policy_status"], "INCOMPLETE")

    def test_cli_rejects_python_path_outside_repository(self):
        self.policy["profiles"][0]["python_paths"] = ["../outside"]
        self.write("tools/quality/python-boundaries.yaml", yaml.safe_dump(self.policy))
        result = self.run_cli("output/unsafe-python-path.json")
        self.assertEqual(result.returncode, 2)
        self.assertIn("Unsafe Profile fixture python_path", result.stderr)
        self.assertFalse((self.root / "output/unsafe-python-path.json").exists())

    def test_cli_reports_missing_profile_interpreter_as_incomplete(self):
        self.policy["profiles"][0]["python_executable_candidates"] = ["missing/bin/python"]
        self.write("tools/quality/python-boundaries.yaml", yaml.safe_dump(self.policy))
        result = self.run_cli("output/missing-interpreter.json")
        self.assertEqual(result.returncode, 2, result.stderr)
        report = json.loads((self.root / "output/missing-interpreter.json").read_text())
        self.assertEqual(report["policy_status"], "INCOMPLETE")
        self.assertEqual(report["runtime_probes"][0]["incomplete_reasons"][0]["kind"], "python_executable_missing")

    def test_empty_scope_and_unknown_boundary_fail_closed(self):
        self.policy["boundaries"][0]["path_prefixes"] = ["missing/"]
        self.write("tools/quality/python-boundaries.yaml", yaml.safe_dump(self.policy))
        empty = self.run_cli("output/empty.json")
        self.assertEqual(empty.returncode, 2)
        self.assertIn("selected no Python files", empty.stderr)
        unknown = self.run_cli("output/unknown.json", "--boundary", "typo")
        self.assertEqual(unknown.returncode, 2)
        self.assertIn("Unknown boundary ID", unknown.stderr)
        unknown_connection = self.run_cli("output/unknown-connection.json", "--connection", "typo")
        self.assertEqual(unknown_connection.returncode, 2)
        self.assertIn("Unknown connection ID", unknown_connection.stderr)
        unknown_cycle = self.run_cli("output/unknown-cycle.json", "--cycle-check", "typo")
        self.assertEqual(unknown_cycle.returncode, 2)
        self.assertIn("Unknown cycle check ID", unknown_cycle.stderr)
        unknown_runtime = self.run_cli("output/unknown-runtime.json", "--runtime-probe", "typo")
        self.assertEqual(unknown_runtime.returncode, 2)
        self.assertIn("Unknown runtime probe ID", unknown_runtime.stderr)

    def test_runtime_selector_runs_only_requested_probe(self):
        result = self.run_cli("output/runtime-only.json", "--runtime-probe", "fixture-import")
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads((self.root / "output/runtime-only.json").read_text())
        self.assertFalse(report["boundaries"])
        self.assertFalse(report["connections"])
        self.assertEqual([item["id"] for item in report["runtime_probes"]], ["fixture-import"])
        self.assertEqual([item["id"] for item in report["lanes"]], ["python-isolated-runtime"])


if __name__ == "__main__":
    unittest.main()
