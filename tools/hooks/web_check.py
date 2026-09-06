"""Run installed web hook tools without network access or dependency mutation."""

from __future__ import annotations

import argparse
import os
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[2]
TOOLS = {"prettier": "prettier/bin/prettier.cjs", "eslint": "eslint/bin/eslint.js"}


def run(tool: str, files: list[str], root: Path = ROOT) -> int:
    web = root / "web"
    entry = web / "node_modules" / TOOLS[tool]
    node = shutil.which("node")
    if not node or not entry.is_file():
        raise RuntimeError("Web tools missing: run python3 tools/hooks/prepare_web.py before hooks")
    paths = [str((root / name).resolve().relative_to(web.resolve())) for name in files]
    if not paths:
        return 0
    check_only = os.environ.get("LEFTHOOK_CHECK_ONLY") == "1"
    flags = ["--check" if check_only else "--write", "--ignore-unknown"] if tool == "prettier" else ([] if check_only else ["--fix"])
    return subprocess.run([node, str(entry), *flags, "--", *paths], cwd=web, check=False).returncode


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tool", choices=TOOLS)
    parser.add_argument("files", nargs="+")
    args = parser.parse_args()
    raise SystemExit(run(args.tool, args.files))
