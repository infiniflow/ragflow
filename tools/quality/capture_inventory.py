"""Capture T0 Git provenance; does not check architecture or modify application code."""

from __future__ import annotations

import argparse
from collections import Counter
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess

import yaml


GENERATED = {"tools/quality/file-inventory.json", "tools/quality/core-changes.yaml"}


def git(repo: Path, *args: str) -> bytes:
    return subprocess.run(["git", "-c", "core.quotepath=false", *args], cwd=repo, check=True, capture_output=True).stdout


def paths(raw: bytes) -> list[str]:
    return [p.decode("utf-8", "surrogateescape") for p in raw.split(b"\0") if p]


def tree(repo: Path, ref: str) -> dict[str, dict]:
    result = {}
    for item in git(repo, "ls-tree", "-rz", "--full-tree", ref).split(b"\0"):
        if item:
            metadata, name = item.split(b"\t", 1)
            mode, kind, oid = metadata.decode().split()
            result[name.decode("utf-8", "surrogateescape")] = {"mode": mode, "kind": kind, "oid": oid}
    return result


def changes(repo: Path, *refs: str) -> dict[str, str]:
    parts = paths(git(repo, "diff", "--no-ext-diff", "--no-renames", "--name-status", "-z", *refs, "--"))
    if len(parts) % 2:
        raise ValueError("Unexpected NUL-separated Git diff")
    return dict(zip(parts[1::2], parts[::2], strict=True))


def safe_path(repo: Path, name: str) -> Path:
    target = repo / name
    if target.is_symlink() or not target.resolve().is_relative_to(repo.resolve()):
        raise ValueError(f"Cannot fingerprint external/symlink target: {name}")
    return target


