"""Select T3 architecture lanes and aggregate their evidence without enforcing merge policy."""

from __future__ import annotations

import argparse
import base64
import binascii
import hashlib
import json
import os
import re
import subprocess
import sys
import xml.etree.ElementTree as ET
from pathlib import Path, PurePosixPath

import yaml
from capture_inventory import capture, git, paths, safe_path

VERSION = "0.3.1"
TRUSTED_BASE_PROTOCOL = 1
MODE = "report_only"
POLICY_REPOSITORY_PATH = "tools/quality/architecture-policy.json"
BASE_GOVERNED_CONTROL_SOURCES = frozenset({POLICY_REPOSITORY_PATH})
CONTRACTS = {"inventory", "python_architecture", "runtime_graph", "go_build_plan", "junit", "policy_fixtures"}
TERMINAL_STATUSES = {"PASS", "FAIL", "INCOMPLETE", "NOT_APPLICABLE", "OBSERVED", "CLASSIFIED", "PLANNED"}


def _sha256(content: bytes) -> str:
    return hashlib.sha256(content).hexdigest()


def _canonical(value: object) -> bytes:
    return json.dumps(value, ensure_ascii=True, sort_keys=True, separators=(",", ":")).encode()


def _resolve(root: Path, value: Path) -> Path:
    return value.resolve() if value.is_absolute() else safe_path(root, value.as_posix()).resolve()


def _fixture_interpreter_path(root: Path, value: Path) -> Path:
    """Return an absolute launcher path without dereferencing virtualenv symlinks."""
    candidate = value if value.is_absolute() else root / value
    return Path(os.path.abspath(candidate))


def _repository_path(value: str, label: str) -> str:
    if not value or "\\" in value or value.startswith("/"):
        raise ValueError(f"{label} must be a normalized repository-relative path")
    normalized = PurePosixPath(value).as_posix()
    if normalized != value or any(part in {"", ".", ".."} for part in PurePosixPath(value).parts):
        raise ValueError(f"{label} must be a normalized repository-relative path")
    return value


def _verified_content_source(content: bytes, candidate: bytes, base: bytes | None, label: str) -> str:
    if base is not None and content == base:
        return "base"
    if content == candidate:
        return "candidate"
    raise ValueError(f"{label} does not match the candidate or comparison base")


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


def _safe_github_output(root: Path, value: Path) -> Path:
    output = value.resolve()
    if output.is_relative_to(root) or not output.is_file():
        raise ValueError("GitHub output must be an existing file outside the repository")
    return output


def _isolated_subprocess_environment(**overrides: str) -> dict[str, str]:
    blocked_names = {
        "NODE_EXTRA_CA_CERTS",
        "NODE_OPTIONS",
        "NODE_PATH",
        "PYTHONHOME",
        "PYTHONPATH",
        "PYTHONSTARTUP",
        "PYTEST_ADDOPTS",
        "PYTEST_PLUGINS",
        "RUNNER_TEMP",
    }
    blocked_prefixes = ("ACTIONS_", "ARCHITECTURE_", "GITHUB_")
    secret_markers = ("CREDENTIAL", "PASSWORD", "SECRET", "TOKEN")
    environment = {
        key: value
        for key, value in os.environ.items()
        if key.upper() not in blocked_names and not key.upper().startswith(blocked_prefixes) and "EVIDENCE" not in key.upper() and not any(marker in key.upper() for marker in secret_markers)
    }
    environment.update(overrides)
    return environment


def _string_list(value: object, label: str, *, allow_empty: bool = True) -> list[str]:
    if not isinstance(value, list) or (not allow_empty and not value):
        raise ValueError(f"{label} must be a {'nonempty ' if not allow_empty else ''}list")
    if any(not isinstance(item, str) or not item for item in value):
        raise ValueError(f"{label} contains an invalid value")
    if len(value) != len(set(value)):
        raise ValueError(f"{label} contains duplicates")
    return value


def _validate_lane_selectors(lane_id: str, lane: dict) -> None:
    selectors = lane.get("selectors", {})
    if not isinstance(selectors, dict) or set(selectors) - {"paths", "prefixes", "suffixes"}:
        raise ValueError(f"Lane {lane_id} has invalid selectors")
    for key in ("paths", "prefixes", "suffixes"):
        values = _string_list(selectors.get(key, []), f"Lane {lane_id} selector {key}")
        invalid = any("*" in item or "?" in item or "\\" in item or item.startswith("/") or ".." in Path(item).parts for item in values)
        if invalid:
            raise ValueError(f"Lane {lane_id} selector {key} must use exact normalized values")


def _validate_lane_sources(lane_id: str, lane: dict) -> None:
    protected = _string_list(lane.get("protected_sources"), f"Lane {lane_id} protected sources", allow_empty=False)
    for source_path in protected:
        _repository_path(source_path, f"Lane {lane_id} protected source")
    if any(not _matches(source_path, lane.get("selectors", {})) for source_path in protected):
        raise ValueError(f"Lane {lane_id} must select every protected source")
    reported = lane.get("reported_hashes")
    if not isinstance(reported, dict) or not reported:
        raise ValueError(f"Lane {lane_id} requires reported hashes")
    for field, source_path in reported.items():
        if not isinstance(field, str) or not re.fullmatch(r"[a-z][a-z0-9_]*_sha256", field):
            raise ValueError(f"Lane {lane_id} has an invalid reported hash field")
        if not isinstance(source_path, str):
            raise ValueError(f"Lane {lane_id} has an invalid reported hash source")
        _repository_path(source_path, f"Lane {lane_id} reported hash source")
    if not set(reported.values()) <= set(protected):
        raise ValueError(f"Lane {lane_id} reported hash sources must be protected")


def _validate_fixture_lane(lane_id: str, lane: dict) -> None:
    fixture_paths = _string_list(lane.get("fixture_paths"), f"Lane {lane_id} fixture paths", allow_empty=False)
    for fixture_path in fixture_paths:
        _repository_path(fixture_path, f"Lane {lane_id} fixture path")
    if not set(fixture_paths) <= set(lane["protected_sources"]):
        raise ValueError(f"Lane {lane_id} fixture paths must be protected")
    required_nodeids = _string_list(lane.get("required_nodeids"), f"Lane {lane_id} required node IDs", allow_empty=False)
    for nodeid in required_nodeids:
        fixture_path, separator, test_id = nodeid.partition("::")
        if not separator or not test_id or fixture_path not in fixture_paths:
            raise ValueError(f"Lane {lane_id} node ID is outside its exact fixture paths")


