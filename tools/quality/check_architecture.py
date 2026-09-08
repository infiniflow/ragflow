"""Evaluate configured Python ARC-01/02/03 checks in T2 report-only mode."""

from __future__ import annotations

import argparse
import hashlib
from io import BytesIO
import json
import os
from pathlib import Path, PurePosixPath
import platform
import subprocess
import sys
import tempfile
import tokenize
import xml.etree.ElementTree as ET

import yaml

from capture_inventory import capture, git, paths, safe_path
from inspect_python import MARKERS, analyze, collect_import_graph


VERSION = "0.6.0"
REPORT_ONLY = "T2_REPORT_ONLY"
MODULE_ACCESS = "<module>"
STATIC_IMPORT_KINDS = {"import", "literal_dynamic_import"}
RUNTIME_WORKER = Path(__file__).with_name("probe_python_runtime.py")
MANUAL_RULES = [
    "ARC-02 computed reverse loading outside configured integration sources is not evaluated",
    "ARC-02 adapter behavior outside configured contract tests remains manual",
    "ARC-03 exports and owned cycles outside configured cycle checks are not evaluated",
    "DEAD-01..03 reachability and safe deletion are not evaluated",
    "SIMP-01..03 require scenario review and before/after evidence",
    "UPG/POL/BUILD/DATA/TEST rules remain separate checks",
]


def _normalized_prefix(value: object) -> str:
    if not isinstance(value, str) or not value or "\\" in value:
        raise ValueError("Boundary path_prefixes must be non-empty repository-relative POSIX paths")
    path = PurePosixPath(value)
    if path.is_absolute() or ".." in path.parts:
        raise ValueError(f"Unsafe boundary path prefix: {value}")
    normalized = path.as_posix().rstrip("/") + "/"
    if normalized.startswith("./") or normalized == "./":
        raise ValueError(f"Unsafe boundary path prefix: {value}")
    return normalized


def _normalized_file(value: object, label: str) -> str:
    if not isinstance(value, str) or not value or "\\" in value:
        raise ValueError(f"{label} must be a repository-relative POSIX file")
    path = PurePosixPath(value)
    if path.is_absolute() or ".." in path.parts or path.as_posix().startswith("./"):
        raise ValueError(f"Unsafe {label}: {value}")
    return path.as_posix()


def _normalized_directory(value: object, label: str, *, allow_root: bool = False) -> str:
    if not isinstance(value, str) or not value or "\\" in value:
        raise ValueError(f"{label} must be a repository-relative POSIX directory")
    path = PurePosixPath(value)
    normalized = path.as_posix().rstrip("/")
    if path.is_absolute() or ".." in path.parts or normalized.startswith("./"):
        raise ValueError(f"Unsafe {label}: {value}")
    if normalized in {"", "."}:
        if allow_root:
            return "."
        raise ValueError(f"{label} must not select the repository root")
    return normalized


def _finding_fingerprint(boundary: dict, coupling: dict) -> str:
    semantic_chain = [{"source": edge["source"], "target": edge["target"], "kind": edge["kind"]} for edge in coupling["chain"]]
    semantic = {
        "rule_id": boundary["rule_id"],
        "boundary": boundary["id"],
        "source": coupling["source"],
        "target": coupling["target"],
        "category": coupling["category"],
        "chain": semantic_chain,
    }
    return hashlib.sha256(json.dumps(semantic, sort_keys=True).encode()).hexdigest()


def evaluate_boundary(
    sources: dict[str, str],
    selected: set[str],
    ownership: dict[str, dict],
    boundary: dict,
    import_roots: set[str] | None = None,
) -> dict:
    forbidden = boundary.get("forbidden_markers")
    if not isinstance(forbidden, list) or not forbidden or any(marker not in MARKERS for marker in forbidden):
        raise ValueError(f"Boundary {boundary.get('id')} has unknown or empty forbidden_markers")
    observation = analyze(sources, selected, ownership, import_roots or frozenset())
    findings = []
    for coupling in observation["coupling_paths"]:
        if coupling["category"] not in forbidden:
            continue
        first_edge = coupling["chain"][0] if coupling["chain"] else None
        findings.append(
            {
                "rule_id": boundary["rule_id"],
                "boundary": boundary["id"],
                "source": coupling["source"],
                "path": coupling["path"],
                "line": first_edge["line"] if first_edge else 1,
                "category": coupling["category"],
                "target": coupling["target"],
                "chain": coupling["chain"],
                "fingerprint": _finding_fingerprint(boundary, coupling),
                "message": f"Clean boundary reaches forbidden {coupling['category']} dependency {coupling['target']}",
            }
        )
    findings.sort(key=lambda item: (item["path"], item["line"], item["category"], item["target"]))
    incomplete = observation["incomplete_reasons"]
    status = "INCOMPLETE" if incomplete else "FAIL" if findings else "PASS"
    return {
        "id": boundary["id"],
        "owner_module": boundary["owner_module"],
        "rule_id": boundary["rule_id"],
        "status": status,
        "rationale": boundary.get("rationale"),
        "scope": observation["scope"],
        "findings": findings,
        "incomplete_reasons": incomplete,
        "complexity_signals": [item for item in observation["functions"] if item["signals"]],
        "limits": observation["limits"],
    }


def _module_prefix_matches(module: str, prefixes: list[str]) -> bool:
    return any(module == prefix or module.startswith(prefix + ".") for prefix in prefixes)


def _flatten_allowed(connection: dict) -> list[dict]:
    flattened = []
    for direction, key in (("forward", "allowed_forward_imports"), ("reverse", "allowed_reverse_imports")):
        for entry in connection[key]:
            for symbol in entry["symbols"]:
                flattened.append({"direction": direction, "source": entry["source"], "target": entry["target"], "symbol": symbol})
    return flattened


def _connection_fingerprint(connection: dict, finding: dict) -> str:
    semantic = {
        "rule_id": connection["rule_id"],
        "connection": connection["id"],
        "kind": finding["kind"],
        "direction": finding["direction"],
        "source": finding["source"],
        "target": finding["target"],
        "symbol": finding["symbol"],
    }
    return hashlib.sha256(json.dumps(semantic, sort_keys=True).encode()).hexdigest()


