"""Explicit, single web dependency install before parallel hooks; never a hook."""

from __future__ import annotations

import argparse
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
WEB_SUFFIXES = {".css", ".less", ".json", ".js", ".jsx", ".ts", ".tsx"}


def needs_web(raw: bytes) -> bool:
    return any(name.startswith(b"web/") and Path(os.fsdecode(name)).suffix in WEB_SUFFIXES for name in raw.split(b"\0") if name)


def prepare(root: Path) -> int:
    npm = shutil.which("npm")
    if not npm:
        raise RuntimeError("npm is required for web dependency preparation")
    key = hashlib.sha256(os.fsencode(str(root.resolve()))).hexdigest()[:24]
    lock = Path(tempfile.gettempdir()) / f"ragflow-web-prepare-{key}.lock"
    try:
        handle = os.open(lock, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError as exc:
        raise RuntimeError(f"Another preparation owns {lock}; check its process before removing a stale lock") from exc
    try:
        with os.fdopen(handle, "w") as stream:
            stream.write(str(os.getpid()))
        # Keep dependency lifecycle scripts (e.g. esbuild), but do not install Git hooks.
        return subprocess.run([npm, "ci", "--prefix", "web", "--no-audit", "--no-fund"], cwd=root, env=dict(os.environ, LEFTHOOK="0"), check=False).returncode
    finally:
        lock.unlink()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--files-from", type=Path, help="NUL-separated changed paths; omit to prepare web explicitly")
    args = parser.parse_args()
    if args.files_from and not needs_web(args.files_from.read_bytes()):
        print("No web hook inputs; dependency preparation not applicable")
        return 0
    return prepare(ROOT)


if __name__ == "__main__":
    raise SystemExit(main())