def _validate_lane(lane: object) -> tuple[str, bool, str]:
    if not isinstance(lane, dict):
        raise ValueError("Architecture lane must be an object")
    lane_id = lane.get("id")
    if not isinstance(lane_id, str) or not re.fullmatch(r"[a-z][a-z0-9-]*", lane_id):
        raise ValueError("Architecture lane has an invalid ID")
    contract = lane.get("contract")
    if contract not in CONTRACTS:
        raise ValueError(f"Lane {lane_id} has an unsupported contract")
    _string_list(lane.get("rules"), f"Lane {lane_id} rules", allow_empty=False)
    _validate_lane_selectors(lane_id, lane)
    always = lane.get("always")
    if always not in (None, False, True):
        raise ValueError(f"Lane {lane_id} has an invalid always flag")
    if contract != "inventory":
        if not isinstance(lane.get("tool"), str):
            raise ValueError(f"Lane {lane_id} requires an exact tool name")
        _validate_lane_sources(lane_id, lane)
    if contract in {"junit", "policy_fixtures"}:
        _string_list(lane.get("required_cases"), f"Lane {lane_id} required cases", allow_empty=False)
    if contract == "policy_fixtures":
        _validate_fixture_lane(lane_id, lane)
    return lane_id, always is True, contract


def load_policy(path: Path) -> dict:
    raw = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(raw, dict) or raw.get("schema_version") != 1 or raw.get("mode") != MODE:
        raise ValueError("Unsupported architecture policy schema or mode")
    if raw.get("job_name") != "architecture-policy":
        raise ValueError("Architecture policy must retain the stable job name")
    if raw.get("tool_path") != "tools/quality/check_architecture_policy.py":
        raise ValueError("Architecture policy must retain the trusted checker path")
    _repository_path(raw["tool_path"], "Architecture policy tool path")
    lanes = raw.get("lanes")
    if not isinstance(lanes, list) or not lanes:
        raise ValueError("Architecture policy requires lanes")
    validated = [_validate_lane(lane) for lane in lanes]
    lane_ids = [lane_id for lane_id, _always, _contract in validated]
    if len(lane_ids) != len(set(lane_ids)):
        raise ValueError("Architecture policy contains duplicate lane IDs")
    always = [(lane_id, contract) for lane_id, selected, contract in validated if selected]
    if len(always) != 1 or always[0][1] != "inventory":
        raise ValueError("Architecture policy requires one always-selected inventory lane")
    fixture_lanes = [lane for lane in lanes if lane["contract"] == "policy_fixtures"]
    if len(fixture_lanes) != 1:
        raise ValueError("Architecture policy requires exactly one policy-fixtures lane")
    fixture_lane = fixture_lanes[0]
    fixture_protected = set(fixture_lane["protected_sources"])
    missing_protection = BASE_GOVERNED_CONTROL_SOURCES - fixture_protected
    if missing_protection:
        raise ValueError("Policy-fixtures lane must protect base-governed controls: " + ", ".join(sorted(missing_protection)))
    selectors = fixture_lane.get("selectors", {})
    unselected_controls = {
        path
        for path in BASE_GOVERNED_CONTROL_SOURCES
        if path not in selectors.get("paths", [])
        and not any(path.startswith(prefix) for prefix in selectors.get("prefixes", []))
        and not any(path.endswith(suffix) for suffix in selectors.get("suffixes", []))
    }
    if unselected_controls:
        raise ValueError("Policy-fixtures lane must select base-governed controls: " + ", ".join(sorted(unselected_controls)))
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


def _lane_source_bundle(root: Path, base: str, lane: dict, source: str) -> tuple[list[dict], dict[str, str], str]:
    protected = set(lane["protected_sources"])
    reported = lane["reported_hashes"]
    fixture_paths = set(lane.get("fixture_paths", []))
    source_paths = sorted(protected | set(reported.values()) | fixture_paths)
    records = []
    hashes = {}
    for source_path in source_paths:
        candidate = safe_path(root, source_path).read_bytes()
        trusted = candidate if source == "candidate-bootstrap" else _base_blob(root, base, source_path)
        if trusted is None:
            raise ValueError(f"Comparison base omitted required architecture source: {source_path}")
        record = {
            "path": source_path,
            "source": source,
            "sha256": _sha256(trusted),
            "candidate_sha256": _sha256(candidate),
            "candidate_matches": candidate == trusted,
            "candidate_change_policy": "BASE_FIXTURE_REVIEW" if source == "base" and source_path in BASE_GOVERNED_CONTROL_SOURCES else "MUST_MATCH",
            "protected": source_path in protected,
        }
        records.append(record)
        hashes[source_path] = record["sha256"]
    expected_report_hashes = {field: hashes[source_path] for field, source_path in reported.items()}
    blocking_changes = [record for record in records if record["protected"] and not record["candidate_matches"] and record["candidate_change_policy"] == "MUST_MATCH"]
    integrity = "PASS" if not blocking_changes else "INCOMPLETE"
    return records, expected_report_hashes, integrity


def create_plan(root: Path, policy_path: Path, base_ref: str, policy_repository_path: str = POLICY_REPOSITORY_PATH) -> dict:
    policy_repository_path = _repository_path(policy_repository_path, "Policy repository path")
    policy_bytes = policy_path.read_bytes()
    policy = load_policy(policy_path)
    candidate_policy = safe_path(root, policy_repository_path).read_bytes()
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
    evaluator_tool = Path(__file__).resolve().read_bytes()
    evaluator_source = _verified_content_source(evaluator_tool, candidate_tool, base_tool, "Architecture evaluator")
    base_policy = _base_blob(root, base, policy_repository_path)
    policy_source = _verified_content_source(policy_bytes, candidate_policy, base_policy, "Architecture policy")
    trusted_base = {
        "tool_present": base_tool is not None,
        "tool_sha256": _sha256(base_tool) if base_tool is not None else None,
        "candidate_tool_sha256": _sha256(candidate_tool),
        "candidate_tool_changed": base_tool != candidate_tool,
        "evaluator_tool_sha256": _sha256(evaluator_tool),
        "evaluator_source": evaluator_source,
        "policy_present": base_policy is not None,
        "policy_sha256": _sha256(base_policy) if base_policy is not None else None,
        "candidate_policy_sha256": _sha256(candidate_policy),
        "candidate_policy_changed": base_policy != candidate_policy,
        "evaluated_policy_sha256": _sha256(policy_bytes),
        "evaluated_policy_source": policy_source,
        "protocol": TRUSTED_BASE_PROTOCOL,
        "bootstrap_required": base_tool is None or base_policy is None or evaluator_source != "base" or policy_source != "base",
    }
    source = "candidate-bootstrap" if trusted_base["bootstrap_required"] else "base"
    policy_lanes = {lane["id"]: lane for lane in policy["lanes"]}
    for planned_lane in lanes:
        if not planned_lane["selected"] or planned_lane["contract"] == "inventory":
            planned_lane["source_integrity"] = "NOT_APPLICABLE"
            continue
        source_bundle, expected_report_hashes, integrity = _lane_source_bundle(root, base, policy_lanes[planned_lane["id"]], source)
        planned_lane.update(
            source=source,
            source_bundle=source_bundle,
            source_integrity=integrity,
            expected_report_hashes=expected_report_hashes,
        )
        if planned_lane["contract"] == "policy_fixtures":
            fixture_paths = set(policy_lanes[planned_lane["id"]]["fixture_paths"])
            planned_lane["fixture_sources"] = [{"path": record["path"], "sha256": record["sha256"]} for record in source_bundle if record["path"] in fixture_paths]
    manual_review = []
    if trusted_base["candidate_tool_changed"]:
        manual_review.append("architecture checker implementation changed relative to the PR base")
    if trusted_base["candidate_policy_changed"]:
        manual_review.append("architecture selector or aggregate policy changed relative to the PR base")
    if trusted_base["bootstrap_required"]:
        manual_review.append("trusted base evaluator or policy is unavailable; this bootstrap run cannot establish a non-bypassable gate")
    reviewed_controls = sorted(
        {record["path"] for lane in lanes for record in lane.get("source_bundle", []) if not record["candidate_matches"] and record["candidate_change_policy"] == "BASE_FIXTURE_REVIEW"}
    )
    if reviewed_controls:
        manual_review.append("base-governed architecture controls changed; trusted base comparison/fixtures apply and manual approval remains required: " + ", ".join(reviewed_controls))
    changed_sources = [lane["id"] for lane in lanes if lane.get("source_integrity") == "INCOMPLETE"]
    if changed_sources:
        manual_review.append("protected analyzer or fixture sources changed relative to the trusted base: " + ", ".join(changed_sources))

    plan = {
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
            "candidate_policy_sha256": _sha256(candidate_policy),
            "candidate_tool_sha256": _sha256(candidate_tool),
            "evaluator_tool_sha256": _sha256(evaluator_tool),
            "policy_repository_path": policy_repository_path,
            "changed_paths_sha256": _sha256(_canonical(changed)),
        },
        "trusted_base": trusted_base,
        "changes": changes,
        "lanes": lanes,
        "required_reports": [lane["id"] for lane in lanes if lane["selected"] and lane["contract"] != "inventory"],
        "manual_review_required": manual_review,
    }
    selection_sha256 = _sha256(_canonical(plan))
    plan["selection_sha256"] = selection_sha256
    plan["input"]["selection_sha256"] = selection_sha256
    return plan


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