def evaluate_connection(sources: dict[str, str], connection: dict, import_roots: set[str] | None = None) -> dict:
    graph = collect_import_graph(sources, import_roots=import_roots or frozenset())
    extension_paths = sorted(path for path in sources if any(path.startswith(prefix) for prefix in connection["extension_path_prefixes"]))
    if not extension_paths:
        raise ValueError(f"Connection {connection['id']} selected no extension Python files")

    observed_by_key = {}
    for edge in graph["edges"]:
        if edge["kind"] not in STATIC_IMPORT_KINDS:
            continue
        source_inside = any(edge["path"].startswith(prefix) for prefix in connection["extension_path_prefixes"])
        target_inside = _module_prefix_matches(edge["target"], connection["extension_module_prefixes"])
        if source_inside and not target_inside and edge["resolution"] in {"local", "namespace"}:
            direction = "forward"
        elif not source_inside and target_inside:
            direction = "reverse"
        else:
            continue
        for symbol in edge["symbols"] or [MODULE_ACCESS]:
            key = (direction, edge["source"], edge["target"], symbol)
            candidate = {
                "direction": direction,
                "source": edge["source"],
                "target": edge["target"],
                "symbol": symbol,
                "path": edge["path"],
                "line": edge["line"],
                "phase": edge["phase"],
                "conditional": edge["conditional"],
                "type_only": edge["type_only"],
            }
            if key not in observed_by_key or candidate["line"] < observed_by_key[key]["line"]:
                observed_by_key[key] = candidate

    allowed_entries = _flatten_allowed(connection)
    allowed = {(item["direction"], item["source"], item["target"], item["symbol"]) for item in allowed_entries}
    observed = set(observed_by_key)
    findings = []
    for key in sorted(observed - allowed):
        item = observed_by_key[key]
        finding = {
            "rule_id": connection["rule_id"],
            "connection": connection["id"],
            "kind": "unapproved_import",
            **item,
            "message": f"Unapproved {item['direction']} import {item['source']} -> {item['target']}:{item['symbol']}",
        }
        finding["fingerprint"] = _connection_fingerprint(connection, finding)
        findings.append(finding)
    for direction, source, target, symbol in sorted(allowed - observed):
        finding = {
            "rule_id": connection["rule_id"],
            "connection": connection["id"],
            "kind": "stale_approval",
            "direction": direction,
            "source": source,
            "target": target,
            "symbol": symbol,
            "path": graph["index"].get(source),
            "line": 0,
            "message": f"Approved {direction} import is not present: {source} -> {target}:{symbol}",
        }
        finding["fingerprint"] = _connection_fingerprint(connection, finding)
        findings.append(finding)

    watched_reverse_sources = {item["source"] for item in allowed_entries if item["direction"] == "reverse"} | {item["source"] for item in observed_by_key.values() if item["direction"] == "reverse"}
    incomplete = [
        issue
        for issue in graph["issues"]
        if not issue["type_only"] and (any(issue["path"].startswith(prefix) for prefix in connection["extension_path_prefixes"]) or issue["source"] in watched_reverse_sources)
    ]
    status = "INCOMPLETE" if incomplete else "FAIL" if findings else "PASS"
    selected_paths = set(extension_paths)
    selected_paths.update(graph["index"][source] for source in watched_reverse_sources if source in graph["index"])
    observed_imports = []
    for key in sorted(observed):
        item = {**observed_by_key[key], "approved": key in allowed}
        observed_imports.append(item)
    return {
        "id": connection["id"],
        "owner_module": connection["owner_module"],
        "rule_id": connection["rule_id"],
        "status": status,
        "rationale": connection.get("rationale"),
        "scope": {
            "selected_paths": sorted(selected_paths),
            "indexed_files": len(graph["index"]),
            "extension_files": len(extension_paths),
            "observed_imports": len(observed_imports),
        },
        "observed_imports": observed_imports,
        "approved_imports": allowed_entries,
        "findings": sorted(findings, key=lambda item: (item["kind"], item["direction"], item["source"], item["target"], item["symbol"])),
        "incomplete_reasons": incomplete,
        "limits": [
            "Static Python imports only; computed reverse loaders outside configured sources remain manual.",
            "An approved import records a boundary edge, not proof that the adapter contract is behaviorally correct.",
            "Runtime registrations, persisted DSL, frontend and Go are not evaluated by this connection.",
        ],
    }


def _strongly_connected_components(adjacency: dict[str, set[str]]) -> list[list[str]]:
    index = 0
    indices = {}
    lowlinks = {}
    stack = []
    on_stack = set()
    components = []

    def visit(node: str) -> None:
        nonlocal index
        indices[node] = index
        lowlinks[node] = index
        index += 1
        stack.append(node)
        on_stack.add(node)
        for target in sorted(adjacency[node]):
            if target not in indices:
                visit(target)
                lowlinks[node] = min(lowlinks[node], lowlinks[target])
            elif target in on_stack:
                lowlinks[node] = min(lowlinks[node], indices[target])
        if lowlinks[node] != indices[node]:
            return
        component = []
        while True:
            member = stack.pop()
            on_stack.remove(member)
            component.append(member)
            if member == node:
                break
        components.append(sorted(component))

    for node in sorted(adjacency):
        if node not in indices:
            visit(node)
    return sorted(components)


def evaluate_cycle_check(
    sources: dict[str, str],
    selected: set[str],
    cycle_check: dict,
    import_roots: set[str] | None = None,
) -> dict:
    graph = collect_import_graph(sources, import_roots=import_roots or frozenset())
    selected_modules = {module for module, path in graph["index"].items() if path in selected}
    adjacency = {module: set() for module in selected_modules}
    relevant_edges = []
    for edge in graph["edges"]:
        if edge["kind"] not in STATIC_IMPORT_KINDS or edge["type_only"]:
            continue
        if edge["source"] in selected_modules and edge["target"] in selected_modules:
            adjacency[edge["source"]].add(edge["target"])
            relevant_edges.append(edge)
    cyclic_components = [component for component in _strongly_connected_components(adjacency) if len(component) > 1 or component[0] in adjacency[component[0]]]
    findings = []
    for component in cyclic_components:
        members = set(component)
        edges = sorted(
            (
                {
                    "source": edge["source"],
                    "target": edge["target"],
                    "path": edge["path"],
                    "line": edge["line"],
                    "kind": edge["kind"],
                    "phase": edge["phase"],
                }
                for edge in relevant_edges
                if edge["source"] in members and edge["target"] in members
            ),
            key=lambda edge: (edge["source"], edge["target"], edge["path"], edge["line"]),
        )
        semantic = {"rule_id": cycle_check["rule_id"], "cycle_check": cycle_check["id"], "modules": component}
        findings.append(
            {
                "rule_id": cycle_check["rule_id"],
                "cycle_check": cycle_check["id"],
                "kind": "owned_import_cycle",
                "modules": component,
                "edges": edges,
                "path": edges[0]["path"],
                "line": edges[0]["line"],
                "message": "Owned modules form an explicit runtime import cycle: " + " -> ".join(component),
                "fingerprint": hashlib.sha256(json.dumps(semantic, sort_keys=True).encode()).hexdigest(),
            }
        )
    incomplete = [issue for issue in graph["issues"] if issue["source"] in selected_modules and not issue["type_only"]]
    status = "INCOMPLETE" if incomplete else "FAIL" if findings else "PASS"
    return {
        "id": cycle_check["id"],
        "owner_module": cycle_check["owner_module"],
        "rule_id": cycle_check["rule_id"],
        "status": status,
        "rationale": cycle_check.get("rationale"),
        "scope": {
            "selected_paths": sorted(selected),
            "selected_modules": len(selected_modules),
            "explicit_runtime_edges": len(relevant_edges),
        },
        "findings": findings,
        "incomplete_reasons": incomplete,
        "limits": [
            "Only explicit static and literal dynamic imports inside the configured owned scope are evaluated.",
            "Type-only and implicit parent-initialization edges do not create ARC-03 cycle findings.",
            "Exports, runtime registries, persisted DSL and modules outside this configured scope remain separate evidence.",
        ],
    }


