"""Runtime-graph classification tests; application code is never executed."""

from __future__ import annotations

import copy
import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import yaml

ROOT = Path(__file__).resolve().parents[4]
sys.path.insert(0, str(ROOT / "tools/quality"))
SPEC = importlib.util.spec_from_file_location("runtime_graph_policy", ROOT / "tools/quality/check_runtime_graph_policy.py")
checker = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(checker)


class ClassificationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Runtime graph fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        self.write("web/src/cycle-a.ts", "import { b } from './cycle-b';\nexport const a = b;\n")
        self.write("web/src/cycle-b.ts", "import { a } from './cycle-a';\nexport const b = a;\n")
        self.write(
            "web/src/hooks/common-hooks.tsx",
            "export async function load(name: string) { return import(name); }\n",
        )
        self.git("add", ".")
        self.git("commit", "-qm", "upstream")
        self.base = self.git("rev-parse", "HEAD")
        self.write("web/src/main.ts", "import type { Shape } from './types';\nvoid 0;\n")
        self.write("web/src/types.ts", "export interface Shape { value: string }\n")
        self.write("web/src/candidate.ts", "export const candidate = true;\n")
        self.write("web/src/entry.test.ts", "void 0;\n")
        self.write("web/src/contracts.d.ts", "export interface Contract {}\n")
        self.write("web/index.html", '<script type="module" src="/src/main.ts"></script>\n')
        self.write(
            "web/.storybook/main.ts",
            "export default { stories: ['../src/**/*.stories.tsx'] };\n",
        )
        self.write("web/package.json", '{"scripts":{"test":"jest"}}\n')
        self.write(
            "web/src/pdf.tsx",
            'export const workerSrc = "/pdfjs-dist/pdf.worker.min.js";\n',
        )
        self.write("web/public/pdfjs-dist/pdf.worker.min.js", "// worker\n")
        self.write("cmd/server.go", "package main\nfunc main() {}\n")
        self.write("cmd/cli.go", "package main\nfunc main() {}\n")
        self.write("tools/dev/main.go", "package main\nfunc main() {}\n")
        self.write("build.sh", "go build cmd/server.go\ngo build cmd/cli.go\n")
        self.write("internal/embed.go", "package internal\n//go:embed assets/a.txt\n")
        self.write("internal/assets/a.txt", "asset\n")

    def write(self, name: str, content: str):
        target = self.root / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")

    def git(self, *args: str) -> str:
        return subprocess.check_output(["git", *args], cwd=self.root).decode().strip()

    def policy(self) -> dict:
        return {
            "schema_version": 1,
            "typescript": {
                "profile_id": "frontend-web",
                "entrypoints": [
                    {
                        "id": "browser",
                        "role": "production",
                        "path": "web/src/main.ts",
                        "evidence": [{"path": "web/index.html", "contains": "/src/main.ts"}],
                    }
                ],
                "auxiliary_roots": [
                    {
                        "id": "storybook",
                        "role": "development",
                        "evidence": [
                            {
                                "path": "web/.storybook/main.ts",
                                "contains": "../src/**/*.stories.tsx",
                            }
                        ],
                    },
                    {
                        "id": "jest",
                        "role": "test",
                        "evidence": [{"path": "web/package.json", "contains": '"test":"jest"'}],
                    },
                    {
                        "id": "pdf-worker",
                        "role": "external_runtime_asset",
                        "evidence": [
                            {
                                "path": "web/src/pdf.tsx",
                                "contains": "/pdfjs-dist/pdf.worker.min.js",
                                "target": "web/public/pdfjs-dist/pdf.worker.min.js",
                            }
                        ],
                    },
                ],
                "dynamic_registrations": [
                    {
                        "id": "icons",
                        "source": "web/src/main.ts",
                        "line": 2,
                        "kind": "import.meta.glob",
                        "computed": False,
                        "patterns": ["./icons/*.svg"],
                        "classification": "runtime_asset_registration",
                        "evidence": "fixture",
                    }
                ],
                "classified_incomplete_reasons": [
                    {
                        "id": "upstream-loader",
                        "source": "web/src/hooks/common-hooks.tsx",
                        "line": 1,
                        "kind": "computed_dynamic_import",
                        "detail": "name",
                        "type_only": False,
                        "classification": "inherited_upstream_dynamic_gap",
                        "require_unchanged_upstream": True,
                        "policy_effect": "manual_on_touch",
                    }
                ],
                "unreachable_owned_runtime": [
                    {
                        "path": "web/src/types.ts",
                        "classification": "type_only_contract",
                        "reason": "compile-time only",
                        "next_action": "keep while consumed",
                    },
                    {
                        "path": "web/src/candidate.ts",
                        "classification": "static_candidate",
                        "reason": "no consumer",
                        "next_action": "verify separately",
                    },
                ],
            },
            "go": {
                "profiles": [
                    {
                        "id": "linux-cgo",
                        "entrypoints": ["cmd/server.go", "cmd/cli.go"],
                    }
                ],
                "roots": [
                    {
                        "id": "server",
                        "role": "production",
                        "evidence": [{"path": "build.sh", "contains": "go build cmd/server.go"}],
                    },
                    {
                        "id": "cli",
                        "role": "production",
                        "evidence": [{"path": "build.sh", "contains": "go build cmd/cli.go"}],
                    },
                ],
                "auxiliary_entrypoints": [
                    {
                        "path": "tools/dev/main.go",
                        "role": "development_tool",
                        "reason": "fixture",
                    }
                ],
                "directives": [
                    {
                        "id": "asset",
                        "source": "internal/embed.go",
                        "kind": "embed",
                        "arguments": ["assets/a.txt"],
                        "classification": "runtime_embedded_asset",
                    }
                ],
            },
            "future_policy_scopes": [
                {
                    "id": "fixture",
                    "mode": "report_only",
                    "rules": ["ARC-03"],
                    "selector": "fixture changes",
                }
            ],
        }

    def typescript_report(self) -> dict:
        cycle_edges = [
            {
                "source": "web/src/cycle-a.ts",
                "target": "web/src/cycle-b.ts",
                "specifier": "./cycle-b",
                "kind": "import",
                "phase": "module",
                "type_only": False,
            },
            {
                "source": "web/src/cycle-b.ts",
                "target": "web/src/cycle-a.ts",
                "specifier": "./cycle-a",
                "kind": "import",
                "phase": "module",
                "type_only": False,
            },
        ]
        nodes = [
            {
                "path": "web/src/main.ts",
                "origin": "extension",
                "owner": "fixture",
                "is_test": False,
                "is_declaration": False,
            },
            {
                "path": "web/src/types.ts",
                "origin": "extension",
                "owner": "fixture",
                "is_test": False,
                "is_declaration": False,
            },
            {
                "path": "web/src/candidate.ts",
                "origin": "extension",
                "owner": "fixture",
                "is_test": False,
                "is_declaration": False,
            },
            {
                "path": "web/src/entry.test.ts",
                "origin": "extension",
                "owner": "fixture",
                "is_test": True,
                "is_declaration": False,
            },
            {
                "path": "web/src/contracts.d.ts",
                "origin": "extension",
                "owner": "fixture",
                "is_test": False,
                "is_declaration": True,
            },
            {
                "path": "web/src/hooks/common-hooks.tsx",
                "origin": "upstream",
                "owner": "upstream",
                "is_test": False,
                "is_declaration": False,
            },
        ]
        return {
            "analysis_status": "INCOMPLETE",
            "profile": {"id": "frontend-web"},
            "input": {
                "upstream_base": self.base,
                "tool_sha256": "1" * 64,
                "policy_sha256": "2" * 64,
                "worker_sha256": "3" * 64,
            },
            "scope": {"entrypoints": ["web/src/main.ts"]},
            "nodes": nodes,
            "reverse_imports": [
                {
                    "path": "web/src/main.ts",
                    "consumer_paths": [],
                    "type_only_consumer_paths": [],
                    "incoming_edges": [],
                },
                {
                    "path": "web/src/types.ts",
                    "consumer_paths": [],
                    "type_only_consumer_paths": ["web/src/main.ts"],
                    "incoming_edges": [{}],
                },
                {
                    "path": "web/src/candidate.ts",
                    "consumer_paths": [],
                    "type_only_consumer_paths": [],
                    "incoming_edges": [],
                },
                {
                    "path": "web/src/entry.test.ts",
                    "consumer_paths": [],
                    "type_only_consumer_paths": [],
                    "incoming_edges": [],
                },
                {
                    "path": "web/src/contracts.d.ts",
                    "consumer_paths": [],
                    "type_only_consumer_paths": [],
                    "incoming_edges": [],
                },
            ],
            "unreachable_owned_runtime_paths": [
                "web/src/candidate.ts",
                "web/src/types.ts",
            ],
            "cycles": [
                {
                    "members": ["web/src/cycle-a.ts", "web/src/cycle-b.ts"],
                    "owned_members": [],
                    "edges": cycle_edges,
                }
            ],
            "dynamic_registrations": [
                {
                    "source": "web/src/main.ts",
                    "line": 2,
                    "kind": "import.meta.glob",
                    "computed": False,
                    "patterns": ["./icons/*.svg"],
                }
            ],
            "runtime_incomplete_reasons": [
                {
                    "source": "web/src/hooks/common-hooks.tsx",
                    "line": 1,
                    "kind": "computed_dynamic_import",
                    "detail": "name",
                    "type_only": False,
                }
            ],
            "other_incomplete_reasons": [],
        }

    def go_report(self) -> dict:
        return {
            "analysis_status": "OBSERVED",
            "input": {
                "upstream_base": self.base,
                "tool_sha256": "4" * 64,
                "policy_sha256": "5" * 64,
            },
            "files": [
                {"path": "cmd/server.go", "package": "main", "is_test": False},
                {"path": "cmd/cli.go", "package": "main", "is_test": False},
                {"path": "tools/dev/main.go", "package": "main", "is_test": False},
            ],
            "profiles": [
                {
                    "id": "linux-cgo",
                    "entrypoints": [
                        {"path": "cmd/server.go"},
                        {"path": "cmd/cli.go"},
                    ],
                    "cycles": [],
                }
            ],
            "unreachable_owned_packages_in_primary_profile": [],
            "directives": [
                {
                    "source": "internal/embed.go",
                    "line": 2,
                    "kind": "embed",
                    "arguments": "assets/a.txt",
                }
            ],
            "incomplete_reasons": [],
        }

    def classify(self, policy=None, typescript=None, go_report=None):
        parser = ROOT / "web/node_modules/typescript/lib/typescript.js"
        with patch.object(checker, "_typescript_parser_path", return_value=parser):
            return checker.classify(
                self.root,
                policy or self.policy(),
                typescript or self.typescript_report(),
                go_report or self.go_report(),
            )

    def test_complete_classification_preserves_source_incomplete_status(self):
        result = self.classify()
        self.assertEqual(result["classification_status"], "COMPLETE")
        self.assertEqual(result["policy_status"], "REPORT_ONLY")
        self.assertEqual(result["architecture_status"], "NOT_EVALUATED")
        self.assertEqual(result["dead_code_status"], "REVIEW_REQUIRED")
        self.assertEqual(result["typescript"]["source_analysis_status"], "INCOMPLETE")
        self.assertEqual(
            result["typescript"]["cycles"][0]["classification"],
            "upstream_only_topology",
        )
        self.assertEqual(result["manual_review_required"]["automatic_deletions"], 0)

    def test_observer_source_hashes_are_flat_and_exact(self):
        source_hashes = checker._observer_source_hashes(self.typescript_report(), self.go_report())
        self.assertEqual(
            source_hashes,
            {
                "typescript_tool_sha256": "1" * 64,
                "typescript_policy_sha256": "2" * 64,
                "typescript_worker_sha256": "3" * 64,
                "go_tool_sha256": "4" * 64,
                "go_policy_sha256": "5" * 64,
            },
        )
        report_input = checker._runtime_report_input(
            {"head": "head", "upstream_base": "base", "snapshot_sha256": "snapshot"},
            {"policy": b"policy", "typescript_report": b"typescript", "go_report": b"go"},
            source_hashes,
        )
        self.assertEqual(report_input["policy_sha256"], checker._sha256(b"policy"))
        self.assertEqual(report_input["tool_sha256"], checker._sha256(Path(checker.__file__).read_bytes()))
        for name, value in source_hashes.items():
            self.assertEqual(report_input[name], value)

    def test_missing_observer_source_hash_fails_closed(self):
        report = self.typescript_report()
        del report["input"]["worker_sha256"]

        with self.assertRaisesRegex(ValueError, "TypeScript report input worker_sha256 is not a valid SHA-256"):
            checker._observer_source_hashes(report, self.go_report())

    def test_invalid_observer_source_hash_fails_closed(self):
        report = self.go_report()
        report["input"]["policy_sha256"] = "not-a-sha256"

        with self.assertRaisesRegex(ValueError, "Go report input policy_sha256 is not a valid SHA-256"):
            checker._observer_source_hashes(self.typescript_report(), report)

    def test_cycle_with_edge_absent_upstream_is_unclassified(self):
        report = self.typescript_report()
        report["cycles"][0]["edges"][0]["specifier"] = "./local-cycle-b"
        result = self.classify(typescript=report)
        self.assertEqual(result["classification_status"], "INCOMPLETE")
        self.assertIn(
            "unclassified_local_cycle_candidate",
            {finding["kind"] for finding in result["findings"]},
        )

    def test_cycle_with_upstream_type_only_edge_is_unclassified(self):
        self.write(
            "web/src/cycle-a.ts",
            "import type { B } from './cycle-b';\nexport type A = B;\n",
        )
        self.git("add", "web/src/cycle-a.ts")
        self.git("commit", "-qm", "type-only upstream edge")
        self.base = self.git("rev-parse", "HEAD")
        self.write("web/src/cycle-a.ts", "import { b } from './cycle-b';\nexport const a = b;\n")

        result = self.classify()

        self.assertEqual(result["classification_status"], "INCOMPLETE")
        finding = next(item for item in result["findings"] if item["kind"] == "unclassified_local_cycle_candidate")
        self.assertTrue(any("type_only=False" in problem for problem in finding["problems"]))

    def test_cycle_with_upstream_call_phase_edge_is_unclassified(self):
        self.write(
            "web/src/cycle-a.ts",
            "export function load() { return require('./cycle-b'); }\n",
        )
        self.git("add", "web/src/cycle-a.ts")
        self.git("commit", "-qm", "call-phase upstream edge")
        self.base = self.git("rev-parse", "HEAD")
        self.write("web/src/cycle-a.ts", "const b = require('./cycle-b');\nexport const a = b;\n")
        report = self.typescript_report()
        report["cycles"][0]["edges"][0]["kind"] = "require"

        result = self.classify(typescript=report)

        self.assertEqual(result["classification_status"], "INCOMPLETE")
        finding = next(item for item in result["findings"] if item["kind"] == "unclassified_local_cycle_candidate")
        self.assertTrue(any("phase='module'" in problem for problem in finding["problems"]))

    def test_upstream_edge_scanner_preserves_semantic_fields(self):
        edges, problems = checker._scan_typescript_edge_semantics(
            ROOT,
            {
                "fixture.ts": (
                    "const marker = '≠';\n"
                    "import type { Shape } from './types';\n"
                    "export { value } from './runtime';\n"
                    "const eager = require('./eager');\n"
                    "export function load() { return import('./lazy'); }\n"
                )
            },
        )

        self.assertEqual(problems, {})
        self.assertIn(
            {"specifier": "./types", "kind": "import", "phase": "module", "type_only": True},
            edges["fixture.ts"],
        )
        self.assertIn(
            {"specifier": "./runtime", "kind": "re_export", "phase": "module", "type_only": False},
            edges["fixture.ts"],
        )
        self.assertIn(
            {"specifier": "./eager", "kind": "require", "phase": "module", "type_only": False},
            edges["fixture.ts"],
        )
        self.assertIn(
            {"specifier": "./lazy", "kind": "dynamic_import", "phase": "call", "type_only": False},
            edges["fixture.ts"],
        )

    def test_upstream_edge_scanner_sanitizes_node_environment(self):
        completed = SimpleNamespace(
            returncode=0,
            stderr="",
            stdout=json.dumps({"fixture.ts": {"edges": [], "parse_errors": []}}),
        )
        poisoned = {
            "PATH": "preserved",
            "NODE_OPTIONS": "--require=attacker.js",
            "GITHUB_ENV": "command-file",
            "ARCHITECTURE_EVIDENCE_DIR": "evidence",
            "API_TOKEN": "secret",
        }
        with (
            patch.dict(os.environ, poisoned, clear=True),
            patch.object(checker.shutil, "which", return_value="node"),
            patch.object(checker, "_typescript_parser_path", return_value=Path(__file__)),
            patch.object(checker.subprocess, "run", return_value=completed) as run,
        ):
            edges, problems = checker._scan_typescript_edge_semantics(
                ROOT,
                {"fixture.ts": "export const value = 1;\n"},
            )
        self.assertEqual(edges, {"fixture.ts": []})
        self.assertEqual(problems, {})
        self.assertEqual(run.call_args.kwargs["env"], {"PATH": "preserved"})

    def test_missing_upstream_edge_parser_fails_closed(self):
        with patch.object(checker.shutil, "which", return_value=None):
            result = self.classify()

        self.assertEqual(result["classification_status"], "INCOMPLETE")
        finding = next(item for item in result["findings"] if item["kind"] == "unclassified_local_cycle_candidate")
        self.assertTrue(any("Node.js is required" in problem for problem in finding["problems"]))

    def test_missing_and_stale_unreachable_entries_are_findings(self):
        policy = self.policy()
        policy["typescript"]["unreachable_owned_runtime"].pop()
        policy["typescript"]["unreachable_owned_runtime"].append(
            {
                "path": "web/src/stale.ts",
                "classification": "static_candidate",
                "reason": "stale",
                "next_action": "remove after review",
            }
        )
        result = self.classify(policy=policy)
        kinds = {finding["kind"] for finding in result["findings"]}
        self.assertIn("unclassified_unreachable_owned_runtime", kinds)
        self.assertIn("stale_unreachable_classification", kinds)

    def test_type_only_contract_requires_a_type_consumer(self):
        report = self.typescript_report()
        type_reverse = next(item for item in report["reverse_imports"] if item["path"] == "web/src/types.ts")
        type_reverse["type_only_consumer_paths"] = []
        result = self.classify(typescript=report)
        self.assertIn(
            "invalid_type_only_classification",
            {finding["kind"] for finding in result["findings"]},
        )

    def test_unclassified_dynamic_signal_and_gap_are_findings(self):
        policy = self.policy()
        policy["typescript"]["dynamic_registrations"] = []
        policy["typescript"]["classified_incomplete_reasons"] = []
        result = self.classify(policy=policy)
        kinds = {finding["kind"] for finding in result["findings"]}
        self.assertIn("unclassified_dynamic_registration", kinds)
        self.assertIn("unclassified_incomplete_reason", kinds)

    def test_classified_upstream_gap_fails_when_source_becomes_owned(self):
        report = self.typescript_report()
        node = next(item for item in report["nodes"] if item["path"] == "web/src/hooks/common-hooks.tsx")
        node["origin"] = "core_change"
        result = self.classify(typescript=report)
        self.assertIn(
            "classified_gap_is_no_longer_upstream",
            {finding["kind"] for finding in result["findings"]},
        )

    def test_unexpected_go_main_and_missing_embed_asset_are_findings(self):
        report = self.go_report()
        report["files"].append({"path": "cmd/unknown.go", "package": "main", "is_test": False})
        (self.root / "internal/assets/a.txt").unlink()
        result = self.classify(go_report=report)
        kinds = {finding["kind"] for finding in result["findings"]}
        self.assertIn("go_main_entrypoint_inventory_mismatch", kinds)
        self.assertIn("missing_go_embed_asset", kinds)

    def test_root_evidence_change_is_not_silently_accepted(self):
        self.write("web/index.html", "<html></html>\n")
        result = self.classify()
        self.assertIn("root_evidence_mismatch", {finding["kind"] for finding in result["findings"]})


