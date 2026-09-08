"""T3 selector and fail-closed report-only aggregate contracts."""

from __future__ import annotations

import copy
import importlib.util
import json
from pathlib import Path
import sys
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[4]
with patch.object(sys, "path", [str(ROOT / "tools/quality"), *sys.path]):
    SPEC = importlib.util.spec_from_file_location(
        "architecture_policy_checker",
        ROOT / "tools/quality/check_architecture_policy.py",
    )
    checker = importlib.util.module_from_spec(SPEC)
    SPEC.loader.exec_module(checker)


POLICY = checker.load_policy(ROOT / "tools/quality/architecture-policy.json")
IDENTITY = {
    "base": "a" * 40,
    "head": "b" * 40,
    "upstream_base": "c" * 40,
    "snapshot_sha256": "d" * 64,
    "policy_sha256": "e" * 64,
    "candidate_tool_sha256": "f" * 64,
    "changed_paths_sha256": "0" * 64,
}


def record(path: str, module: str) -> dict:
    return {
        "path": path,
        "module": module,
        "origin": "extension",
        "working_file": {"present": True},
    }


def selected_plan(changed: list[str]) -> dict:
    records = [record(path, "quality-governance") for path in changed]
    lanes, changes = checker.select_lanes(POLICY, changed, records, {})
    return {
        "schema_version": 1,
        "mode": "report_only",
        "input": copy.deepcopy(IDENTITY),
        "lanes": lanes,
        "changes": changes,
        "manual_review_required": [],
    }


def report(tool: str, **values) -> bytes:
    payload = {
        "schema_version": 1,
        "tool": {"name": tool},
        "input": {
            "head": IDENTITY["head"],
            "upstream_base": IDENTITY["upstream_base"],
            "snapshot_sha256": IDENTITY["snapshot_sha256"],
        },
        **values,
    }
    return json.dumps(payload).encode()


def junit(*, failure: bool = False, error: bool = False, skipped: bool = False) -> bytes:
    elements = []
    for name in next(lane for lane in POLICY["lanes"] if lane["id"] == "policy-fixtures")["required_cases"]:
        child = "<failure/>" if failure else "<error/>" if error else "<skipped/>" if skipped else ""
        elements.append(f'<testcase name="{name}">{child}</testcase>')
    return ("<testsuite>" + "".join(elements) + "</testsuite>").encode()


