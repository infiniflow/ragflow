"""TypeScript graph and report-only CLI tests; frontend code is never executed."""

from __future__ import annotations

import importlib.util
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import yaml

ROOT = Path(__file__).resolve().parents[4]
TYPESCRIPT = ROOT / "web/node_modules/typescript/lib/typescript.js"
WORKER = ROOT / "tools/quality/inspect_typescript.cjs"
sys.path.insert(0, str(ROOT / "tools/quality"))
SPEC = importlib.util.spec_from_file_location("typescript_observer", ROOT / "tools/quality/inspect_typescript.py")
observer = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(observer)


@unittest.skipUnless(shutil.which("node") and TYPESCRIPT.is_file(), "locked Node/TypeScript parser is required")
class WorkerAnalysisTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.write(
            "tsconfig.json",
            json.dumps(
                {
                    "compilerOptions": {
                        "module": "ESNext",
                        "moduleResolution": "Bundler",
                        "baseUrl": ".",
                        "paths": {"@/*": ["src/*"]},
                    }
                }
            ),
        )

    def write(self, name, content):
        target = self.root / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")

    def run_worker(self):
        files = sorted(path.relative_to(self.root).as_posix() for path in self.root.glob("src/**/*") if path.suffix in {".ts", ".tsx"})
        payload = {
            "root": str(self.root),
            "files": files,
            "aliases": {"@": "src"},
            "test_markers": [".test.", "/__tests__/"],
            "tsconfig": "tsconfig.json",
            "typescript_module": str(TYPESCRIPT),
        }
        result = subprocess.run(
            [shutil.which("node"), str(WORKER)],
            input=json.dumps(payload),
            capture_output=True,
            text=True,
            timeout=20,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        return json.loads(result.stdout), files

    def fixture(self):
        self.write(
            "src/main.ts",
            "import feature, { value as renamed } from '@/feature';\n"
            "import type { Shape } from './types';\n"
            "export { publicValue } from './exported';\n"
            "void import('./lazy');\n"
            "import './style.css';\n"
            "const icons = import.meta.glob('./icons/*.svg');\n"
            "void feature; void renamed; void icons;\n",
        )
        self.write("src/feature.ts", "import { cycle } from './cycle';\nexport default function feature() {}\nexport const value = cycle;\n")
        self.write("src/cycle.ts", "import { value } from './feature';\nexport const cycle = value;\n")
        self.write("src/types.ts", "export interface Shape { value: string }\n")
        self.write("src/exported.ts", "export const publicValue = 1;\n")
        self.write("src/lazy.ts", "export default 1;\n")
        self.write("src/unreachable.ts", "export const unusedSignal = 1;\n")
        self.write("src/style.css", "body {}\n")
        self.write("src/icons/check.svg", "<svg/>\n")

    def test_worker_resolves_runtime_type_dynamic_reexport_and_asset_edges(self):
        self.fixture()
        result, _files = self.run_worker()
        main_edges = [edge for edge in result["edges"] if edge["source"] == "src/main.ts"]
        feature = next(edge for edge in main_edges if edge["target"] == "src/feature.ts")
        types = next(edge for edge in main_edges if edge["target"] == "src/types.ts")
        lazy = next(edge for edge in main_edges if edge["target"] == "src/lazy.ts")
        style = next(edge for edge in main_edges if edge["target"] == "src/style.css")
        self.assertEqual(feature["symbols"], ["default", "value"])
        self.assertFalse(feature["type_only"])
        self.assertTrue(types["type_only"])
        self.assertEqual(lazy["kind"], "dynamic_import")
        self.assertEqual(lazy["phase"], "module")
        self.assertEqual(style["resolution"], "asset")
        self.assertTrue(any(edge["kind"] == "re_export" and edge["target"] == "src/exported.ts" for edge in main_edges))
        self.assertEqual(result["dynamic_registrations"][0]["patterns"], ["./icons/*.svg"])
        feature_node = next(node for node in result["nodes"] if node["path"] == "src/feature.ts")
        self.assertEqual(feature_node["exports"], ["default", "value"])

    def test_analysis_reports_reachability_cycles_and_reverse_consumers_without_dead_code_claim(self):
        self.fixture()
        worker_result, files = self.run_worker()
        ownership = {path: {"origin": "extension", "owner": "fixture"} for path in files}
        report = observer.analyze(worker_result, ["src/main.ts"], ownership)
        self.assertEqual(report["analysis_status"], "OBSERVED")
        self.assertEqual(report["policy_status"], "NOT_EVALUATED")
        self.assertEqual(report["dead_code_status"], "NOT_ANALYZED")
        self.assertEqual(report["cycles"][0]["members"], ["src/cycle.ts", "src/feature.ts"])
        self.assertIn("src/unreachable.ts", report["unreachable_owned_runtime_paths"])
        reverse = next(item for item in report["reverse_imports"] if item["path"] == "src/feature.ts")
        self.assertEqual(reverse["consumer_paths"], ["src/cycle.ts", "src/main.ts"])
        type_reverse = next(item for item in report["reverse_imports"] if item["path"] == "src/types.ts")
        self.assertEqual(type_reverse["type_only_consumer_paths"], ["src/main.ts"])

    def test_computed_import_and_missing_local_target_are_explicitly_incomplete(self):
        self.write("src/main.ts", "const target = './lazy';\nvoid import(target);\nimport missing from './missing';\nvoid missing;\n")
        result, files = self.run_worker()
        ownership = {path: {"origin": "extension", "owner": "fixture"} for path in files}
        report = observer.analyze(result, ["src/main.ts"], ownership)
        self.assertEqual(report["analysis_status"], "INCOMPLETE")
        self.assertEqual({issue["kind"] for issue in report["runtime_incomplete_reasons"]}, {"computed_dynamic_import", "unresolved_local"})

    def test_code_target_outside_declared_profile_is_not_silently_external(self):
        self.write("src/main.ts", "import { shared } from '../shared';\nvoid shared;\n")
        self.write("shared.ts", "export const shared = 1;\n")
        result, _files = self.run_worker()
        self.assertEqual(result["issues"][0]["kind"], "local_outside_profile")


class CommandTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "TypeScript fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        self.write("web/src/upstream.ts", "export const upstream = 1;\n")
        self.git("add", "web/src/upstream.ts")
        self.git("commit", "-qm", "upstream")
        self.base = {"commit": self.git("rev-parse", "HEAD"), "tree": self.git("rev-parse", "HEAD^{tree}")}
        self.write(".gitignore", "output/\n")
        self.write("web/src/main.ts", "import { feature } from './feature';\nvoid feature;\n")
        self.write("web/src/feature.ts", "export const feature = 1;\n")
        self.write("web/tsconfig.json", "{}\n")
        self.write("tools/quality/inspect_typescript.cjs", "// fixture\n")
        self.write("tools/quality/typescript-placeholder.js", "// fixture\n")
        self.write("tools/quality/upstream-base.json", json.dumps(self.base))
        self.policy = {
            "schema_version": 1,
            "profiles": [
                {
                    "id": "frontend-web",
                    "source_root": "web/src",
                    "entrypoints": ["web/src/main.ts"],
                    "extensions": [".ts", ".tsx"],
                    "aliases": {"@": "web/src"},
                    "tsconfig": "web/tsconfig.json",
                    "typescript_module": "tools/quality/typescript-placeholder.js",
                    "test_markers": [".test."],
                }
            ],
        }
        self.write("tools/quality/typescript-analysis.yaml", yaml.safe_dump(self.policy))
        mapped = [
            ".gitignore",
            "web/src/main.ts",
            "web/src/feature.ts",
            "web/tsconfig.json",
            "tools/quality/inspect_typescript.cjs",
            "tools/quality/typescript-placeholder.js",
            "tools/quality/typescript-analysis.yaml",
            "tools/quality/upstream-base.json",
            "tools/quality/module-map.yaml",
            "tools/quality/file-inventory.json",
            "tools/quality/core-changes.yaml",
        ]
        self.write("tools/quality/module-map.yaml", yaml.safe_dump({"modules": [{"id": "fixture", "paths": mapped}]}))

    def write(self, name, content):
        target = self.root / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root).decode().strip()

    @staticmethod
    def worker_result(files, issues=None):
        edges = []
        if {"web/src/main.ts", "web/src/feature.ts"} <= set(files):
            edges.append(
                {
                    "source": "web/src/main.ts",
                    "target": "web/src/feature.ts",
                    "specifier": "./feature",
                    "line": 1,
                    "kind": "import",
                    "phase": "module",
                    "conditional": False,
                    "type_only": False,
                    "resolution": "local",
                    "symbols": ["feature"],
                }
            )
        return {
            "compiler": {"version": "fixture", "configuration_errors": []},
            "nodes": [{"path": path, "is_test": False, "is_declaration": False, "exports": []} for path in files],
            "edges": edges,
            "issues": issues or [],
            "dynamic_registrations": [],
        }

    def run_main(self, *extra, worker=None):
        args = ["--root", str(self.root), "--output", str(self.root / "output/report.json"), *extra]
        implementation = worker or (lambda _root, _profile, files: self.worker_result(files))
        with patch.object(observer, "run_worker", side_effect=implementation):
            return observer.main(args)

    def test_cli_records_provenance_and_preserves_source_tree(self):
        before = self.git("status", "--porcelain")
        self.assertEqual(self.run_main(), 0)
        report = json.loads((self.root / "output/report.json").read_text(encoding="utf-8"))
        self.assertEqual(report["input"]["upstream_base"], self.base["commit"])
        self.assertEqual(report["analysis_status"], "OBSERVED")
        self.assertEqual(report["scope"]["indexed_files"], 3)
        self.assertEqual(self.git("status", "--porcelain"), before)

    def test_cli_writes_incomplete_report_for_unresolved_graph(self):
        def incomplete(_root, _profile, files):
            return self.worker_result(
                files,
                [{"source": "web/src/main.ts", "line": 1, "kind": "computed_dynamic_import", "detail": "target", "type_only": False}],
            )

        self.assertEqual(self.run_main(worker=incomplete), 2)
        report = json.loads((self.root / "output/report.json").read_text(encoding="utf-8"))
        self.assertEqual(report["analysis_status"], "INCOMPLETE")

    def test_unknown_profile_and_source_output_fail_closed(self):
        self.assertEqual(self.run_main("--profile", "missing"), 2)
        with patch.object(observer, "run_worker", side_effect=lambda _root, _profile, files: self.worker_result(files)):
            code = observer.main(["--root", str(self.root), "--output", str(self.root / "web/src/main.ts")])
        self.assertEqual(code, 2)
        self.assertEqual((self.root / "web/src/main.ts").read_text(encoding="utf-8"), "import { feature } from './feature';\nvoid feature;\n")

    def test_concurrent_source_change_refuses_report(self):
        def mutate(_root, _profile, files):
            result = self.worker_result(files)
            self.write("web/src/main.ts", "export const changed = true;\n")
            return result

        self.assertEqual(self.run_main(worker=mutate), 2)
        self.assertFalse((self.root / "output/report.json").exists())

    def test_missing_node_is_incomplete(self):
        profile = self.policy["profiles"][0]
        with patch.object(observer.shutil, "which", return_value=None), self.assertRaisesRegex(ValueError, "Node.js"):
            observer.run_worker(self.root, profile, ["web/src/main.ts"])

    def test_worker_sanitizes_node_environment(self):
        files = ["web/src/main.ts"]
        completed = SimpleNamespace(returncode=0, stderr="", stdout=json.dumps(self.worker_result(files)))
        poisoned = {
            "PATH": "preserved",
            "NODE_OPTIONS": "--require=attacker.js",
            "GITHUB_ENV": "command-file",
            "ARCHITECTURE_EVIDENCE_DIR": "evidence",
            "API_TOKEN": "secret",
        }
        with (
            patch.dict(os.environ, poisoned, clear=True),
            patch.object(observer.shutil, "which", return_value="node"),
            patch.object(observer.subprocess, "run", return_value=completed) as run,
        ):
            observer.run_worker(self.root, self.policy["profiles"][0], files)
        self.assertEqual(run.call_args.kwargs["env"], {"PATH": "preserved"})


if __name__ == "__main__":
    unittest.main()
