"""Observe the complete frontend TypeScript import graph without modifying source code."""

from __future__ import annotations

import argparse
import hashlib
import json
import platform
import shutil
import subprocess
import sys
from collections import deque
from pathlib import Path

import yaml
from capture_inventory import capture, git, paths, safe_path
from run_isolated_python import sanitized_child_environment

VERSION = "0.2.0"
LIMITS = [
    "Observations only: no approved TypeScript layer allowlist, baseline or architecture acceptance.",
    "Static reachability includes literal dynamic imports but is not an execution trace or dead-code verdict.",
    "TypeScript path aliases and local assets are resolved from the configured Vite-facing profile; plugin transforms are not executed.",
    "Literal import.meta.glob patterns are recorded but not expanded into asset or module edges.",
    "Computed imports, requires, dynamic registrations and code evaluation make the graph incomplete.",
    "No export-usage verdict, deletion, bundle-size proof, runtime benchmark or browser behavior is inferred.",
    "Tests and declarations are indexed but excluded from browser-entrypoint reachability counts.",
]


def _cycle_components(nodes: set[str], edges: list[dict]) -> list[list[str]]:
    outgoing = {node: [] for node in nodes}
    for edge in edges:
        if edge["source"] in nodes and edge["target"] in nodes:
            outgoing[edge["source"]].append(edge["target"])
    index = 0
    stack = []
    on_stack = set()
    indices = {}
    lowlinks = {}
    components = []

    def connect(node):
        nonlocal index
        indices[node] = lowlinks[node] = index
        index += 1
        stack.append(node)
        on_stack.add(node)
        for target in outgoing[node]:
            if target not in indices:
                connect(target)
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
        if len(component) > 1 or node in outgoing[node]:
            components.append(sorted(component))

    for node in sorted(nodes):
        if node not in indices:
            connect(node)
    return sorted(components, key=lambda component: (component[0], len(component)))


