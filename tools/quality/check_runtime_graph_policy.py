"""Classify TypeScript and Go runtime-graph signals without enforcing policy."""

from __future__ import annotations

import argparse
import hashlib
import json
import platform
import re
import shutil
import subprocess
import sys
from pathlib import Path

import yaml
from capture_inventory import capture, git, paths, safe_path
from run_isolated_python import sanitized_child_environment

VERSION = "0.4.0"
OWNED_ORIGINS = {"core_change", "extension"}
UNREACHABLE_CLASSIFICATIONS = {"static_candidate", "type_only_contract"}
OBSERVER_SOURCE_HASHES = {
    "TypeScript": {
        "tool_sha256": "typescript_tool_sha256",
        "policy_sha256": "typescript_policy_sha256",
        "worker_sha256": "typescript_worker_sha256",
    },
    "Go": {
        "tool_sha256": "go_tool_sha256",
        "policy_sha256": "go_policy_sha256",
    },
}


_TYPESCRIPT_EDGE_SCANNER = r"""
const fs = require("fs");
const ts = require(process.argv[1]);
const sources = JSON.parse(fs.readFileSync(0, "utf8"));

function importGroups(clause) {
  if (!clause) return [false];
  if (clause.isTypeOnly) return [true];
  let runtime = Boolean(clause.name);
  let types = false;
  if (clause.namedBindings) {
    if (ts.isNamespaceImport(clause.namedBindings)) runtime = true;
    else {
      for (const element of clause.namedBindings.elements) {
        if (element.isTypeOnly) types = true;
        else runtime = true;
      }
    }
  }
  const groups = [];
  if (runtime || !types) groups.push(false);
  if (types) groups.push(true);
  return groups;
}

function exportGroups(node) {
  if (!node.exportClause || !ts.isNamedExports(node.exportClause)) {
    return [Boolean(node.isTypeOnly)];
  }
  if (node.isTypeOnly) return [true];
  let runtime = false;
  let types = false;
  for (const element of node.exportClause.elements) {
    if (element.isTypeOnly) types = true;
    else runtime = true;
  }
  const groups = [];
  if (runtime) groups.push(false);
  if (types) groups.push(true);
  return groups;
}

function isFunctionBoundary(node) {
  return ts.isFunctionDeclaration(node) ||
    ts.isFunctionExpression(node) ||
    ts.isArrowFunction(node) ||
    ts.isMethodDeclaration(node) ||
    ts.isGetAccessorDeclaration(node) ||
    ts.isSetAccessorDeclaration(node) ||
    ts.isConstructorDeclaration(node);
}

function inspect(path, text) {
  const kind = path.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS;
  const source = ts.createSourceFile(path, text, ts.ScriptTarget.Latest, true, kind);
  const edges = [];

  function edge(specifier, kindName, typeOnly, state) {
    edges.push({specifier, kind: kindName, phase: state.phase, type_only: typeOnly});
  }

  function visit(node, state) {
    if (ts.isImportDeclaration(node) && ts.isStringLiteralLike(node.moduleSpecifier)) {
      for (const typeOnly of importGroups(node.importClause)) {
        edge(node.moduleSpecifier.text, "import", typeOnly, state);
      }
    } else if (ts.isExportDeclaration(node)) {
      if (node.moduleSpecifier && ts.isStringLiteralLike(node.moduleSpecifier)) {
        for (const typeOnly of exportGroups(node)) {
          edge(node.moduleSpecifier.text, "re_export", typeOnly, state);
        }
      }
    } else if (ts.isImportEqualsDeclaration(node) && ts.isExternalModuleReference(node.moduleReference)) {
      const expression = node.moduleReference.expression;
      if (expression && ts.isStringLiteralLike(expression)) {
        edge(expression.text, "import_equals", false, state);
      }
    } else if (ts.isImportTypeNode(node) && ts.isLiteralTypeNode(node.argument) && ts.isStringLiteralLike(node.argument.literal)) {
      edge(node.argument.literal.text, "import_type", true, state);
    } else if (ts.isCallExpression(node)) {
      if (node.expression.kind === ts.SyntaxKind.ImportKeyword) {
        const target = node.arguments[0];
        if (target && ts.isStringLiteralLike(target)) edge(target.text, "dynamic_import", false, state);
      } else if (ts.isIdentifier(node.expression) && node.expression.text === "require") {
        const target = node.arguments[0];
        if (target && ts.isStringLiteralLike(target)) edge(target.text, "require", false, state);
      }
    }

    const childState = {phase: isFunctionBoundary(node) ? "call" : state.phase};
    ts.forEachChild(node, (child) => visit(child, childState));
  }

  visit(source, {phase: "module"});
  return {
    edges,
    parse_errors: source.parseDiagnostics.map((diagnostic) =>
      ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n")
    ),
  };
}

const result = {};
for (const [path, text] of Object.entries(sources)) result[path] = inspect(path, text);
process.stdout.write(JSON.stringify(result));
"""