def _python_architecture_contract(_lane: dict, report: dict) -> tuple[str, str]:
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


def _runtime_graph_contract(_lane: dict, report: dict) -> tuple[str, str]:
    findings = report.get("findings")
    if not isinstance(findings, list):
        return "INCOMPLETE", "runtime graph report omitted findings"
    if report.get("classification_status") == "COMPLETE" and report.get("policy_status") == "REPORT_ONLY" and not findings and report.get("exit_code") == 0:
        return "CLASSIFIED", "runtime graph signals are completely classified in report-only mode"
    return "INCOMPLETE", f"runtime graph classification is {report.get('classification_status')!r} with {len(findings)} finding(s)"


def _go_build_plan_contract(_lane: dict, report: dict) -> tuple[str, str]:
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


def _validate_fixture_attestation_identity(lane: dict, planned: dict, report: dict) -> None:
    case_names = report.get("case_names")
    if not isinstance(case_names, list) or any(not isinstance(name, str) for name in case_names):
        raise ValueError("fixture attestation omitted test case names")
    if report.get("required_cases") != lane["required_cases"]:
        raise ValueError("fixture attestation used different required cases")
    if report.get("required_nodeids") != lane["required_nodeids"] or report.get("executed_nodeids") != lane["required_nodeids"]:
        raise ValueError("fixture attestation used different exact test node IDs")
    if report.get("fixture_source") != planned.get("source") or _canonical(report.get("fixture_sources")) != _canonical(planned.get("fixture_sources")):
        raise ValueError("fixture attestation used different test sources")


def _decode_attested_junit(lane: dict, report: dict) -> tuple[bytes, list[str]]:
    if not isinstance(report.get("junit_sha256"), str) or not re.fullmatch(r"[0-9a-f]{64}", report["junit_sha256"]):
        raise ValueError("fixture attestation omitted the JUnit digest")
    encoded = report.get("junit_base64")
    if not isinstance(encoded, str):
        raise ValueError("fixture attestation omitted raw JUnit evidence")
    try:
        content = base64.b64decode(encoded, validate=True)
    except (ValueError, binascii.Error):
        raise ValueError("fixture attestation contains invalid JUnit encoding") from None
    if _sha256(content) != report["junit_sha256"]:
        raise ValueError("fixture attestation JUnit digest mismatch")
    try:
        parsed_names = sorted(case.get("name") for case in ET.fromstring(content).iter("testcase") if case.get("name"))
    except ET.ParseError as error:
        raise ValueError(f"unreadable attested JUnit: {error}") from error
    if len(parsed_names) != len(set(parsed_names)):
        raise ValueError("attested JUnit contains duplicate test case names")
    if len(parsed_names) != len(lane["required_nodeids"]):
        raise ValueError("fixture attestation case inventory differs from raw JUnit")
    return content, parsed_names


def _policy_fixtures_contract(lane: dict, planned: dict, report: dict) -> tuple[str, str]:
    status = report.get("fixture_status")
    try:
        _validate_fixture_attestation_identity(lane, planned, report)
        content, parsed_names = _decode_attested_junit(lane, report)
    except ValueError as error:
        return "INCOMPLETE", str(error)
    case_names = report["case_names"]
    if case_names != parsed_names or report.get("case_count") != len(parsed_names) or len(parsed_names) != len(lane["required_nodeids"]):
        return "INCOMPLETE", "fixture attestation case inventory differs from raw JUnit"
    missing = sorted(set(lane["required_cases"]) - set(case_names))
    if missing:
        return "INCOMPLETE", "required policy fixtures missing: " + ", ".join(missing)
    junit_status, junit_reason = _junit_contract(lane, content)
    if junit_status != "PASS":
        return junit_status, junit_reason
    if status == "INCOMPLETE" or report.get("exit_code") == 2:
        return "INCOMPLETE", report.get("reason", "policy fixture execution is incomplete")
    if status == "FAIL" and report.get("exit_code") == 1 and report.get("pytest_exit_code") == 1:
        return "FAIL", report.get("reason", "policy fixture failed")
    if status == "PASS" and report.get("exit_code") == 0 and report.get("pytest_exit_code") == 0:
        return "PASS", f"{len(case_names)} attested policy fixture(s) passed"
    return "INCOMPLETE", "fixture attestation contains inconsistent status or exit codes"


JSON_CONTRACT_HANDLERS = {
    "python_architecture": _python_architecture_contract,
    "runtime_graph": _runtime_graph_contract,
    "go_build_plan": _go_build_plan_contract,
}


