"""Go import graph and report-only CLI tests; Go source is never compiled."""

from __future__ import annotations

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
SPEC = importlib.util.spec_from_file_location("go_observer", ROOT / "tools/quality/inspect_go.py")
observer = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(observer)


class GoAnalysisTests(unittest.TestCase):
    @staticmethod
    def profile(profile_id="linux-cgo", cgo=True, entrypoints=None):
        return {
            "id": profile_id,
            "goos": "linux",
            "goarch": "amd64",
            "cgo": cgo,
            "tags": ["unix"],
            "go_version": "1.26",
            "entrypoints": entrypoints or [{"path": "cmd/main.go"}],
        }

    def nodes(self, sources):
        return [observer.parse_go_source(path, source) for path, source in sources.items()]

    def test_parser_reads_import_aliases_header_constraints_and_directives(self):
        node = observer.parse_go_source(
            "internal/a/a.go",
            '//go:build cgo && linux\n// +build cgo,linux\npackage a\nimport (\n_ `ragflow/internal/register`\nalias "example.com/lib"\n"C"\n)\n//go:embed templates/*.xml\nvar _ = alias.Value\n',
        )
        self.assertEqual(node["package"], "a")
        self.assertEqual(node["build_expression"], "cgo && linux")
        self.assertEqual([(item["alias"], item["path"]) for item in node["imports"]], [("_", "ragflow/internal/register"), ("alias", "example.com/lib"), (None, "C")])
        self.assertEqual(node["directives"], [{"kind": "embed", "arguments": "templates/*.xml", "line": 9}])
        self.assertEqual(node["issues"], [])

    def test_build_expression_profiles_keep_cgo_variants_separate(self):
        cgo = observer.parse_go_source("internal/a/cgo.go", "//go:build cgo\npackage a\n")
        nocgo = observer.parse_go_source("internal/a/nocgo.go", "//go:build !cgo\npackage a\n")
        self.assertEqual(observer._file_active(cgo, self.profile(cgo=True)), (True, None))
        self.assertEqual(observer._file_active(nocgo, self.profile(cgo=True)), (False, None))
        self.assertEqual(observer._file_active(cgo, self.profile(cgo=False)), (False, None))
        self.assertEqual(observer._file_active(nocgo, self.profile(cgo=False)), (True, None))

    def test_legacy_build_constraints_apply_negation_and_boolean_groups(self):
        not_windows = observer.parse_go_source(
            "internal/a/guard.go",
            "// +build !windows\n\npackage a\n",
        )
        linux_cgo_or_darwin = observer.parse_go_source(
            "internal/a/platform.go",
            "// +build linux,cgo darwin\n\npackage a\n",
        )
        linux_and_cgo = observer.parse_go_source(
            "internal/a/two_lines.go",
            "// +build linux\n// +build cgo\n\npackage a\n",
        )
        windows = {**self.profile(), "goos": "windows"}
        darwin = {**self.profile(cgo=False), "goos": "darwin"}

        self.assertEqual(observer._file_active(not_windows, self.profile()), (True, None))
        self.assertEqual(observer._file_active(not_windows, windows), (False, None))
        self.assertEqual(observer._file_active(linux_cgo_or_darwin, self.profile()), (True, None))
        self.assertEqual(observer._file_active(linux_cgo_or_darwin, self.profile(cgo=False)), (False, None))
        self.assertEqual(observer._file_active(linux_cgo_or_darwin, darwin), (True, None))
        self.assertEqual(observer._file_active(linux_and_cgo, self.profile()), (True, None))
        self.assertEqual(observer._file_active(linux_and_cgo, self.profile(cgo=False)), (False, None))

    def test_invalid_legacy_build_constraint_is_incomplete(self):
        invalid = observer.parse_go_source(
            "internal/a/invalid.go",
            "// +build linux,,cgo\n\npackage a\n",
        )

        active, error = observer._file_active(invalid, self.profile())

        self.assertFalse(active)
        self.assertEqual(error, "Invalid legacy build option: linux,,cgo")

    def test_analysis_reports_packages_reachability_cycles_and_reverse_consumers(self):
        sources = {
            "cmd/main.go": 'package main\nimport "ragflow/internal/a"\n',
            "internal/a/a.go": 'package a\nimport "ragflow/internal/b"\n',
            "internal/b/b.go": 'package b\nimport "ragflow/internal/a"\n',
            "internal/unused/unused.go": "package unused\n",
            "internal/a/a_test.go": 'package a_test\nimport "ragflow/internal/a"\n',
        }
        ownership = {path: {"origin": "extension", "owner": "fixture"} for path in sources}
        report = observer.analyze(self.nodes(sources), {"profiles": [self.profile()]}, ownership, "ragflow")
        self.assertEqual(report["analysis_status"], "OBSERVED")
        self.assertEqual(report["policy_status"], "NOT_EVALUATED")
        self.assertEqual(report["dead_code_status"], "NOT_ANALYZED")
        profile = report["profiles"][0]
        self.assertEqual(profile["reachable_packages"], ["cmd", "internal/a", "internal/b"])
        self.assertEqual(profile["cycles"][0]["members"], ["internal/a", "internal/b"])
        self.assertIn("internal/unused", report["unreachable_owned_packages_in_primary_profile"])
        reverse = next(item for item in report["reverse_consumers"] if item["package"] == "internal/a")
        self.assertEqual(reverse["consumer_packages"], ["cmd", "internal/b"])

    def test_inactive_variant_does_not_create_false_cycle(self):
        sources = {
            "cmd/main.go": 'package main\nimport "ragflow/internal/a"\n',
            "internal/a/a.go": "package a\n",
            "internal/a/a_cgo.go": '//go:build cgo\npackage a\nimport "ragflow/internal/b"\n',
            "internal/b/b_nocgo.go": '//go:build !cgo\npackage b\nimport "ragflow/internal/a"\n',
            "internal/b/b_cgo.go": "//go:build cgo\npackage b\n",
        }
        report = observer.analyze(self.nodes(sources), {"profiles": [self.profile()]}, {}, "ragflow")
        self.assertEqual(report["analysis_status"], "OBSERVED")
        self.assertEqual(report["profiles"][0]["cycles"], [])

    def test_invalid_build_expression_and_missing_local_target_are_incomplete(self):
        sources = {
            "cmd/main.go": 'package main\nimport "ragflow/internal/missing"\n',
            "internal/a/a.go": "//go:build cgo &&\npackage a\n",
        }
        report = observer.analyze(self.nodes(sources), {"profiles": [self.profile()]}, {}, "ragflow")
        self.assertEqual(report["analysis_status"], "INCOMPLETE")
        self.assertEqual({item["kind"] for item in report["incomplete_reasons"]}, {"unresolved_local_import", "invalid_build_expression"})


class CommandTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Go fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        self.write("internal/upstream/upstream.go", "package upstream\n")
        self.git("add", "internal/upstream/upstream.go")
        self.git("commit", "-qm", "upstream")
        self.base = {"commit": self.git("rev-parse", "HEAD"), "tree": self.git("rev-parse", "HEAD^{tree}")}
        self.write(".gitignore", "output/\n")
        self.write("go.mod", "module ragflow\n\ngo 1.26.4\n")
        self.write("cmd/main.go", 'package main\nimport "ragflow/internal/feature"\n')
        self.write("internal/feature/feature.go", "package feature\n")
        self.write("tools/quality/upstream-base.json", json.dumps(self.base))
        self.policy = {
            "schema_version": 1,
            "module_file": "go.mod",
            "source_root": ".",
            "profiles": [GoAnalysisTests.profile(entrypoints=[{"path": "cmd/main.go"}])],
        }
        self.write("tools/quality/go-analysis.yaml", yaml.safe_dump(self.policy))
        mapped = [
            ".gitignore",
            "go.mod",
            "cmd/main.go",
            "internal/feature/feature.go",
            "tools/quality/upstream-base.json",
            "tools/quality/go-analysis.yaml",
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

    def run_main(self, *extra):
        return observer.main(["--root", str(self.root), "--output", str(self.root / "output/report.json"), *extra])

    def test_cli_records_provenance_and_preserves_source_tree(self):
        before = self.git("status", "--porcelain")
        self.assertEqual(self.run_main(), 0)
        report = json.loads((self.root / "output/report.json").read_text(encoding="utf-8"))
        self.assertEqual(report["input"]["upstream_base"], self.base["commit"])
        self.assertEqual(report["analysis_status"], "OBSERVED")
        self.assertEqual(report["scope"]["indexed_files"], 3)
        self.assertEqual(self.git("status", "--porcelain"), before)

    def test_unknown_profile_and_source_output_fail_closed(self):
        self.assertEqual(self.run_main("--profile", "missing"), 2)
        code = observer.main(["--root", str(self.root), "--output", str(self.root / "cmd/main.go")])
        self.assertEqual(code, 2)
        self.assertEqual((self.root / "cmd/main.go").read_text(encoding="utf-8"), 'package main\nimport "ragflow/internal/feature"\n')

    def test_concurrent_source_change_refuses_report(self):
        original = observer.analyze

        def mutate(*args, **kwargs):
            result = original(*args, **kwargs)
            self.write("cmd/main.go", "package main\n")
            return result

        with patch.object(observer, "analyze", side_effect=mutate):
            self.assertEqual(self.run_main(), 2)
        self.assertFalse((self.root / "output/report.json").exists())

    def test_incomplete_graph_writes_report_with_exit_two(self):
        self.write("cmd/main.go", 'package main\nimport "ragflow/internal/missing"\n')
        self.assertEqual(self.run_main(), 2)
        report = json.loads((self.root / "output/report.json").read_text(encoding="utf-8"))
        self.assertEqual(report["analysis_status"], "INCOMPLETE")
        self.assertEqual(report["exit_code"], 2)


if __name__ == "__main__":
    unittest.main()