def _runtime_fingerprint(probe: dict, finding: dict) -> str:
    semantic = {
        "rule_id": probe["rule_id"],
        "probe": probe["id"],
        "kind": finding.get("kind"),
        "module": finding.get("module"),
        "path": finding.get("path"),
        "methods": finding.get("methods"),
        "endpoint": finding.get("endpoint"),
        "event": finding.get("event"),
        "symbol": finding.get("symbol"),
        "case": finding.get("case"),
    }
    return hashlib.sha256(json.dumps(semantic, sort_keys=True).encode()).hexdigest()


def _runtime_selected_paths(probe: dict) -> list[str]:
    if probe["kind"] == "pytest_contract":
        return sorted({test_id.split("::", 1)[0] for test_id in probe["test_ids"]})
    selected_paths = [target["path"] for target in probe.get("targets", [])]
    if "target" in probe:
        selected_paths.append(probe["target"]["path"])
    return sorted(selected_paths)


def _evaluate_pytest_contract(
    root: Path,
    probe: dict,
    python_paths: list[str] | None = None,
    python_executable: Path | None = None,
) -> dict:
    try:
        working_directory = (root / probe.get("working_directory", ".")).resolve()
        if not working_directory.is_relative_to(root) or not working_directory.is_dir():
            raise ValueError(f"Runtime probe {probe['id']} working_directory does not exist inside the repository")
        resolved_python_paths = [(root / path).resolve() for path in python_paths or ()]
        if any(not path.is_relative_to(root) or not path.is_dir() for path in resolved_python_paths):
            raise ValueError(f"Runtime probe {probe['id']} has a missing or unsafe profile python_path")
        python_path = os.pathsep.join([*(str(path) for path in resolved_python_paths), os.environ.get("PYTHONPATH", "")]).rstrip(os.pathsep)
        pytest_nodes = []
        for test_id in probe["test_ids"]:
            test_path, *node = test_id.split("::")
            pytest_nodes.append("::".join([str(safe_path(root, test_path)), *node]))
        with tempfile.TemporaryDirectory(prefix="ragflow-architecture-contract-") as directory:
            report_path = Path(directory) / "pytest.xml"
            completed = subprocess.run(
                [
                    str(python_executable or sys.executable),
                    "-B",
                    "-m",
                    "pytest",
                    *pytest_nodes,
                    "-q",
                    "-p",
                    "pytest_asyncio.plugin",
                    f"--junitxml={report_path}",
                ],
                cwd=working_directory,
                env={**os.environ, "PYTEST_DISABLE_PLUGIN_AUTOLOAD": "1", "PYTHONPATH": python_path},
                capture_output=True,
                text=True,
                timeout=probe["timeout_seconds"],
                check=False,
            )
            captured_stdout = completed.stdout[-4000:]
            captured_stderr = completed.stderr[-4000:]
            if not report_path.is_file():
                return {
                    "status": "INCOMPLETE",
                    "observations": [],
                    "loaded_repo_modules": [],
                    "findings": [],
                    "captured_stdout": captured_stdout,
                    "captured_stderr": captured_stderr,
                    "incomplete_reasons": [
                        {
                            "kind": "pytest_contract_report_missing",
                            "message": f"pytest exited with {completed.returncode} without a JUnit report",
                        }
                    ],
                }
            try:
                tree = ET.parse(report_path)
            except ET.ParseError as exc:
                return {
                    "status": "INCOMPLETE",
                    "observations": [],
                    "loaded_repo_modules": [],
                    "findings": [],
                    "captured_stdout": captured_stdout,
                    "captured_stderr": captured_stderr,
                    "incomplete_reasons": [{"kind": "pytest_contract_report_invalid", "message": str(exc)}],
                }
            cases = tree.findall(".//testcase")
            failures = [case for case in cases if case.find("failure") is not None]
            errors = [case for case in cases if case.find("error") is not None]
            skipped = [case for case in cases if case.find("skipped") is not None]
            observations = [
                {
                    "test_ids": probe["test_ids"],
                    "expected_tests": probe["expected_tests"],
                    "executed_tests": len(cases),
                    "failures": len(failures),
                    "errors": len(errors),
                    "skipped": len(skipped),
                    "pytest_exit_code": completed.returncode,
                    "third_party_plugin_autoload": False,
                    "explicit_plugins": ["pytest_asyncio.plugin"],
                }
            ]
            findings = [
                {
                    "kind": "contract_test_failure",
                    "case": "::".join(filter(None, (case.get("classname"), case.get("name")))),
                    "path": _runtime_selected_paths(probe)[0] if len(_runtime_selected_paths(probe)) == 1 else None,
                    "message": (case.find("failure").get("message") or "Contract assertion failed")[-1000:],
                }
                for case in failures
            ]
            incomplete_reasons = []
            if errors:
                incomplete_reasons.append(
                    {
                        "kind": "pytest_contract_error",
                        "message": f"{len(errors)} required contract test(s) ended with an error",
                    }
                )
            if skipped:
                incomplete_reasons.append(
                    {
                        "kind": "pytest_contract_skipped",
                        "message": f"{len(skipped)} required contract test(s) were skipped",
                    }
                )
            if len(cases) != probe["expected_tests"]:
                incomplete_reasons.append(
                    {
                        "kind": "pytest_contract_count_mismatch",
                        "message": f"Expected {probe['expected_tests']} executed test(s), observed {len(cases)}",
                    }
                )
            if completed.returncode != 0 and not failures and not errors:
                incomplete_reasons.append(
                    {
                        "kind": "pytest_contract_exit",
                        "message": f"pytest exited with {completed.returncode} without a classified failure or error",
                    }
                )
            status = "INCOMPLETE" if incomplete_reasons else "FAIL" if findings else "PASS"
            return {
                "status": status,
                "observations": observations,
                "loaded_repo_modules": [],
                "findings": findings,
                "captured_stdout": captured_stdout,
                "captured_stderr": captured_stderr,
                "incomplete_reasons": incomplete_reasons,
            }
    except subprocess.TimeoutExpired as exc:
        return {
            "status": "INCOMPLETE",
            "observations": [],
            "loaded_repo_modules": [],
            "findings": [],
            "captured_stdout": (exc.stdout or "")[-4000:] if isinstance(exc.stdout, str) else "",
            "captured_stderr": (exc.stderr or "")[-4000:] if isinstance(exc.stderr, str) else "",
            "incomplete_reasons": [
                {
                    "kind": "pytest_contract_timeout",
                    "message": f"Contract tests exceeded {probe['timeout_seconds']} seconds",
                }
            ],
        }


