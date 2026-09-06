"""Content-addressed source snapshots. Integrity is not a test or a signature."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import stat
import subprocess
import tomllib


MANIFEST = "candidate.json"
OMITTED = {"docker/.env"}
BUILD_KEYS = {"platform", "node", "package_manager", "minify", "sourcemap", "mode"}
# Published protocol fixtures from accepted upstream cb93883f3f8c975eecb2fed81210effeb3bdb06f.
# A locally substituted private key is NOT allowed by this exception.
UPSTREAM_PUBLIC_FIXTURES = {
    "conf/private.pem": "41cf8fb7e403f76f884e516392e12c2c8cc9c7b45d3ff1dc70359ee320b16ee5",
    "conf/public.pem": "6745cb2fb7215142aca36a2b0b7d4794a7a21894b991263c715fa26367f11e8d",
}


def canonical(value: object) -> bytes:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode()


def digest(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def file_digest(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def _relative_parts(name: str) -> tuple[str, ...]:
    relative = PurePosixPath(name)
    if not name or "\\" in name or relative.is_absolute() or any(p in {"..", "."} or ":" in p for p in relative.parts):
        raise ValueError(f"unsafe relative path: {name!r}")
    return relative.parts


def safe_path(root: Path, name: str) -> Path:
    if root.is_symlink() or root.is_junction():
        raise ValueError("linked root")
    path = root.joinpath(*_relative_parts(name))
    for parent in (path, *path.parents):
        if parent == root:
            break
        if parent.is_symlink() or parent.is_junction():
            raise ValueError(f"linked path: {name}")
    if not path.resolve().is_relative_to(root.resolve()):
        raise ValueError(f"path escapes root: {name}")
    return path


def tree_files(root: Path, excluded_dirs: frozenset[str] = frozenset()) -> dict:
    """Hash a regular, unlinked tree; exclusions are exact relative directories.

    Check ancestors once and each encountered entry once, rather than resolving
    every file's full ancestry repeatedly. Inputs must stay stable while read;
    this is not protection against a hostile concurrent filesystem writer.
    """
    _check_artifact_path(root)
    if not root.is_dir():
        raise ValueError("source tree root must be a directory")
    result = {}

    def failed_scan(error):
        raise error

    for directory, dirs, files in os.walk(root, followlinks=False, onerror=failed_scan):
        parent = Path(directory)
        relative_dir = parent.relative_to(root)
        dirs[:] = [name for name in dirs if (relative_dir / name).as_posix() not in excluded_dirs]
        for name in [*dirs, *files]:
            path = parent / name
            relative = (relative_dir / name).as_posix()
            _relative_parts(relative)
            info = path.lstat()
            if stat.S_ISLNK(info.st_mode) or getattr(info, "st_reparse_tag", 0) == getattr(stat, "IO_REPARSE_TAG_MOUNT_POINT", None):
                raise ValueError(f"linked source path: {relative}")
            if stat.S_ISREG(info.st_mode):
                result[relative] = {"sha256": file_digest(path), "size": path.stat().st_size}
            elif not stat.S_ISDIR(info.st_mode):
                raise ValueError(f"nonregular source path: {relative}")
    return result


def _git(root: Path, *args: str) -> bytes:
    return subprocess.check_output(["git", "-C", str(root), *args])


def _private(name: str) -> bool:
    path = PurePosixPath(name)
    return (
        any(p in {".git", ".venv", "node_modules", "__pycache__", "output"} for p in path.parts)
        or path.name.endswith((".pem", ".key", ".p12", ".pfx"))
        or (path.name.startswith(".env") and name not in {"web/.env", "web/.env.development", "web/.env.production"} and not path.name.endswith((".example", "-example")))
        or path.name.startswith("registry-images-")
    )


def snapshot(root: Path, destination: Path, include: list[str], allow_dirty: bool, build: dict) -> dict:
    _check_artifact_path(root)
    _check_artifact_path(destination)
    root = root.resolve()
    destination = destination.resolve()
    if destination.is_relative_to(root) or root.is_relative_to(destination):
        raise ValueError("candidate must be outside the repository and its ancestors")
    if build.keys() - BUILD_KEYS or build.get("platform") != "linux/amd64":
        raise ValueError("explicit linux/amd64 build profile and only allowlisted build parameters required")
    status = _git(root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
    if status and not allow_dirty:
        raise ValueError("dirty repository: commit first or explicitly use --allow-dirty for a non-publishable snapshot")
    tracked = {p.decode("utf-8") for p in _git(root, "ls-files", "-z").split(b"\0") if p}
    untracked = {p.decode("utf-8") for p in _git(root, "ls-files", "--others", "--exclude-standard", "-z").split(b"\0") if p}
    if set(include) - untracked:
        raise ValueError("--include must name exact non-ignored untracked files")
    selected = sorted((tracked | set(include)) - OMITTED)
    entries = {}
    for name in selected:
        path = safe_path(root, name)
        if _private(name) and (name not in UPSTREAM_PUBLIC_FIXTURES or digest(path.read_bytes().replace(b"\r\n", b"\n")) != UPSTREAM_PUBLIC_FIXTURES[name]):
            raise ValueError(f"private/generated source path requires policy review: {name}")
        if not path.exists() and name in tracked:
            continue  # Intentional tracked deletion is represented by absence.
        if not path.is_file():
            raise ValueError(f"not a regular source file: {name}")
        entries[name] = {"sha256": file_digest(path), "size": path.stat().st_size}
    version = tomllib.loads((root / "pyproject.toml").read_text(encoding="utf-8"))["project"]["version"]
    identity = {
        "schema": 1,
        "version": version,
        "head": _git(root, "rev-parse", "HEAD").decode().strip(),
        "dirty": bool(status),
        "included_untracked": sorted(include),
        "omitted": sorted(OMITTED & tracked),
        "build": build,
        "files": entries,
    }
    manifest = {"identity": identity, "source_id": digest(canonical(identity))}
    destination.mkdir(parents=True, exist_ok=False)
    source = destination / "source"
    source.mkdir()
    # On interruption the incomplete directory stays inspectable, without a manifest.
    for name, expected in entries.items():
        target = safe_path(source, name)
        target.parent.mkdir(parents=True, exist_ok=True)
        data = safe_path(root, name).read_bytes()
        if digest(data) != expected["sha256"]:
            raise ValueError(f"source changed during snapshot: {name}")
        target.write_bytes(data)
    if status != _git(root, "status", "--porcelain=v1", "-z", "--untracked-files=all"):
        raise ValueError("repository status changed during snapshot")
    for name, expected in entries.items():
        if file_digest(safe_path(root, name)) != expected["sha256"]:
            raise ValueError(f"source changed during snapshot: {name}")
    (destination / MANIFEST).write_bytes(canonical(manifest) + b"\n")
    verify(destination)
    return manifest


def verify(candidate: Path) -> dict:
    _check_artifact_path(candidate)
    candidate = candidate.resolve()
    manifest = json.loads((candidate / MANIFEST).read_bytes())
    identity = manifest["identity"]
    if identity.get("schema") != 1 or digest(canonical(identity)) != manifest["source_id"]:
        raise ValueError("candidate identity mismatch")
    source = candidate / "source"
    for name in identity["files"]:
        _relative_parts(name)
    actual = tree_files(source)
    if actual.keys() != identity["files"].keys():
        raise ValueError("source file set changed")
    for name, expected in identity["files"].items():
        if actual[name]["sha256"] != expected["sha256"] or actual[name]["size"] != expected["size"]:
            raise ValueError(f"source integrity mismatch: {name}")
    version = tomllib.loads((source / "pyproject.toml").read_text(encoding="utf-8"))["project"]["version"]
    if version != identity["version"]:
        raise ValueError("version mismatch")
    return manifest


def attest_artifact(candidate: Path, artifact: Path, name: str) -> dict:
    """Record bytes, not successful validation. Callers must separately require gates."""
    manifest = verify(candidate)
    if not name or not all(c.isalnum() or c in "-_" for c in name):
        raise ValueError("invalid artifact name")
    _check_artifact_path(artifact)
    if not artifact.is_file():
        raise ValueError("artifact must be a regular file")
    receipt = {"schema": 1, "source_id": manifest["source_id"], "name": name, "size": artifact.stat().st_size, "sha256": file_digest(artifact)}
    target = candidate / f"artifact-{name}.json"
    if target.exists() and json.loads(target.read_bytes()) != receipt:
        raise ValueError("artifact identity conflict; use a new candidate, not an overwrite")
    if not target.exists():
        with target.open("xb") as stream:
            stream.write(canonical(receipt) + b"\n")
    return receipt


def verify_artifact(candidate: Path, artifact: Path, name: str) -> dict:
    _check_artifact_path(artifact)
    manifest = verify(candidate)
    receipt = json.loads(safe_path(candidate, f"artifact-{name}.json").read_bytes())
    if receipt["source_id"] != manifest["source_id"] or receipt["name"] != name:
        raise ValueError("artifact belongs to another candidate")
    if artifact.is_symlink() or not artifact.is_file() or receipt["sha256"] != file_digest(artifact) or receipt["size"] != artifact.stat().st_size:
        raise ValueError("artifact integrity mismatch")
    return receipt


def _check_artifact_path(artifact: Path) -> None:
    for path in (artifact, *artifact.parents):
        if path.is_symlink() or path.is_junction():
            raise ValueError("linked artifact path")


def compare_installed(incoming: dict, installed: dict) -> str:
    if incoming["identity"]["version"] == installed["identity"]["version"]:
        if incoming["source_id"] != installed["source_id"]:
            raise ValueError("same version with different source identity")
        return "same"
    return "upgrade"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["snapshot", "verify", "record-artifact", "verify-artifact"])
    parser.add_argument("--candidate", type=Path, required=True)
    parser.add_argument("--root", type=Path, default=Path.cwd())
    parser.add_argument("--include", action="append", default=[])
    parser.add_argument("--allow-dirty", action="store_true")
    parser.add_argument("--build-profile", type=Path)
    parser.add_argument("--artifact", type=Path)
    parser.add_argument("--name")
    args = parser.parse_args()
    try:
        if args.action == "snapshot":
            if args.build_profile is None:
                raise ValueError("--build-profile JSON is required; never pass environment dumps")
            result = snapshot(args.root, args.candidate, args.include, args.allow_dirty, json.loads(args.build_profile.read_bytes()))
        elif args.action == "verify":
            result = verify(args.candidate)
        else:
            if args.artifact is None or args.name is None:
                raise ValueError("--artifact and --name required")
            function = attest_artifact if args.action == "record-artifact" else verify_artifact
            result = function(args.candidate, args.artifact, args.name)
        print(json.dumps({"source_id": result["source_id"], "integrity": "PASS", "validation": "NOT_ASSERTED"}))
        return 0
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as exc:
        print(f"INCOMPLETE: {exc}")
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