def fingerprint(repo: Path, name: str) -> dict:
    target = safe_path(repo, name)
    if not target.exists():
        return {"present": False}
    if not target.is_file():
        raise ValueError(f"Expected regular file: {name}")
    if name in GENERATED:
        return {"present": True, "digest_policy": "self-generated manifest; excluded from recursive hashing"}
    raw = target.read_bytes()
    return {"present": True, "bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest()}


def capture(repo: Path, base_config: dict, module_map: dict) -> dict:
    base = base_config["commit"]
    head = git(repo, "rev-parse", "HEAD").decode().strip()
    status_before = git(repo, "status", "--porcelain=v1", "-z", "--untracked-files=all")
    base_tree, head_tree = tree(repo, base), tree(repo, head)
    if git(repo, "rev-parse", f"{base}^{{tree}}").decode().strip() != base_config["tree"]:
        raise ValueError("Upstream tree differs from verified evidence")
    git(repo, "merge-base", "--is-ancestor", base, head)
    committed = changes(repo, base, head)
    staged = changes(repo, "--cached", head)
    unstaged = changes(repo)
    current = changes(repo, base)
    untracked = set(paths(git(repo, "ls-files", "--others", "--exclude-standard", "-z")))
    entries = {}
    index_before = git(repo, "ls-files", "--stage", "-z")
    for entry in index_before.split(b"\0"):
        if not entry:
            continue
        metadata, path = entry.split(b"\t", 1)
        mode, oid, stage = metadata.decode().split()
        if stage != "0":
            raise ValueError("Unmerged index: inventory cannot be certified")
        if mode == "160000":
            raise ValueError("Gitlinks require an explicit nested repository inventory")
        entries[path.decode("utf-8", "surrogateescape")] = {"mode": mode, "oid": oid}
    assigned = {}
    for module in module_map["modules"]:
        for path in module["paths"]:
            if path in assigned:
                raise ValueError(f"Duplicate module assignment: {path}")
            assigned[path] = module["id"]
    candidates = set(committed) | set(staged) | set(unstaged) | set(current) | untracked | GENERATED
    missing = candidates - assigned.keys()
    if missing:
        raise ValueError("Unclassified paths: " + ", ".join(sorted(missing)))
    extra = assigned.keys() - candidates
    if extra:
        raise ValueError("Stale registry paths: " + ", ".join(sorted(extra)))
    records = []
    for path in sorted(candidates):
        origin = "core_change" if path in base_tree else "extension"
        record = {
            "path": path,
            "module": assigned[path],
            "origin": origin,
            "base_entry": base_tree.get(path),
            "head_entry": head_tree.get(path),
            "index_entry": entries.get(path),
            "committed_change": committed.get(path),
            "staged_change": staged.get(path),
            "unstaged_change": unstaged.get(path),
            "working_change_from_base": current.get(path, "A" if path in untracked else None),
            "untracked": path in untracked,
            "working_file": fingerprint(repo, path),
        }
        records.append(record)
    status_after = git(repo, "status", "--porcelain=v1", "-z", "--untracked-files=all")
    if status_after != status_before or git(repo, "rev-parse", "HEAD").decode().strip() != head:
        raise ValueError("Working tree/index changed during capture; retry after inspection")
    for record in records:
        if fingerprint(repo, record["path"]) != record["working_file"]:
            raise ValueError(f"Concurrent edit: {record['path']}")
    if git(repo, "ls-files", "--stage", "-z") != index_before:
        raise ValueError("Index contents changed during capture; retry after inspection")
    input_records = [{"path": r["path"], "file": r["working_file"], "index": r["index_entry"]} for r in records if r["path"] not in GENERATED]
    return {
        "schema_version": 1,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "head": head,
        "upstream_base": base,
        "branch": git(repo, "branch", "--show-current").decode().strip(),
        "shallow": git(repo, "rev-parse", "--is-shallow-repository").decode().strip() == "true",
        "snapshot_sha256": hashlib.sha256(json.dumps(input_records, ensure_ascii=True, sort_keys=True).encode()).hexdigest(),
        "scope": "All tracked deltas from the content baseline, staged/unstaged changes and nonignored untracked files; generated outputs explicitly listed without recursive digest",
        "rename_policy": "Renames represented by explicit delete/add paths; no paths discarded by similarity heuristics",
        "counts": {
            "base_files": len(base_tree),
            "head_files": len(head_tree),
            "index_files": len(entries),
            "committed_delta": len(committed),
            "staged_delta": len(staged),
            "unstaged_delta": len(unstaged),
            "untracked": len(untracked),
            "inventory_records": len(records),
            "by_origin": dict(Counter(r["origin"] for r in records)),
            "by_module": dict(Counter(r["module"] for r in records)),
            "unchanged_upstream_index_paths": len(set(entries) & set(base_tree) - candidates),
            "unclassified": 0,
        },
        "records": records,
    }


def core_changes(snapshot: dict, module_map: dict) -> dict:
    groups = []
    for module in module_map["modules"]:
        selected = [r for r in snapshot["records"] if r["module"] == module["id"] and r["origin"] == "core_change"]
        if selected:
            groups.append(
                {
                    "id": "CORE-" + module["id"],
                    "owner": module["owner"],
                    "reason": module["core_change_reason"],
                    "contract": module["preserve"],
                    "tests": module["tests"],
                    "validation": "Paths and provenance verified in T0; behavior and necessity of individual hunks await T1/review",
                    "upstream_status": "Local delta against the verified content baseline; not certified for newer upstream",
                    "paths": [{"path": r["path"], "committed": r["committed_change"], "staged": r["staged_change"], "unstaged": r["unstaged_change"]} for r in selected],
                }
            )
    return {"schema_version": 1, "upstream_base": snapshot["upstream_base"], "head": snapshot["head"], "changes": groups}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write", action="store_true", help="Refresh only file-inventory.json and core-changes.yaml")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[2]
    folder = repo / "tools/quality"
    base = json.loads((folder / "upstream-base.json").read_text(encoding="utf-8"))
    modules = yaml.safe_load((folder / "module-map.yaml").read_text(encoding="utf-8"))
    snapshot = capture(repo, base, modules)
    if args.write:
        (folder / "file-inventory.json").write_text(json.dumps(snapshot, ensure_ascii=True, indent=2) + "\n", encoding="utf-8")
        (folder / "core-changes.yaml").write_text(yaml.safe_dump(core_changes(snapshot, modules), allow_unicode=True, sort_keys=False), encoding="utf-8")
    print(json.dumps({"head": snapshot["head"], "snapshot_sha256": snapshot["snapshot_sha256"], "counts": snapshot["counts"]}, indent=2))


if __name__ == "__main__":
    main()