def _json_contract(lane: dict, planned: dict, report: dict, expected: dict) -> tuple[str, str]:
    if not isinstance(report, dict) or report.get("schema_version") != 1:
        return "INCOMPLETE", "invalid report schema"
    tool = report.get("tool", {})
    tool_name = tool.get("name") if isinstance(tool, dict) else tool
    if tool_name != lane["tool"]:
        return "INCOMPLETE", f"unexpected report tool: {tool_name!r}"
    problem = _identity_problem(report, expected)
    if problem:
        return "INCOMPLETE", problem
    report_input = report.get("input")
    if not isinstance(report_input, dict):
        return "INCOMPLETE", "report omitted source identity"
    for field, digest in planned.get("expected_report_hashes", {}).items():
        if report_input.get(field) != digest:
            return "INCOMPLETE", f"report producer/config digest mismatch: {field}"
    if report_input.get("selection_sha256") != expected.get("selection_sha256"):
        return "INCOMPLETE", "report selection digest mismatch"
    if lane["contract"] == "policy_fixtures":
        return _policy_fixtures_contract(lane, planned, report)
    handler = JSON_CONTRACT_HANDLERS.get(lane["contract"])
    return handler(lane, report) if handler else ("INCOMPLETE", f"unsupported JSON contract: {lane['contract']}")


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
    names = [case.get("name") for case in cases if case.get("name")]
    if len(names) != len(set(names)):
        return "INCOMPLETE", "JUnit contains duplicate test case names"
    missing = sorted(set(lane["required_cases"]) - set(names))
    if missing:
        return "INCOMPLETE", "required policy fixtures missing: " + ", ".join(missing)
    return "PASS", f"{len(cases)} policy fixture(s) passed"


def _base_fixture_paths(root: Path, base: str, fixture_paths: list[str], sources: dict[str, str], fixture_root: Path) -> dict[str, Path]:
    if fixture_root.exists():
        raise ValueError(f"Refusing stale trusted fixture directory: {fixture_root}")
    source_paths = {}
    for fixture_path in fixture_paths:
        content = _base_blob(root, base, fixture_path)
        if content is None or _sha256(content) != sources[fixture_path]:
            raise ValueError(f"Trusted fixture differs from the selection plan: {fixture_path}")
        target = (fixture_root / PurePosixPath(fixture_path)).resolve()
        if not target.is_relative_to(fixture_root.resolve()):
            raise ValueError(f"Invalid trusted fixture path: {fixture_path}")
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(content)
        source_paths[fixture_path] = target
    return source_paths


def _candidate_fixture_paths(root: Path, fixture_paths: list[str], sources: dict[str, str]) -> dict[str, Path]:
    source_paths = {}
    for fixture_path in fixture_paths:
        target = safe_path(root, fixture_path)
        if _sha256(target.read_bytes()) != sources[fixture_path]:
            raise ValueError(f"Candidate fixture differs from the selection plan: {fixture_path}")
        source_paths[fixture_path] = target
    return source_paths


def _fixture_nodes(root: Path, base: str, lane: dict, planned: dict, junit_output: Path) -> list[str]:
    sources = {item["path"]: item["sha256"] for item in planned.get("fixture_sources", [])}
    fixture_paths = lane["fixture_paths"]
    if set(sources) != set(fixture_paths):
        raise ValueError("Architecture plan omitted exact fixture sources")
    if planned.get("source") == "base":
        source_paths = _base_fixture_paths(root, base, fixture_paths, sources, junit_output.parent / "trusted-fixtures")
    elif planned.get("source") == "candidate-bootstrap":
        source_paths = _candidate_fixture_paths(root, fixture_paths, sources)
    else:
        raise ValueError("Architecture plan has no trusted fixture source")

    nodes = []
    for nodeid in lane["required_nodeids"]:
        fixture_path, *test_parts = nodeid.split("::")
        nodes.append("::".join([str(source_paths[fixture_path]), *test_parts]))
    return nodes


def run_policy_fixtures(root: Path, policy_path: Path, policy: dict, python: Path, junit_output: Path, plan: dict) -> dict:
    lane = next((item for item in policy["lanes"] if item["id"] == "policy-fixtures"), None)
    planned = next((item for item in plan.get("lanes", []) if item.get("id") == "policy-fixtures"), None)
    if lane is None or lane.get("contract") != "policy_fixtures":
        raise ValueError("Architecture policy has no attested policy-fixtures lane")
    if planned is None or not planned.get("selected"):
        raise ValueError("Architecture plan did not select policy fixtures")
    if not python.is_file():
        raise ValueError(f"Policy fixture interpreter is unavailable: {python}")

    expected = plan.get("input", {})
    base_config = json.loads(safe_path(root, "tools/quality/upstream-base.json").read_text(encoding="utf-8"))
    mapping = yaml.safe_load(safe_path(root, "tools/quality/module-map.yaml").read_text(encoding="utf-8"))
    before = capture(root, base_config, mapping)
    identity = (expected.get("head"), expected.get("upstream_base"), expected.get("snapshot_sha256"))
    if (before["head"], before["upstream_base"], before["snapshot_sha256"]) != identity:
        raise ValueError("Policy fixture plan is stale")

    fixture_nodes = _fixture_nodes(root, expected.get("base", ""), lane, planned, junit_output)
    command = [
        str(python),
        "-B",
        "-m",
        "pytest",
        "--noconftest",
        "-p",
        "pytest_asyncio.plugin",
        "--rootdir",
        str(root),
        "-q",
        f"--junitxml={junit_output}",
        *fixture_nodes,
    ]
    environment = _isolated_subprocess_environment(
        PYTEST_DISABLE_PLUGIN_AUTOLOAD="1",
        ARCHITECTURE_CANDIDATE_ROOT=str(root),
    )
    completed = subprocess.run(command, cwd=root, env=environment, check=False)
    after = capture(root, base_config, mapping)

    content = junit_output.read_bytes() if junit_output.is_file() else b""
    junit_status, reason = _junit_contract(lane, content) if content else ("INCOMPLETE", "pytest did not create JUnit evidence")
    case_names = []
    if content:
        try:
            case_names = sorted(case.get("name") for case in ET.fromstring(content).iter("testcase") if case.get("name"))
        except ET.ParseError:
            pass
    if (before["head"], before["snapshot_sha256"]) != (after["head"], after["snapshot_sha256"]):
        status, reason = "INCOMPLETE", "candidate changed while policy fixtures were running"
    elif completed.returncode == 0 and junit_status == "PASS":
        status = "PASS"
    elif completed.returncode == 1 and junit_status == "FAIL":
        status = "FAIL"
    else:
        status = "INCOMPLETE"
        reason = f"pytest exit {completed.returncode}; {reason}"
    exit_code = 0 if status == "PASS" else 1 if status == "FAIL" else 2
    return {
        "schema_version": 1,
        "tool": {"name": "check_architecture_policy", "version": VERSION},
        "mode": MODE,
        "fixture_status": status,
        "input": {
            "head": before["head"],
            "upstream_base": before["upstream_base"],
            "snapshot_sha256": before["snapshot_sha256"],
            "tool_sha256": _sha256(Path(__file__).resolve().read_bytes()),
            "policy_sha256": _sha256(policy_path.read_bytes()),
            "selection_sha256": plan["selection_sha256"],
        },
        "fixture_source": planned["source"],
        "fixture_sources": planned["fixture_sources"],
        "required_cases": lane["required_cases"],
        "required_nodeids": lane["required_nodeids"],
        "executed_nodeids": lane["required_nodeids"],
        "case_names": case_names,
        "case_count": len(case_names),
        "pytest_exit_code": completed.returncode,
        "junit_sha256": _sha256(content),
        "junit_base64": base64.b64encode(content).decode("ascii"),
        "reason": reason,
        "exit_code": exit_code,
    }


