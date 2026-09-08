"""Capture T0 Git provenance; does not check architecture or modify application code."""

from __future__ import annotations

import argparse
from collections import Counter
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
from pathlib import PurePosixPath
import subprocess

import yaml


GENERATED = {"tools/quality/file-inventory.json", "tools/quality/core-changes.yaml"}
IGNORED_SCOPES = ["deployment/linux-pg", "agent/business_requirements", "services/asr-online-service"]
IGNORED_DIRECTORY_PATTERNS = [
    "**/__pycache__",
    "**/.pytest_cache",
    "**/.venv",
    "services/asr-online-service/artifacts",
    "services/asr-online-service/uploads",
    "deployment/linux-pg/release-*/stage-*",
    "deployment/linux-pg/release-*/validation",
]
HASHED_IGNORED_SUFFIXES = {".puml", ".ps1", ".sh", ".sha256"}
HASHED_IGNORED_NAMES = {"uv.lock"}


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
                    "validation": "Paths and provenance verified in T0; behavior evidence and remaining limits are tracked by T1/T2 reports and focused review",
                    "upstream_status": "Local delta against the verified content baseline; not certified for newer upstream",
                    "paths": [{"path": r["path"], "committed": r["committed_change"], "staged": r["staged_change"], "unstaged": r["unstaged_change"]} for r in selected],
                }
            )
    return {"schema_version": 1, "upstream_base": snapshot["upstream_base"], "head": snapshot["head"], "changes": groups}


def _first_parent_commits(repo: Path, base: str, head: str) -> list[dict]:
    git(repo, "merge-base", "--is-ancestor", base, head)
    shas = git(repo, "rev-list", "--first-parent", "--reverse", f"{base}..{head}").decode().splitlines()
    commits = []
    for sha in shas:
        metadata = git(repo, "show", "-s", "--format=%P%x00%T%x00%cI%x00%s", sha).decode().rstrip("\n").split("\0")
        if len(metadata) != 4:
            raise ValueError(f"Unexpected commit metadata for {sha}")
        parents = metadata[0].split()
        if not parents:
            raise ValueError(f"Fork commit has no first parent: {sha}")
        commits.append(
            {
                "sha": sha,
                "parents": parents,
                "tree": metadata[1],
                "committed_at": metadata[2],
                "subject": metadata[3],
                "changes_from_first_parent": changes(repo, parents[0], sha),
            }
        )
    return commits


def refresh_upstream_evidence(repo: Path, base_config: dict, existing: dict) -> dict:
    head = git(repo, "rev-parse", "HEAD").decode().strip()
    local_refs = (
        git(
            repo,
            "for-each-ref",
            "--format=%(refname) %(objectname)",
            "refs/remotes/origin",
            "refs/remotes/upstream",
        )
        .decode()
        .splitlines()
    )
    graph_merge_base = existing.get("graph_merge_base")
    try:
        graph_merge_base = git(repo, "merge-base", head, "refs/remotes/upstream/main").decode().strip()
    except subprocess.CalledProcessError:
        pass
    return {
        "schema_version": 1,
        "verified_at": datetime.now(timezone.utc).isoformat(),
        "head": head,
        "official_commits": existing["official_commits"],
        "first_parent_fork_commits": _first_parent_commits(repo, base_config["commit"], head),
        "local_refs_only_no_fetch": local_refs,
        "graph_merge_base": graph_merge_base,
        "baseline_diff_summary": git(repo, "diff", "--shortstat", base_config["commit"], head).decode().strip(),
        "ancestry_only_merge": existing["ancestry_only_merge"],
        "limitation": "Official commit evidence is retained from its recorded source retrieval; local history and remote-tracking refs were refreshed without network fetch. Shallow history still limits earlier local ancestry. No upstream update performed.",
    }


def _ignored_directory(path: str) -> bool:
    parts = PurePosixPath(path).parts
    if any(part in {"__pycache__", ".pytest_cache", ".venv"} for part in parts):
        return True
    if path.startswith(("services/asr-online-service/artifacts/", "services/asr-online-service/uploads/")):
        return True
    return len(parts) >= 4 and parts[:2] == ("deployment", "linux-pg") and parts[2].startswith("release-") and (parts[3].startswith("stage-") or parts[3] == "validation")


def refresh_ignored_artifacts(repo: Path) -> dict:
    ignored = paths(git(repo, "ls-files", "--others", "--ignored", "--exclude-standard", "-z", "--", *IGNORED_SCOPES))
    records = []
    for name in sorted(path for path in ignored if not _ignored_directory(path)):
        target = safe_path(repo, name)
        if not target.is_file():
            raise ValueError(f"Expected regular ignored file: {name}")
        stat = target.stat()
        record = {
            "path": name,
            "bytes": stat.st_size,
            "modified_at": datetime.fromtimestamp(stat.st_mtime, timezone.utc).isoformat(),
        }
        path = PurePosixPath(name)
        if path.suffix.lower() in HASHED_IGNORED_SUFFIXES or path.name in HASHED_IGNORED_NAMES:
            record.update(
                {
                    "kind": "ignored_source_or_document",
                    "sha256": hashlib.sha256(target.read_bytes()).hexdigest(),
                }
            )
        else:
            record.update(
                {
                    "kind": "ignored_artifact_or_local_config",
                    "digest_policy": "metadata only; contents not read or backed up",
                }
            )
        records.append(record)
    return {
        "schema_version": 1,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "scopes": IGNORED_SCOPES,
        "scope_policy": "Bounded scan of extension/release roots, untracked ignored files only. Unpacked release trees, runtime outputs, dependencies and caches are excluded by policy; tracked files remain in file-inventory.json.",
        "excluded_directory_patterns": IGNORED_DIRECTORY_PATTERNS,
        "records": records,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write", action="store_true", help="Refresh only file-inventory.json and core-changes.yaml")
    parser.add_argument(
        "--refresh-supporting",
        action="store_true",
        help="With --write, refresh upstream-evidence.json and ignored-artifacts.json before the main snapshot",
    )
    args = parser.parse_args()
    if args.refresh_supporting and not args.write:
        parser.error("--refresh-supporting requires --write")
    repo = Path(__file__).resolve().parents[2]
    folder = repo / "tools/quality"
    base = json.loads((folder / "upstream-base.json").read_text(encoding="utf-8"))
    modules = yaml.safe_load((folder / "module-map.yaml").read_text(encoding="utf-8"))
    if args.refresh_supporting:
        existing_evidence = json.loads((folder / "upstream-evidence.json").read_text(encoding="utf-8"))
        evidence = refresh_upstream_evidence(repo, base, existing_evidence)
        ignored = refresh_ignored_artifacts(repo)
        (folder / "upstream-evidence.json").write_text(json.dumps(evidence, ensure_ascii=True, indent=2) + "\n", encoding="utf-8")
        (folder / "ignored-artifacts.json").write_text(json.dumps(ignored, ensure_ascii=True, indent=2) + "\n", encoding="utf-8")
    snapshot = capture(repo, base, modules)
    if args.write:
        (folder / "file-inventory.json").write_text(json.dumps(snapshot, ensure_ascii=True, indent=2) + "\n", encoding="utf-8")
        (folder / "core-changes.yaml").write_text(yaml.safe_dump(core_changes(snapshot, modules), allow_unicode=True, sort_keys=False), encoding="utf-8")
    print(json.dumps({"head": snapshot["head"], "snapshot_sha256": snapshot["snapshot_sha256"], "counts": snapshot["counts"]}, indent=2))


if __name__ == "__main__":
    main()