def evaluate_runtime_probe(
    root: Path,
    probe: dict,
    worker: Path = RUNTIME_WORKER,
    source_roots: set[str] | None = None,
    python_paths: list[str] | None = None,
    python_executable: Path | None = None,
    unavailable_reason: str | None = None,
) -> dict:
    if unavailable_reason:
        worker_result = {
            "status": "INCOMPLETE",
            "observations": [],
            "loaded_repo_modules": [],
            "forbidden_loaded_modules": [],
            "audit_events": [],
            "new_non_daemon_threads": [],
            "captured_stdout": "",
            "captured_stderr": "",
            "findings": [],
            "incomplete_reasons": [{"kind": "python_executable_missing", "message": unavailable_reason}],
        }
    elif probe["kind"] == "pytest_contract":
        worker_result = _evaluate_pytest_contract(root, probe, python_paths, python_executable)
    else:
        request = {key: value for key, value in probe.items() if key not in {"id", "owner_module", "profile", "rule_id", "rationale"}}
        request["root"] = str(root)
        request["observed_source_roots"] = sorted(source_roots or ())
        request["python_paths"] = list(python_paths or ())
        try:
            completed = subprocess.run(
                [str(python_executable or sys.executable), "-I", "-B", str(worker)],
                cwd=root,
                input=json.dumps(request, ensure_ascii=False),
                capture_output=True,
                text=True,
                timeout=probe["timeout_seconds"],
                check=False,
            )
        except subprocess.TimeoutExpired as exc:
            worker_result = {
                "status": "INCOMPLETE",
                "observations": [],
                "loaded_repo_modules": [],
                "forbidden_loaded_modules": [],
                "audit_events": [],
                "new_non_daemon_threads": [],
                "captured_stdout": (exc.stdout or "")[-2000:] if isinstance(exc.stdout, str) else "",
                "captured_stderr": (exc.stderr or "")[-2000:] if isinstance(exc.stderr, str) else "",
                "findings": [],
                "incomplete_reasons": [{"kind": "runtime_probe_timeout", "message": f"Probe exceeded {probe['timeout_seconds']} seconds"}],
            }
        else:
            if completed.returncode != 0:
                worker_result = {
                    "status": "INCOMPLETE",
                    "observations": [],
                    "loaded_repo_modules": [],
                    "forbidden_loaded_modules": [],
                    "audit_events": [],
                    "new_non_daemon_threads": [],
                    "captured_stdout": completed.stdout[-2000:],
                    "captured_stderr": completed.stderr[-2000:],
                    "findings": [],
                    "incomplete_reasons": [{"kind": "runtime_worker_exit", "message": f"Runtime worker exited with {completed.returncode}"}],
                }
            else:
                try:
                    worker_result = json.loads(completed.stdout)
                except (json.JSONDecodeError, TypeError) as exc:
                    worker_result = {
                        "status": "INCOMPLETE",
                        "observations": [],
                        "loaded_repo_modules": [],
                        "forbidden_loaded_modules": [],
                        "audit_events": [],
                        "new_non_daemon_threads": [],
                        "captured_stdout": completed.stdout[-2000:],
                        "captured_stderr": completed.stderr[-2000:],
                        "findings": [],
                        "incomplete_reasons": [{"kind": "invalid_runtime_worker_output", "message": str(exc)}],
                    }
    required_keys = {"status", "observations", "loaded_repo_modules", "findings", "incomplete_reasons"}
    if not isinstance(worker_result, dict) or not required_keys.issubset(worker_result) or worker_result.get("status") not in {"PASS", "FAIL", "INCOMPLETE"}:
        worker_result = {
            "status": "INCOMPLETE",
            "observations": [],
            "loaded_repo_modules": [],
            "forbidden_loaded_modules": [],
            "audit_events": [],
            "new_non_daemon_threads": [],
            "captured_stdout": "",
            "captured_stderr": "",
            "findings": [],
            "incomplete_reasons": [{"kind": "invalid_runtime_worker_schema", "message": "Runtime worker returned an invalid result"}],
        }
    findings = []
    for raw_finding in worker_result["findings"]:
        finding = {"rule_id": probe["rule_id"], "runtime_probe": probe["id"], **raw_finding}
        finding["fingerprint"] = _runtime_fingerprint(probe, finding)
        findings.append(finding)
    selected_paths = _runtime_selected_paths(probe)
    limits = (
        [
            "The exact pytest node runs in a child process with third-party plugin autoload disabled; this is not an operating-system sandbox.",
            "The configured contract test controls its own side effects and can prove only its explicit assertions.",
            "Unselected tests, external services and historical persisted data are not exercised unless the exact contract explicitly covers them.",
        ]
        if probe["kind"] == "pytest_contract"
        else [
            "The child process blocks configured socket/process audit events; it is not an operating-system sandbox.",
            "The RAGFlow Quart probe stubs api.apps authentication objects and proves registration, not request authorization behavior.",
            "External services and adapter calls are not exercised by import/registration probes.",
        ]
    )
    return {
        "id": probe["id"],
        "owner_module": probe["owner_module"],
        "rule_id": probe["rule_id"],
        "kind": probe["kind"],
        "status": worker_result["status"],
        "rationale": probe.get("rationale"),
        "scope": {"selected_paths": sorted(selected_paths), "loaded_repo_modules": len(worker_result["loaded_repo_modules"])},
        "observations": worker_result["observations"],
        "loaded_repo_modules": worker_result["loaded_repo_modules"],
        "forbidden_loaded_modules": worker_result.get("forbidden_loaded_modules", []),
        "audit_events": worker_result.get("audit_events", []),
        "new_non_daemon_threads": worker_result.get("new_non_daemon_threads", []),
        "captured_stdout": worker_result.get("captured_stdout", ""),
        "captured_stderr": worker_result.get("captured_stderr", ""),
        "findings": findings,
        "incomplete_reasons": worker_result["incomplete_reasons"],
        "limits": limits,
    }


def _normalized_module(value: object, label: str) -> str:
    if not isinstance(value, str) or not value or not all(part.isidentifier() for part in value.split(".")):
        raise ValueError(f"{label} must be an exact dotted Python module")
    return value


def _normalized_allowed_imports(value: object, connection_id: str, key: str) -> list[dict]:
    if value is None:
        return []
    if not isinstance(value, list):
        raise ValueError(f"Connection {connection_id} {key} must be a list")
    normalized = []
    seen = set()
    for entry in value:
        if not isinstance(entry, dict):
            raise ValueError(f"Connection {connection_id} {key} entries must be mappings")
        source = _normalized_module(entry.get("source"), f"Connection {connection_id} source")
        target = _normalized_module(entry.get("target"), f"Connection {connection_id} target")
        symbols = entry.get("symbols")
        if not isinstance(symbols, list) or not symbols:
            raise ValueError(f"Connection {connection_id} {source} -> {target} needs exact symbols")
        if any(not isinstance(symbol, str) or symbol != MODULE_ACCESS and not symbol.isidentifier() for symbol in symbols):
            raise ValueError(f"Connection {connection_id} has invalid or wildcard symbol")
        item = {"source": source, "target": target, "symbols": sorted(set(symbols))}
        semantic = {(source, target, symbol) for symbol in item["symbols"]}
        if semantic & seen:
            raise ValueError(f"Connection {connection_id} has duplicate allowed symbol")
        seen.update(semantic)
        normalized.append(item)
    return normalized


def _normalized_symbols(value: object, label: str) -> list[str]:
    if value is None:
        return []
    if not isinstance(value, list) or any(not isinstance(symbol, str) or not symbol.isidentifier() for symbol in value):
        raise ValueError(f"{label} must contain exact Python identifiers")
    if len(value) != len(set(value)):
        raise ValueError(f"{label} contains duplicates")
    return sorted(value)