class ArchitecturePolicyTests(unittest.TestCase):
    def test_selector_preserves_deleted_and_renamed_path_ownership(self):
        changed = ["api/new.py", "web/old.ts", "web/new.ts", "docs/guide.md"]
        current = [
            record("api/new.py", "business-documents"),
            record("web/new.ts", "frontend-foundation"),
            record("docs/guide.md", "quality-governance"),
        ]
        lanes, changes = checker.select_lanes(POLICY, changed, current, {"web/old.ts": "frontend-foundation"})
        by_path = {item["path"]: item for item in changes}
        self.assertEqual(by_path["web/old.ts"]["owner"], "frontend-foundation")
        self.assertEqual(by_path["web/old.ts"]["provenance_source"], "base")
        selected = {lane["id"] for lane in lanes if lane["selected"]}
        self.assertEqual(selected, {"provenance", "python-architecture", "runtime-graph"})

    def test_docs_only_change_keeps_code_lanes_not_applicable(self):
        lanes, _ = checker.select_lanes(POLICY, ["README.md"], [record("README.md", "quality-governance")], {})
        self.assertEqual([lane["id"] for lane in lanes if lane["selected"]], ["provenance"])

    def test_control_change_selects_every_lane(self):
        path = "tools/quality/module-map.yaml"
        lanes, _ = checker.select_lanes(POLICY, [path], [record(path, "quality-governance")], {})
        self.assertTrue(all(lane["selected"] for lane in lanes))

    def test_unknown_changed_path_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "no current or base provenance"):
            checker.select_lanes(POLICY, ["unknown/new.bin"], [], {})

    def test_policy_rejects_enforcement_wildcards_and_duplicate_lanes(self):
        temporary = self.enterContext(__import__("tempfile").TemporaryDirectory())
        path = Path(temporary) / "policy.json"
        for mutate in (
            lambda value: value.update(mode="enforced"),
            lambda value: value["lanes"][1]["selectors"]["prefixes"].append("api/**"),
            lambda value: value["lanes"].append(copy.deepcopy(value["lanes"][0])),
        ):
            with self.subTest(mutate=mutate):
                altered = copy.deepcopy(POLICY)
                mutate(altered)
                path.write_text(json.dumps(altered), encoding="utf-8")
                with self.assertRaises(ValueError):
                    checker.load_policy(path)

    def test_complete_report_only_evidence_is_not_an_enforced_pass(self):
        plan = selected_plan(["tools/quality/module-map.yaml"])
        evidence = {
            "python-architecture": report("check_architecture", policy_status="PASS", findings=[], exit_code=0),
            "runtime-graph": report(
                "check_runtime_graph_policy",
                classification_status="COMPLETE",
                policy_status="REPORT_ONLY",
                findings=[],
                exit_code=0,
            ),
            "go-build-plan": report(
                "check_go_build_profiles",
                analysis_status="READY",
                policy_status="NOT_EVALUATED",
                findings=[],
                exit_code=0,
            ),
            "policy-fixtures": junit(),
        }
        result = checker.aggregate(POLICY, plan, evidence, IDENTITY)
        self.assertEqual(result["aggregate_status"], "REPORT_ONLY_COMPLETE")
        self.assertEqual(result["enforcement_status"], "NOT_ENABLED")
        self.assertEqual(result["exit_code"], 0)
        statuses = {item["id"]: item["status"] for item in result["results"]}
        self.assertEqual(statuses["provenance"], "OBSERVED")
        self.assertEqual(statuses["runtime-graph"], "CLASSIFIED")
        self.assertEqual(statuses["go-build-plan"], "PLANNED")

    def test_aggregate_rejects_missing_stale_and_forged_evidence(self):
        plan = selected_plan(["tools/quality/module-map.yaml"])
        valid = {
            "python-architecture": report("check_architecture", policy_status="PASS", findings=[], exit_code=0),
            "runtime-graph": report(
                "check_runtime_graph_policy",
                classification_status="COMPLETE",
                policy_status="REPORT_ONLY",
                findings=[],
                exit_code=0,
            ),
            "go-build-plan": report(
                "check_go_build_profiles",
                analysis_status="READY",
                policy_status="NOT_EVALUATED",
                findings=[],
                exit_code=0,
            ),
            "policy-fixtures": junit(),
        }
        missing = checker.aggregate(POLICY, plan, {key: value for key, value in valid.items() if key != "runtime-graph"}, IDENTITY)
        self.assertEqual(missing["aggregate_status"], "INCOMPLETE")
        self.assertEqual(missing["exit_code"], 2)

        stale = json.loads(valid["python-architecture"])
        stale["input"]["snapshot_sha256"] = "1" * 64
        stale_result = checker.aggregate(POLICY, plan, {**valid, "python-architecture": json.dumps(stale).encode()}, IDENTITY)
        self.assertEqual(next(item for item in stale_result["results"] if item["id"] == "python-architecture")["status"], "INCOMPLETE")

        forged = json.loads(valid["runtime-graph"])
        forged["tool"]["name"] = "candidate_always_passes"
        forged_result = checker.aggregate(POLICY, plan, {**valid, "runtime-graph": json.dumps(forged).encode()}, IDENTITY)
        self.assertEqual(next(item for item in forged_result["results"] if item["id"] == "runtime-graph")["status"], "INCOMPLETE")

    def test_incomplete_takes_precedence_over_failure_and_junit_skip(self):
        plan = selected_plan(["tools/quality/module-map.yaml"])
        evidence = {
            "python-architecture": report("check_architecture", policy_status="INCOMPLETE", findings=[{"rule": "ARC-01"}], exit_code=2),
            "runtime-graph": report(
                "check_runtime_graph_policy",
                classification_status="COMPLETE",
                policy_status="REPORT_ONLY",
                findings=[],
                exit_code=0,
            ),
            "go-build-plan": report(
                "check_go_build_profiles",
                analysis_status="INCOMPLETE",
                policy_status="NOT_EVALUATED",
                findings=[{"rule": "BUILD-01"}],
                exit_code=2,
            ),
            "policy-fixtures": junit(skipped=True),
        }
        result = checker.aggregate(POLICY, plan, evidence, IDENTITY)
        self.assertEqual(result["aggregate_status"], "INCOMPLETE")
        self.assertEqual(result["exit_code"], 2)
        statuses = {item["id"]: item["status"] for item in result["results"]}
        self.assertEqual(statuses["python-architecture"], "INCOMPLETE")
        self.assertEqual(statuses["go-build-plan"], "INCOMPLETE")

    def test_recomputed_selection_rejects_lane_bypass(self):
        plan = selected_plan(["tools/quality/module-map.yaml"])
        narrowed = copy.deepcopy(plan)
        runtime = next(lane for lane in narrowed["lanes"] if lane["id"] == "runtime-graph")
        runtime.update(selected=False, status="NOT_APPLICABLE", matched_paths=[])
        with self.assertRaisesRegex(ValueError, "fresh selector run"):
            checker.validate_replanned(narrowed, plan)

    def test_junit_summary_counts_cannot_hide_failure_or_incomplete_result(self):
        lane = next(item for item in POLICY["lanes"] if item["id"] == "policy-fixtures")
        cases = b'<testcase name="test_selector_preserves_deleted_and_renamed_path_ownership"/><testcase name="test_aggregate_rejects_missing_stale_and_forged_evidence"/><testcase name="test_recomputed_selection_rejects_lane_bypass"/><testcase name="test_workflow_is_unconditional_report_only_and_fail_closed"/>'
        self.assertEqual(checker._junit_contract(lane, b'<testsuite failures="1">' + cases + b"</testsuite>")[0], "FAIL")
        self.assertEqual(checker._junit_contract(lane, b'<testsuite errors="1">' + cases + b"</testsuite>")[0], "INCOMPLETE")
        self.assertEqual(checker._junit_contract(lane, b'<testsuite skipped="bad">' + cases + b"</testsuite>")[0], "INCOMPLETE")

    def test_plan_identity_change_is_rejected(self):
        plan = selected_plan(["README.md"])
        current = {**IDENTITY, "snapshot_sha256": "1" * 64}
        with self.assertRaisesRegex(ValueError, "plan is stale"):
            checker.aggregate(POLICY, plan, {}, current)


if __name__ == "__main__":
    unittest.main()
