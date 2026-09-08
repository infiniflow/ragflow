"""Report static Python dead-code candidates without deleting or confirming code."""

from __future__ import annotations

import argparse
import ast
from collections import deque
import hashlib
import json
from pathlib import Path, PurePosixPath
import platform
import subprocess
import sys
import tokenize
from io import BytesIO

import yaml

from capture_inventory import capture, git, paths, safe_path
from inspect_python import collect_import_graph


VERSION = "0.2.0"
MODE = "T2_SCAN_PLAN_ONLY"


def _prefix(value: object) -> str:
    if not isinstance(value, str) or not value or "\\" in value:
        raise ValueError(f"Unsafe path prefix: {value}")
    path = PurePosixPath(value)
    if path.is_absolute() or ".." in path.parts or path.as_posix().startswith("./"):
        raise ValueError(f"Unsafe path prefix: {value}")
    return path.as_posix().rstrip("/") + "/"


def _symbol_fingerprint(profile: dict, item: dict) -> str:
    semantic = {
        "rule_id": "DEAD-01",
        "profile": profile["id"],
        "owner_module": profile["owner_module"],
        "module": item["module"],
        "symbol": item["symbol"],
        "kind": item["kind"],
    }
    return hashlib.sha256(json.dumps(semantic, sort_keys=True).encode()).hexdigest()


def load_policy(path: Path, known_owners: set[str]) -> list[dict]:
    raw = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not isinstance(raw, dict) or raw.get("schema_version") != 1:
        raise ValueError("Unsupported Python dead-code policy schema")
    profiles = raw.get("profiles")
    if not isinstance(profiles, list) or not profiles:
        raise ValueError("Python dead-code policy needs profiles")
    ids = set()
    normalized = []
    for profile in profiles:
        if not isinstance(profile, dict):
            raise ValueError("Python dead-code profile must be a mapping")
        profile_id = profile.get("id")
        if not isinstance(profile_id, str) or not profile_id or profile_id in ids:
            raise ValueError(f"Invalid or duplicate dead-code profile ID: {profile_id}")
        ids.add(profile_id)
        owner = profile.get("owner_module")
        if owner not in known_owners:
            raise ValueError(f"Profile {profile_id} has unknown owner module")
        roots = profile.get("source_roots")
        if not isinstance(roots, list) or not roots or any(not isinstance(root, str) or not root or "/" in root or "\\" in root for root in roots):
            raise ValueError(f"Profile {profile_id} has invalid source_roots")
        prefixes = profile.get("path_prefixes")
        if not isinstance(prefixes, list) or not prefixes:
            raise ValueError(f"Profile {profile_id} has no path_prefixes")
        python_paths = profile.get("python_paths", [])
        if not isinstance(python_paths, list):
            raise ValueError(f"Profile {profile_id} python_paths must be a list")
        normalized_python_paths = [_prefix(path).rstrip("/") for path in python_paths]
        normalized.append(
            {
                **profile,
                "source_roots": sorted(set(roots)),
                "path_prefixes": [_prefix(prefix) for prefix in prefixes],
                "python_paths": sorted(set(normalized_python_paths)),
            }
        )
    return normalized


def _sources(root: Path, files: list[str], profiles: list[dict]) -> tuple[dict[str, bytes], dict[str, str]]:
    def included(path: str, profile: dict) -> bool:
        python_paths = profile["python_paths"]
        if python_paths:
            return any(path.startswith(import_root.rstrip("/") + "/") for import_root in python_paths)
        return path.split("/", 1)[0] in profile["source_roots"]

    selected = sorted(path for path in files if path.endswith(".py") and any(included(path, profile) for profile in profiles) and safe_path(root, path).is_file())
    raw = {path: safe_path(root, path).read_bytes() for path in selected}
    decoded = {path: content.decode(tokenize.detect_encoding(BytesIO(content).readline)[0]) for path, content in raw.items()}
    return raw, decoded


