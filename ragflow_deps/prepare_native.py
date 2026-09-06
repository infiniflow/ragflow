"""Prepare or verify pinned linux-amd64 native resources; no application imports."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import tarfile
import tempfile
import urllib.request

BASE = Path(__file__).resolve().parent


def dependencies() -> dict:
    deps = json.loads((BASE / "native-deps.json").read_text())["dependencies"]
    go_mod = (BASE.parent / "go.mod").read_text()
    for name in ("office_oxide", "pdf_oxide"):
        expected = re.escape(f"github.com/yfedoseev/{name}/go v{deps[name]['version']}")
        if not re.search(r"^\s*" + expected + r"\s*$", go_mod, re.MULTILINE):
            raise ValueError(f"Native manifest does not match go.mod for {name}")
    return deps


def native_urls(china_mirrors: bool = False) -> list[list[str]]:
    prefix = "https://gh-proxy.com/" if china_mirrors else ""
    return [[prefix + dep["url"], dep["archive"]] for dep in dependencies().values()]


def digest(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def verify(target: Path, dep: dict) -> None:
    if target.is_symlink():
        raise ValueError(f"Native target must not be a symlink: {target}")
    for name, expected in dep["files"].items():
        path = target / name
        if not path.resolve().is_relative_to(target.resolve()) or path.is_symlink() or not path.is_file() or digest(path) != expected:
            raise ValueError(f"Missing/corrupt native {dep['version']} file: {path}; prepare in a clean target, do not reuse this cache")


def prepare_one(name: str, dep: dict, archives: Path, root: Path) -> None:
    target = root / name
    if target.exists():
        verify(target, dep)
        print(f"Verified cached {name} {dep['version']}")
        return
    archive = archives / dep["archive"]
    if not archive.is_file() or digest(archive) != dep["sha256"]:
        raise ValueError(f"Missing/corrupt archive: {archive}")
    root.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=f".{name}-", dir=root) as directory:
        staged = Path(directory) / "payload"
        staged.mkdir()
        with tarfile.open(archive) as bundle:
            for member in bundle.getmembers():
                if not (member.isfile() or member.isdir()):
                    raise ValueError(f"Unsupported native archive entry: {member.name}")
            bundle.extractall(staged, filter="data")
        verify(staged, dep)
        # No overwrite of stale resources. Concurrent prepare publishes only complete trees.
        if target.exists():
            verify(target, dep)
        else:
            staged.rename(target)
    print(f"Prepared {name} {dep['version']}")


def prepare_all(archives: Path = BASE, root: Path | None = None) -> None:
    root = root or Path.home() / "ragflow-native-libs"
    for name, dep in dependencies().items():
        prepare_one(name, dep, archives, root)


def download(dep: dict, archives: Path) -> None:
    target = archives / dep["archive"]
    if target.exists():
        if digest(target) != dep["sha256"]:
            raise ValueError(f"Corrupt archive, refusing overwrite: {target}")
        return
    archives.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".native-download-", dir=archives) as directory:
        partial = Path(directory) / dep["archive"]
        with urllib.request.urlopen(dep["url"], timeout=60) as response, partial.open("wb") as stream:
            shutil.copyfileobj(response, stream)
        if digest(partial) != dep["sha256"]:
            raise ValueError(f"Downloaded checksum mismatch: {dep['url']}")
        if target.exists():
            if digest(target) != dep["sha256"]:
                raise ValueError(f"Concurrent archive checksum mismatch: {target}")
        else:
            # Link publishes atomically without overwriting a concurrent downloader.
            target.hardlink_to(partial)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--download", action="store_true")
    parser.add_argument("--archives", type=Path, default=BASE)
    parser.add_argument("--target-root", type=Path, default=Path.home() / "ragflow-native-libs")
    parser.add_argument("--name", choices=("pdfium-static", "pdf_oxide", "office_oxide"))
    args = parser.parse_args()
    for name, dep in dependencies().items():
        if args.name and name != args.name:
            continue
        if args.check:
            verify(args.target_root / name, dep)
            print(f"Verified {name} {dep['version']}")
        else:
            if args.download:
                download(dep, args.archives)
            prepare_one(name, dep, args.archives, args.target_root)


if __name__ == "__main__":
    main()