def _normalized_pytest_node(value: object, probe_id: str) -> str:
    if not isinstance(value, str) or not value:
        raise ValueError(f"Runtime probe {probe_id} test_ids must contain exact pytest nodes")
    parts = value.split("::")
    path = _normalized_file(parts[0], f"Runtime probe {probe_id} test path")
    if not path.endswith(".py") or len(parts) < 2 or any(not part.isidentifier() for part in parts[1:]):
        raise ValueError(f"Runtime probe {probe_id} test_ids must select exact Python test nodes")
    return "::".join([path, *parts[1:]])


def _normalized_runtime_probe(value: object, profiles: dict, modules: dict) -> dict:
    if not isinstance(value, dict):
        raise ValueError("Runtime probe entries must be mappings")
    probe = dict(value)
    probe_id = probe.get("id")
    if not isinstance(probe_id, str) or not probe_id:
        raise ValueError("Runtime probe ID must be a non-empty string")
    if probe.get("owner_module") not in modules:
        raise ValueError(f"Runtime probe {probe_id} has unknown owner module")
    if probe.get("profile") not in profiles:
        raise ValueError(f"Runtime probe {probe_id} has unknown profile")
    if probe.get("rule_id") not in {"ARC-01", "ARC-02"}:
        raise ValueError(f"Runtime probe {probe_id} uses an unsupported rule")
    kind = probe.get("kind")
    if kind not in {"isolated_import", "ragflow_quart_blueprint", "pytest_contract"}:
        raise ValueError(f"Runtime probe {probe_id} has unsupported kind")
    timeout = probe.get("timeout_seconds", 30)
    if not isinstance(timeout, int) or isinstance(timeout, bool) or not 1 <= timeout <= 300:
        raise ValueError(f"Runtime probe {probe_id} timeout_seconds must be 1..300")
    probe["timeout_seconds"] = timeout
    if kind == "pytest_contract":
        test_ids = probe.get("test_ids")
        if not isinstance(test_ids, list) or not test_ids:
            raise ValueError(f"Runtime probe {probe_id} needs test_ids")
        probe["test_ids"] = [_normalized_pytest_node(test_id, probe_id) for test_id in test_ids]
        if len(probe["test_ids"]) != len(set(probe["test_ids"])):
            raise ValueError(f"Runtime probe {probe_id} contains duplicate test_ids")
        expected_tests = probe.get("expected_tests")
        if not isinstance(expected_tests, int) or isinstance(expected_tests, bool) or expected_tests < 1:
            raise ValueError(f"Runtime probe {probe_id} expected_tests must be a positive integer")
        probe["expected_tests"] = expected_tests
        probe["working_directory"] = _normalized_directory(
            probe.get("working_directory", "."),
            f"Runtime probe {probe_id} working_directory",
            allow_root=True,
        )
        return probe
    if not isinstance(probe.get("allow_output", False), bool):
        raise ValueError(f"Runtime probe {probe_id} allow_output must be boolean")
    probe["allow_output"] = probe.get("allow_output", False)
    if not isinstance(probe.get("audit_events_are_findings", True), bool):
        raise ValueError(f"Runtime probe {probe_id} audit_events_are_findings must be boolean")
    probe["audit_events_are_findings"] = probe.get("audit_events_are_findings", True)
    prefixes = probe.get("forbidden_module_prefixes", [])
    if not isinstance(prefixes, list):
        raise ValueError(f"Runtime probe {probe_id} forbidden_module_prefixes must be a list")
    probe["forbidden_module_prefixes"] = sorted({_normalized_module(prefix, f"Runtime probe {probe_id} forbidden module prefix") for prefix in prefixes})
    audit_events = probe.get("blocked_audit_events")
    if audit_events is not None:
        if not isinstance(audit_events, list) or any(not isinstance(event, str) or not event for event in audit_events):
            raise ValueError(f"Runtime probe {probe_id} blocked_audit_events must be non-empty strings")
        probe["blocked_audit_events"] = sorted(set(audit_events))

    def target(raw: object, label: str) -> dict:
        if not isinstance(raw, dict):
            raise ValueError(f"Runtime probe {probe_id} {label} must be a mapping")
        return {
            "module": _normalized_module(raw.get("module"), f"Runtime probe {probe_id} {label} module"),
            "path": _normalized_file(raw.get("path"), f"Runtime probe {probe_id} {label} path"),
            "required_symbols": _normalized_symbols(raw.get("required_symbols"), f"Runtime probe {probe_id} {label} required_symbols"),
        }

    if kind == "isolated_import":
        raw_targets = probe.get("targets")
        if not isinstance(raw_targets, list) or not raw_targets:
            raise ValueError(f"Runtime probe {probe_id} needs import targets")
        probe["targets"] = [target(item, "target") for item in raw_targets]
    else:
        probe["target"] = target(probe.get("target"), "target")
        parent = probe.get("parent_stub")
        if not isinstance(parent, dict):
            raise ValueError(f"Runtime probe {probe_id} parent_stub must be a mapping")
        probe["parent_stub"] = {
            "module": _normalized_module(parent.get("module"), f"Runtime probe {probe_id} parent module"),
            "path": _normalized_file(parent.get("path"), f"Runtime probe {probe_id} parent path"),
        }
        blueprint_name = probe.get("blueprint_name")
        if not isinstance(blueprint_name, str) or not blueprint_name.isidentifier():
            raise ValueError(f"Runtime probe {probe_id} blueprint_name must be an identifier")
        url_prefix = probe.get("url_prefix")
        if not isinstance(url_prefix, str) or not url_prefix.startswith("/") or ".." in url_prefix:
            raise ValueError(f"Runtime probe {probe_id} url_prefix must be an absolute URL path")
        routes = probe.get("expected_routes")
        if not isinstance(routes, list) or not routes:
            raise ValueError(f"Runtime probe {probe_id} needs expected_routes")
        normalized_routes = []
        seen_routes = set()
        for route in routes:
            if not isinstance(route, dict):
                raise ValueError(f"Runtime probe {probe_id} routes must be mappings")
            path = route.get("path")
            methods = route.get("methods")
            endpoint = route.get("endpoint")
            if not isinstance(path, str) or not path.startswith("/") or ".." in path:
                raise ValueError(f"Runtime probe {probe_id} has invalid route path")
            if not isinstance(methods, list) or not methods or any(not isinstance(method, str) or not method.isalpha() or method != method.upper() for method in methods):
                raise ValueError(f"Runtime probe {probe_id} has invalid route methods")
            if not isinstance(endpoint, str) or not endpoint.isidentifier():
                raise ValueError(f"Runtime probe {probe_id} has invalid route endpoint")
            normalized = {"path": path, "methods": sorted(set(methods)), "endpoint": endpoint}
            semantic = (path, tuple(normalized["methods"]), endpoint)
            if semantic in seen_routes:
                raise ValueError(f"Runtime probe {probe_id} has duplicate expected route")
            seen_routes.add(semantic)
            normalized_routes.append(normalized)
        probe["expected_routes"] = normalized_routes
        stub_modules = probe.get("stub_modules", [])
        if not isinstance(stub_modules, list):
            raise ValueError(f"Runtime probe {probe_id} stub_modules must be a list")
        normalized_stubs = []
        seen_stubs = set()
        for stub in stub_modules:
            if not isinstance(stub, dict):
                raise ValueError(f"Runtime probe {probe_id} stub_modules entries must be mappings")
            module = _normalized_module(stub.get("module"), f"Runtime probe {probe_id} stub module")
            symbols = stub.get("symbols")
            if not isinstance(symbols, dict) or not symbols:
                raise ValueError(f"Runtime probe {probe_id} stub module {module} needs symbols")
            if any(not isinstance(symbol, str) or not symbol.isidentifier() for symbol in symbols):
                raise ValueError(f"Runtime probe {probe_id} stub module {module} has invalid symbol")
            if any(kind not in {"class", "exception", "function"} for kind in symbols.values()):
                raise ValueError(f"Runtime probe {probe_id} stub module {module} has invalid symbol kind")
            if module in seen_stubs:
                raise ValueError(f"Runtime probe {probe_id} has duplicate stub module")
            seen_stubs.add(module)
            normalized_stubs.append({"module": module, "symbols": dict(sorted(symbols.items()))})
        probe["stub_modules"] = normalized_stubs
    return probe