def _sha256(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def _observer_source_hashes(typescript: dict, go_report: dict) -> dict[str, str]:
    flattened = {}
    for name, report in (("TypeScript", typescript), ("Go", go_report)):
        report_input = report.get("input") if isinstance(report, dict) else None
        if not isinstance(report_input, dict):
            raise ValueError(f"{name} report omitted its input identity")
        for source_key, output_key in OBSERVER_SOURCE_HASHES[name].items():
            value = report_input.get(source_key)
            if not isinstance(value, str) or re.fullmatch(r"[0-9a-f]{64}", value) is None:
                raise ValueError(f"{name} report input {source_key} is not a valid SHA-256")
            flattened[output_key] = value
    return flattened


def _runtime_report_input(
    snapshot: dict,
    inputs: dict[str, bytes],
    observer_source_hashes: dict[str, str],
    selection_sha256: str | None = None,
) -> dict:
    return {
        "head": snapshot["head"],
        "upstream_base": snapshot["upstream_base"],
        "snapshot_sha256": snapshot["snapshot_sha256"],
        "selection_sha256": selection_sha256,
        "policy_sha256": _sha256(inputs["policy"]),
        "typescript_report_sha256": _sha256(inputs["typescript_report"]),
        "go_report_sha256": _sha256(inputs["go_report"]),
        "tool_sha256": _sha256(Path(__file__).read_bytes()),
        **observer_source_hashes,
    }


def _canonical_cycle(cycle: dict) -> dict:
    edge_fields = ("source", "target", "specifier", "kind", "phase", "type_only")
    edges = [{field: edge.get(field) for field in edge_fields} for edge in cycle.get("edges", [])]
    return {
        "members": sorted(cycle.get("members", [])),
        "edges": sorted(
            edges,
            key=lambda edge: tuple(str(edge.get(field, "")) for field in edge_fields),
        ),
    }


def _cycle_signature(cycle: dict) -> str:
    encoded = json.dumps(_canonical_cycle(cycle), ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    return _sha256(encoded)


def _git_blob(root: Path, revision: str, path: str) -> bytes | None:
    try:
        return git(root, "show", f"{revision}:{path}")
    except subprocess.CalledProcessError:
        return None


def _typescript_parser_path(root: Path) -> Path:
    return safe_path(root, "web/node_modules/typescript/lib/typescript.js")


def _scan_typescript_edge_semantics(root: Path, sources: dict[str, str]) -> tuple[dict[str, list[dict]], dict[str, str]]:
    if not sources:
        return {}, {}
    node = shutil.which("node")
    typescript = _typescript_parser_path(root)
    if node is None:
        detail = "Node.js is required to compare upstream TypeScript edge semantics"
        return {}, {path: detail for path in sources}
    if not typescript.is_file():
        detail = "Locked TypeScript parser is required to compare upstream edge semantics"
        return {}, {path: detail for path in sources}
    try:
        completed = subprocess.run(
            [node, "-e", _TYPESCRIPT_EDGE_SCANNER, str(typescript)],
            input=json.dumps(sources, ensure_ascii=False),
            env=sanitized_child_environment(),
            capture_output=True,
            text=True,
            encoding="utf-8",
            timeout=30,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as error:
        detail = f"TypeScript upstream edge parser could not complete: {error}"
        return {}, {path: detail for path in sources}
    if completed.returncode:
        detail = f"TypeScript upstream edge parser failed: {completed.stderr[-2000:]}"
        return {}, {path: detail for path in sources}
    try:
        raw = json.loads(completed.stdout)
    except json.JSONDecodeError as error:
        detail = f"TypeScript upstream edge parser returned invalid JSON: {error}"
        return {}, {path: detail for path in sources}

    edges = {}
    problems = {}
    for path in sources:
        record = raw.get(path) if isinstance(raw, dict) else None
        if not isinstance(record, dict) or not isinstance(record.get("edges"), list):
            problems[path] = "TypeScript upstream edge parser omitted the source"
            continue
        parse_errors = record.get("parse_errors")
        if not isinstance(parse_errors, list):
            problems[path] = "TypeScript upstream edge parser returned invalid diagnostics"
            continue
        if parse_errors:
            problems[path] = f"upstream TypeScript parse errors: {'; '.join(str(item) for item in parse_errors)}"
            continue
        edges[path] = record["edges"]
    return edges, problems


def _load_upstream_cycle_edges(
    root: Path,
    revision: str,
    cycles: list[dict],
    blob_cache: dict[str, bytes | None],
) -> tuple[dict[str, list[dict]], dict[str, str]]:
    sources = {}
    problems = {}
    source_paths = sorted({edge.get("source") for cycle in cycles for edge in cycle.get("edges", []) if isinstance(edge.get("source"), str)})
    for path in source_paths:
        if path not in blob_cache:
            blob_cache[path] = _git_blob(root, revision, path)
        blob = blob_cache[path]
        if blob is None:
            problems[path] = f"source absent in upstream: {path}"
            continue
        try:
            sources[path] = blob.decode("utf-8")
        except UnicodeDecodeError:
            problems[path] = f"upstream source is not UTF-8: {path}"
    edges, scan_problems = _scan_typescript_edge_semantics(root, sources)
    problems.update(scan_problems)
    return edges, problems


def _cycle_existed_upstream(
    root: Path,
    revision: str,
    cycle: dict,
    blob_cache: dict[str, bytes | None] | None = None,
    edge_cache: dict[str, list[dict]] | None = None,
    edge_problems: dict[str, str] | None = None,
) -> tuple[bool, list[str]]:
    cache = blob_cache if blob_cache is not None else {}
    if edge_cache is None or edge_problems is None:
        edge_cache, edge_problems = _load_upstream_cycle_edges(root, revision, [cycle], cache)

    def upstream_blob(path: str) -> bytes | None:
        if path not in cache:
            cache[path] = _git_blob(root, revision, path)
        return cache[path]

    problems = []
    for edge in _canonical_cycle(cycle)["edges"]:
        source_blob = upstream_blob(edge["source"])
        target_blob = upstream_blob(edge["target"])
        if source_blob is None:
            problems.append(f"source absent in upstream: {edge['source']}")
            continue
        if target_blob is None:
            problems.append(f"target absent in upstream: {edge['target']}")
        source_problem = edge_problems.get(edge["source"])
        if source_problem:
            problems.append(source_problem)
            continue
        semantic_fields = ("specifier", "kind", "phase", "type_only")
        if not any(all(candidate.get(field) == edge.get(field) for field in semantic_fields) for candidate in edge_cache.get(edge["source"], [])):
            semantics = ", ".join(f"{field}={edge.get(field)!r}" for field in semantic_fields)
            problems.append(f"edge absent in upstream with matching semantics: {edge['source']} -> {edge['specifier']} -> {edge['target']} [{semantics}]")
    return not problems, problems


def _validate_evidence(root: Path, items: list[dict], findings: list[dict]) -> list[dict]:
    observed = []
    for item in items:
        item_id = item.get("id")
        role = item.get("role")
        evidence = item.get("evidence", [])
        if not isinstance(item_id, str) or not item_id or not isinstance(role, str):
            findings.append({"kind": "invalid_root_classification", "detail": str(item_id)})
            continue
        if not isinstance(evidence, list) or not evidence:
            findings.append({"kind": "missing_root_evidence", "detail": item_id})
            continue
        evidence_results = []
        for record in evidence:
            path = record.get("path")
            contains = record.get("contains")
            target = record.get("target")
            if not isinstance(path, str) or not isinstance(contains, str):
                findings.append({"kind": "invalid_root_evidence", "detail": item_id})
                continue
            source_path = safe_path(root, path)
            matched = source_path.is_file() and contains in source_path.read_text(encoding="utf-8")
            target_exists = True
            if target is not None:
                target_exists = isinstance(target, str) and safe_path(root, target).is_file()
            evidence_results.append(
                {
                    "path": path,
                    "contains": contains,
                    "matched": matched,
                    "target": target,
                    "target_exists": target_exists,
                }
            )
            if not matched or not target_exists:
                findings.append({"kind": "root_evidence_mismatch", "detail": item_id})
        observed.append({"id": item_id, "role": role, "evidence": evidence_results})
    return observed


def _exact_matches(item: dict, expected: dict, fields: tuple[str, ...]) -> bool:
    return all(item.get(field) == expected.get(field) for field in fields)


def _classify_typescript(root: Path, policy: dict, report: dict, upstream_base: str) -> tuple[dict, list[dict]]:
    findings = []
    expected_profile = policy.get("profile_id")
    if report.get("profile", {}).get("id") != expected_profile:
        findings.append(
            {
                "kind": "typescript_profile_mismatch",
                "detail": report.get("profile", {}).get("id"),
            }
        )

    configured_entrypoints = [item.get("path") for item in policy.get("entrypoints", [])]
    observed_entrypoints = report.get("scope", {}).get("entrypoints", [])
    if configured_entrypoints != observed_entrypoints:
        findings.append(
            {
                "kind": "typescript_entrypoint_mismatch",
                "expected": configured_entrypoints,
                "observed": observed_entrypoints,
            }
        )
    roots = _validate_evidence(
        root,
        policy.get("entrypoints", []) + policy.get("auxiliary_roots", []),
        findings,
    )

    nodes = {node["path"]: node for node in report.get("nodes", [])}
    reverse = {item["path"]: item for item in report.get("reverse_imports", [])}
    configured_unreachable = {item.get("path"): item for item in policy.get("unreachable_owned_runtime", [])}
    observed_unreachable = set(report.get("unreachable_owned_runtime_paths", []))
    stale = sorted(set(configured_unreachable) - observed_unreachable)
    missing = sorted(observed_unreachable - set(configured_unreachable))
    for path in stale:
        findings.append({"kind": "stale_unreachable_classification", "detail": path})
    for path in missing:
        findings.append({"kind": "unclassified_unreachable_owned_runtime", "detail": path})

    unreachable = []
    for path in sorted(observed_unreachable & set(configured_unreachable)):
        configured = configured_unreachable[path]
        classification = configured.get("classification")
        reason = configured.get("reason")
        next_action = configured.get("next_action")
        if classification not in UNREACHABLE_CLASSIFICATIONS:
            findings.append({"kind": "invalid_unreachable_classification", "detail": path})
            continue
        if not isinstance(reason, str) or not reason or not isinstance(next_action, str) or not next_action:
            findings.append({"kind": "incomplete_unreachable_evidence", "detail": path})
            continue
        reverse_item = reverse.get(
            path,
            {"consumer_paths": [], "type_only_consumer_paths": [], "incoming_edges": []},
        )
        if classification == "type_only_contract":
            if reverse_item.get("consumer_paths") or not reverse_item.get("type_only_consumer_paths"):
                findings.append({"kind": "invalid_type_only_classification", "detail": path})
        unreachable.append(
            {
                "path": path,
                "origin": nodes.get(path, {}).get("origin"),
                "owner": nodes.get(path, {}).get("owner"),
                "classification": classification,
                "reason": reason,
                "next_action": next_action,
                "runtime_consumers": reverse_item.get("consumer_paths", []),
                "type_only_consumers": reverse_item.get("type_only_consumer_paths", []),
            }
        )

    no_consumer = []
    for item in report.get("reverse_imports", []):
        if item.get("consumer_paths") or item.get("type_only_consumer_paths"):
            continue
        node = nodes.get(item["path"], {})
        if node.get("origin") not in OWNED_ORIGINS:
            continue
        if node.get("is_test"):
            classification = "test_entrypoint"
        elif node.get("is_declaration"):
            classification = "type_declaration"
        elif item["path"] in configured_entrypoints:
            classification = "production_entrypoint"
        elif item["path"] in configured_unreachable:
            classification = configured_unreachable[item["path"]].get("classification")
        else:
            classification = "unclassified"
            findings.append({"kind": "unclassified_owned_without_consumers", "detail": item["path"]})
        no_consumer.append({"path": item["path"], "classification": classification})

    source_cycles = report.get("cycles", [])
    cycles = []
    upstream_blob_cache: dict[str, bytes | None] = {}
    upstream_edge_cache, upstream_edge_problems = _load_upstream_cycle_edges(
        root,
        upstream_base,
        source_cycles,
        upstream_blob_cache,
    )
    for cycle in source_cycles:
        inherited, evidence_problems = _cycle_existed_upstream(
            root,
            upstream_base,
            cycle,
            upstream_blob_cache,
            upstream_edge_cache,
            upstream_edge_problems,
        )
        if inherited:
            classification = "inherited_upstream_topology_with_local_members" if cycle.get("owned_members") else "upstream_only_topology"
        else:
            classification = "unclassified_local_cycle_candidate"
            findings.append(
                {
                    "kind": "unclassified_local_cycle_candidate",
                    "signature": _cycle_signature(cycle),
                    "problems": evidence_problems,
                }
            )
        cycles.append(
            {
                "signature": _cycle_signature(cycle),
                "members": cycle.get("members", []),
                "owned_members": cycle.get("owned_members", []),
                "classification": classification,
                "upstream_edge_evidence_complete": inherited,
            }
        )

    registration_fields = ("source", "line", "kind", "computed", "patterns")
    expected_registrations = policy.get("dynamic_registrations", [])
    unmatched_registrations = list(report.get("dynamic_registrations", []))
    registrations = []
    for expected in expected_registrations:
        matches = [item for item in unmatched_registrations if _exact_matches(item, expected, registration_fields)]
        if len(matches) != 1:
            findings.append(
                {
                    "kind": "dynamic_registration_mismatch",
                    "detail": expected.get("id"),
                    "matches": len(matches),
                }
            )
            continue
        unmatched_registrations.remove(matches[0])
        registrations.append(
            {
                **{field: matches[0].get(field) for field in registration_fields},
                "id": expected.get("id"),
                "classification": expected.get("classification"),
                "evidence": expected.get("evidence"),
            }
        )
    for item in unmatched_registrations:
        findings.append({"kind": "unclassified_dynamic_registration", "detail": item})

    incomplete_fields = ("source", "line", "kind", "detail", "type_only")
    observed_incomplete = report.get("runtime_incomplete_reasons", []) + report.get("other_incomplete_reasons", [])
    unmatched_incomplete = list(observed_incomplete)
    classified_incomplete = []
    for expected in policy.get("classified_incomplete_reasons", []):
        matches = [item for item in unmatched_incomplete if _exact_matches(item, expected, incomplete_fields)]
        if len(matches) != 1:
            findings.append(
                {
                    "kind": "incomplete_reason_mismatch",
                    "detail": expected.get("id"),
                    "matches": len(matches),
                }
            )
            continue
        match = matches[0]
        unmatched_incomplete.remove(match)
        source_origin = nodes.get(match.get("source"), {}).get("origin")
        require_upstream = expected.get("require_unchanged_upstream", False)
        if require_upstream and source_origin != "upstream":
            findings.append(
                {
                    "kind": "classified_gap_is_no_longer_upstream",
                    "detail": expected.get("id"),
                }
            )
        classified_incomplete.append(
            {
                **{field: match.get(field) for field in incomplete_fields},
                "id": expected.get("id"),
                "classification": expected.get("classification"),
                "source_origin": source_origin,
                "policy_effect": expected.get("policy_effect"),
            }
        )
    for item in unmatched_incomplete:
        findings.append({"kind": "unclassified_incomplete_reason", "detail": item})

    return (
        {
            "source_analysis_status": report.get("analysis_status"),
            "entrypoints_and_auxiliary_roots": roots,
            "cycles": cycles,
            "unreachable_owned_runtime": unreachable,
            "owned_without_direct_consumers": no_consumer,
            "dynamic_registrations": registrations,
            "classified_incomplete_reasons": classified_incomplete,
        },
        findings,
    )


def _classify_go(root: Path, policy: dict, report: dict) -> tuple[dict, list[dict]]:
    findings = []
    configured_profiles = {profile.get("id"): profile.get("entrypoints", []) for profile in policy.get("profiles", [])}
    observed_profiles = {profile.get("id"): [entry.get("path") for entry in profile.get("entrypoints", [])] for profile in report.get("profiles", [])}
    if configured_profiles != observed_profiles:
        findings.append(
            {
                "kind": "go_profile_entrypoint_mismatch",
                "expected": configured_profiles,
                "observed": observed_profiles,
            }
        )
    roots = _validate_evidence(root, policy.get("roots", []), findings)

    production_paths = {path for entrypoints in configured_profiles.values() for path in entrypoints}
    auxiliary_paths = {item.get("path") for item in policy.get("auxiliary_entrypoints", [])}
    observed_main = {item["path"] for item in report.get("files", []) if item.get("package") == "main" and not item.get("is_test")}
    expected_main = production_paths | auxiliary_paths
    if observed_main != expected_main:
        findings.append(
            {
                "kind": "go_main_entrypoint_inventory_mismatch",
                "expected": sorted(expected_main),
                "observed": sorted(observed_main),
            }
        )

    main_entrypoints = []
    for path in sorted(observed_main):
        role = "production" if path in production_paths else "development_tool"
        main_entrypoints.append({"path": path, "role": role})

    if report.get("incomplete_reasons"):
        findings.append(
            {
                "kind": "go_analysis_incomplete",
                "detail": report.get("incomplete_reasons"),
            }
        )
    for profile in report.get("profiles", []):
        if profile.get("cycles"):
            findings.append(
                {
                    "kind": "go_package_cycle_candidate",
                    "profile": profile.get("id"),
                    "detail": profile.get("cycles"),
                }
            )
    if report.get("unreachable_owned_packages_in_primary_profile"):
        findings.append(
            {
                "kind": "unclassified_go_unreachable_owned_package",
                "detail": report.get("unreachable_owned_packages_in_primary_profile"),
            }
        )

    expected_directives = policy.get("directives", [])
    unmatched_directives = list(report.get("directives", []))
    directives = []
    for expected in expected_directives:
        source = expected.get("source")
        kind = expected.get("kind")
        arguments = expected.get("arguments", [])
        matches = [item for item in unmatched_directives if item.get("source") == source and item.get("kind") == kind and item.get("arguments") in arguments]
        if sorted(item.get("arguments") for item in matches) != sorted(arguments):
            findings.append(
                {
                    "kind": "go_directive_mismatch",
                    "detail": expected.get("id"),
                }
            )
            continue
        for match in matches:
            unmatched_directives.remove(match)
            asset_exists = None
            if expected.get("classification") == "runtime_embedded_asset":
                asset = safe_path(root, str(Path(source).parent / match["arguments"]))
                asset_exists = asset.is_file()
                if not asset_exists:
                    findings.append(
                        {
                            "kind": "missing_go_embed_asset",
                            "detail": f"{source}: {match['arguments']}",
                        }
                    )
            directives.append(
                {
                    **match,
                    "id": expected.get("id"),
                    "classification": expected.get("classification"),
                    "asset_exists": asset_exists,
                }
            )
    for item in unmatched_directives:
        findings.append({"kind": "unclassified_go_directive", "detail": item})

    return (
        {
            "source_analysis_status": report.get("analysis_status"),
            "profile_entrypoints": observed_profiles,
            "root_evidence": roots,
            "main_entrypoints": main_entrypoints,
            "package_cycle_count": sum(len(profile.get("cycles", [])) for profile in report.get("profiles", [])),
            "unreachable_owned_packages": report.get("unreachable_owned_packages_in_primary_profile", []),
            "directives": directives,
        },
        findings,
    )


def classify(root: Path, policy: dict, typescript: dict, go_report: dict) -> dict:
    upstream_base = typescript.get("input", {}).get("upstream_base")
    if not isinstance(upstream_base, str) or upstream_base != go_report.get("input", {}).get("upstream_base"):
        raise ValueError("Observer reports do not share one upstream base")
    ts_result, ts_findings = _classify_typescript(root, policy["typescript"], typescript, upstream_base)
    go_result, go_findings = _classify_go(root, policy["go"], go_report)
    findings = ts_findings + go_findings
    manual = {
        "inherited_typescript_cycles": sum(item["classification"].startswith(("inherited_", "upstream_")) for item in ts_result["cycles"]),
        "typescript_static_candidates": sum(item["classification"] == "static_candidate" for item in ts_result["unreachable_owned_runtime"]),
        "classified_source_gaps": len(ts_result["classified_incomplete_reasons"]),
        "automatic_deletions": 0,
    }
    return {
        "classification_status": "COMPLETE" if not findings else "INCOMPLETE",
        "policy_status": "REPORT_ONLY",
        "architecture_status": "NOT_EVALUATED",
        "dead_code_status": "REVIEW_REQUIRED",
        "typescript": ts_result,
        "go": go_result,
        "future_policy_scopes": policy.get("future_policy_scopes", []),
        "manual_review_required": manual,
        "findings": findings,
        "limits": [
            "Classification is report-only and is not an ARC, DEAD, BUILD or CI pass.",
            "Inherited topology is proven only against the accepted upstream commit and must be reviewed when touched.",
            "Static candidates are never deletion verdicts; dynamic consumers, saved data and external contracts remain required evidence.",
            "Auxiliary roots describe known non-production loaders but are not executed by this checker.",
            "No baseline or ignore is generated and no source file is changed.",
        ],
    }


def _load_policy(path: Path) -> dict:
    raw = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not isinstance(raw, dict) or raw.get("schema_version") != 1:
        raise ValueError("Unsupported runtime graph policy schema")
    if not isinstance(raw.get("typescript"), dict) or not isinstance(raw.get("go"), dict):
        raise ValueError("Runtime graph policy requires TypeScript and Go sections")

    scopes = raw.get("future_policy_scopes")
    if not isinstance(scopes, list) or not scopes:
        raise ValueError("Runtime graph policy requires future policy scopes")
    scope_ids = []
    for scope in scopes:
        if not isinstance(scope, dict) or scope.get("mode") != "report_only" or not scope.get("rules") or not scope.get("selector") or not isinstance(scope.get("id"), str) or not scope["id"]:
            raise ValueError("Invalid future policy scope")
        scope_ids.append(scope["id"])
    if len(scope_ids) != len(set(scope_ids)):
        raise ValueError("Duplicate future policy scope")

    def unique_records(items: object, key: str, label: str) -> list[dict]:
        if not isinstance(items, list):
            raise ValueError(f"{label} must be a list")
        values = []
        for item in items:
            if not isinstance(item, dict) or not isinstance(item.get(key), str) or not item[key]:
                raise ValueError(f"Invalid {label} record")
            values.append(item[key])
        if len(values) != len(set(values)):
            raise ValueError(f"Duplicate {label} record")
        return items

    typescript = raw["typescript"]
    if not isinstance(typescript.get("profile_id"), str):
        raise ValueError("TypeScript classification requires a profile ID")
    unique_records(typescript.get("entrypoints"), "path", "TypeScript entrypoint")
    unique_records(typescript.get("auxiliary_roots"), "id", "TypeScript auxiliary root")
    unreachable = unique_records(
        typescript.get("unreachable_owned_runtime"),
        "path",
        "TypeScript unreachable classification",
    )
    for item in unreachable:
        if item.get("classification") not in UNREACHABLE_CLASSIFICATIONS or not item.get("reason") or not item.get("next_action"):
            raise ValueError("Invalid TypeScript unreachable classification")
    registrations = unique_records(
        typescript.get("dynamic_registrations"),
        "id",
        "TypeScript dynamic registration",
    )
    for item in registrations:
        if not item.get("classification") or not item.get("evidence"):
            raise ValueError("Invalid TypeScript dynamic registration classification")
    gaps = unique_records(
        typescript.get("classified_incomplete_reasons"),
        "id",
        "TypeScript incomplete reason",
    )
    for item in gaps:
        if not item.get("classification") or not item.get("policy_effect"):
            raise ValueError("Invalid TypeScript incomplete-reason classification")

    go_policy = raw["go"]
    profiles = unique_records(go_policy.get("profiles"), "id", "Go profile")
    for profile in profiles:
        entrypoints = profile.get("entrypoints")
        if not isinstance(entrypoints, list) or not entrypoints or any(not isinstance(item, str) or not item for item in entrypoints):
            raise ValueError("Invalid Go profile entrypoints")
    unique_records(go_policy.get("roots"), "id", "Go root")
    auxiliary = unique_records(go_policy.get("auxiliary_entrypoints"), "path", "Go auxiliary entrypoint")
    if any(not item.get("role") or not item.get("reason") for item in auxiliary):
        raise ValueError("Invalid Go auxiliary entrypoint classification")
    directives = unique_records(go_policy.get("directives"), "id", "Go directive")
    for item in directives:
        if not item.get("source") or not item.get("kind") or not isinstance(item.get("arguments"), list) or not item["arguments"] or not item.get("classification"):
            raise ValueError("Invalid Go directive classification")

    return raw


def _resolve_input(root: Path, value: Path) -> Path:
    return value.resolve() if value.is_absolute() else safe_path(root, value.as_posix()).resolve()


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--policy", type=Path, default=Path("tools/quality/runtime-graph-policy.yaml"))
    parser.add_argument(
        "--typescript-report",
        type=Path,
        default=Path("output/quality/typescript-analysis.json"),
    )
    parser.add_argument(
        "--go-report",
        type=Path,
        default=Path("output/quality/go-analysis.json"),
    )
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--selection-sha256", help="Optional T3 selection digest binding this report to an architecture plan")
    args = parser.parse_args(argv)
    root = args.root.resolve()
    output = _resolve_input(root, args.output)
    try:
        if output.is_relative_to(root):
            git(
                root,
                "check-ignore",
                "--no-index",
                "-q",
                output.relative_to(root).as_posix(),
            )
        repository_files = paths(git(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z"))
        if output.is_relative_to(root) and output.relative_to(root).as_posix() in repository_files:
            raise ValueError("Report must not overwrite source files, including tracked files under ignore rules")
        policy_path = _resolve_input(root, args.policy)
        typescript_path = _resolve_input(root, args.typescript_report)
        go_path = _resolve_input(root, args.go_report)
        inputs = {
            "policy": policy_path.read_bytes(),
            "typescript_report": typescript_path.read_bytes(),
            "go_report": go_path.read_bytes(),
        }
        policy = _load_policy(policy_path)
        typescript = json.loads(inputs["typescript_report"])
        go_report = json.loads(inputs["go_report"])
        observer_source_hashes = _observer_source_hashes(typescript, go_report)
        base_config = json.loads(safe_path(root, "tools/quality/upstream-base.json").read_text(encoding="utf-8"))
        mapping = yaml.safe_load(safe_path(root, "tools/quality/module-map.yaml").read_text(encoding="utf-8"))
        before = capture(root, base_config, mapping)
        expected_snapshot = before["snapshot_sha256"]
        for name, report in (("TypeScript", typescript), ("Go", go_report)):
            if report.get("input", {}).get("snapshot_sha256") != expected_snapshot:
                raise ValueError(f"{name} report is stale for the current T0 snapshot")
            if report.get("input", {}).get("head") != before["head"]:
                raise ValueError(f"{name} report uses another HEAD")
            if report.get("input", {}).get("upstream_base") != before["upstream_base"]:
                raise ValueError(f"{name} report uses another upstream base")
        result = classify(root, policy, typescript, go_report)
        for name, content in inputs.items():
            path = {
                "policy": policy_path,
                "typescript_report": typescript_path,
                "go_report": go_path,
            }[name]
            if path.read_bytes() != content:
                raise ValueError(f"Input changed during classification: {path}")
        after = capture(root, base_config, mapping)
        if before["snapshot_sha256"] != after["snapshot_sha256"] or before["head"] != after["head"]:
            raise ValueError("Provenance snapshot changed during classification")
        result.update(
            {
                "schema_version": 1,
                "tool": {
                    "name": "check_runtime_graph_policy",
                    "version": VERSION,
                    "python": platform.python_version(),
                    "os": platform.system(),
                },
                "input": _runtime_report_input(before, inputs, observer_source_hashes, args.selection_sha256),
                "command": sys.argv if argv is None else argv,
            }
        )
        code = 0 if result["classification_status"] == "COMPLETE" else 2
        result["exit_code"] = code
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(
            json.dumps(
                {
                    "classification_status": result["classification_status"],
                    "policy_status": result["policy_status"],
                    "typescript_cycles": len(result["typescript"]["cycles"]),
                    "typescript_static_candidates": result["manual_review_required"]["typescript_static_candidates"],
                    "go_package_cycles": result["go"]["package_cycle_count"],
                    "findings": len(result["findings"]),
                    "output": str(output),
                }
            )
        )
        return code
    except (
        KeyError,
        TypeError,
        ValueError,
        OSError,
        UnicodeError,
        json.JSONDecodeError,
        subprocess.CalledProcessError,
    ) as error:
        print(f"INCOMPLETE: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