def _module_definitions(source: str, path: str) -> tuple[list[dict], dict[str, set[str]], set[str], set[str]]:
    tree = ast.parse(source, filename=path)
    nodes = [node for node in tree.body if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef))]
    names = {node.name for node in nodes}
    dependencies = {}
    definitions = []
    for node in nodes:
        dependencies[node.name] = {part.id for part in ast.walk(node) if isinstance(part, ast.Name) and isinstance(part.ctx, ast.Load) and part.id in names and part.id != node.name}
        effectful = bool(node.decorator_list) or isinstance(node, ast.ClassDef) and bool(node.bases or node.keywords)
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            effectful = effectful or bool(node.args.defaults or any(node.args.kw_defaults))
        definitions.append(
            {
                "path": path,
                "symbol": node.name,
                "kind": "class" if isinstance(node, ast.ClassDef) else "function",
                "line": node.lineno,
                "public": not node.name.startswith("_"),
                "declaration_effect": effectful,
            }
        )
    module_roots = {
        part.id
        for node in tree.body
        if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef))
        for part in ast.walk(node)
        if isinstance(part, ast.Name) and isinstance(part.ctx, ast.Load) and part.id in names
    }
    explicit_exports = set()
    for node in tree.body:
        if not isinstance(node, (ast.Assign, ast.AnnAssign)):
            continue
        targets = node.targets if isinstance(node, ast.Assign) else [node.target]
        if not any(isinstance(target, ast.Name) and target.id == "__all__" for target in targets):
            continue
        value = node.value
        if isinstance(value, (ast.List, ast.Tuple, ast.Set)) and all(isinstance(item, ast.Constant) and isinstance(item.value, str) for item in value.elts):
            explicit_exports.update(item.value for item in value.elts if item.value in names)
    return definitions, dependencies, module_roots, explicit_exports