def validate_replanned(plan: dict, replanned: dict) -> None:
    if _canonical(plan) != _canonical(replanned):
        raise ValueError("Architecture selection plan differs from a fresh selector run")


def _removed_values(base_lane: dict, candidate_lane: dict, keys: tuple[str, ...]) -> list[str]:
    problems = []
    for key in keys:
        missing = sorted(set(base_lane.get(key, [])) - set(candidate_lane.get(key, [])))
        if missing:
            problems.append(f"lane {base_lane['id']} removed {key}: {', '.join(missing)}")
    return problems


def _lane_coverage_problems(base_lane: dict, candidate_lane: dict | None) -> list[str]:
    lane_id = base_lane["id"]
    if candidate_lane is None:
        return [f"lane {lane_id} was removed"]
    problems = [f"lane {lane_id} changed {key}" for key in ("contract", "tool") if candidate_lane.get(key) != base_lane.get(key)]
    if base_lane.get("always") is True and candidate_lane.get("always") is not True:
        problems.append(f"lane {lane_id} is no longer always selected")
    problems.extend(_removed_values(base_lane, candidate_lane, ("rules", "required_cases", "required_nodeids", "fixture_paths", "protected_sources")))
    for field, source_path in base_lane.get("reported_hashes", {}).items():
        if candidate_lane.get("reported_hashes", {}).get(field) != source_path:
            problems.append(f"lane {lane_id} changed reported hash {field}")
    base_selectors = {"id": lane_id, **base_lane.get("selectors", {})}
    candidate_selectors = candidate_lane.get("selectors", {})
    for problem in _removed_values(base_selectors, candidate_selectors, ("paths", "prefixes", "suffixes")):
        problems.append(problem.replace(" removed ", " narrowed selector ", 1))
    return problems


def _plan_coverage_problems(base_plan: dict, candidate_plan: dict) -> list[str]:
    problems = []
    base_input = base_plan.get("input", {})
    candidate_input = candidate_plan.get("input", {})
    identity_keys = ("base", "head", "upstream_base", "snapshot_sha256", "changed_paths_sha256")
    if any(base_input.get(key) != candidate_input.get(key) for key in identity_keys):
        problems.append("base and candidate policy plans use different candidate identity")
    if _canonical(base_plan.get("changes")) != _canonical(candidate_plan.get("changes")):
        problems.append("base and candidate policy plans classify different changed paths")
    candidate_plan_lanes = {lane["id"]: lane for lane in candidate_plan["lanes"]}
    omitted = [lane["id"] for lane in base_plan.get("lanes", []) if lane.get("selected") is True and candidate_plan_lanes.get(lane["id"], {}).get("selected") is not True]
    problems.extend(f"candidate plan omitted base-selected lane {lane_id}" for lane_id in omitted)
    return problems


def _compatibility_expectation(plan: dict) -> tuple[str, str, str, str]:
    trusted_base = plan.get("trusted_base")
    if not isinstance(trusted_base, dict) or type(trusted_base.get("bootstrap_required")) is not bool:
        raise ValueError("Authoritative plan omitted the trusted-base bootstrap decision")
    if trusted_base["bootstrap_required"]:
        return (
            "candidate-bootstrap",
            "BOOTSTRAP_SELF_CHECK",
            "OBSERVED",
            "candidate-bootstrap self-consistency only; trusted base coverage was not evaluated",
        )
    return (
        "base",
        "COMPATIBLE",
        "PASS",
        "candidate policy preserves trusted base coverage",
    )


def compare_policy_coverage(base_policy: dict, candidate_policy: dict, base_plan: dict, candidate_plan: dict) -> dict:
    authority_source, compatibility_status, _, reason = _compatibility_expectation(base_plan)
    problems = []
    if candidate_policy.get("job_name") != base_policy.get("job_name"):
        problems.append("stable job name changed")
    if candidate_policy.get("tool_path") != base_policy.get("tool_path"):
        problems.append("trusted checker path changed")
    candidate_lanes = {lane["id"]: lane for lane in candidate_policy["lanes"]}
    for base_lane in base_policy["lanes"]:
        problems.extend(_lane_coverage_problems(base_lane, candidate_lanes.get(base_lane["id"])))
    base_input = base_plan.get("input", {})
    candidate_input = candidate_plan.get("input", {})
    problems.extend(_plan_coverage_problems(base_plan, candidate_plan))
    if problems:
        coverage = "trusted coverage" if authority_source == "base" else "bootstrap policy coverage"
        raise ValueError(f"Candidate architecture policy narrows {coverage}: " + "; ".join(problems))
    if authority_source == "base":
        limits = [
            "Compatibility proves that the candidate policy does not narrow the current base policy.",
            "It is not branch protection and does not approve newly added candidate-only rules.",
        ]
    else:
        limits = [
            "Bootstrap comparison checks candidate policy and plan self-consistency only.",
            "Trusted base coverage was not evaluated; this is not branch protection or compatibility proof.",
        ]
    return {
        "schema_version": 1,
        "tool": {"name": "check_architecture_policy", "version": VERSION},
        "mode": MODE,
        "authority_source": authority_source,
        "compatibility_status": compatibility_status,
        "reason": reason,
        "enforcement_status": "NOT_ENABLED",
        "input": {
            "base": base_input.get("base"),
            "head": base_input.get("head"),
            "upstream_base": base_input.get("upstream_base"),
            "snapshot_sha256": base_input.get("snapshot_sha256"),
            "base_policy_sha256": base_input.get("policy_sha256"),
            "candidate_policy_sha256": candidate_input.get("policy_sha256"),
            "base_selection_sha256": base_plan.get("selection_sha256"),
            "candidate_selection_sha256": candidate_plan.get("selection_sha256"),
        },
        "checked_lanes": [lane["id"] for lane in base_policy["lanes"]],
        "limits": limits,
        "exit_code": 0,
    }


