"""T3 selector and fail-closed report-only aggregate contracts."""

from __future__ import annotations

import base64
import copy
import hashlib
import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(os.environ.get("ARCHITECTURE_CANDIDATE_ROOT", Path(__file__).resolve().parents[4])).resolve()
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
    "candidate_policy_sha256": "1" * 64,
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


def source_hash(lane_id: str, field: str) -> str:
    return hashlib.sha256(f"{lane_id}:{field}".encode()).hexdigest()


def selected_plan(changed: list[str], policy: dict = POLICY, *, bootstrap: bool = False) -> dict:
    records = [record(path, "quality-governance") for path in changed]
    lanes, changes = checker.select_lanes(policy, changed, records, {})
    policy_lanes = {lane["id"]: lane for lane in policy["lanes"]}
    source = "candidate-bootstrap" if bootstrap else "base"
    for planned in lanes:
        if not planned["selected"] or planned["contract"] == "inventory":
            planned["source_integrity"] = "NOT_APPLICABLE"
            continue
        lane = policy_lanes[planned["id"]]
        planned.update(
            source=source,
            source_bundle=[],
            source_integrity="PASS",
            expected_report_hashes={field: source_hash(planned["id"], field) for field in lane["reported_hashes"]},
        )
        if planned["contract"] == "policy_fixtures":
            planned["fixture_sources"] = [{"path": path, "sha256": source_hash(planned["id"], path)} for path in lane["fixture_paths"]]
    plan = {
        "schema_version": 1,
        "mode": "report_only",
        "input": copy.deepcopy(IDENTITY),
        "trusted_base": {"bootstrap_required": bootstrap},
        "lanes": lanes,
        "changes": changes,
        "manual_review_required": [],
    }
    selection_sha256 = checker._sha256(checker._canonical(plan))
    plan["selection_sha256"] = selection_sha256
    plan["input"]["selection_sha256"] = selection_sha256
    return plan