def analyze_profile(sources: dict[str, str], selected: set[str], profile: dict, import_roots: set[str] | None = None) -> dict:
    graph = collect_import_graph(sources, import_roots=import_roots or frozenset())
    module_by_path = {path: module for module, path in graph["index"].items()}
    definitions = []
    dependencies = {}
    roots = set()
    review_keys = set()
    reasons: dict[tuple[str, str], list[dict]] = {}
    for path in sorted(selected):
        module = module_by_path[path]
        found, local_edges, module_roots, explicit_exports = _module_definitions(sources[path], path)
        for item in found:
            key = (module, item["symbol"])
            definitions.append({**item, "module": module})
            dependencies[key] = {(module, name) for name in local_edges[item["symbol"]]}
            if item["symbol"] in module_roots or item["declaration_effect"]:
                roots.add(key)
                reasons.setdefault(key, []).append({"kind": "module_runtime_reference_or_declaration_effect"})
            if item["symbol"] in explicit_exports:
                review_keys.add(key)
                reasons.setdefault(key, []).append({"kind": "explicit_export"})
    definition_keys = {(item["module"], item["symbol"]) for item in definitions}
    selected_modules = {module_by_path[path] for path in selected}
    module_access = set()
    for edge in graph["edges"]:
        if edge["source"] == edge["target"]:
            continue
        if edge["symbols"] == ["*"] and edge["target"] in selected_modules:
            module_access.add(edge["target"])
            continue
        for symbol in edge["symbols"]:
            key = (edge["target"], symbol)
            if key in definition_keys:
                classification = "type_only_import" if edge["type_only"] else "static_import"
                (review_keys if edge["type_only"] else roots).add(key)
                reasons.setdefault(key, []).append({"kind": classification, "source": edge["source"], "path": edge["path"], "line": edge["line"]})
        if edge["type_only"]:
            continue
        if not edge["symbols"] and edge["target"] in {module for module, _ in definition_keys}:
            module_access.add(edge["target"])
    reachable = set(roots)
    queue = deque(roots)
    while queue:
        current = queue.popleft()
        for target in dependencies.get(current, set()):
            if target not in reachable:
                reachable.add(target)
                reasons.setdefault(target, []).append({"kind": "reachable_from", "module": current[0], "symbol": current[1]})
                queue.append(target)
    issues = [issue for issue in graph["issues"] if not issue["type_only"] and (issue["source"] in selected_modules or issue.get("detail") in selected_modules)]
    candidates = []
    reviewed = []
    for item in definitions:
        key = (item["module"], item["symbol"])
        if key in reachable:
            classification = "PRESERVE"
        elif key in review_keys or item["public"] or item["module"] in module_access:
            classification = "REVIEW_REQUIRED"
        elif issues:
            classification = "INCOMPLETE"
        else:
            classification = "STATIC_CANDIDATE"
        record = {
            **item,
            "classification": classification,
            "fingerprint": _symbol_fingerprint(profile, item),
            "evidence": reasons.get(key, []),
        }
        reviewed.append(record)
        if classification == "STATIC_CANDIDATE":
            candidates.append(record)
    return {
        "id": profile["id"],
        "owner_module": profile["owner_module"],
        "rule_id": "DEAD-01",
        "analysis_status": "INCOMPLETE" if issues else "OBSERVED",
        "policy_status": "NOT_EVALUATED",
        "dead_code_status": "NOT_CONFIRMED",
        "scope": {"selected_paths": sorted(selected), "definitions": len(definitions)},
        "symbols": reviewed,
        "static_candidates": candidates,
        "incomplete_reasons": issues,
        "plan": {"action": "NO_AUTOMATIC_PATCH", "candidate_count": len(candidates)},
        "limits": [
            "A static candidate is not a deletion verdict.",
            "Persisted DSL, reflection, string references, external SDK/API consumers and runtime coverage are not proven by this scan.",
            "Public symbols and unresolved module access require review; apply mode does not exist.",
        ],
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--policy", type=Path)
    parser.add_argument("--profile", action="append", dest="profiles")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args(argv)
    root = args.root.resolve()
    output = args.output.resolve()
    try:
        if output.is_relative_to(root):
            git(root, "check-ignore", "--no-index", "-q", output.relative_to(root).as_posix())
        mapping = yaml.safe_load((root / "tools/quality/module-map.yaml").read_text(encoding="utf-8"))
        owners = {module["id"] for module in mapping["modules"]}
        policy_path = args.policy.resolve() if args.policy else root / "tools/quality/python-dead-code.yaml"
        profiles = load_policy(policy_path, owners)
        requested = set(args.profiles or ())
        known = {profile["id"] for profile in profiles}
        if requested - known:
            raise ValueError("Unknown dead-code profile: " + ", ".join(sorted(requested - known)))
        profiles = [profile for profile in profiles if not requested or profile["id"] in requested]
        base = json.loads((root / "tools/quality/upstream-base.json").read_text(encoding="utf-8"))
        before = capture(root, base, mapping)
        files = paths(git(root, "ls-files", "--cached", "--others", "--exclude-standard", "-z"))
        if output.is_relative_to(root) and output.relative_to(root).as_posix() in files:
            raise ValueError("Report must not overwrite source files, including tracked files under ignore rules")
        raw, sources = _sources(root, files, profiles)
        import_roots = {path for profile in profiles for path in profile["python_paths"]}
        ownership = {record["path"]: record["module"] for record in before["records"]}
        results = []
        for profile in profiles:
            selected = {path for path in sources if any(path.startswith(prefix) for prefix in profile["path_prefixes"])}
            if not selected:
                raise ValueError(f"Profile {profile['id']} selected no Python files")
            wrong = sorted(path for path in selected if ownership.get(path, "upstream") != profile["owner_module"])
            if wrong:
                raise ValueError(f"Profile {profile['id']} contains paths outside owner module: {', '.join(wrong)}")
            results.append(analyze_profile(sources, selected, profile, import_roots))
        for path, content in raw.items():
            if safe_path(root, path).read_bytes() != content:
                raise ValueError(f"Source changed during analysis: {path}")
        after = capture(root, base, mapping)
        if before["head"] != after["head"] or before["snapshot_sha256"] != after["snapshot_sha256"]:
            raise ValueError("Provenance snapshot changed during analysis")
        report = {
            "schema_version": 1,
            "tool": {"name": "check_python_dead_code", "version": VERSION, "python": platform.python_version()},
            "mode": MODE,
            "analysis_status": "INCOMPLETE" if any(result["analysis_status"] == "INCOMPLETE" for result in results) else "OBSERVED",
            "policy_status": "NOT_EVALUATED",
            "dead_code_status": "NOT_CONFIRMED",
            "input": {
                "candidate_sha": before["head"],
                "upstream_sha": before["upstream_base"],
                "dirty_snapshot_sha256": before["snapshot_sha256"],
                "policy_sha256": hashlib.sha256(policy_path.read_bytes()).hexdigest(),
                "tool_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
            },
            "profiles": results,
            "plan": {"action": "NO_AUTOMATIC_PATCH", "apply_supported": False},
        }
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(json.dumps({"analysis_status": report["analysis_status"], "profiles": len(results), "candidates": sum(len(result["static_candidates"]) for result in results), "output": str(output)}))
        return 2 if report["analysis_status"] == "INCOMPLETE" else 0
    except (OSError, ValueError, KeyError, TypeError, SyntaxError, UnicodeError, subprocess.CalledProcessError, yaml.YAMLError) as error:
        print(f"INCOMPLETE: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