def _compatibility_contract(policy: dict, plan: dict, candidate_plan: dict | None, report: dict) -> tuple[str, str]:
    if not isinstance(report, dict) or report.get("schema_version") != 1 or report.get("mode") != MODE:
        return "INCOMPLETE", "invalid policy compatibility report schema"
    tool = report.get("tool")
    if not isinstance(tool, dict) or tool.get("name") != "check_architecture_policy":
        return "INCOMPLETE", "policy compatibility report used an unexpected tool"
    report_input = report.get("input")
    plan_input = plan.get("input")
    candidate_input = candidate_plan.get("input") if isinstance(candidate_plan, dict) else None
    if not isinstance(report_input, dict) or not isinstance(plan_input, dict) or not isinstance(candidate_input, dict):
        return "INCOMPLETE", "policy compatibility report omitted input identity"
    expected = {
        "base": plan_input.get("base"),
        "head": plan_input.get("head"),
        "upstream_base": plan_input.get("upstream_base"),
        "snapshot_sha256": plan_input.get("snapshot_sha256"),
        "base_policy_sha256": plan_input.get("policy_sha256"),
        "candidate_policy_sha256": plan_input.get("candidate_policy_sha256"),
        "base_selection_sha256": plan.get("selection_sha256"),
        "candidate_selection_sha256": candidate_plan.get("selection_sha256"),
    }
    if any(report_input.get(key) != value for key, value in expected.items()):
        return "INCOMPLETE", "policy compatibility report identity or trusted digest mismatch"
    identity_keys = ("base", "head", "upstream_base", "snapshot_sha256")
    if any(candidate_input.get(key) != plan_input.get(key) for key in identity_keys):
        return "INCOMPLETE", "candidate policy plan used a different candidate identity"
    if candidate_input.get("policy_sha256") != plan_input.get("candidate_policy_sha256"):
        return "INCOMPLETE", "candidate policy plan digest differs from the authoritative plan"
    if report.get("checked_lanes") != [lane["id"] for lane in policy["lanes"]]:
        return "INCOMPLETE", "policy compatibility report omitted a trusted lane"
    try:
        authority_source, expected_status, aggregate_status, reason = _compatibility_expectation(plan)
    except ValueError as error:
        return "INCOMPLETE", str(error)
    valid_outcome = (
        report.get("authority_source") == authority_source
        and report.get("compatibility_status") == expected_status
        and report.get("reason") == reason
        and report.get("enforcement_status") == "NOT_ENABLED"
        and report.get("exit_code") == 0
    )
    if not valid_outcome:
        return "INCOMPLETE", "policy compatibility outcome differs from the authoritative base mode"
    return aggregate_status, reason


def _aggregate_compatibility(policy: dict, candidate_policy: dict | None, plan: dict, candidate_plan: dict | None, report: dict) -> tuple[str, str]:
    if not isinstance(candidate_policy, dict) or not isinstance(candidate_plan, dict):
        return "INCOMPLETE", "aggregate omitted the candidate policy or its exact plan"
    try:
        compare_policy_coverage(policy, candidate_policy, plan, candidate_plan)
    except ValueError as error:
        return "INCOMPLETE", str(error)
    return _compatibility_contract(policy, plan, candidate_plan, report)


def _aggregate_context(policy: dict, plan: dict, evidence: dict[str, bytes], current: dict) -> tuple[dict, dict[str, dict], list[dict], set[str]]:
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
    valid_lanes = isinstance(planned_lanes, list) and len(planned_lanes) == len(policy_lanes) and all(isinstance(lane, dict) for lane in planned_lanes)
    if not valid_lanes or {lane.get("id") for lane in planned_lanes} != set(policy_lanes):
        raise ValueError("Architecture plan lanes differ from policy")
    selected = {lane["id"] for lane in planned_lanes if lane.get("selected") is True}
    expected_evidence = {lane_id for lane_id in selected if policy_lanes[lane_id]["contract"] != "inventory"}
    unknown = set(evidence) - expected_evidence
    if unknown:
        raise ValueError("Unexpected architecture evidence: " + ", ".join(sorted(unknown)))
    return expected, policy_lanes, planned_lanes, selected


def _evidence_contract(lane: dict, planned: dict, content: bytes, expected: dict) -> tuple[str, str]:
    if lane["contract"] == "junit":
        return _junit_contract(lane, content)
    try:
        report = json.loads(content)
    except json.JSONDecodeError as error:
        return "INCOMPLETE", f"invalid JSON report: {error}"
    return _json_contract(lane, planned, report, expected)


def _lane_result(lane: dict, planned: dict, selected: set[str], evidence: dict[str, bytes], expected: dict) -> dict:
    lane_id = planned["id"]
    if lane_id not in selected:
        return {"id": lane_id, "status": "NOT_APPLICABLE", "reason": planned.get("reason")}
    if lane["contract"] == "inventory":
        return {"id": lane_id, "status": "OBSERVED", "reason": "fresh T0 capture completed with classified paths"}
    if planned.get("source_integrity") != "PASS":
        return {
            "id": lane_id,
            "status": "INCOMPLETE",
            "reason": "protected analyzer, configuration or fixture source differs from the trusted base",
        }
    content = evidence.get(lane_id)
    if content is None:
        return {"id": lane_id, "status": "INCOMPLETE", "reason": "required evidence is missing"}
    status, reason = _evidence_contract(lane, planned, content, expected)
    if status not in TERMINAL_STATUSES:
        raise ValueError(f"Internal invalid aggregate status: {status}")
    return {"id": lane_id, "status": status, "reason": reason, "sha256": _sha256(content)}


def _aggregate_outcome(statuses: set[str]) -> tuple[str, int]:
    if "INCOMPLETE" in statuses:
        return "INCOMPLETE", 2
    if "FAIL" in statuses:
        return "FAIL", 1
    return "REPORT_ONLY_COMPLETE", 0


def aggregate(
    policy: dict,
    plan: dict,
    evidence: dict[str, bytes],
    current: dict,
    compatibility: dict | None = None,
    candidate_plan: dict | None = None,
    candidate_policy: dict | None = None,
) -> dict:
    expected, policy_lanes, planned_lanes, selected = _aggregate_context(policy, plan, evidence, current)

    results = [_lane_result(policy_lanes[planned["id"]], planned, selected, evidence, expected) for planned in planned_lanes]
    compatibility_status, compatibility_reason = _aggregate_compatibility(policy, candidate_policy, plan, candidate_plan, compatibility)
    statuses = {item["status"] for item in results} | {compatibility_status}
    aggregate_status, exit_code = _aggregate_outcome(statuses)
    return {
        "schema_version": 1,
        "tool": {"name": "check_architecture_policy", "version": VERSION},
        "mode": MODE,
        "aggregate_status": aggregate_status,
        "enforcement_status": "NOT_ENABLED",
        "input": expected,
        "compatibility": {"status": compatibility_status, "reason": compatibility_reason},
        "results": results,
        "manual_review_required": plan.get("manual_review_required", []),
        "limits": [
            "This aggregate is report-only and is not branch protection or a full architecture/dead-code/build verdict.",
            "CLASSIFIED and PLANNED are not PASS; native Go execution and manual review remain separate evidence.",
            "A required check becomes effective only after repository branch protection is configured and verified.",
        ],
        "exit_code": exit_code,
    }


def _write_github_outputs(path: Path, plan: dict) -> None:
    with path.open("a", encoding="utf-8", newline="\n") as stream:
        stream.write(f"selection_sha256={plan['selection_sha256']}\n")
        for lane in plan["lanes"]:
            key = lane["id"].replace("-", "_")
            runnable = lane["selected"] and (lane["contract"] == "inventory" or lane.get("source_integrity") == "PASS")
            stream.write(f"{key}_selected={'true' if lane['selected'] else 'false'}\n")
            stream.write(f"{key}={'true' if runnable else 'false'}\n")


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