def _load_configuration(
    root: Path,
    policy_path: Path,
    selected_boundary_ids: list[str] | None,
    selected_connection_ids: list[str] | None,
    selected_cycle_check_ids: list[str] | None,
    selected_runtime_probe_ids: list[str] | None,
) -> tuple[dict, dict, list[dict], list[dict], list[dict], list[dict]]:
    policy = yaml.safe_load(policy_path.read_text(encoding="utf-8"))
    if not isinstance(policy, dict) or policy.get("schema_version") != 1:
        raise ValueError("Unsupported Python boundary policy schema")
    mapping = yaml.safe_load((root / "tools/quality/module-map.yaml").read_text(encoding="utf-8"))
    modules = {module["id"]: module for module in mapping["modules"]}
    raw_profiles = policy.get("profiles")
    if not isinstance(raw_profiles, list) or not raw_profiles:
        raise ValueError("Python boundary policy needs profiles")
    profiles = {}
    for raw_profile in raw_profiles:
        if not isinstance(raw_profile, dict):
            raise ValueError("Python boundary profiles must be mappings")
        profile_id = raw_profile.get("id")
        if not isinstance(profile_id, str) or not profile_id or profile_id in profiles:
            raise ValueError(f"Invalid or duplicate Python boundary profile ID: {profile_id}")
        source_roots = raw_profile.get("source_roots")
        if not isinstance(source_roots, list) or not source_roots or any(not isinstance(item, str) or not item or "/" in item or "\\" in item for item in source_roots):
            raise ValueError(f"Profile {profile_id} has invalid source_roots")
        python_paths = raw_profile.get("python_paths", [])
        if not isinstance(python_paths, list):
            raise ValueError(f"Profile {profile_id} python_paths must be a list")
        python_executable_candidates = raw_profile.get("python_executable_candidates", [])
        if not isinstance(python_executable_candidates, list):
            raise ValueError(f"Profile {profile_id} python_executable_candidates must be a list")
        normalized_executable_candidates = [_normalized_file(candidate, f"Profile {profile_id} python executable candidate") for candidate in python_executable_candidates]
        if len(normalized_executable_candidates) != len(set(normalized_executable_candidates)):
            raise ValueError(f"Profile {profile_id} has duplicate python executable candidates")
        profile = {
            **raw_profile,
            "source_roots": sorted(set(source_roots)),
            "python_paths": sorted({_normalized_directory(path, f"Profile {profile_id} python_path") for path in python_paths}),
            "python_executable_candidates": normalized_executable_candidates,
        }
        profiles[profile_id] = profile
    policy["profiles"] = list(profiles.values())
    boundaries = policy.get("boundaries", [])
    connections = policy.get("connections", [])
    cycle_checks = policy.get("cycle_checks", [])
    runtime_probes = policy.get("runtime_probes", [])
    if (
        not isinstance(boundaries, list)
        or not isinstance(connections, list)
        or not isinstance(cycle_checks, list)
        or not isinstance(runtime_probes, list)
        or not boundaries
        and not connections
        and not cycle_checks
        and not runtime_probes
    ):
        raise ValueError("Python boundary policy has no boundaries, connections, cycle checks or runtime probes")
    boundary_ids = [boundary.get("id") for boundary in boundaries]
    connection_ids = [connection.get("id") for connection in connections]
    cycle_check_ids = [cycle_check.get("id") for cycle_check in cycle_checks]
    runtime_probe_ids = [probe.get("id") for probe in runtime_probes]
    if any(not isinstance(boundary_id, str) or not boundary_id for boundary_id in boundary_ids) or len(boundary_ids) != len(set(boundary_ids)):
        raise ValueError("Python boundary IDs must be unique non-empty strings")
    if any(not isinstance(connection_id, str) or not connection_id for connection_id in connection_ids) or len(connection_ids) != len(set(connection_ids)):
        raise ValueError("Python connection IDs must be unique non-empty strings")
    if any(not isinstance(cycle_check_id, str) or not cycle_check_id for cycle_check_id in cycle_check_ids) or len(cycle_check_ids) != len(set(cycle_check_ids)):
        raise ValueError("Python cycle check IDs must be unique non-empty strings")
    if any(not isinstance(probe_id, str) or not probe_id for probe_id in runtime_probe_ids) or len(runtime_probe_ids) != len(set(runtime_probe_ids)):
        raise ValueError("Python runtime probe IDs must be unique non-empty strings")
    explicit_selection = selected_boundary_ids is not None or selected_connection_ids is not None or selected_cycle_check_ids is not None or selected_runtime_probe_ids is not None
    if selected_boundary_ids is not None:
        unknown = set(selected_boundary_ids) - set(boundary_ids)
        if unknown:
            raise ValueError("Unknown boundary ID: " + ", ".join(sorted(unknown)))
        boundaries = [boundary for boundary in boundaries if boundary["id"] in selected_boundary_ids]
    elif explicit_selection:
        boundaries = []
    if selected_connection_ids is not None:
        unknown = set(selected_connection_ids) - set(connection_ids)
        if unknown:
            raise ValueError("Unknown connection ID: " + ", ".join(sorted(unknown)))
        connections = [connection for connection in connections if connection["id"] in selected_connection_ids]
    elif explicit_selection:
        connections = []
    if selected_cycle_check_ids is not None:
        unknown = set(selected_cycle_check_ids) - set(cycle_check_ids)
        if unknown:
            raise ValueError("Unknown cycle check ID: " + ", ".join(sorted(unknown)))
        cycle_checks = [cycle_check for cycle_check in cycle_checks if cycle_check["id"] in selected_cycle_check_ids]
    elif explicit_selection:
        cycle_checks = []
    if selected_runtime_probe_ids is not None:
        unknown = set(selected_runtime_probe_ids) - set(runtime_probe_ids)
        if unknown:
            raise ValueError("Unknown runtime probe ID: " + ", ".join(sorted(unknown)))
        runtime_probes = [probe for probe in runtime_probes if probe["id"] in selected_runtime_probe_ids]
    elif explicit_selection:
        runtime_probes = []
    for boundary in boundaries:
        if boundary.get("owner_module") not in modules:
            raise ValueError(f"Boundary {boundary['id']} has unknown owner module")
        if boundary.get("profile") not in profiles:
            raise ValueError(f"Boundary {boundary['id']} has unknown profile")
        if boundary.get("rule_id") != "ARC-01":
            raise ValueError(f"Boundary {boundary['id']} uses an unsupported rule")
        prefixes = boundary.get("path_prefixes")
        if not isinstance(prefixes, list) or not prefixes:
            raise ValueError(f"Boundary {boundary['id']} has no path_prefixes")
        boundary["path_prefixes"] = [_normalized_prefix(prefix) for prefix in prefixes]
    for connection in connections:
        if connection.get("owner_module") not in modules:
            raise ValueError(f"Connection {connection['id']} has unknown owner module")
        if connection.get("profile") not in profiles:
            raise ValueError(f"Connection {connection['id']} has unknown profile")
        if connection.get("rule_id") != "ARC-02":
            raise ValueError(f"Connection {connection['id']} uses an unsupported rule")
        path_prefixes = connection.get("extension_path_prefixes")
        module_prefixes = connection.get("extension_module_prefixes")
        if not isinstance(path_prefixes, list) or not path_prefixes:
            raise ValueError(f"Connection {connection['id']} has no extension_path_prefixes")
        if not isinstance(module_prefixes, list) or not module_prefixes:
            raise ValueError(f"Connection {connection['id']} has no extension_module_prefixes")
        connection["extension_path_prefixes"] = [_normalized_prefix(prefix) for prefix in path_prefixes]
        connection["extension_module_prefixes"] = [_normalized_module(prefix, f"Connection {connection['id']} module prefix") for prefix in module_prefixes]
        connection["allowed_forward_imports"] = _normalized_allowed_imports(connection.get("allowed_forward_imports"), connection["id"], "allowed_forward_imports")
        connection["allowed_reverse_imports"] = _normalized_allowed_imports(connection.get("allowed_reverse_imports"), connection["id"], "allowed_reverse_imports")
    for cycle_check in cycle_checks:
        if cycle_check.get("owner_module") not in modules:
            raise ValueError(f"Cycle check {cycle_check['id']} has unknown owner module")
        if cycle_check.get("profile") not in profiles:
            raise ValueError(f"Cycle check {cycle_check['id']} has unknown profile")
        if cycle_check.get("rule_id") != "ARC-03":
            raise ValueError(f"Cycle check {cycle_check['id']} uses an unsupported rule")
        prefixes = cycle_check.get("path_prefixes")
        if not isinstance(prefixes, list) or not prefixes:
            raise ValueError(f"Cycle check {cycle_check['id']} has no path_prefixes")
        cycle_check["path_prefixes"] = [_normalized_prefix(prefix) for prefix in prefixes]
    runtime_probes = [_normalized_runtime_probe(probe, profiles, modules) for probe in runtime_probes]
    return policy, mapping, boundaries, connections, cycle_checks, runtime_probes


