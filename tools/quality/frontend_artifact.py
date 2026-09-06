"""Build a candidate SPA once in a new workspace; verify reusable bytes, not tests.

The receipt is unsigned provenance from this local runner, not a trusted attestation.
Build scripts are trusted candidate code, not sandboxed. No live app is touched.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import signal
import subprocess
import tarfile

try:
    from . import candidate as identity
except ImportError:
    import candidate as identity


def _unlinked(path: Path) -> None:
    for part in (path, *path.parents):
        if part.is_symlink() or part.is_junction():
            raise ValueError("linked frontend artifact/workspace path")


def _profile(value: dict) -> dict:
    if set(value) != identity.BUILD_KEYS:
        raise ValueError("frontend requires the complete explicit candidate build profile")
    if value["platform"] != "linux/amd64" or value["mode"] != "production":
        raise ValueError("frontend target must be linux/amd64 production")
    if not re.fullmatch(r"v?\d+\.\d+\.\d+", str(value["node"])) or not re.fullmatch(r"pnpm@\d+\.\d+\.\d+", str(value["package_manager"])):
        raise ValueError("exact node version and pnpm@version are required")
    if value["minify"] not in ("esbuild", "terser", "false") or type(value["sourcemap"]) is not bool:
        raise ValueError("minify must be esbuild/terser/false and sourcemap must be boolean")
    return value


def _tools(pnpm_cli: Path | None = None) -> tuple[list[str], list[str]]:
    node = shutil.which("node")
    if not node:
        raise ValueError("node executable is required")
    if pnpm_cli is not None:
        _unlinked(pnpm_cli)
        if pnpm_cli.suffix.lower() not in {".cjs", ".js"} or not pnpm_cli.is_file():
            raise ValueError("--pnpm-cli requires an existing regular .cjs/.js entrypoint")
        return [node], [node, str(pnpm_cli.resolve())]
    pnpm = shutil.which("pnpm")
    if not pnpm:
        raise ValueError("pnpm executable or --pnpm-cli is required")
    return [node], [pnpm]


def _run(command: list[str], cwd: Path, env: dict, log: Path, timeout: int) -> str:
    with log.open("wb") as stream:
        process = subprocess.Popen(command, cwd=cwd, env=env, stdout=stream, stderr=subprocess.STDOUT, start_new_session=os.name != "nt")
        try:
            code = process.wait(timeout=timeout)
        except BaseException:
            if os.name == "nt":
                subprocess.run(["taskkill", "/PID", str(process.pid), "/T", "/F"], capture_output=True, check=False)
            else:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            process.wait(timeout=10)
            raise
    if code:
        raise ValueError(f"frontend command failed ({code}); inspect {log.name}")
    # Only version calls need captured text; callers do not print build logs.
    return log.read_text(encoding="utf-8", errors="replace").strip() if log.name.endswith("version.log") else ""


def _file_map(root: Path, *, source: bool = False) -> dict:
    return identity.tree_files(root, frozenset({"web/node_modules", "web/dist"}) if source else frozenset())


def _check_inputs(work_source: Path, manifest: dict) -> None:
    if _file_map(work_source, source=True) != manifest["identity"]["files"]:
        raise ValueError("frontend source inputs changed in build workspace")


def _environment(work: Path, profile: dict) -> dict:
    # No inherited provider credentials, proxy tokens, NODE_OPTIONS or VITE_*.
    allowed = {"PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "OS", "PROCESSOR_ARCHITECTURE", "NUMBER_OF_PROCESSORS"}
    env = {key: value for key, value in os.environ.items() if key.upper() in allowed}
    home = work / "home"
    temp = work / "tmp"
    home.mkdir()
    temp.mkdir()
    config = home / ".npmrc"
    config.write_text("", encoding="utf-8")
    env.update(
        HOME=str(home),
        USERPROFILE=str(home),
        APPDATA=str(home / "AppData/Roaming"),
        LOCALAPPDATA=str(home / "AppData/Local"),
        TMP=str(temp),
        TEMP=str(temp),
        TMPDIR=str(temp),
        NPM_CONFIG_USERCONFIG=str(config),
        CI="1",
        LEFTHOOK="0",
        NODE_OPTIONS="--max-old-space-size=8192",
        NODE_ENV="production",
        VITE_MINIFY=profile["minify"],
        VITE_BUILD_SOURCEMAP=str(profile["sourcemap"]).lower(),
    )
    return env


def build(candidate: Path, work: Path, output: Path, pnpm_cli: Path | None = None) -> dict:
    """Produce output tar.gz and output.tar.gz.json; never accept an existing dist."""
    for path in (candidate, work, output):
        _unlinked(path)
    candidate, work, output = candidate.resolve(), work.resolve(), output.resolve()
    if work.is_relative_to(candidate) or candidate.is_relative_to(work) or output.is_relative_to(candidate) or output.is_relative_to(work):
        raise ValueError("candidate, work and artifact must have separate paths")
    receipt_path = Path(str(output) + ".json")
    if work.exists() or output.exists() or receipt_path.exists():
        raise ValueError("fresh work directory and artifact paths required; no overwrite")
    manifest = identity.verify(candidate)
    profile = _profile(manifest["identity"]["build"])
    if any(name.startswith(("web/dist/", "web/node_modules/")) for name in manifest["identity"]["files"]):
        raise ValueError("candidate must not contain prebuilt frontend or dependencies")
    for required in ("web/package.json", "web/pnpm-lock.yaml"):
        if required not in manifest["identity"]["files"]:
            raise ValueError(f"frontend source missing {required}")
    node, pnpm = _tools(pnpm_cli)
    work.mkdir(parents=True, exist_ok=False)
    source = work / "source"
    shutil.copytree(candidate / "source", source)
    _check_inputs(source, manifest)
    env = _environment(work, profile)
    versions = {
        "node": _run([*node, "--version"], source / "web", env, work / "node-version.log", 30).removeprefix("v"),
        "pnpm": _run([*pnpm, "--version"], source / "web", env, work / "pnpm-version.log", 30),
    }
    if versions["node"] != profile["node"].removeprefix("v") or versions["pnpm"] != profile["package_manager"].removeprefix("pnpm@"):
        raise ValueError("installed frontend toolchain does not match candidate profile")
    _run([*pnpm, "install", "--frozen-lockfile", "--ignore-scripts", "--prod=false", "--store-dir", str(work / "pnpm-store")], source / "web", env, work / "install.log", 1200)
    _check_inputs(source, manifest)
    _run([*pnpm, "exec", "vite", "build", "--mode", profile["mode"]], source / "web", env, work / "build.log", 1200)
    _check_inputs(source, manifest)
    current = identity.verify(candidate)
    if current["source_id"] != manifest["source_id"]:
        raise ValueError("candidate changed during frontend build")
    dist = source / "web/dist"
    files = _file_map(dist)
    if "index.html" not in files or not files["index.html"]["size"]:
        raise ValueError("frontend build did not produce a nonempty dist/index.html")
    output.parent.mkdir(parents=True, exist_ok=True)
    # Exclusive creation prevents concurrent writers from replacing published bytes.
    with output.open("xb") as stream:
        with tarfile.open(fileobj=stream, mode="w:gz") as archive:
            for name in sorted(files):
                archive.add(identity.safe_path(dist, name), arcname=name, recursive=False)
    receipt = {
        "schema": 1,
        "kind": "frontend-build",
        "source_id": manifest["source_id"],
        "profile": profile,
        "toolchain": versions,
        "build_host": {"system": platform.system(), "machine": platform.machine()},
        "archive": {"sha256": identity.file_digest(output), "size": output.stat().st_size},
        "files": files,
        "validation": "NOT_ASSERTED",
    }
    receipt["receipt_id"] = identity.digest(identity.canonical(receipt))
    # Verify the archive before publishing its completion receipt.
    _verify_archive(output, receipt)
    with receipt_path.open("xb") as stream:
        stream.write(identity.canonical(receipt) + b"\n")
    return verify(candidate, output, receipt_path)


def _verify_archive(archive: Path, receipt: dict) -> None:
    if receipt["archive"] != {"sha256": identity.file_digest(archive), "size": archive.stat().st_size}:
        raise ValueError("frontend archive integrity mismatch")
    files = {}
    with tarfile.open(archive, mode="r:gz") as bundle:
        for member in bundle:
            # The builder emits regular files only; no directories/links/duplicates.
            identity.safe_path(archive.parent, member.name)
            if not member.isfile() or member.name in files:
                raise ValueError("unsupported frontend archive entry")
            stream = bundle.extractfile(member)
            files[member.name] = {"sha256": hashlib.file_digest(stream, "sha256").hexdigest(), "size": member.size}
    if files != receipt["files"] or not files.get("index.html", {}).get("size"):
        raise ValueError("frontend archive file set mismatch")


def verify(candidate: Path, archive: Path, receipt: Path) -> dict:
    for path in (candidate, archive, receipt):
        _unlinked(path)
    manifest = identity.verify(candidate)
    data = json.loads(receipt.read_bytes())
    content = {key: value for key, value in data.items() if key != "receipt_id"}
    if data.get("schema") != 1 or data.get("kind") != "frontend-build" or data.get("receipt_id") != identity.digest(identity.canonical(content)):
        raise ValueError("frontend receipt integrity mismatch")
    if data["source_id"] != manifest["source_id"] or data["profile"] != _profile(manifest["identity"]["build"]):
        raise ValueError("frontend receipt belongs to a different candidate/profile")
    expected_tools = {"node": data["profile"]["node"].removeprefix("v"), "pnpm": data["profile"]["package_manager"].removeprefix("pnpm@")}
    if data["toolchain"] != expected_tools:
        raise ValueError("frontend receipt toolchain/profile mismatch")
    if data["validation"] != "NOT_ASSERTED":
        raise ValueError("frontend receipt is not a test attestation")
    _verify_archive(archive, data)
    return data


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["build", "verify"])
    parser.add_argument("--candidate", type=Path, required=True)
    parser.add_argument("--work", type=Path)
    parser.add_argument("--archive", type=Path, required=True)
    parser.add_argument("--receipt", type=Path)
    parser.add_argument("--pnpm-cli", type=Path, help="Explicit pnpm .cjs/.js entrypoint, executed by the selected node; avoids wrappers depending on the caller's HOME/APPDATA")
    args = parser.parse_args()
    try:
        if args.action == "build":
            if args.work is None:
                raise ValueError("--work is required")
            data = build(args.candidate, args.work, args.archive, args.pnpm_cli)
        else:
            data = verify(args.candidate, args.archive, args.receipt or Path(str(args.archive) + ".json"))
        print(json.dumps({"source_id": data["source_id"], "integrity": "PASS", "validation": "NOT_ASSERTED"}))
        return 0
    except (OSError, ValueError, KeyError, subprocess.SubprocessError, tarfile.TarError) as exc:
        print(f"INCOMPLETE: {exc}")
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