def _write_json(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def _validate_plan_context(root: Path, policy_path: Path, policy_repository_path: str, policy: dict, plan: dict) -> dict:
    plan_input = plan.get("input") if isinstance(plan, dict) else None
    if not isinstance(plan_input, dict):
        raise ValueError("Architecture selection plan omitted input identity")
    if plan_input.get("policy_repository_path") != policy_repository_path:
        raise ValueError("Architecture policy repository path changed after selection")
    if plan_input.get("policy_sha256") != _sha256(policy_path.read_bytes()):
        raise ValueError("Architecture policy changed after selection")
    candidate_policy = safe_path(root, policy_repository_path).read_bytes()
    if plan_input.get("candidate_policy_sha256") != _sha256(candidate_policy):
        raise ValueError("Candidate architecture policy changed after selection")
    candidate_tool = safe_path(root, policy["tool_path"]).read_bytes()
    if plan_input.get("candidate_tool_sha256") != _sha256(candidate_tool):
        raise ValueError("Architecture checker changed after selection")
    evaluator_tool = Path(__file__).resolve().read_bytes()
    if plan_input.get("evaluator_tool_sha256") != _sha256(evaluator_tool):
        raise ValueError("Architecture evaluator changed after selection")
    validate_replanned(plan, create_plan(root, policy_path, plan_input.get("base", ""), policy_repository_path))
    return plan_input


def _materialized_source_content(root: Path, base: str, record: dict) -> bytes:
    source_path = _repository_path(record.get("path"), "Protected source path")
    source = record.get("source")
    if source == "base":
        content = _base_blob(root, base, source_path)
        if content is None:
            raise ValueError(f"Comparison base omitted protected source: {source_path}")
    elif source == "candidate-bootstrap":
        content = safe_path(root, source_path).read_bytes()
    else:
        raise ValueError(f"Protected source {source_path} has an invalid authority: {source!r}")
    if not re.fullmatch(r"[0-9a-f]{64}", str(record.get("sha256"))) or _sha256(content) != record["sha256"]:
        raise ValueError(f"Protected source digest mismatch: {source_path}")
    return content


def materialize_protected_sources(root: Path, plan: dict, output_directory: Path) -> dict:
    output_directory = output_directory.resolve()
    if output_directory.exists():
        raise ValueError(f"Refusing stale protected source bundle: {output_directory}")
    plan_input = plan.get("input") if isinstance(plan, dict) else None
    lanes = plan.get("lanes") if isinstance(plan, dict) else None
    if not isinstance(plan_input, dict) or not isinstance(lanes, list):
        raise ValueError("Architecture selection plan omitted protected source metadata")
    base = plan_input.get("base")
    if not isinstance(base, str) or not base:
        raise ValueError("Architecture selection plan omitted its comparison base")

    sources: dict[str, tuple[dict, bytes]] = {}
    for lane in lanes:
        if not isinstance(lane, dict) or not lane.get("selected") or lane.get("contract") == "inventory":
            continue
        lane_id = lane.get("id")
        if lane.get("source_integrity") != "PASS":
            raise ValueError(f"Selected lane {lane_id} has no intact protected source bundle")
        bundle = lane.get("source_bundle")
        if not isinstance(bundle, list) or not bundle:
            raise ValueError(f"Selected lane {lane_id} omitted its protected source bundle")
        for record in bundle:
            if not isinstance(record, dict) or record.get("protected") is not True or record.get("source") != lane.get("source"):
                raise ValueError(f"Selected lane {lane_id} has an invalid protected source record")
            content = _materialized_source_content(root, base, record)
            source_path = record["path"]
            existing = sources.get(source_path)
            if existing and (existing[0]["source"], existing[0]["sha256"]) != (record["source"], record["sha256"]):
                raise ValueError(f"Protected source authority conflicts across lanes: {source_path}")
            sources[source_path] = (record, content)

    output_directory.mkdir(parents=True)
    manifest_sources = []
    for source_path, (record, content) in sorted(sources.items()):
        target = (output_directory / PurePosixPath(source_path)).resolve()
        if not target.is_relative_to(output_directory):
            raise ValueError(f"Protected source escaped the materialized bundle: {source_path}")
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(content)
        manifest_sources.append({"path": source_path, "source": record["source"], "sha256": record["sha256"]})
    manifest = {
        "schema_version": 1,
        "tool": {"name": "check_architecture_policy", "version": VERSION},
        "mode": MODE,
        "input": plan_input,
        "source_count": len(manifest_sources),
        "sources": manifest_sources,
    }
    _write_json(output_directory / "materialized-sources.json", manifest)
    return manifest


def _select_action(args: argparse.Namespace, root: Path, policy_path: Path, policy_repository_path: str, _policy: dict) -> int:
    output = _safe_new_output(root, args.output)
    github_output = _safe_github_output(root, args.github_output) if args.github_output else None
    plan = create_plan(root, policy_path, args.base, policy_repository_path)
    _write_json(output, plan)
    if github_output:
        _write_github_outputs(github_output, plan)
    print(json.dumps({"planning_status": plan["planning_status"], "required_reports": plan["required_reports"], "output": str(output)}))
    return 0


def _materialize_action(args: argparse.Namespace, root: Path, policy_path: Path, policy_repository_path: str, policy: dict) -> int:
    plan_path = _resolve(root, args.plan)
    output_directory = _safe_new_output(root, args.output_directory)
    if not plan_path.is_file() or plan_path.parent != output_directory.parent:
        raise ValueError("Protected source plan and bundle must share one fresh evidence directory")
    plan_bytes = plan_path.read_bytes()
    plan = json.loads(plan_bytes)
    expected = _validate_plan_context(root, policy_path, policy_repository_path, policy, plan)
    result = materialize_protected_sources(root, plan, output_directory)
    base_config = json.loads(safe_path(root, "tools/quality/upstream-base.json").read_text(encoding="utf-8"))
    mapping = yaml.safe_load(safe_path(root, "tools/quality/module-map.yaml").read_text(encoding="utf-8"))
    after = capture(root, base_config, mapping)
    identity = (expected.get("head"), expected.get("upstream_base"), expected.get("snapshot_sha256"))
    if (after["head"], after["upstream_base"], after["snapshot_sha256"]) != identity:
        raise ValueError("Candidate changed while protected sources were materialized")
    if plan_path.read_bytes() != plan_bytes:
        raise ValueError("Architecture plan changed while protected sources were materialized")
    print(json.dumps({"source_count": result["source_count"], "output_directory": str(output_directory)}))
    return 0


def _fixtures_action(args: argparse.Namespace, root: Path, policy_path: Path, policy_repository_path: str, policy: dict) -> int:
    output = _safe_new_output(root, args.output)
    junit_output = _safe_new_output(root, args.junit_output)
    plan_path = _resolve(root, args.plan)
    if not plan_path.is_file() or plan_path.parent != output.parent or junit_output.parent != output.parent:
        raise ValueError("Policy fixture plan and outputs must share one fresh evidence directory")
    plan_bytes = plan_path.read_bytes()
    plan = json.loads(plan_bytes)
    _validate_plan_context(root, policy_path, policy_repository_path, policy, plan)
    python = _fixture_interpreter_path(root, args.python)
    result = run_policy_fixtures(root, policy_path, policy, python, junit_output, plan)
    if plan_path.read_bytes() != plan_bytes:
        raise ValueError("Architecture fixture plan changed during execution")
    _write_json(output, result)
    print(json.dumps({"fixture_status": result["fixture_status"], "exit_code": result["exit_code"], "output": str(output)}))
    return result["exit_code"]


def _compare_action(args: argparse.Namespace, root: Path, policy_path: Path, policy_repository_path: str, policy: dict) -> int:
    output = _safe_new_output(root, args.output)
    base_plan_path = _resolve(root, args.base_plan)
    candidate_plan_path = _resolve(root, args.candidate_plan)
    if not base_plan_path.is_file() or not candidate_plan_path.is_file():
        raise ValueError("Architecture comparison requires two selection plans")
    if base_plan_path.parent != output.parent or candidate_plan_path.parent != output.parent:
        raise ValueError("Architecture comparison plans and output must share one fresh evidence directory")
    base_plan_bytes = base_plan_path.read_bytes()
    candidate_plan_bytes = candidate_plan_path.read_bytes()
    base_plan = json.loads(base_plan_bytes)
    candidate_plan = json.loads(candidate_plan_bytes)
    candidate_policy_path = _resolve(root, args.candidate_policy)
    candidate_policy_bytes = candidate_policy_path.read_bytes()
    candidate_policy = load_policy(candidate_policy_path)
    base_ref = base_plan.get("input", {}).get("base", "")
    validate_replanned(base_plan, create_plan(root, policy_path, base_ref, policy_repository_path))
    validate_replanned(candidate_plan, create_plan(root, candidate_policy_path, base_ref, policy_repository_path))
    result = compare_policy_coverage(policy, candidate_policy, base_plan, candidate_plan)
    if base_plan_path.read_bytes() != base_plan_bytes or candidate_plan_path.read_bytes() != candidate_plan_bytes or candidate_policy_path.read_bytes() != candidate_policy_bytes:
        raise ValueError("Architecture policy comparison inputs changed during evaluation")
    _write_json(output, result)
    print(json.dumps({"compatibility_status": result["compatibility_status"], "exit_code": result["exit_code"], "output": str(output)}))
    return result["exit_code"]


def _aggregate_action(args: argparse.Namespace, root: Path, policy_path: Path, policy_repository_path: str, policy: dict) -> int:
    plan_path = _resolve(root, args.plan)
    output = _safe_new_output(root, args.output)
    plan_bytes = plan_path.read_bytes()
    plan = json.loads(plan_bytes)
    plan_input = _validate_plan_context(root, policy_path, policy_repository_path, policy, plan)
    candidate_plan_path = _resolve(root, args.candidate_plan)
    if not candidate_plan_path.is_file() or candidate_plan_path.parent != plan_path.parent:
        raise ValueError("Candidate policy plan must be a regular file beside the authoritative plan")
    candidate_plan_bytes = candidate_plan_path.read_bytes()
    candidate_plan = json.loads(candidate_plan_bytes)
    candidate_policy_path = safe_path(root, policy_repository_path)
    candidate_policy_bytes = candidate_policy_path.read_bytes()
    candidate_policy = load_policy(candidate_policy_path)
    validate_replanned(candidate_plan, create_plan(root, candidate_policy_path, plan_input.get("base", ""), policy_repository_path))
    compatibility_path = _resolve(root, args.compatibility)
    if not compatibility_path.is_file() or compatibility_path.parent != plan_path.parent:
        raise ValueError("Policy compatibility evidence must be a regular file beside the plan")
    compatibility_bytes = compatibility_path.read_bytes()
    compatibility = json.loads(compatibility_bytes)
    evidence, snapshots = _parse_evidence(root, plan_path, args.report)
    base_config = json.loads(safe_path(root, "tools/quality/upstream-base.json").read_text(encoding="utf-8"))
    mapping = yaml.safe_load(safe_path(root, "tools/quality/module-map.yaml").read_text(encoding="utf-8"))
    before = capture(root, base_config, mapping)
    result = aggregate(policy, plan, evidence, before, compatibility, candidate_plan, candidate_policy)
    after = capture(root, base_config, mapping)
    if (before["head"], before["snapshot_sha256"]) != (after["head"], after["snapshot_sha256"]):
        raise ValueError("Candidate changed during architecture aggregation")
    stable_inputs = (
        plan_path.read_bytes() == plan_bytes
        and candidate_plan_path.read_bytes() == candidate_plan_bytes
        and candidate_policy_path.read_bytes() == candidate_policy_bytes
        and compatibility_path.read_bytes() == compatibility_bytes
    )
    if not stable_inputs or any(path.read_bytes() != content for path, content in snapshots.items()):
        raise ValueError("Architecture evidence changed during aggregation")
    _write_json(output, result)
    print(json.dumps({"aggregate_status": result["aggregate_status"], "exit_code": result["exit_code"], "output": str(output)}))
    return result["exit_code"]


def _parse_arguments(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--policy", type=Path, default=Path("tools/quality/architecture-policy.json"))
    parser.add_argument("--policy-repository-path", default=POLICY_REPOSITORY_PATH)
    subparsers = parser.add_subparsers(dest="action", required=True)
    select_parser = subparsers.add_parser("select")
    select_parser.add_argument("--base", required=True)
    select_parser.add_argument("--output", type=Path, required=True)
    select_parser.add_argument("--github-output", type=Path)
    materialize_parser = subparsers.add_parser("materialize")
    materialize_parser.add_argument("--plan", type=Path, required=True)
    materialize_parser.add_argument("--output-directory", type=Path, required=True)
    fixtures_parser = subparsers.add_parser("fixtures")
    fixtures_parser.add_argument("--plan", type=Path, required=True)
    fixtures_parser.add_argument("--python", type=Path, required=True)
    fixtures_parser.add_argument("--junit-output", type=Path, required=True)
    fixtures_parser.add_argument("--output", type=Path, required=True)
    compare_parser = subparsers.add_parser("compare")
    compare_parser.add_argument("--base-plan", type=Path, required=True)
    compare_parser.add_argument("--candidate-plan", type=Path, required=True)
    compare_parser.add_argument("--candidate-policy", type=Path, default=Path(POLICY_REPOSITORY_PATH))
    compare_parser.add_argument("--output", type=Path, required=True)
    aggregate_parser = subparsers.add_parser("aggregate")
    aggregate_parser.add_argument("--plan", type=Path, required=True)
    aggregate_parser.add_argument("--candidate-plan", type=Path, required=True)
    aggregate_parser.add_argument("--compatibility", type=Path, required=True)
    aggregate_parser.add_argument("--report", action="append", default=[])
    aggregate_parser.add_argument("--output", type=Path, required=True)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = _parse_arguments(argv)
    root = args.root.resolve()
    try:
        policy_repository_path = _repository_path(args.policy_repository_path, "Policy repository path")
        policy_path = _resolve(root, args.policy)
        policy = load_policy(policy_path)
        actions = {
            "select": _select_action,
            "materialize": _materialize_action,
            "fixtures": _fixtures_action,
            "compare": _compare_action,
            "aggregate": _aggregate_action,
        }
        return actions[args.action](args, root, policy_path, policy_repository_path, policy)
    except (KeyError, TypeError, ValueError, OSError, UnicodeError, json.JSONDecodeError, subprocess.CalledProcessError) as error:
        print(f"INCOMPLETE: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