def report(plan: dict, lane_id: str, tool: str, **values) -> bytes:
    planned = next(lane for lane in plan["lanes"] if lane["id"] == lane_id)
    payload = {
        "schema_version": 1,
        "tool": {"name": tool},
        "input": {
            "head": IDENTITY["head"],
            "upstream_base": IDENTITY["upstream_base"],
            "snapshot_sha256": IDENTITY["snapshot_sha256"],
            **planned.get("expected_report_hashes", {}),
            "selection_sha256": plan["selection_sha256"],
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


def fixture_attestation(plan: dict, *, status: str = "PASS", pytest_exit_code: int = 0, exit_code: int = 0) -> bytes:
    lane = next(lane for lane in POLICY["lanes"] if lane["id"] == "policy-fixtures")
    planned = next(lane for lane in plan["lanes"] if lane["id"] == "policy-fixtures")
    required = lane["required_cases"]
    content = junit()
    return report(
        plan,
        "policy-fixtures",
        "check_architecture_policy",
        fixture_status=status,
        required_cases=required,
        required_nodeids=lane["required_nodeids"],
        executed_nodeids=lane["required_nodeids"],
        fixture_source=planned["source"],
        fixture_sources=planned["fixture_sources"],
        case_names=sorted(required),
        case_count=len(required),
        pytest_exit_code=pytest_exit_code,
        junit_sha256=hashlib.sha256(content).hexdigest(),
        junit_base64=base64.b64encode(content).decode("ascii"),
        reason="fixture result",
        exit_code=exit_code,
    )


def complete_evidence(plan: dict) -> dict[str, bytes]:
    return {
        "python-architecture": report(plan, "python-architecture", "check_architecture", policy_status="PASS", findings=[], exit_code=0),
        "runtime-graph": report(
            plan,
            "runtime-graph",
            "check_runtime_graph_policy",
            classification_status="COMPLETE",
            policy_status="REPORT_ONLY",
            findings=[],
            exit_code=0,
        ),
        "go-build-plan": report(
            plan,
            "go-build-plan",
            "check_go_build_profiles",
            analysis_status="READY",
            policy_status="NOT_EVALUATED",
            findings=[],
            exit_code=0,
        ),
        "policy-fixtures": fixture_attestation(plan),
    }


def candidate_policy_plan(plan: dict) -> dict:
    candidate = copy.deepcopy(plan)
    candidate.pop("selection_sha256", None)
    candidate["input"].pop("selection_sha256", None)
    candidate["input"]["policy_sha256"] = plan["input"]["candidate_policy_sha256"]
    selection_sha256 = checker._sha256(checker._canonical(candidate))
    candidate["selection_sha256"] = selection_sha256
    candidate["input"]["selection_sha256"] = selection_sha256
    return candidate


def compatibility_report(plan: dict, candidate_plan: dict) -> dict:
    plan_input = plan["input"]
    bootstrap = plan["trusted_base"]["bootstrap_required"]
    return {
        "schema_version": 1,
        "tool": {"name": "check_architecture_policy"},
        "mode": "report_only",
        "authority_source": "candidate-bootstrap" if bootstrap else "base",
        "compatibility_status": "BOOTSTRAP_SELF_CHECK" if bootstrap else "COMPATIBLE",
        "reason": ("candidate-bootstrap self-consistency only; trusted base coverage was not evaluated" if bootstrap else "candidate policy preserves trusted base coverage"),
        "enforcement_status": "NOT_ENABLED",
        "input": {
            "base": plan_input.get("base"),
            "head": plan_input.get("head"),
            "upstream_base": plan_input.get("upstream_base"),
            "snapshot_sha256": plan_input.get("snapshot_sha256"),
            "base_policy_sha256": plan_input.get("policy_sha256"),
            "candidate_policy_sha256": plan_input.get("candidate_policy_sha256"),
            "base_selection_sha256": plan.get("selection_sha256"),
            "candidate_selection_sha256": candidate_plan.get("selection_sha256"),
        },
        "checked_lanes": [lane["id"] for lane in POLICY["lanes"]],
        "exit_code": 0,
    }


def aggregate(plan: dict, evidence: dict[str, bytes], current: dict = IDENTITY, compatibility: dict | None = None) -> dict:
    candidate_plan = candidate_policy_plan(plan)
    return checker.aggregate(
        POLICY,
        plan,
        evidence,
        current,
        compatibility or compatibility_report(plan, candidate_plan),
        candidate_plan,
        POLICY,
    )


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

    def test_producer_and_control_paths_select_their_own_lanes(self):
        cases = {
            "services/asr-online-service/architecture-contract-requirements.in": "python-architecture",
            "services/asr-online-service/architecture-contract-requirements.txt": "python-architecture",
            "tools/quality/check_architecture.py": "python-architecture",
            "tools/quality/check_runtime_graph_policy.py": "runtime-graph",
            "tools/quality/inspect_typescript.py": "runtime-graph",
            "tools/quality/inspect_go.py": "runtime-graph",
            "tools/quality/check_go_build_profiles.py": "go-build-plan",
        }
        for path, expected_lane in cases.items():
            with self.subTest(path=path):
                lanes, _ = checker.select_lanes(POLICY, [path], [record(path, "quality-governance")], {})
                selected = {lane["id"] for lane in lanes if lane["selected"]}
                self.assertIn(expected_lane, selected)

        shared = "tools/quality/capture_inventory.py"
        lanes, _ = checker.select_lanes(POLICY, [shared], [record(shared, "quality-governance")], {})
        selected = {lane["id"] for lane in lanes if lane["selected"]}
        self.assertTrue({"python-architecture", "runtime-graph", "go-build-plan", "policy-fixtures"} <= selected)

        for path in (".github/workflows/architecture.yml", "tools/quality/check_architecture_policy.py"):
            with self.subTest(control=path):
                lanes, _ = checker.select_lanes(POLICY, [path], [record(path, "quality-governance")], {})
                self.assertTrue(all(lane["selected"] for lane in lanes))

        node_install_inputs = {"web/.npmrc", "web/package.json", "web/pnpm-lock.yaml"}
        for path in node_install_inputs:
            with self.subTest(node_install_input=path):
                lanes, _ = checker.select_lanes(POLICY, [path], [record(path, "frontend-foundation")], {})
                selected = {lane["id"] for lane in lanes if lane["selected"]}
                self.assertLessEqual({"python-architecture", "runtime-graph"}, selected)
        for lane_id in ("python-architecture", "runtime-graph"):
            lane = next(item for item in POLICY["lanes"] if item["id"] == lane_id)
            self.assertLessEqual(node_install_inputs, set(lane["protected_sources"]))

        report_lanes = [lane for lane in POLICY["lanes"] if lane["contract"] != "inventory"]
        self.assertTrue(all("tools/quality/run_isolated_python.py" in lane["protected_sources"] for lane in report_lanes))
        for lane in report_lanes:
            for path in lane["protected_sources"]:
                with self.subTest(lane=lane["id"], protected_source=path):
                    lanes, _ = checker.select_lanes(POLICY, [path], [record(path, "quality-governance")], {})
                    selected = {planned["id"] for planned in lanes if planned["selected"]}
                    self.assertIn(lane["id"], selected)
        python_lane = next(lane for lane in POLICY["lanes"] if lane["id"] == "python-architecture")
        runtime_test_closure = {
            "services/asr-online-service/tests/conftest.py",
            "test/testcases/restful_api/test_connector_routes_unit.py",
            "test/testcases/restful_api/test_file_commit_routes_unit.py",
            "test/testcases/restful_api/test_user_tenant_routes_unit.py",
            "test/testcases/test_web_api/test_dataset_management/test_dataset_sdk_routes_unit.py",
            "test/testcases/test_web_api/test_search_app/test_search_routes_unit.py",
            "test/testcases/test_web_api/test_system_app/test_system_routes_unit.py",
            "test/unit_test/api/apps/business_documents/helpers.py",
            "test/unit_test/api/apps/restful_apis/test_agentbots_access_control.py",
            "test/unit_test/rag/conftest.py",
        }
        self.assertLessEqual(runtime_test_closure, set(python_lane["protected_sources"]))

    def test_unknown_changed_path_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "no current or base provenance"):
            checker.select_lanes(POLICY, ["unknown/new.bin"], [], {})

    def test_policy_rejects_enforcement_wildcards_and_duplicate_lanes(self):
        temporary = self.enterContext(__import__("tempfile").TemporaryDirectory())
        path = Path(temporary) / "policy.json"
        for mutate in (
            lambda value: value.update(mode="enforced"),
            lambda value: value.update(tool_path="tools/quality/always_pass.py"),
            lambda value: value["lanes"][1]["selectors"]["prefixes"].append("api/**"),
            lambda value: value["lanes"].append(copy.deepcopy(value["lanes"][0])),
            lambda value: value["lanes"][2]["protected_sources"].remove("tools/quality/check_runtime_graph_policy.py"),
            lambda value: value["lanes"][2]["selectors"]["paths"].remove("tools/quality/inspect_typescript.cjs"),
            lambda value: next(lane for lane in value["lanes"] if lane["contract"] == "policy_fixtures")["protected_sources"].remove("tools/quality/check_architecture_policy.py"),
            lambda value: next(lane for lane in value["lanes"] if lane["contract"] == "policy_fixtures")["selectors"]["paths"].remove(".github/workflows/architecture.yml"),
            lambda value: next(lane for lane in value["lanes"] if lane["contract"] == "policy_fixtures").update(contract="junit"),
        ):
            with self.subTest(mutate=mutate):
                altered = copy.deepcopy(POLICY)
                mutate(altered)
                path.write_text(json.dumps(altered), encoding="utf-8")
                with self.assertRaises(ValueError):
                    checker.load_policy(path)

    def test_complete_report_only_evidence_is_not_an_enforced_pass(self):
        plan = selected_plan(["tools/quality/module-map.yaml"])
        evidence = complete_evidence(plan)
        result = aggregate(plan, evidence)
        self.assertEqual(result["aggregate_status"], "REPORT_ONLY_COMPLETE")
        self.assertEqual(result["enforcement_status"], "NOT_ENABLED")
        self.assertEqual(result["exit_code"], 0)
        statuses = {item["id"]: item["status"] for item in result["results"]}
        self.assertEqual(statuses["provenance"], "OBSERVED")
        self.assertEqual(statuses["runtime-graph"], "CLASSIFIED")
        self.assertEqual(statuses["go-build-plan"], "PLANNED")
        self.assertEqual(result["compatibility"]["status"], "PASS")

    def test_aggregate_requires_exact_policy_compatibility_evidence(self):
        plan = selected_plan(["tools/quality/module-map.yaml"])
        candidate_plan = candidate_policy_plan(plan)
        evidence = complete_evidence(plan)
        missing = checker.aggregate(POLICY, plan, evidence, IDENTITY, None, candidate_plan, POLICY)
        self.assertEqual(missing["aggregate_status"], "INCOMPLETE")

        valid = checker.aggregate(POLICY, plan, evidence, IDENTITY, compatibility_report(plan, candidate_plan), candidate_plan, POLICY)
        self.assertEqual(valid["compatibility"]["status"], "PASS")
        self.assertEqual(valid["compatibility"]["reason"], "candidate policy preserves trusted base coverage")

        mutations = (
            lambda payload: payload["input"].update(candidate_policy_sha256="7" * 64),
            lambda payload: payload["input"].update(candidate_selection_sha256="forged"),
            lambda payload: payload.update(checked_lanes=[]),
            lambda payload: payload.update(authority_source="candidate-bootstrap"),
            lambda payload: payload.update(compatibility_status="INCOMPLETE", exit_code=2),
        )
        for mutate in mutations:
            with self.subTest(mutate=mutate):
                forged = compatibility_report(plan, candidate_plan)
                mutate(forged)
                result = checker.aggregate(POLICY, plan, evidence, IDENTITY, forged, candidate_plan, POLICY)
                self.assertEqual(result["aggregate_status"], "INCOMPLETE")
                self.assertEqual(result["compatibility"]["status"], "INCOMPLETE")

        narrowed_policy = copy.deepcopy(POLICY)
        runtime_lane = next(lane for lane in narrowed_policy["lanes"] if lane["id"] == "runtime-graph")
        runtime_lane["selectors"]["paths"].remove("build.sh")
        narrowed_plan = candidate_policy_plan(selected_plan(["tools/quality/module-map.yaml"], narrowed_policy))
        forged = compatibility_report(plan, narrowed_plan)
        narrowed = checker.aggregate(POLICY, plan, evidence, IDENTITY, forged, narrowed_plan, narrowed_policy)
        self.assertEqual(narrowed["aggregate_status"], "INCOMPLETE")
        self.assertEqual(narrowed["compatibility"]["status"], "INCOMPLETE")

        bootstrap_plan = selected_plan(["tools/quality/module-map.yaml"], bootstrap=True)
        bootstrap_candidate_plan = candidate_policy_plan(bootstrap_plan)
        bootstrap_evidence = complete_evidence(bootstrap_plan)
        bootstrap_report = checker.compare_policy_coverage(POLICY, POLICY, bootstrap_plan, bootstrap_candidate_plan)
        self.assertEqual(bootstrap_report["authority_source"], "candidate-bootstrap")
        self.assertEqual(bootstrap_report["compatibility_status"], "BOOTSTRAP_SELF_CHECK")
        bootstrap = checker.aggregate(
            POLICY,
            bootstrap_plan,
            bootstrap_evidence,
            IDENTITY,
            bootstrap_report,
            bootstrap_candidate_plan,
            POLICY,
        )
        self.assertEqual(bootstrap["aggregate_status"], "REPORT_ONLY_COMPLETE")
        self.assertEqual(bootstrap["enforcement_status"], "NOT_ENABLED")
        self.assertEqual(bootstrap["compatibility"]["status"], "OBSERVED")
        self.assertIn("trusted base coverage was not evaluated", bootstrap["compatibility"]["reason"])

        bootstrap_mutations = (
            lambda payload: payload.update(authority_source="base"),
            lambda payload: payload.update(compatibility_status="COMPATIBLE"),
        )
        for mutate in bootstrap_mutations:
            with self.subTest(bootstrap_mutation=mutate):
                forged = copy.deepcopy(bootstrap_report)
                mutate(forged)
                result = checker.aggregate(
                    POLICY,
                    bootstrap_plan,
                    bootstrap_evidence,
                    IDENTITY,
                    forged,
                    bootstrap_candidate_plan,
                    POLICY,
                )
                self.assertEqual(result["aggregate_status"], "INCOMPLETE")
                self.assertEqual(result["compatibility"]["status"], "INCOMPLETE")

    def test_aggregate_rejects_missing_stale_and_forged_evidence(self):
        plan = selected_plan(["tools/quality/module-map.yaml"])
        valid = complete_evidence(plan)
        missing = aggregate(plan, {key: value for key, value in valid.items() if key != "runtime-graph"})
        self.assertEqual(missing["aggregate_status"], "INCOMPLETE")
        self.assertEqual(missing["exit_code"], 2)

        stale = json.loads(valid["python-architecture"])
        stale["input"]["snapshot_sha256"] = "1" * 64
        stale_result = aggregate(plan, {**valid, "python-architecture": json.dumps(stale).encode()})
        self.assertEqual(next(item for item in stale_result["results"] if item["id"] == "python-architecture")["status"], "INCOMPLETE")

        forged = json.loads(valid["runtime-graph"])
        forged["tool"]["name"] = "candidate_always_passes"
        forged_result = aggregate(plan, {**valid, "runtime-graph": json.dumps(forged).encode()})
        self.assertEqual(next(item for item in forged_result["results"] if item["id"] == "runtime-graph")["status"], "INCOMPLETE")

    def test_report_contract_rejects_wrong_producer_and_fixture_attestation(self):
        plan = selected_plan(["tools/quality/module-map.yaml"])
        evidence = complete_evidence(plan)

        wrong_producer = json.loads(evidence["python-architecture"])
        wrong_producer["input"]["tool_sha256"] = "9" * 64
        result = aggregate(
            plan,
            {**evidence, "python-architecture": json.dumps(wrong_producer).encode()},
        )
        self.assertEqual(next(item for item in result["results"] if item["id"] == "python-architecture")["status"], "INCOMPLETE")

        unbound_report = json.loads(evidence["runtime-graph"])
        unbound_report["input"].pop("selection_sha256")
        result = aggregate(
            plan,
            {**evidence, "runtime-graph": json.dumps(unbound_report).encode()},
        )
        self.assertEqual(next(item for item in result["results"] if item["id"] == "runtime-graph")["status"], "INCOMPLETE")

        fixture_mutations = (
            lambda payload: payload.update(case_count=-99),
            lambda payload: payload.update(junit_sha256="0" * 64),
            lambda payload: payload.update(required_nodeids=[]),
            lambda payload: payload.update(fixture_sources=[]),
            lambda payload: payload["input"].update(selection_sha256="8" * 64),
        )
        for mutate in fixture_mutations:
            with self.subTest(mutate=mutate):
                forged_fixture = json.loads(evidence["policy-fixtures"])
                mutate(forged_fixture)
                result = aggregate(
                    plan,
                    {**evidence, "policy-fixtures": json.dumps(forged_fixture).encode()},
                )
                self.assertEqual(next(item for item in result["results"] if item["id"] == "policy-fixtures")["status"], "INCOMPLETE")

    def test_incomplete_takes_precedence_over_failure_and_junit_skip(self):
        plan = selected_plan(["tools/quality/module-map.yaml"])
        evidence = {
            "python-architecture": report(
                plan,
                "python-architecture",
                "check_architecture",
                policy_status="INCOMPLETE",
                findings=[{"rule": "ARC-01"}],
                exit_code=2,
            ),
            "runtime-graph": report(
                plan,
                "runtime-graph",
                "check_runtime_graph_policy",
                classification_status="COMPLETE",
                policy_status="REPORT_ONLY",
                findings=[],
                exit_code=0,
            ),
            "go-build-plan": report(
                plan,
                "go-build-plan",
                "check_go_build_profiles",
                analysis_status="INCOMPLETE",
                policy_status="NOT_EVALUATED",
                findings=[{"rule": "BUILD-01"}],
                exit_code=2,
            ),
            "policy-fixtures": fixture_attestation(plan, status="INCOMPLETE", pytest_exit_code=1, exit_code=2),
        }
        result = aggregate(plan, evidence)
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

    def test_fixture_attestation_rejects_stale_or_incomplete_junit(self):
        plan = selected_plan(["tools/quality/module-map.yaml"])
        evidence = complete_evidence(plan)
        stale = json.loads(evidence["policy-fixtures"])
        stale["input"]["snapshot_sha256"] = "1" * 64
        stale_result = aggregate(plan, {**evidence, "policy-fixtures": json.dumps(stale).encode()})
        self.assertEqual(next(item for item in stale_result["results"] if item["id"] == "policy-fixtures")["status"], "INCOMPLETE")

        incomplete = fixture_attestation(plan, status="INCOMPLETE", pytest_exit_code=1, exit_code=2)
        incomplete_result = aggregate(plan, {**evidence, "policy-fixtures": incomplete})
        self.assertEqual(next(item for item in incomplete_result["results"] if item["id"] == "policy-fixtures")["status"], "INCOMPLETE")

        raw_junit_result = aggregate(plan, {**evidence, "policy-fixtures": junit()})
        self.assertEqual(next(item for item in raw_junit_result["results"] if item["id"] == "policy-fixtures")["status"], "INCOMPLETE")

    def test_policy_coverage_rejects_narrowing_and_accepts_expansion(self):
        changed = ["tools/quality/module-map.yaml"]
        base_plan = selected_plan(changed)
        expanded = copy.deepcopy(POLICY)
        expanded["lanes"][1]["selectors"]["paths"].append("tools/quality/new-boundary.yaml")
        expanded_plan = selected_plan(changed, expanded)
        result = checker.compare_policy_coverage(POLICY, expanded, base_plan, expanded_plan)
        self.assertEqual(result["authority_source"], "base")
        self.assertEqual(result["compatibility_status"], "COMPATIBLE")

        narrowed = copy.deepcopy(POLICY)
        runtime = next(lane for lane in narrowed["lanes"] if lane["id"] == "runtime-graph")
        runtime["selectors"]["paths"].remove("tools/quality/module-map.yaml")
        narrowed_plan = selected_plan(changed, narrowed)
        with self.assertRaisesRegex(ValueError, "narrows trusted coverage"):
            checker.compare_policy_coverage(POLICY, narrowed, base_plan, narrowed_plan)

    def test_external_evaluator_sources_must_match_base_or_candidate(self):
        self.assertEqual(checker._verified_content_source(b"base", b"candidate", b"base", "tool"), "base")
        self.assertEqual(checker._verified_content_source(b"candidate", b"candidate", b"base", "tool"), "candidate")
        with self.assertRaisesRegex(ValueError, "does not match"):
            checker._verified_content_source(b"forged", b"candidate", b"base", "tool")

    def test_fixture_interpreter_path_preserves_virtualenv_symlink(self):
        root = Path(self.enterContext(tempfile.TemporaryDirectory()))
        interpreter = root / "managed-python"
        interpreter.write_text("fixture", encoding="utf-8")
        launcher = root / ".venv/bin/python"
        launcher.parent.mkdir(parents=True)
        try:
            launcher.symlink_to(interpreter)
        except (NotImplementedError, OSError) as error:
            self.skipTest(f"symlinks unavailable: {error}")

        selected = checker._fixture_interpreter_path(root, Path(".venv/bin/python"))
        self.assertEqual(selected, launcher)
        self.assertTrue(selected.is_symlink())
        self.assertEqual(selected.resolve(), interpreter)

    def test_protected_source_bundle_uses_base_and_blocks_changed_candidate(self):
        self.assertEqual(
            checker.BASE_GOVERNED_CONTROL_SOURCES,
            {"tools/quality/architecture-policy.json"},
        )
        fixture_lane = next(lane for lane in POLICY["lanes"] if lane["contract"] == "policy_fixtures")
        self.assertLessEqual(checker.BASE_GOVERNED_CONTROL_SOURCES, set(fixture_lane["protected_sources"]))
        self.assertTrue(all(checker._matches(path, fixture_lane["selectors"]) for path in checker.BASE_GOVERNED_CONTROL_SOURCES))

        with patch.dict(
            os.environ,
            {
                "GITHUB_ENV": "command-file",
                "ARCHITECTURE_EVIDENCE_DIR": "evidence",
                "API_TOKEN": "secret",
                "NODE_OPTIONS": "--require=attacker.js",
                "NODE_PATH": "attacker-modules",
                "NODE_EXTRA_CA_CERTS": "attacker.pem",
                "SAFE_VALUE": "kept",
            },
            clear=True,
        ):
            environment = checker._isolated_subprocess_environment(ARCHITECTURE_CANDIDATE_ROOT="candidate")
        self.assertEqual(environment, {"SAFE_VALUE": "kept", "ARCHITECTURE_CANDIDATE_ROOT": "candidate"})

        root = Path(self.enterContext(__import__("tempfile").TemporaryDirectory()))
        subprocess.run(["git", "init", "-q"], cwd=root, check=True)
        subprocess.run(["git", "config", "user.name", "Architecture Policy Fixture"], cwd=root, check=True)
        subprocess.run(["git", "config", "user.email", "fixture@example.invalid"], cwd=root, check=True)
        subprocess.run(["git", "config", "core.autocrlf", "false"], cwd=root, check=True)
        producer = root / "producer.py"
        config = root / "config.yaml"
        controls = {
            ".github/workflows/architecture.yml": "name: base\n",
            "tools/quality/architecture-policy.json": '{"mode": "base"}\n',
            "tools/quality/check_architecture_policy.py": "VERSION = 'base'\n",
        }
        producer.write_text("print('base')\n", encoding="utf-8")
        config.write_text("mode: base\n", encoding="utf-8")
        for path, content in controls.items():
            target = root / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content, encoding="utf-8")
        subprocess.run(["git", "add", "."], cwd=root, check=True)
        subprocess.run(["git", "commit", "-qm", "trusted sources"], cwd=root, check=True)
        base = subprocess.run(["git", "rev-parse", "HEAD"], cwd=root, check=True, capture_output=True, text=True).stdout.strip()
        producer.write_text("print('candidate')\n", encoding="utf-8")
        for path in controls:
            (root / path).write_text("candidate\n", encoding="utf-8")

        lane = {
            "protected_sources": ["producer.py", "config.yaml"],
            "reported_hashes": {"tool_sha256": "producer.py", "policy_sha256": "config.yaml"},
        }
        bundle, reported, integrity = checker._lane_source_bundle(root, base, lane, "base")
        by_path = {item["path"]: item for item in bundle}
        self.assertEqual(integrity, "INCOMPLETE")
        self.assertFalse(by_path["producer.py"]["candidate_matches"])
        self.assertEqual(by_path["producer.py"]["candidate_change_policy"], "MUST_MATCH")
        self.assertTrue(by_path["config.yaml"]["candidate_matches"])
        self.assertEqual(reported["tool_sha256"], by_path["producer.py"]["sha256"])

        control_lane = {"protected_sources": list(checker.BASE_GOVERNED_CONTROL_SOURCES), "reported_hashes": {}}
        control_bundle, _control_hashes, control_integrity = checker._lane_source_bundle(root, base, control_lane, "base")
        self.assertEqual(control_integrity, "PASS")
        self.assertTrue(all(not item["candidate_matches"] for item in control_bundle))
        self.assertTrue(all(item["candidate_change_policy"] == "BASE_FIXTURE_REVIEW" for item in control_bundle))
        ordinary_controls = set(controls) - checker.BASE_GOVERNED_CONTROL_SOURCES
        ordinary_bundle, _ordinary_hashes, ordinary_integrity = checker._lane_source_bundle(
            root,
            base,
            {"protected_sources": sorted(ordinary_controls), "reported_hashes": {}},
            "base",
        )
        self.assertEqual(ordinary_integrity, "INCOMPLETE")
        self.assertTrue(all(item["candidate_change_policy"] == "MUST_MATCH" for item in ordinary_bundle))
        bootstrap_bundle, _bootstrap_hashes, bootstrap_integrity = checker._lane_source_bundle(root, base, control_lane, "candidate-bootstrap")
        self.assertEqual(bootstrap_integrity, "PASS")
        self.assertTrue(all(item["candidate_matches"] for item in bootstrap_bundle))
        self.assertTrue(all(item["candidate_change_policy"] == "MUST_MATCH" for item in bootstrap_bundle))

        plan = {
            "input": {"base": base},
            "lanes": [
                {
                    "id": "fixture",
                    "selected": True,
                    "contract": "runtime_graph",
                    "source": "base",
                    "source_integrity": "PASS",
                    "source_bundle": bundle,
                }
            ],
        }
        materialized = root / "materialized"
        manifest = checker.materialize_protected_sources(root, plan, materialized)
        self.assertEqual(manifest["source_count"], 2)
        self.assertEqual((materialized / "producer.py").read_text(encoding="utf-8"), "print('base')\n")
        self.assertEqual((materialized / "config.yaml").read_text(encoding="utf-8"), "mode: base\n")
        with self.assertRaisesRegex(ValueError, "stale protected source bundle"):
            checker.materialize_protected_sources(root, plan, materialized)

        control_plan = {
            "input": {"base": base},
            "lanes": [
                {
                    "id": "controls",
                    "selected": True,
                    "contract": "runtime_graph",
                    "source": "base",
                    "source_integrity": control_integrity,
                    "source_bundle": control_bundle,
                }
            ],
        }
        materialized_controls = root / "materialized-controls"
        checker.materialize_protected_sources(root, control_plan, materialized_controls)
        for path in checker.BASE_GOVERNED_CONTROL_SOURCES:
            content = controls[path]
            self.assertEqual((materialized_controls / path).read_text(encoding="utf-8"), content)

        isolated_quality = root / "isolated/tools/quality"
        isolated_quality.mkdir(parents=True)
        (isolated_quality / "run_isolated_python.py").write_bytes((ROOT / "tools/quality/run_isolated_python.py").read_bytes())
        (isolated_quality / "helper.py").write_text("VALUE = 'trusted'\n", encoding="utf-8")
        (isolated_quality / "tool.py").write_text("from helper import VALUE\nprint(VALUE)\n", encoding="utf-8")
        (root / "helper.py").write_text("raise RuntimeError('candidate import namespace used')\n", encoding="utf-8")
        completed = subprocess.run(
            [
                sys.executable,
                "-I",
                "-B",
                str(isolated_quality / "run_isolated_python.py"),
                str(root / "isolated"),
                "tools/quality/tool.py",
            ],
            cwd=root,
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(completed.returncode, 0, completed.stderr)
        self.assertEqual(completed.stdout.strip(), "trusted")

    def test_trusted_base_protocol_uses_committed_evaluator_and_policy(self):
        temporary = Path(self.enterContext(__import__("tempfile").TemporaryDirectory()))
        root = temporary / "repo"
        root.mkdir()

        def git(*args: str) -> str:
            return subprocess.run(["git", *args], cwd=root, check=True, capture_output=True, text=True).stdout.strip()

        git("init", "-q")
        git("config", "user.name", "Architecture Policy Fixture")
        git("config", "user.email", "fixture@example.invalid")
        git("config", "core.autocrlf", "false")
        (root / "upstream.txt").write_text("upstream\n", encoding="utf-8")
        git("add", "upstream.txt")
        git("commit", "-qm", "upstream")
        upstream = git("rev-parse", "HEAD")
        upstream_tree = git("rev-parse", "HEAD^{tree}")

        policy = copy.deepcopy(POLICY)
        protected_sources = sorted(
            {path for lane in policy["lanes"] for path in (set(lane.get("protected_sources", [])) | set(lane.get("reported_hashes", {}).values()) | set(lane.get("fixture_paths", [])))}
        )
        for path in protected_sources:
            target = root / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes((ROOT / path).read_bytes())

        quality = root / "tools/quality"
        (root / ".gitignore").write_text("output/\n", encoding="utf-8")
        (quality / "upstream-base.json").write_text(json.dumps({"commit": upstream, "tree": upstream_tree}), encoding="utf-8")
        mapped = sorted({".gitignore", "tools/quality/core-changes.yaml", "tools/quality/file-inventory.json", *protected_sources})
        (quality / "module-map.yaml").write_text(json.dumps({"modules": [{"id": "quality-governance", "paths": mapped}]}), encoding="utf-8")
        git("add", ".")
        git("commit", "-qm", "trusted base policy")
        base = git("rev-parse", "HEAD")

        trusted_policy = temporary / "trusted-architecture-policy.json"
        trusted_policy.write_bytes((quality / "architecture-policy.json").read_bytes())
        trusted_tools = temporary / "trusted-tools"
        trusted_tools.mkdir()
        (trusted_tools / "check_architecture_policy.py").write_bytes((quality / "check_architecture_policy.py").read_bytes())
        (trusted_tools / "capture_inventory.py").write_bytes((quality / "capture_inventory.py").read_bytes())
        base_controls = {path: (root / path).read_bytes() for path in checker.BASE_GOVERNED_CONTROL_SOURCES}

        candidate_policy = copy.deepcopy(policy)
        next(lane for lane in candidate_policy["lanes"] if lane["id"] == "policy-fixtures")["rules"].append("POL-03")
        (quality / "architecture-policy.json").write_text(json.dumps(candidate_policy, indent=2) + "\n", encoding="utf-8")

        plan = checker.create_plan(root, trusted_policy, base)
        self.assertFalse(plan["trusted_base"]["bootstrap_required"])
        self.assertEqual(plan["trusted_base"]["evaluator_source"], "base")
        self.assertEqual(plan["trusted_base"]["evaluated_policy_source"], "base")
        self.assertEqual(set(plan["required_reports"]), {"python-architecture", "runtime-graph", "go-build-plan", "policy-fixtures"})
        selected_lanes = [lane for lane in plan["lanes"] if lane["selected"] and lane["contract"] != "inventory"]
        self.assertTrue(all(lane["source_integrity"] == "PASS" for lane in selected_lanes))
        changed_records = {record["path"]: record for lane in selected_lanes for record in lane["source_bundle"] if not record["candidate_matches"]}
        self.assertEqual(set(changed_records), checker.BASE_GOVERNED_CONTROL_SOURCES)
        self.assertTrue(all(record["candidate_change_policy"] == "BASE_FIXTURE_REVIEW" for record in changed_records.values()))
        control_reviews = [item for item in plan["manual_review_required"] if "base-governed architecture controls changed" in item]
        self.assertEqual(len(control_reviews), 1)
        self.assertTrue(all(path in control_reviews[0] for path in checker.BASE_GOVERNED_CONTROL_SOURCES))

        candidate_plan = checker.create_plan(root, quality / "architecture-policy.json", base)
        compatibility = checker.compare_policy_coverage(policy, candidate_policy, plan, candidate_plan)
        self.assertEqual(compatibility["authority_source"], "base")
        self.assertEqual(compatibility["compatibility_status"], "COMPATIBLE")

        materialized = temporary / "protected-sources"
        manifest = checker.materialize_protected_sources(root, plan, materialized)
        self.assertEqual(manifest["source_count"], len(protected_sources))
        for path, content in base_controls.items():
            self.assertEqual((materialized / path).read_bytes(), content)

        lock_path = root / "web/pnpm-lock.yaml"
        lock_content = lock_path.read_bytes()
        lock_path.write_bytes(lock_content + b"\n# candidate lock change\n")
        lock_plan = checker.create_plan(root, trusted_policy, base)
        lock_lanes = {lane["id"]: lane for lane in lock_plan["lanes"]}
        self.assertEqual(lock_lanes["python-architecture"]["source_integrity"], "INCOMPLETE")
        self.assertEqual(lock_lanes["runtime-graph"]["source_integrity"], "INCOMPLETE")
        lock_path.write_bytes(lock_content)

        checker_path = quality / "check_architecture_policy.py"
        checker_path.write_text(checker_path.read_text(encoding="utf-8") + "\n# candidate checker extension\n", encoding="utf-8")
        workflow_path = root / ".github/workflows/architecture.yml"
        workflow_path.write_text(workflow_path.read_text(encoding="utf-8") + "\n# candidate workflow extension\n", encoding="utf-8")
        blocked_plan = checker.create_plan(root, trusted_policy, base)
        blocked_lanes = [lane for lane in blocked_plan["lanes"] if lane["selected"] and lane["contract"] != "inventory"]
        self.assertTrue(all(lane["source_integrity"] == "INCOMPLETE" for lane in blocked_lanes))

        output = root / "output/selection.json"
        completed = subprocess.run(
            [
                sys.executable,
                "-B",
                str(trusted_tools / "check_architecture_policy.py"),
                "--root",
                str(root),
                "--policy",
                str(trusted_policy),
                "select",
                "--base",
                base,
                "--output",
                str(output),
            ],
            cwd=root,
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(completed.returncode, 0, completed.stderr)
        external_plan = json.loads(output.read_text(encoding="utf-8"))
        self.assertEqual(external_plan["trusted_base"]["evaluator_source"], "base")
        self.assertEqual(external_plan["trusted_base"]["evaluated_policy_source"], "base")
        self.assertTrue(all(lane["source_integrity"] == "INCOMPLETE" for lane in external_plan["lanes"] if lane["selected"] and lane["contract"] != "inventory"))

    def test_github_output_cannot_modify_repository_file(self):
        temporary = Path(self.enterContext(__import__("tempfile").TemporaryDirectory()))
        root = temporary / "repo"
        root.mkdir()
        inside = root / "tracked.txt"
        outside = temporary / "github-output.txt"
        inside.write_text("tracked\n", encoding="utf-8")
        outside.write_text("", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "outside the repository"):
            checker._safe_github_output(root, inside)
        self.assertEqual(checker._safe_github_output(root, outside), outside.resolve())

        checker._write_github_outputs(
            outside,
            {
                "selection_sha256": "a" * 64,
                "lanes": [
                    {"id": "python-architecture", "contract": "python_architecture", "selected": True, "source_integrity": "INCOMPLETE"},
                    {"id": "runtime-graph", "contract": "runtime_graph", "selected": True, "source_integrity": "PASS"},
                ],
            },
        )
        self.assertEqual(
            outside.read_text(encoding="utf-8").splitlines(),
            [
                f"selection_sha256={'a' * 64}",
                "python_architecture_selected=true",
                "python_architecture=false",
                "runtime_graph_selected=true",
                "runtime_graph=true",
            ],
        )

    def test_pr_probe_allows_documentation_only_change(self):
        plan = selected_plan(["README.md"])
        result = aggregate(plan, {})
        self.assertEqual(result["aggregate_status"], "REPORT_ONLY_COMPLETE")
        self.assertEqual({item["status"] for item in result["results"]}, {"OBSERVED", "NOT_APPLICABLE"})

    def test_pr_probe_blocks_forbidden_architecture_finding(self):
        plan = selected_plan(["tools/quality/module-map.yaml"])
        evidence = complete_evidence(plan)
        evidence["python-architecture"] = report(
            plan,
            "python-architecture",
            "check_architecture",
            policy_status="FAIL",
            findings=[{"rule": "ARC-01", "message": "forbidden dependency"}],
            exit_code=1,
        )
        result = aggregate(plan, evidence)
        self.assertEqual(result["aggregate_status"], "FAIL")
        self.assertEqual(result["exit_code"], 1)

    def test_pr_probe_fails_closed_for_unclassified_path_and_missing_analyzer(self):
        with self.assertRaisesRegex(ValueError, "no current or base provenance"):
            checker.select_lanes(POLICY, ["unclassified/new.py"], [], {})
        plan = selected_plan(["tools/quality/module-map.yaml"])
        evidence = complete_evidence(plan)
        evidence.pop("python-architecture")
        result = aggregate(plan, evidence)
        self.assertEqual(result["aggregate_status"], "INCOMPLETE")
        self.assertEqual(result["exit_code"], 2)

    def test_plan_identity_change_is_rejected(self):
        plan = selected_plan(["README.md"])
        current = {**IDENTITY, "snapshot_sha256": "1" * 64}
        with self.assertRaisesRegex(ValueError, "plan is stale"):
            aggregate(plan, {}, current)


if __name__ == "__main__":
    unittest.main()