def _profile_sources(root: Path, files: list[str], profiles: list[dict]) -> tuple[dict[str, bytes], dict[str, str]]:
    def included(path: str, profile: dict) -> bool:
        python_paths = profile["python_paths"]
        if python_paths:
            return any(path.startswith(import_root.rstrip("/") + "/") for import_root in python_paths)
        return path.split("/", 1)[0] in profile["source_roots"]

    selected = sorted(path for path in files if path.endswith(".py") and any(included(path, profile) for profile in profiles) and (root / path).exists())
    raw = {path: safe_path(root, path).read_bytes() for path in selected}
    sources = {path: content.decode(tokenize.detect_encoding(BytesIO(content).readline)[0]) for path, content in raw.items()}
    return raw, sources


def _profile_python_executable(root: Path, profile: dict) -> tuple[Path | None, str | None]:
    candidates = profile["python_executable_candidates"]
    if not candidates:
        return Path(sys.executable), None
    for candidate in candidates:
        resolved = safe_path(root, candidate)
        if resolved.is_file():
            return resolved, None
    return None, f"Profile {profile['id']} found none of its Python executable candidates: {', '.join(candidates)}"


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--policy", type=Path, help="Boundary policy; defaults to tools/quality/python-boundaries.yaml")
    parser.add_argument("--base-ref", help="Optional PR base ref recorded as a resolved commit in the local report")
    parser.add_argument("--boundary", action="append", dest="boundaries", help="Exact configured boundary ID; repeat to select more")
    parser.add_argument("--connection", action="append", dest="connections", help="Exact configured ARC-02 connection ID; repeat to select more")
    parser.add_argument("--cycle-check", action="append", dest="cycle_checks", help="Exact configured ARC-03 cycle check ID; repeat to select more")
    parser.add_argument("--runtime-probe", action="append", dest="runtime_probes", help="Exact configured runtime probe/contract ID; repeat to select more")
    parser.add_argument("--output", type=Path, required=True, help="Report path outside tracked/nonignored source files")
    args = parser.parse_args(argv)
    root = args.root.resolve()
    output = args.output.resolve()
    policy_path = args.policy.resolve() if args.policy else root / "tools/quality/python-boundaries.yaml"
    try:
        if output.is_relative_to(root):
            git(root, "check-ignore", "--no-index", "-q", output.relative_to(root).as_posix())
        policy, mapping, boundaries, connections, cycle_checks, runtime_probes = _load_configuration(
            root,
            policy_path,
            args.boundaries,
            args.connections,
            args.cycle_checks,
            args.runtime_probes,
        )
        worker_probes = [probe for probe in runtime_probes if probe.get("kind") != "pytest_contract"]
        if worker_probes and not RUNTIME_WORKER.is_file():
            raise ValueError(f"Runtime probe worker is missing: {RUNTIME_WORKER}")
        base = json.loads((root / "tools/quality/upstream-base.json").read_text(encoding="utf-8"))
        before = capture(root, base, mapping)
        pr_base_sha = git(root, "rev-parse", f"{args.base_ref}^{{commit}}").decode().strip() if args.base_ref else None
        files = paths(git(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z"))
        file_set = set(files)
        if output.is_relative_to(root) and output.relative_to(root).as_posix() in files:
            raise ValueError("Report must not overwrite source files, including tracked files under ignore rules")
        profile_ids = {item["profile"] for item in [*boundaries, *connections, *cycle_checks, *runtime_probes]}
        profiles = {profile["id"]: profile for profile in policy["profiles"]}
        import_roots = {path for profile_id in profile_ids for path in profiles[profile_id]["python_paths"]}
        raw, sources = _profile_sources(root, files, [profiles[profile_id] for profile_id in profile_ids])
        owners = {record["path"]: {"origin": record["origin"], "owner": record["module"]} for record in before["records"]}
        for path in sources:
            owners.setdefault(path, {"origin": "upstream", "owner": "upstream"})

        boundary_results = []
        for boundary in boundaries:
            selected = {path for path in sources if any(path.startswith(prefix) for prefix in boundary["path_prefixes"])}
            if not selected:
                raise ValueError(f"Boundary {boundary['id']} selected no Python files")
            wrong_owner = sorted(path for path in selected if owners[path]["owner"] != boundary["owner_module"])
            if wrong_owner:
                raise ValueError(f"Boundary {boundary['id']} contains paths outside owner module: {', '.join(wrong_owner)}")
            boundary_results.append(evaluate_boundary(sources, selected, owners, boundary, import_roots))

        connection_results = []
        for connection in connections:
            extension_paths = {path for path in sources if any(path.startswith(prefix) for prefix in connection["extension_path_prefixes"])}
            wrong_owner = sorted(path for path in extension_paths if owners[path]["owner"] != connection["owner_module"])
            if wrong_owner:
                raise ValueError(f"Connection {connection['id']} contains paths outside owner module: {', '.join(wrong_owner)}")
            connection_results.append(evaluate_connection(sources, connection, import_roots))

        cycle_check_results = []
        for cycle_check in cycle_checks:
            selected = {path for path in sources if any(path.startswith(prefix) for prefix in cycle_check["path_prefixes"])}
            if not selected:
                raise ValueError(f"Cycle check {cycle_check['id']} selected no Python files")
            wrong_owner = sorted(path for path in selected if owners[path]["owner"] != cycle_check["owner_module"])
            if wrong_owner:
                raise ValueError(f"Cycle check {cycle_check['id']} contains paths outside owner module: {', '.join(wrong_owner)}")
            cycle_check_results.append(evaluate_cycle_check(sources, selected, cycle_check, import_roots))

        runtime_probe_results = []
        runtime_selected_raw = {}
        for probe in runtime_probes:
            probe_profile = profiles[probe["profile"]]
            python_executable, unavailable_reason = _profile_python_executable(root, probe_profile)
            selected_paths = _runtime_selected_paths(probe)
            if probe["kind"] == "pytest_contract":
                missing = sorted(path for path in selected_paths if path not in file_set or not safe_path(root, path).is_file())
                missing_label = "repository paths"
            else:
                missing = sorted(path for path in selected_paths if path not in sources)
                missing_label = "profile paths"
            if missing:
                raise ValueError(f"Runtime probe {probe['id']} selected missing {missing_label}: {', '.join(missing)}")
            wrong_owner = sorted(path for path in selected_paths if owners[path]["owner"] != probe["owner_module"])
            if wrong_owner:
                raise ValueError(f"Runtime probe {probe['id']} contains paths outside owner module: {', '.join(wrong_owner)}")
            for path in selected_paths:
                runtime_selected_raw[path] = raw[path] if path in raw else safe_path(root, path).read_bytes()
            runtime_probe_results.append(
                evaluate_runtime_probe(
                    root,
                    probe,
                    source_roots=set(probe_profile["source_roots"]),
                    python_paths=probe_profile["python_paths"],
                    python_executable=python_executable,
                    unavailable_reason=unavailable_reason,
                )
            )

        for path, content in raw.items():
            if safe_path(root, path).read_bytes() != content:
                raise ValueError(f"Source changed during analysis: {path}")
        for path, content in runtime_selected_raw.items():
            if safe_path(root, path).read_bytes() != content:
                raise ValueError(f"Runtime contract source changed during analysis: {path}")
        after = capture(root, base, mapping)
        if before["snapshot_sha256"] != after["snapshot_sha256"] or before["head"] != after["head"]:
            raise ValueError("Provenance snapshot changed during analysis")

        results = [*boundary_results, *connection_results, *cycle_check_results, *runtime_probe_results]
        statuses = {result["status"] for result in results}
        status = "INCOMPLETE" if "INCOMPLETE" in statuses else "FAIL" if "FAIL" in statuses else "PASS"
        exit_code = 2 if status == "INCOMPLETE" else 1 if status == "FAIL" else 0
        selected_sha = {path: hashlib.sha256(raw[path] if path in raw else runtime_selected_raw[path]).hexdigest() for result in results for path in result["scope"]["selected_paths"]}
        report = {
            "schema_version": 1,
            "tool": {"name": "check_architecture", "version": VERSION, "python": platform.python_version(), "os": platform.system()},
            "mode": REPORT_ONLY,
            "policy_status": status,
            "exit_code": exit_code,
            "input": {
                "candidate_sha": before["head"],
                "pr_base_sha": pr_base_sha,
                "pr_base_status": "RESOLVED" if pr_base_sha else "NOT_PROVIDED_LOCAL_REPORT",
                "upstream_sha": before["upstream_base"],
                "dirty_snapshot_sha256": before["snapshot_sha256"],
                "policy_sha256": hashlib.sha256(policy_path.read_bytes()).hexdigest(),
                "tool_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                "runtime_worker_sha256": hashlib.sha256(RUNTIME_WORKER.read_bytes()).hexdigest() if worker_probes else None,
                "selected_source_sha256": dict(sorted(selected_sha.items())),
            },
            "profiles": sorted(profile_ids),
            "rules": sorted({result["rule_id"] for result in results}),
            "boundaries": boundary_results,
            "connections": connection_results,
            "cycle_checks": cycle_check_results,
            "runtime_probes": runtime_probe_results,
            "findings": [finding for result in results for finding in result["findings"]],
            "exceptions": [],
            "manual_review_required": MANUAL_RULES,
            "lanes": [
                *(
                    [
                        {
                            "id": "python-static-boundaries",
                            "status": "INCOMPLETE"
                            if "INCOMPLETE" in {item["status"] for item in [*boundary_results, *connection_results, *cycle_check_results]}
                            else "FAIL"
                            if "FAIL" in {item["status"] for item in [*boundary_results, *connection_results, *cycle_check_results]}
                            else "PASS",
                            "evidence": "AST import graph with parent initializers and transitive paths",
                        }
                    ]
                    if boundary_results or connection_results or cycle_check_results
                    else []
                ),
                *(
                    [
                        {
                            "id": "python-isolated-runtime",
                            "status": "INCOMPLETE"
                            if "INCOMPLETE" in {item["status"] for item in runtime_probe_results}
                            else "FAIL"
                            if "FAIL" in {item["status"] for item in runtime_probe_results}
                            else "PASS",
                            "evidence": "Fresh child-process imports, configured registration inventory, and exact required pytest contracts",
                        }
                    ]
                    if runtime_probe_results
                    else []
                ),
            ],
            "command": sys.argv if argv is None else argv,
        }
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(
            json.dumps(
                {
                    "policy_status": status,
                    "boundaries": len(boundary_results),
                    "connections": len(connection_results),
                    "cycle_checks": len(cycle_check_results),
                    "runtime_probes": len(runtime_probe_results),
                    "findings": len(report["findings"]),
                    "output": str(output),
                }
            )
        )
        return exit_code
    except (KeyError, TypeError, ValueError, OSError, SyntaxError, UnicodeError, subprocess.CalledProcessError, yaml.YAMLError) as error:
        print(f"INCOMPLETE: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