class PolicyValidationTests(unittest.TestCase):
    def test_policy_requires_report_only_scopes(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "policy.yaml"
            path.write_text(
                yaml.safe_dump(
                    {
                        "schema_version": 1,
                        "typescript": {},
                        "go": {},
                        "future_policy_scopes": [
                            {
                                "id": "bad",
                                "mode": "enforced",
                                "rules": ["ARC-03"],
                                "selector": "all",
                            }
                        ],
                    }
                ),
                encoding="utf-8",
            )
            with self.assertRaisesRegex(ValueError, "Invalid future policy scope"):
                checker._load_policy(path)

            policy = yaml.safe_load((ROOT / "tools/quality/runtime-graph-policy.yaml").read_text(encoding="utf-8"))
            policy["typescript"]["unreachable_owned_runtime"][0].pop("next_action")
            path.write_text(yaml.safe_dump(policy), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "Invalid TypeScript unreachable classification"):
                checker._load_policy(path)

    def test_cycle_signature_is_order_independent(self):
        cycle = {
            "members": ["b.ts", "a.ts"],
            "edges": [
                {
                    "source": "b.ts",
                    "target": "a.ts",
                    "specifier": "./a",
                    "kind": "import",
                    "phase": "module",
                    "type_only": False,
                },
                {
                    "source": "a.ts",
                    "target": "b.ts",
                    "specifier": "./b",
                    "kind": "import",
                    "phase": "module",
                    "type_only": False,
                },
            ],
        }
        reordered = copy.deepcopy(cycle)
        reordered["members"].reverse()
        reordered["edges"].reverse()
        self.assertEqual(checker._cycle_signature(cycle), checker._cycle_signature(reordered))


if __name__ == "__main__":
    unittest.main()
