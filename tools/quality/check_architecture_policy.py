"""Select T3 architecture lanes and aggregate their evidence without enforcing merge policy."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
import xml.etree.ElementTree as ET

import yaml

from capture_inventory import capture, git, paths, safe_path


VERSION = "0.1.0"
MODE = "report_only"
CONTRACTS = {"inventory", "python_architecture", "runtime_graph", "go_build_plan", "junit"}
TERMINAL_STATUSES = {"PASS", "FAIL", "INCOMPLETE", "NOT_APPLICABLE", "OBSERVED", "CLASSIFIED", "PLANNED"}


def _sha256(content: bytes) -> str:
    return hashlib.sha256(content).hexdigest()


def _canonical(value: object) -> bytes:
    return json.dumps(value, ensure_ascii=True, sort_keys=True, separators=(",", ":")).encode()


def _resolve(root: Path, value: Path) -> Path:
    return value.resolve() if value.is_absolute() else safe_path(root, value.as_posix()).resolve()


def _safe_new_output(root: Path, value: Path) -> Path:
    output = _resolve(root, value)
    if output.exists():
        raise ValueError(f"Refusing to overwrite architecture evidence: {output}")
    repository_files = set(paths(git(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")))
    if output.is_relative_to(root):
        relative = output.relative_to(root).as_posix()
        if relative in repository_files:
            raise ValueError("Architecture evidence must not overwrite a repository file")
        try:
            git(root, "check-ignore", "--no-index", "-q", relative)
        except subprocess.CalledProcessError as error:
            raise ValueError("Architecture evidence inside the repository must be ignored") from error
    return output


def _string_list(value: object, label: str, *, allow_empty: bool = True) -> list[str]:
    if not isinstance(value, list) or (not allow_empty and not value):
        raise ValueError(f"{label} must be a {'nonempty ' if not allow_empty else ''}list")
    if any(not isinstance(item, str) or not item for item in value):
        raise ValueError(f"{label} contains an invalid value")
    if len(value) != len(set(value)):
        raise ValueError(f"{label} contains duplicates")
    return value


def load_policy(path: Path) -> dict:
    raw = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(raw, dict) or raw.get("schema_version") != 1 or raw.get("mode") != MODE:
        raise ValueError("Unsupported architecture policy schema or mode")
    if raw.get("job_name") != "architecture-policy":
        raise ValueError("Architecture policy must retain the stable job name")
    if not isinstance(raw.get("tool_path"), str) or not raw["tool_path"]:
        raise ValueError("Architecture policy requires its exact tool path")
    lanes = raw.get("lanes")
    if not isinstance(lanes, list) or not lanes:
        raise ValueError("Architecture policy requires lanes")
    lane_ids = []
    always = []
    for lane in lanes:
        if not isinstance(lane, dict):
            raise ValueError("Architecture lane must be an object")
        lane_id = lane.get("id")
        if not isinstance(lane_id, str) or not re.fullmatch(r"[a-z][a-z0-9-]*", lane_id):
            raise ValueError("Architecture lane has an invalid ID")
        lane_ids.append(lane_id)
        contract = lane.get("contract")
        if contract not in CONTRACTS:
            raise ValueError(f"Lane {lane_id} has an unsupported contract")
        _string_list(lane.get("rules"), f"Lane {lane_id} rules", allow_empty=False)
        selectors = lane.get("selectors", {})
        if not isinstance(selectors, dict) or set(selectors) - {"paths", "prefixes", "suffixes"}:
            raise ValueError(f"Lane {lane_id} has invalid selectors")
        for key in ("paths", "prefixes", "suffixes"):
            values = _string_list(selectors.get(key, []), f"Lane {lane_id} selector {key}")
            if any("*" in item or "?" in item or "\\" in item or item.startswith("/") or ".." in Path(item).parts for item in values):
                raise ValueError(f"Lane {lane_id} selector {key} must use exact normalized values")
        if lane.get("always") is True:
            always.append(lane_id)
        elif lane.get("always") not in (None, False):
            raise ValueError(f"Lane {lane_id} has an invalid always flag")
        if contract != "inventory" and not isinstance(lane.get("tool"), str):
            raise ValueError(f"Lane {lane_id} requires an exact tool name")
        if contract == "junit":
            _string_list(lane.get("required_cases"), f"Lane {lane_id} required cases", allow_empty=False)
    if len(lane_ids) != len(set(lane_ids)):
        raise ValueError("Architecture policy contains duplicate lane IDs")
    if len(always) != 1 or next(lane for lane in lanes if lane["id"] == always[0])["contract"] != "inventory":
        raise ValueError("Architecture policy requires one always-selected inventory lane")
    return raw


def _module_assignments(mapping: dict) -> dict[str, str]:
    modules = mapping.get("modules") if isinstance(mapping, dict) else None
    if not isinstance(modules, list):
        raise ValueError("Invalid module map")
    assigned = {}
    for module in modules:
        if not isinstance(module, dict) or not isinstance(module.get("id"), str):
            raise ValueError("Invalid module record")
        for path in _string_list(module.get("paths"), f"Module {module['id']} paths"):
            if path in assigned:
                raise ValueError(f"Duplicate module assignment: {path}")
            assigned[path] = module["id"]
    return assigned


def _base_blob(root: Path, revision: str, path: str) -> bytes | None:
    try:
        return git(root, "show", f"{revision}:{path}")
    except subprocess.CalledProcessError:
        return None


def _changed_paths(root: Path, base: str, head: str) -> list[str]:
    git(root, "merge-base", "--is-ancestor", base, head)
    changed = set()
    commands = (
        ("diff", "--name-only", "--no-renames", "-z", base, head, "--"),
        ("diff", "--cached", "--name-only", "--no-renames", "-z", head, "--"),
        ("diff", "--name-only", "--no-renames", "-z", "--"),
        ("ls-files", "--others", "--exclude-standard", "-z"),
    )
    for command in commands:
        changed.update(paths(git(root, *command)))
    return sorted(changed)


def _matches(path: str, selectors: dict) -> bool:
    if path in selectors.get("paths", []):
        return True
    if any(path.startswith(prefix) for prefix in selectors.get("prefixes", [])):
        return True
    return any(path.endswith(suffix) for suffix in selectors.get("suffixes", []))


def select_lanes(policy: dict, changed: list[str], current_records: list[dict], base_owners: dict[str, str]) -> tuple[list[dict], list[dict]]:
    records = {record["path"]: record for record in current_records}
    changes = []
    for path in changed:
        record = records.get(path)
        owner = record.get("module") if record else base_owners.get(path)
        if owner is None:
            raise ValueError(f"Changed path has no current or base provenance: {path}")
        changes.append(
            {
                "path": path,
                "owner": owner,
                "origin": record.get("origin") if record else "extension_or_reverted_core_path",
                "present": bool(record and record.get("working_file", {}).get("present")),
                "provenance_source": "candidate" if record else "base",
            }
        )
    lanes = []
    for lane in policy["lanes"]:
        matched = [path for path in changed if _matches(path, lane.get("selectors", {}))]
        selected = lane.get("always") is True or bool(matched)
        lanes.append(
            {
                "id": lane["id"],
                "contract": lane["contract"],
                "tool": lane.get("tool"),
                "rules": lane["rules"],
                "selected": selected,
                "status": "REQUIRED" if selected else "NOT_APPLICABLE",
                "matched_paths": matched,
                "reason": "always verify candidate provenance" if lane.get("always") is True else ("matched exact selector" if matched else "no changed path matched the lane"),
            }
        )
    return lanes, changes


def create_plan(root: Path, policy_path: Path, base_ref: str) -> dict:
    policy_bytes = policy_path.read_bytes()
    policy = load_policy(policy_path)
    base_config = json.loads(safe_path(root, "tools/quality/upstream-base.json").read_text(encoding="utf-8"))
    mapping_path = safe_path(root, "tools/quality/module-map.yaml")
    mapping = yaml.safe_load(mapping_path.read_text(encoding="utf-8"))
    snapshot = capture(root, base_config, mapping)
    base = git(root, "rev-parse", f"{base_ref}^{{commit}}").decode().strip()
    changed = _changed_paths(root, base, snapshot["head"])
    base_mapping_blob = _base_blob(root, base, "tools/quality/module-map.yaml")
    if base_mapping_blob is None:
        raise ValueError("Base revision has no provenance module map")
    base_owners = _module_assignments(yaml.safe_load(base_mapping_blob.decode("utf-8")))
    lanes, changes = select_lanes(policy, changed, snapshot["records"], base_owners)

    candidate_tool = safe_path(root, policy["tool_path"]).read_bytes()
    base_tool = _base_blob(root, base, policy["tool_path"])
    policy_relative = policy_path.relative_to(root).as_posix() if policy_path.is_relative_to(root) else None
    base_policy = _base_blob(root, base, policy_relative) if policy_relative else None
    trusted_base = {
        "tool_present": base_tool is not None,
        "tool_sha256": _sha256(base_tool) if base_tool is not None else None,
        "candidate_tool_sha256": _sha256(candidate_tool),
        "candidate_tool_changed": base_tool != candidate_tool,
        "policy_present": base_policy is not None,
        "policy_sha256": _sha256(base_policy) if base_policy is not None else None,
        "candidate_policy_changed": base_policy != policy_bytes,
        "bootstrap_required": base_tool is None or base_policy is None,
    }
    manual_review = []
    if trusted_base["candidate_tool_changed"]:
        manual_review.append("architecture checker implementation changed relative to the PR base")
    if trusted_base["candidate_policy_changed"]:
        manual_review.append("architecture selector or aggregate policy changed relative to the PR base")
    if trusted_base["bootstrap_required"]:
        manual_review.append("trusted base checker is absent; this bootstrap run cannot establish a non-bypassable gate")

    return {
        "schema_version": 1,
        "tool": {"name": "check_architecture_policy", "version": VERSION},
        "mode": MODE,
        "planning_status": "PLANNED",
        "enforcement_status": "NOT_ENABLED",
        "input": {
            "base": base,
            "head": snapshot["head"],
            "upstream_base": snapshot["upstream_base"],
            "snapshot_sha256": snapshot["snapshot_sha256"],
            "policy_sha256": _sha256(policy_bytes),
            "candidate_tool_sha256": _sha256(candidate_tool),
            "changed_paths_sha256": _sha256(_canonical(changed)),
        },
        "trusted_base": trusted_base,
        "changes": changes,
        "lanes": lanes,
        "required_reports": [lane["id"] for lane in lanes if lane["selected"] and lane["contract"] != "inventory"],
        "manual_review_required": manual_review,
    }


def _identity(report: dict) -> tuple[object, object, object]:
    source = report.get("input", {}) if isinstance(report, dict) else {}
    return (
        source.get("head", source.get("candidate_sha")),
        source.get("upstream_base", source.get("upstream_sha")),
        source.get("snapshot_sha256", source.get("dirty_snapshot_sha256")),
    )


def _identity_problem(report: dict, expected: dict) -> str | None:
    observed = _identity(report)
    wanted = (expected["head"], expected["upstream_base"], expected["snapshot_sha256"])
    return None if observed == wanted else f"report identity mismatch: expected {wanted}, observed {observed}"


def _json_contract(lane: dict, report: dict, expected: dict) -> tuple[str, str]:
    if not isinstance(report, dict) or report.get("schema_version") != 1:
        return "INCOMPLETE", "invalid report schema"
    tool = report.get("tool", {})
    tool_name = tool.get("name") if isinstance(tool, dict) else tool
    if tool_name != lane["tool"]:
        return "INCOMPLETE", f"unexpected report tool: {tool_name!r}"
    problem = _identity_problem(report, expected)
    if problem:
        return "INCOMPLETE", problem

    contract = lane["contract"]
    if contract == "python_architecture":
        status = report.get("policy_status")
        findings = report.get("findings")
        if not isinstance(findings, list):
            return "INCOMPLETE", "architecture report omitted findings"
        if status == "INCOMPLETE" or report.get("exit_code") == 2:
            return "INCOMPLETE", f"architecture report is incomplete with {len(findings)} finding(s)"
        if status == "PASS" and not findings and report.get("exit_code") == 0:
            return "PASS", "configured Python architecture checks passed"
        if status == "FAIL" or findings:
            return "FAIL", f"architecture report contains {len(findings)} finding(s)"
        return "INCOMPLETE", f"architecture report status is {status!r}"
    if contract == "runtime_graph":
        findings = report.get("findings")
        if not isinstance(findings, list):
            return "INCOMPLETE", "runtime graph report omitted findings"
        if report.get("classification_status") == "COMPLETE" and report.get("policy_status") == "REPORT_ONLY" and not findings and report.get("exit_code") == 0:
            return "CLASSIFIED", "runtime graph signals are completely classified in report-only mode"
        return "INCOMPLETE", f"runtime graph classification is {report.get('classification_status')!r} with {len(findings)} finding(s)"
    if contract == "go_build_plan":
        findings = report.get("findings")
        if not isinstance(findings, list):
            return "INCOMPLETE", "Go build plan omitted findings"
        if report.get("analysis_status") == "INCOMPLETE" or report.get("exit_code") == 2:
            return "INCOMPLETE", f"Go build plan is incomplete with {len(findings)} finding(s)"
        if report.get("analysis_status") == "READY" and report.get("policy_status") == "NOT_EVALUATED" and not findings and report.get("exit_code") == 0:
            return "PLANNED", "all changed Go paths have a build.sh plan; native execution is not evaluated"
        if report.get("analysis_status") == "FAIL" or findings:
            return "FAIL", f"Go build plan contains {len(findings)} finding(s)"
        return "INCOMPLETE", f"Go build plan status is {report.get('analysis_status')!r}"
    return "INCOMPLETE", f"unsupported JSON contract: {contract}"


def _junit_contract(lane: dict, content: bytes) -> tuple[str, str]:
    try:
        root = ET.fromstring(content)
    except ET.ParseError as error:
        return "INCOMPLETE", f"unreadable JUnit: {error}"
    if root.tag not in {"testsuite", "testsuites"}:
        return "INCOMPLETE", "invalid JUnit root"
    cases = list(root.iter("testcase"))
    if not cases:
        return "INCOMPLETE", "JUnit contains no test cases"
    has_failure = any(case.find("failure") is not None for case in cases)
    has_incomplete = any(case.find("error") is not None or case.find("skipped") is not None for case in cases)
    try:
        for suite in (item for item in root.iter() if item.tag in {"testsuite", "testsuites"}):
            has_failure = has_failure or int(suite.get("failures", "0")) > 0
            has_incomplete = has_incomplete or int(suite.get("errors", "0")) > 0 or int(suite.get("skipped", "0")) > 0
    except ValueError:
        return "INCOMPLETE", "JUnit contains invalid summary counts"
    if has_incomplete:
        return "INCOMPLETE", "required policy fixture errored or skipped"
    if has_failure:
        return "FAIL", "policy fixture failed"
    names = {case.get("name") for case in cases}
    missing = sorted(set(lane["required_cases"]) - names)
    if missing:
        return "INCOMPLETE", "required policy fixtures missing: " + ", ".join(missing)
    return "PASS", f"{len(cases)} policy fixture(s) passed"


def validate_replanned(plan: dict, replanned: dict) -> None:
    if _canonical(plan) != _canonical(replanned):
        raise ValueError("Architecture selection plan differs from a fresh selector run")


def aggregate(policy: dict, plan: dict, evidence: dict[str, bytes], current: dict) -> dict:
    if not isinstance(plan, dict) or plan.get("schema_version") != 1 or plan.get("mode") != MODE:
        raise ValueError("Invalid architecture selection plan")
    expected = plan.get("input")
    if not isinstance(expected, dict):
        raise ValueError("Architecture plan omitted input identity")
    actual_identity = (current["head"], current["upstream_base"], current["snapshot_sha256"])
    plan_identity = (expected.get("head"), expected.get("upstream_base"), expected.get("snapshot_sha256"))
    if actual_identity != plan_identity:
        raise ValueError(f"Architecture plan is stale: expected {plan_identity}, observed {actual_identity}")
    policy_lanes = {lane["id"]: lane for lane in policy["lanes"]}
    planned_lanes = plan.get("lanes")
    if not isinstance(planned_lanes, list) or {lane.get("id") for lane in planned_lanes if isinstance(lane, dict)} != set(policy_lanes):
        raise ValueError("Architecture plan lanes differ from policy")
    selected = {lane["id"] for lane in planned_lanes if lane.get("selected") is True}
    expected_evidence = {lane_id for lane_id in selected if policy_lanes[lane_id]["contract"] != "inventory"}
    unknown = set(evidence) - expected_evidence
    if unknown:
        raise ValueError("Unexpected architecture evidence: " + ", ".join(sorted(unknown)))

    results = []
    for planned in planned_lanes:
        lane = policy_lanes[planned["id"]]
        if planned["id"] not in selected:
            results.append({"id": planned["id"], "status": "NOT_APPLICABLE", "reason": planned.get("reason")})
            continue
        if lane["contract"] == "inventory":
            results.append({"id": planned["id"], "status": "OBSERVED", "reason": "fresh T0 capture completed with classified paths"})
            continue
        content = evidence.get(planned["id"])
        if content is None:
            results.append({"id": planned["id"], "status": "INCOMPLETE", "reason": "required evidence is missing"})
            continue
        if lane["contract"] == "junit":
            status, reason = _junit_contract(lane, content)
        else:
            try:
                report = json.loads(content)
            except json.JSONDecodeError as error:
                status, reason = "INCOMPLETE", f"invalid JSON report: {error}"
            else:
                status, reason = _json_contract(lane, report, expected)
        if status not in TERMINAL_STATUSES:
            raise ValueError(f"Internal invalid aggregate status: {status}")
        results.append({"id": planned["id"], "status": status, "reason": reason, "sha256": _sha256(content)})

    statuses = {item["status"] for item in results}
    if "INCOMPLETE" in statuses:
        aggregate_status, exit_code = "INCOMPLETE", 2
    elif "FAIL" in statuses:
        aggregate_status, exit_code = "FAIL", 1
    else:
        aggregate_status, exit_code = "REPORT_ONLY_COMPLETE", 0
    return {
        "schema_version": 1,
        "tool": {"name": "check_architecture_policy", "version": VERSION},
        "mode": MODE,
        "aggregate_status": aggregate_status,
        "enforcement_status": "NOT_ENABLED",
        "input": expected,
        "results": results,
        "manual_review_required": plan.get("manual_review_required", []),
        "limits": [
            "This aggregate is report-only and is not branch protection or a full architecture/dead-code/build verdict.",
            "CLASSIFIED and PLANNED are not PASS; native Go execution and manual review remain separate evidence.",
            "A required check becomes effective only after repository branch protection is configured and verified.",
        ],
        "exit_code": exit_code,
    }


def _write_github_outputs(path: Path, lanes: list[dict]) -> None:
    with path.open("a", encoding="utf-8", newline="\n") as stream:
        for lane in lanes:
            key = lane["id"].replace("-", "_")
            stream.write(f"{key}={'true' if lane['selected'] else 'false'}\n")


def _parse_evidence(root: Path, plan_path: Path, values: list[str]) -> tuple[dict[str, bytes], dict[Path, bytes]]:
    evidence = {}
    snapshots = {}
    for value in values:
        lane_id, separator, raw_path = value.partition("=")
        if not separator or not lane_id or not raw_path or lane_id in evidence:
            raise ValueError(f"Invalid or duplicate --report value: {value}")
        path = _resolve(root, Path(raw_path))
        if not path.is_file() or not path.is_relative_to(plan_path.parent):
            raise ValueError(f"Evidence must be a regular file beside the plan: {path}")
        content = path.read_bytes()
        evidence[lane_id] = content
        snapshots[path] = content
    return evidence, snapshots


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--policy", type=Path, default=Path("tools/quality/architecture-policy.json"))
    subparsers = parser.add_subparsers(dest="action", required=True)
    select_parser = subparsers.add_parser("select")
    select_parser.add_argument("--base", required=True)
    select_parser.add_argument("--output", type=Path, required=True)
    select_parser.add_argument("--github-output", type=Path)
    aggregate_parser = subparsers.add_parser("aggregate")
    aggregate_parser.add_argument("--plan", type=Path, required=True)
    aggregate_parser.add_argument("--report", action="append", default=[])
    aggregate_parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args(argv)
    root = args.root.resolve()
    policy_path = _resolve(root, args.policy)
    try:
        policy = load_policy(policy_path)
        if args.action == "select":
            output = _safe_new_output(root, args.output)
            plan = create_plan(root, policy_path, args.base)
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_text(json.dumps(plan, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
            if args.github_output:
                _write_github_outputs(args.github_output.resolve(), plan["lanes"])
            print(json.dumps({"planning_status": plan["planning_status"], "required_reports": plan["required_reports"], "output": str(output)}))
            return 0

        plan_path = _resolve(root, args.plan)
        output = _safe_new_output(root, args.output)
        plan_bytes = plan_path.read_bytes()
        plan = json.loads(plan_bytes)
        if plan.get("input", {}).get("policy_sha256") != _sha256(policy_path.read_bytes()):
            raise ValueError("Architecture policy changed after selection")
        candidate_tool = safe_path(root, policy["tool_path"]).read_bytes()
        if plan.get("input", {}).get("candidate_tool_sha256") != _sha256(candidate_tool):
            raise ValueError("Architecture checker changed after selection")
        replanned = create_plan(root, policy_path, plan.get("input", {}).get("base", ""))
        validate_replanned(plan, replanned)
        evidence, snapshots = _parse_evidence(root, plan_path, args.report)
        base_config = json.loads(safe_path(root, "tools/quality/upstream-base.json").read_text(encoding="utf-8"))
        mapping = yaml.safe_load(safe_path(root, "tools/quality/module-map.yaml").read_text(encoding="utf-8"))
        before = capture(root, base_config, mapping)
        result = aggregate(policy, plan, evidence, before)
        after = capture(root, base_config, mapping)
        if (before["head"], before["snapshot_sha256"]) != (after["head"], after["snapshot_sha256"]):
            raise ValueError("Candidate changed during architecture aggregation")
        if plan_path.read_bytes() != plan_bytes or any(path.read_bytes() != content for path, content in snapshots.items()):
            raise ValueError("Architecture evidence changed during aggregation")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(json.dumps({"aggregate_status": result["aggregate_status"], "exit_code": result["exit_code"], "output": str(output)}))
        return result["exit_code"]
    except (KeyError, TypeError, ValueError, OSError, UnicodeError, json.JSONDecodeError, subprocess.CalledProcessError) as error:
        print(f"INCOMPLETE: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