def analyze(worker_result: dict, entrypoints: list[str], ownership: dict[str, dict]) -> dict:
    node_by_path = {node["path"]: node for node in worker_result["nodes"]}
    missing_entrypoints = sorted(set(entrypoints) - node_by_path.keys())
    runtime_paths = {path for path, node in node_by_path.items() if not node["is_test"] and not node["is_declaration"]}
    runtime_edges = [edge for edge in worker_result["edges"] if edge["resolution"] == "local" and not edge["type_only"] and edge["source"] in runtime_paths and edge["target"] in runtime_paths]
    eager_runtime_edges = [edge for edge in runtime_edges if edge["phase"] == "module" and edge["kind"] in {"import", "re_export", "import_equals", "require"}]
    outgoing = {}
    for edge in runtime_edges:
        outgoing.setdefault(edge["source"], []).append(edge["target"])
    reachable = set()
    queue = deque(entrypoint for entrypoint in entrypoints if entrypoint in runtime_paths)
    while queue:
        current = queue.popleft()
        if current in reachable:
            continue
        reachable.add(current)
        queue.extend(target for target in outgoing.get(current, []) if target not in reachable)

    owned_paths = {path for path in node_by_path if ownership.get(path, {}).get("origin") in {"extension", "core_change"}}
    incoming = {path: [] for path in owned_paths}
    for edge in worker_result["edges"]:
        if edge["resolution"] == "local" and edge["target"] in incoming and edge["source"] != edge["target"]:
            incoming[edge["target"]].append(edge)
    reverse_imports = []
    for path in sorted(owned_paths):
        edges = sorted(incoming[path], key=lambda edge: (edge["source"], edge["line"], edge["kind"], edge["type_only"]))
        reverse_imports.append(
            {
                "path": path,
                "consumer_paths": sorted({edge["source"] for edge in edges if not edge["type_only"]}),
                "type_only_consumer_paths": sorted({edge["source"] for edge in edges if edge["type_only"]}),
                "incoming_edges": edges,
                "interpretation": "direct potential imports only; absence is not a dead-code or unused-export verdict",
            }
        )

    issues = list(worker_result["issues"])
    for detail in worker_result["compiler"].get("configuration_errors", []):
        issues.append({"source": None, "line": 0, "kind": "configuration_error", "detail": detail, "type_only": False})
    for entrypoint in missing_entrypoints:
        issues.append({"source": entrypoint, "line": 0, "kind": "missing_entrypoint", "detail": entrypoint, "type_only": False})
    runtime_issues = [issue for issue in issues if issue["source"] in reachable and not issue.get("type_only", False)]
    other_issues = [issue for issue in issues if issue not in runtime_issues]

    cycles = []
    for members in _cycle_components(reachable, eager_runtime_edges):
        member_set = set(members)
        cycles.append(
            {
                "members": members,
                "owned_members": [member for member in members if member in owned_paths],
                "edges": [edge for edge in eager_runtime_edges if edge["source"] in member_set and edge["target"] in member_set],
                "interpretation": "reachable eager module-initialization cycle signal; lazy imports are excluded and policy is not evaluated",
            }
        )

    nodes = []
    for path, node in sorted(node_by_path.items()):
        nodes.append(
            {
                **node,
                **ownership.get(path, {"origin": "upstream", "owner": "upstream"}),
                "reachable_from_browser_entrypoint": path in reachable,
            }
        )
    unreachable_runtime = sorted(runtime_paths - reachable)
    return {
        "analysis_status": "INCOMPLETE" if issues else "OBSERVED",
        "policy_status": "NOT_EVALUATED",
        "dead_code_status": "NOT_ANALYZED",
        "scope": {
            "entrypoints": entrypoints,
            "indexed_files": len(node_by_path),
            "runtime_files": len(runtime_paths),
            "reachable_runtime_files": len(reachable),
            "unreachable_runtime_files": len(unreachable_runtime),
            "owned_or_core_files": len(owned_paths),
            "owned_without_direct_import_consumers": sum(not item["incoming_edges"] for item in reverse_imports),
            "runtime_local_edges": len(runtime_edges),
            "eager_runtime_local_edges": len(eager_runtime_edges),
            "all_import_edges": len(worker_result["edges"]),
            "reachable_cycles": len(cycles),
        },
        "nodes": nodes,
        "edges": worker_result["edges"],
        "reverse_imports": reverse_imports,
        "unreachable_runtime_paths": unreachable_runtime,
        "unreachable_owned_runtime_paths": sorted((runtime_paths & owned_paths) - reachable),
        "cycles": cycles,
        "dynamic_registrations": worker_result["dynamic_registrations"],
        "runtime_incomplete_reasons": runtime_issues,
        "other_incomplete_reasons": other_issues,
        "limits": LIMITS,
    }


def run_worker(root: Path, profile: dict, files: list[str]) -> dict:
    node = shutil.which("node")
    if node is None:
        raise ValueError("Node.js is required for TypeScript analysis")
    worker = safe_path(root, "tools/quality/inspect_typescript.cjs")
    typescript_module = safe_path(root, profile["typescript_module"])
    if not worker.is_file():
        raise ValueError("TypeScript analysis worker is missing")
    if not typescript_module.is_file():
        raise ValueError("Install the locked frontend TypeScript dependency before analysis")
    payload = {
        "root": str(root),
        "files": files,
        "aliases": profile.get("aliases", {}),
        "test_markers": profile.get("test_markers", []),
        "tsconfig": profile["tsconfig"],
        "typescript_module": profile["typescript_module"],
    }
    completed = subprocess.run(
        [node, str(worker)],
        cwd=root,
        input=json.dumps(payload),
        env=sanitized_child_environment(),
        capture_output=True,
        text=True,
        timeout=120,
        check=False,
    )
    if completed.returncode:
        raise ValueError(f"TypeScript worker failed: {completed.stderr[-4000:]}")
    try:
        result = json.loads(completed.stdout)
    except json.JSONDecodeError as error:
        raise ValueError("TypeScript worker returned invalid JSON") from error
    returned = sorted(node["path"] for node in result.get("nodes", []))
    if returned != sorted(files):
        raise ValueError("TypeScript worker returned an incomplete source inventory")
    return result


