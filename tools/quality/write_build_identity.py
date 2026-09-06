"""Write runtime VERSION metadata without shipping Git history into Docker.

Callers pass git describe and rev-parse from their checkout. Missing version uses
pyproject's version with '-unverified'; missing revision is 'unverified'. Supplied
metadata is not a signature, clean-tree assertion, or successful test evidence.
"""

from __future__ import annotations

import argparse
from pathlib import Path
import re
import tomllib


def write(root: Path, version: str = "", revision: str = "") -> tuple[str, str]:
    if not version:
        version = str(tomllib.loads((root / "pyproject.toml").read_text(encoding="utf-8"))["project"]["version"]) + "-unverified"
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._+-]{0,127}", version):
        raise ValueError("build version must be 1-128 safe ASCII version characters")
    if revision and not re.fullmatch(r"[0-9a-fA-F]{40}", revision):
        raise ValueError("source revision must be empty or an exact 40-hex Git commit")
    revision = revision.lower() if revision else "unverified"
    (root / "VERSION").write_text(version + "\n", encoding="utf-8", newline="\n")
    (root / "SOURCE_REVISION").write_text(revision + "\n", encoding="utf-8", newline="\n")
    return version, revision


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path.cwd())
    parser.add_argument("--version", default="")
    parser.add_argument("--revision", default="")
    args = parser.parse_args()
    try:
        version, revision = write(args.root, args.version, args.revision)
        print(f"RAGFlow version: {version}; source revision: {revision} (metadata only)")
        return 0
    except (OSError, ValueError, KeyError) as exc:
        print(f"Invalid build identity: {exc}")
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
