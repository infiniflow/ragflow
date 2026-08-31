#!/usr/bin/env python3
"""Check that the root uv.lock metadata mirrors project dependencies."""

from __future__ import annotations

import argparse
import re
import sys
import tomllib
from pathlib import Path


_REQUIREMENT_NAME = re.compile(r"^\s*([A-Za-z0-9][A-Za-z0-9._-]*)")


def normalize_name(name: str) -> str:
    """Normalize a Python distribution name according to PEP 503."""

    return re.sub(r"[-_.]+", "-", name).lower()


def requirement_name(requirement: str) -> str:
    """Extract and normalize the distribution name from a PEP 508 string."""

    match = _REQUIREMENT_NAME.match(requirement)
    if not match:
        raise ValueError(f"cannot parse dependency requirement: {requirement!r}")
    return normalize_name(match.group(1))


def _names(values: list[object], *, source: str) -> set[str]:
    names = set()
    for value in values:
        if isinstance(value, dict):
            name = value.get("name")
            if not isinstance(name, str):
                raise ValueError(f"{source} contains an entry without a name: {value!r}")
            names.add(normalize_name(name))
        elif isinstance(value, str):
            names.add(requirement_name(value))
        else:
            raise ValueError(f"{source} contains an invalid dependency: {value!r}")
    return names


def check_metadata(project_path: Path, lock_path: Path) -> tuple[set[str], set[str], set[str]]:
    project = tomllib.loads(project_path.read_text())
    lock = tomllib.loads(lock_path.read_text())

    project_table = project.get("project")
    if not isinstance(project_table, dict):
        raise ValueError(f"{project_path} has no [project] table")
    project_name = project_table.get("name")
    if not isinstance(project_name, str):
        raise ValueError(f"{project_path} has no project.name")

    packages = lock.get("package", [])
    roots = [package for package in packages if package.get("name") == project_name]
    if len(roots) != 1:
        raise ValueError(f"{lock_path} must contain exactly one root package named {project_name!r}")
    root = roots[0]

    project_dependencies = _names(project_table.get("dependencies", []), source="project.dependencies")
    lock_dependencies = _names(root.get("dependencies", []), source="root package dependencies")
    lock_requires_dist = _names(
        root.get("metadata", {}).get("requires-dist", []),
        source="root package metadata.requires-dist",
    )
    return project_dependencies, lock_dependencies, lock_requires_dist


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project-file", type=Path, default=Path("pyproject.toml"))
    parser.add_argument("--lock-file", type=Path, default=Path("uv.lock"))
    args = parser.parse_args()

    try:
        project, lock, requires_dist = check_metadata(args.project_file, args.lock_file)
    except (OSError, tomllib.TOMLDecodeError, ValueError) as exc:
        print(f"uv lock metadata check failed: {exc}", file=sys.stderr)
        return 1

    checks = {
        "root package dependencies": (project, lock),
        "root package metadata.requires-dist": (project, requires_dist),
    }
    failures = []
    for label, (expected, actual) in checks.items():
        missing = sorted(expected - actual)
        unexpected = sorted(actual - expected)
        if missing or unexpected:
            failures.append((label, missing, unexpected))

    if failures:
        print("uv lock metadata check failed:", file=sys.stderr)
        for label, missing, unexpected in failures:
            if missing:
                print(f"  {label} missing: {', '.join(missing)}", file=sys.stderr)
            if unexpected:
                print(f"  {label} unexpected: {', '.join(unexpected)}", file=sys.stderr)
        return 1

    print(f"uv lock metadata is consistent ({len(project)} direct dependencies)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