def _load_profile(root: Path, profile_id: str) -> tuple[dict, Path]:
    policy_path = safe_path(root, "tools/quality/typescript-analysis.yaml")
    policy = yaml.safe_load(policy_path.read_text(encoding="utf-8"))
    if policy.get("schema_version") != 1:
        raise ValueError("Unsupported TypeScript analysis policy schema")
    profiles = {profile["id"]: profile for profile in policy.get("profiles", [])}
    if profile_id not in profiles:
        raise ValueError("Unknown TypeScript profile ID")
    profile = profiles[profile_id]
    required = {"source_root", "entrypoints", "extensions", "tsconfig", "typescript_module"}
    if required - profile.keys():
        raise ValueError("Incomplete TypeScript profile")
    return profile, policy_path


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--profile", default="frontend-web", help="Exact profile ID from typescript-analysis.yaml")
    parser.add_argument("--output", type=Path, required=True, help="Report path outside tracked/nonignored source files")
    args = parser.parse_args(argv)
    root, output = args.root.resolve(), args.output.resolve()
    try:
        if output.is_relative_to(root):
            git(root, "check-ignore", "--no-index", "-q", output.relative_to(root).as_posix())
        base_config = json.loads(safe_path(root, "tools/quality/upstream-base.json").read_text(encoding="utf-8"))
        mapping = yaml.safe_load(safe_path(root, "tools/quality/module-map.yaml").read_text(encoding="utf-8"))
        profile, policy_path = _load_profile(root, args.profile)
        before = capture(root, base_config, mapping)
        repository_files = paths(git(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z"))
        if output.is_relative_to(root) and output.relative_to(root).as_posix() in repository_files:
            raise ValueError("Report must not overwrite source files, including tracked files under ignore rules")
        source_root = profile["source_root"].rstrip("/") + "/"
        extensions = tuple(profile["extensions"])
        files = sorted(path for path in repository_files if path.startswith(source_root) and path.endswith(extensions) and safe_path(root, path).is_file())
        if not files:
            raise ValueError("TypeScript profile contains no source files")
        entrypoints = [safe_path(root, path).relative_to(root).as_posix() for path in profile["entrypoints"]]
        raw = {path: safe_path(root, path).read_bytes() for path in files}
        ownership = {record["path"]: {"origin": record["origin"], "owner": record["module"]} for record in before["records"]}
        worker_result = run_worker(root, profile, files)
        report = analyze(worker_result, entrypoints, ownership)
        for path, content in raw.items():
            if safe_path(root, path).read_bytes() != content:
                raise ValueError(f"Source changed during analysis: {path}")
        after = capture(root, base_config, mapping)
        if before["snapshot_sha256"] != after["snapshot_sha256"] or before["head"] != after["head"]:
            raise ValueError("Provenance snapshot changed during analysis")
        worker_path = safe_path(root, "tools/quality/inspect_typescript.cjs")
        report.update(
            {
                "schema_version": 1,
                "tool": {
                    "name": "inspect_typescript",
                    "version": VERSION,
                    "typescript": worker_result["compiler"]["version"],
                    "python": platform.python_version(),
                    "os": platform.system(),
                },
                "profile": {**profile, "id": args.profile},
                "input": {
                    "head": before["head"],
                    "upstream_base": before["upstream_base"],
                    "snapshot_sha256": before["snapshot_sha256"],
                    "source_sha256": {path: hashlib.sha256(content).hexdigest() for path, content in raw.items()},
                    "tool_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                    "worker_sha256": hashlib.sha256(worker_path.read_bytes()).hexdigest(),
                    "policy_sha256": hashlib.sha256(policy_path.read_bytes()).hexdigest(),
                },
                "command": sys.argv if argv is None else argv,
            }
        )
        code = 2 if report["analysis_status"] == "INCOMPLETE" else 0
        report["exit_code"] = code
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(
            json.dumps(
                {
                    "analysis_status": report["analysis_status"],
                    "policy_status": report["policy_status"],
                    "indexed_files": report["scope"]["indexed_files"],
                    "reachable_runtime_files": report["scope"]["reachable_runtime_files"],
                    "cycles": report["scope"]["reachable_cycles"],
                    "incomplete_reasons": len(report["runtime_incomplete_reasons"]) + len(report["other_incomplete_reasons"]),
                    "output": str(output),
                }
            )
        )
        return code
    except (KeyError, TypeError, ValueError, OSError, UnicodeError, subprocess.CalledProcessError) as error:
        print(f"INCOMPLETE: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
