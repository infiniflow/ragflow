"""Failure-oriented tests for the T2 Go build profile planner."""

import copy
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[4]
with patch.object(sys, "path", [str(ROOT / "tools/quality"), *sys.path]):
    SPEC = importlib.util.spec_from_file_location("go_build_profiles", ROOT / "tools/quality/check_go_build_profiles.py")
    go_profiles = importlib.util.module_from_spec(SPEC)
    SPEC.loader.exec_module(go_profiles)


def fixture_policy() -> dict:
    return {
        "schema_version": 1,
        "profiles": [
            {
                "id": "fixture",
                "rule_id": "BUILD-01",
                "platform": "linux",
                "driver": "build.sh",
                "required_tools": ["bash", "go"],
                "c_compilers": ["clang", "gcc"],
                "native_dependencies": ["office_oxide", "pdfium-static", "pdf_oxide"],
                "timeout_seconds": 30,
                "test_args": ["-json", "-timeout", "1m"],
                "targets": [
                    {
                        "id": "owner-package",
                        "owners": ["owner"],
                        "path_prefixes": ["internal/owner/"],
                        "packages": ["./internal/owner"],
                    }
                ],
            }
        ],
    }


def snapshot(path: str = "internal/owner/file.go", owner: str = "owner") -> dict:
    return {
        "head": "a" * 40,
        "upstream_base": "b" * 40,
        "snapshot_sha256": "c" * 64,
        "records": [{"path": path, "module": owner, "origin": "core_change", "working_file": {"present": True, "sha256": "d" * 64}}],
    }


class GoBuildProfileTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        (self.root / "internal/owner").mkdir(parents=True)
        (self.root / "build.sh").write_text("#!/usr/bin/env bash\n", encoding="utf-8")

    def plan(self, policy=None, evidence=None):
        return go_profiles.evaluate_plan(self.root, go_profiles.normalize_policy(policy or fixture_policy()), evidence or snapshot())

    def test_valid_profile_maps_owner_and_uses_only_build_sh_test(self):
        plan = self.plan()
        self.assertEqual(plan["analysis_status"], "READY")
        self.assertEqual(plan["policy_status"], "NOT_EVALUATED")
        self.assertEqual(
            plan["commands"], [{"kind": "test", "targets": ["owner-package"], "argv": ["bash", "build.sh", "--test", "-json", "-timeout", "1m", "./internal/owner"], "packages": ["./internal/owner"]}]
        )
        self.assertEqual(plan["go_files"][0]["target"], "owner-package")

    def test_entrypoint_target_uses_only_repository_go_build_action(self):
        policy = fixture_policy()
        policy["profiles"][0]["targets"][0] = {
            "id": "entrypoint",
            "owners": ["owner"],
            "path_prefixes": ["cmd/"],
            "build_args": ["--go"],
            "artifacts": ["bin/ragflow_server", "bin/ragflow-cli"],
        }
        (self.root / "cmd").mkdir()
        plan = self.plan(policy, snapshot("cmd/ragflow_server.go"))
        self.assertEqual(
            plan["commands"],
            [{"kind": "build", "target": "entrypoint", "argv": ["bash", "build.sh", "--go"], "artifacts": ["bin/ragflow_server", "bin/ragflow-cli"]}],
        )

    def test_entrypoint_target_cannot_bypass_go_action_or_artifact_evidence(self):
        for build_args, artifacts, message in ((["--all"], ["bin/server"], "--go"), (["--go"], [], "declare artifacts")):
            with self.subTest(build_args=build_args, artifacts=artifacts):
                policy = fixture_policy()
                policy["profiles"][0]["targets"][0].pop("packages")
                policy["profiles"][0]["targets"][0].update(build_args=build_args, artifacts=artifacts)
                with self.assertRaisesRegex(ValueError, message):
                    go_profiles.normalize_policy(policy)

    def test_unmapped_go_file_is_a_policy_failure(self):
        plan = self.plan(evidence=snapshot("internal/new/file.go"))
        self.assertEqual(plan["analysis_status"], "FAIL")
        self.assertEqual(plan["findings"][0]["reason"], "changed Go file has no build target")

    def test_target_must_declare_provenance_owner(self):
        plan = self.plan(evidence=snapshot(owner="different-owner"))
        self.assertEqual(plan["analysis_status"], "FAIL")
        self.assertIn("does not declare", plan["findings"][0]["reason"])

    def test_missing_package_directory_is_incomplete(self):
        (self.root / "internal/owner").rmdir()
        plan = self.plan()
        self.assertEqual(plan["analysis_status"], "INCOMPLETE")
        self.assertEqual(plan["findings"], [])

    def test_overlapping_prefixes_are_rejected(self):
        policy = fixture_policy()
        policy["profiles"][0]["targets"].append({"id": "nested", "owners": ["owner"], "path_prefixes": ["internal/owner/nested/"], "packages": ["./internal/owner/nested"]})
        with self.assertRaisesRegex(ValueError, "Overlapping Go prefixes"):
            go_profiles.normalize_policy(policy)

    def test_driver_packages_and_native_prerequisites_cannot_be_bypassed(self):
        for key, value, message in (
            ("driver", "go", "build.sh"),
            ("test_args", ["-json", "./..."], "may not contain packages"),
            ("test_args", ["-json", "-run=TestOnlyOne"], "may not narrow"),
            ("required_tools", ["bash"], "bash and go"),
            ("c_compilers", ["clang"], "clang and gcc"),
            ("native_dependencies", ["office_oxide"], "native dependencies"),
        ):
            with self.subTest(key=key):
                policy = fixture_policy()
                policy["profiles"][0][key] = value
                with self.assertRaisesRegex(ValueError, message):
                    go_profiles.normalize_policy(policy)

    def test_go_json_distinguishes_failure_missing_evidence_and_pass(self):
        passed = "\n".join(("build preamble", json.dumps({"Action": "pass", "Package": "ragflow/internal/owner"})))
        failed = json.dumps({"Action": "fail", "Package": "ragflow/internal/owner"})
        skipped = "\n".join(
            (
                json.dumps({"Action": "skip", "Package": "ragflow/internal/owner", "Test": "TestNeedsService"}),
                json.dumps({"Action": "pass", "Package": "ragflow/internal/owner"}),
            )
        )
        self.assertEqual(go_profiles.parse_go_test_json(passed, 0, ["./internal/owner"], "ragflow")[0], "PASS")
        self.assertEqual(go_profiles.parse_go_test_json(failed, 1)[0], "FAIL")
        self.assertEqual(go_profiles.parse_go_test_json("native library missing", 1)[0], "INCOMPLETE")
        self.assertEqual(go_profiles.parse_go_test_json("", 0)[0], "INCOMPLETE")
        self.assertEqual(go_profiles.parse_go_test_json(skipped, 0)[0], "PASS")
        required = [{"package": "./internal/owner", "test": "TestNeedsService", "path": "internal/owner/file_test.go"}]
        self.assertEqual(go_profiles.parse_go_test_json(skipped, 0, ["./internal/owner"], "ragflow", required)[0], "INCOMPLETE")
        self.assertEqual(go_profiles.parse_go_test_json(passed, 0, ["./internal/missing"], "ragflow")[0], "INCOMPLETE")

    def test_wrong_platform_and_missing_tools_are_incomplete_prerequisites(self):
        profile = go_profiles.normalize_policy(fixture_policy())["profiles"][0]
        with patch.object(go_profiles.shutil, "which", return_value=None):
            reasons = go_profiles.prerequisite_reasons(profile, current_platform="win32")
        self.assertIn("platform:linux", reasons)
        self.assertEqual(set(reasons[1:]), {"tool:bash", "tool:go", "tool:any-of:clang|gcc"})

    def test_changed_go_test_functions_are_required(self):
        path = self.root / "internal/owner/file_test.go"
        path.write_text('package owner\nimport "testing"\nfunc TestRequired(t *testing.T) {}\nfunc helper(t *testing.T) {}\n', encoding="utf-8")
        evidence = snapshot("internal/owner/file_test.go")
        plan = self.plan(evidence=evidence)
        self.assertEqual(plan["required_tests"], [{"path": "internal/owner/file_test.go", "package": "./internal/owner", "test": "TestRequired"}])

    def test_repository_policy_covers_every_current_go_delta(self):
        policy = go_profiles.load_policy(ROOT / "tools/quality/go-build-profiles.yaml")
        inventory = json.loads((ROOT / "tools/quality/file-inventory.json").read_text(encoding="utf-8"))
        plan = go_profiles.evaluate_plan(ROOT, policy, inventory)
        self.assertEqual(plan["analysis_status"], "READY", plan)
        self.assertEqual(len(plan["go_files"]), 16)
        self.assertEqual(len(plan["selected_targets"]), 6)
        self.assertEqual(plan["commands"][0]["argv"], ["bash", "build.sh", "--go"])
        self.assertEqual(plan["commands"][1]["argv"][:3], ["bash", "build.sh", "--test"])

    def test_unknown_profile_selector_is_rejected(self):
        policy = go_profiles.normalize_policy(copy.deepcopy(fixture_policy()))
        with self.assertRaisesRegex(ValueError, "Unknown Go build profile"):
            go_profiles.evaluate_plan(self.root, policy, snapshot(), {"missing"})


if __name__ == "__main__":
    unittest.main()
