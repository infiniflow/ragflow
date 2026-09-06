"""Verified, content-keyed docker-save cache. No docker load or runtime mutation.

Receipts bind image bytes/tags to candidate inputs, not image build provenance or
successful tests. Supports classic Docker save and containerd OCI-backed saves.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import signal
import subprocess
import tarfile
import uuid

try:
    from . import candidate as source_identity
except ImportError:
    import candidate as source_identity


def _unlinked(path: Path) -> None:
    for part in (path, *path.parents):
        if part.is_symlink() or part.is_junction():
            raise ValueError("linked Docker bundle path")


def _docker_command() -> list[str]:
    docker = shutil.which("docker")
    if not docker:
        raise ValueError("docker executable is required")
    return [docker]


def _docker(args: list[str], timeout: int = 60) -> str:
    process = subprocess.Popen([*_docker_command(), *args], stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=os.name != "nt")
    try:
        stdout, _ = process.communicate(timeout=timeout)
    except BaseException:
        if os.name == "nt":
            subprocess.run(["taskkill", "/PID", str(process.pid), "/T", "/F"], capture_output=True, check=False)
        else:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        process.communicate(timeout=10)
        raise
    if process.returncode:
        # Docker stderr/config may contain daemon credentials/addresses.
        raise ValueError(f"docker {args[0]} operation failed ({process.returncode})")
    return stdout.decode("utf-8").strip()


def inspect(tags: list[str]) -> dict:
    if not tags or len(tags) != len(set(tags)):
        raise ValueError("nonempty unique Docker tags required")
    for tag in tags:
        if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._/:-]*", tag) or ":" not in tag.rsplit("/", 1)[-1]:
            raise ValueError("explicit Docker name:tag required, not digest-only or option strings")
    tags = sorted(tags)
    lines = _docker(["image", "inspect", "--format", "{{.Id}} {{.Os}}/{{.Architecture}}", *tags]).splitlines()
    if len(lines) != len(tags):
        raise ValueError("incomplete Docker image inventory")
    inventory = {}
    for tag, line in zip(tags, lines, strict=True):
        values = line.split()
        if len(values) != 2 or not re.fullmatch(r"sha256:[0-9a-f]{64}", values[0]) or values[1] != "linux/amd64":
            raise ValueError("Docker image inventory must contain SHA256 IDs and linux/amd64")
        inventory[tag] = {"id": values[0], "platform": values[1]}
    return inventory


def _member_name(name: str) -> None:
    path = PurePosixPath(name)
    if not name or name != path.as_posix() or path.is_absolute() or "\\" in name or any(part in {"..", "."} or ":" in part for part in path.parts):
        raise ValueError("unsafe Docker archive path")


def _canonical_tag(tag: str) -> str:
    first, separator, rest = tag.partition("/")
    if separator and ("." in first or ":" in first or first == "localhost"):
        registry, repository = first, rest
    else:
        registry, repository = "docker.io", tag
    if registry == "index.docker.io":
        registry = "docker.io"
    if registry == "docker.io" and "/" not in repository:
        repository = "library/" + repository
    return registry + "/" + repository


def _archive_inventory(archive: Path, expected: dict) -> None:
    _unlinked(archive)
    if not archive.is_file():
        raise ValueError("Docker archive must be regular")
    with tarfile.open(archive, mode="r:") as bundle:
        entries = {}
        for member in bundle:
            # Directories are valid in Docker save, links/device files are not.
            _member_name(member.name.rstrip("/") if member.isdir() else member.name)
            if member.name in entries or not (member.isfile() or member.isdir()):
                raise ValueError("duplicate or unsupported Docker archive member")
            entries[member.name] = member

        def read_json(name: str, limit: int) -> tuple[bytes, object]:
            _member_name(name)
            member = entries.get(name)
            if member is None or not member.isfile() or member.size > limit:
                raise ValueError("missing/nonregular/oversized Docker metadata")
            with bundle.extractfile(member) as stream:
                data = stream.read(limit + 1)
            return data, json.loads(data)

        _, manifests = read_json("manifest.json", 4 * 1024 * 1024)
        if not isinstance(manifests, list) or not manifests:
            raise ValueError("unsupported Docker save manifest")

        def descriptor_json(descriptor: dict) -> dict:
            digest = descriptor["digest"]
            if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
                raise ValueError("unsupported OCI descriptor digest")
            raw, value = read_json("blobs/sha256/" + digest.removeprefix("sha256:"), 16 * 1024 * 1024)
            if len(raw) != descriptor["size"] or "sha256:" + hashlib.sha256(raw).hexdigest() != digest:
                raise ValueError("OCI descriptor size/digest mismatch")
            return value

        def oci_image_id(tag: str, config_id: str, config_size: int, layers: list[str]) -> str:
            # containerd reports the OCI index/manifest ID, rather than Config ID.
            _, index = read_json("index.json", 4 * 1024 * 1024)
            matching = []
            for descriptor in index["manifests"]:
                full_tag = descriptor.get("annotations", {}).get("io.containerd.image.name", "")
                if full_tag and _canonical_tag(full_tag) == _canonical_tag(tag):
                    matching.append(descriptor)
            if len(matching) != 1 or matching[0]["digest"] != expected[tag]["id"]:
                raise ValueError("OCI tag/root image identity mismatch")
            root = matching[0]
            image_manifest = descriptor_json(root)
            if "manifests" in image_manifest:
                # Other architectures/attestations can be absent in a valid save.
                selected = [entry for entry in image_manifest["manifests"] if entry.get("platform", {}).get("os") == "linux" and entry.get("platform", {}).get("architecture") == "amd64"]
                if len(selected) != 1:
                    raise ValueError("OCI index needs exactly one linux/amd64 image")
                image_manifest = descriptor_json(selected[0])
            config_descriptor = image_manifest["config"]
            if config_descriptor["digest"] != config_id or config_descriptor["size"] != config_size:
                raise ValueError("OCI config does not match Docker save Config")
            selected_layers = image_manifest["layers"]
            if any(not re.fullmatch(r"sha256:[0-9a-f]{64}", layer["digest"]) for layer in selected_layers):
                raise ValueError("unsupported OCI layer digest")
            if ["blobs/sha256/" + layer["digest"].removeprefix("sha256:") for layer in selected_layers] != layers:
                raise ValueError("OCI layer list does not match Docker save Layers")
            if any(entries[name].size != layer["size"] for name, layer in zip(layers, selected_layers, strict=True)):
                raise ValueError("OCI layer size mismatch")
            return root["digest"]

        actual = {}
        for image in manifests:
            raw_config, config = read_json(image["Config"], 16 * 1024 * 1024)
            image_id = "sha256:" + hashlib.sha256(raw_config).hexdigest()
            image_platform = f"{config['os']}/{config['architecture']}"
            tags = image["RepoTags"]
            if not isinstance(tags, list) or not tags:
                raise ValueError("Docker image without preserved tags")
            for layer in image["Layers"]:
                _member_name(layer)
                if layer not in entries or not entries[layer].isfile():
                    raise ValueError("missing/nonregular Docker layer")
            for tag in tags:
                if tag in actual or tag not in expected:
                    raise ValueError("unexpected or duplicate Docker tag mapping")
                inspected_id = image_id
                if image_id != expected[tag]["id"] and "index.json" in entries:
                    inspected_id = oci_image_id(tag, image_id, len(raw_config), image["Layers"])
                actual[tag] = {"id": inspected_id, "platform": image_platform}
        if actual != expected:
            raise ValueError("Docker save tags/config IDs/platform do not match inspected inventory")


def verify(candidate: Path, archive: Path, receipt: Path, tags: list[str]) -> dict:
    for path in (candidate, archive, receipt):
        _unlinked(path)
    manifest = source_identity.verify(candidate)
    data = json.loads(receipt.read_bytes())
    content = {key: value for key, value in data.items() if key != "receipt_id"}
    if data.get("schema") != 1 or data.get("kind") != "docker-save" or data.get("receipt_id") != source_identity.digest(source_identity.canonical(content)):
        raise ValueError("Docker receipt integrity mismatch")
    if data["source_id"] != manifest["source_id"] or data["images"] != inspect(tags):
        raise ValueError("Docker bundle candidate or current tags changed")
    if data["validation"] != "NOT_ASSERTED":
        raise ValueError("Docker cache is not validation evidence")
    if data["archive"] != {"sha256": source_identity.file_digest(archive), "size": archive.stat().st_size}:
        raise ValueError("Docker bundle archive integrity mismatch; corrupt cache must not be reused")
    _archive_inventory(archive, data["images"])
    return data


def materialize(candidate: Path, cache: Path, tags: list[str]) -> tuple[Path, Path]:
    for path in (candidate, cache):
        _unlinked(path)
    candidate, cache = candidate.resolve(), cache.resolve()
    if cache.is_relative_to(candidate) or candidate.is_relative_to(cache):
        raise ValueError("Docker cache must be separate from candidate")
    manifest = source_identity.verify(candidate)
    images = inspect(tags)
    key = source_identity.digest(source_identity.canonical({"schema": 1, "source_id": manifest["source_id"], "images": images}))
    destination = cache / key
    archive, receipt = destination / "docker-images.tar", destination / "bundle.json"
    if destination.exists():
        verify(candidate, archive, receipt, tags)
        return archive, receipt
    cache.mkdir(parents=True, exist_ok=True)
    staged = cache / (".pending-" + uuid.uuid4().hex)
    staged.mkdir(exist_ok=False)
    saved = staged / "docker-images.tar"
    # Failed/incomplete .pending directories remain inspectable; never reuse them.
    _docker(["image", "save", "--output", str(saved), *sorted(tags)], timeout=1800)
    if images != inspect(tags):
        raise ValueError("Docker tags changed during save")
    if source_identity.verify(candidate)["source_id"] != manifest["source_id"]:
        raise ValueError("candidate changed during Docker save")
    _archive_inventory(saved, images)
    data = {
        "schema": 1,
        "kind": "docker-save",
        "source_id": manifest["source_id"],
        "images": images,
        "archive": {"sha256": source_identity.file_digest(saved), "size": saved.stat().st_size},
        "validation": "NOT_ASSERTED",
    }
    data["receipt_id"] = source_identity.digest(source_identity.canonical(data))
    (staged / "bundle.json").write_bytes(source_identity.canonical(data) + b"\n")
    # An existing cache entry is never replaced, including when another writer won.
    staged.rename(destination)
    verify(candidate, archive, receipt, tags)
    return archive, receipt


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["materialize", "verify"])
    parser.add_argument("--candidate", type=Path, required=True)
    parser.add_argument("--cache", type=Path)
    parser.add_argument("--archive", type=Path)
    parser.add_argument("--receipt", type=Path)
    parser.add_argument("--image", action="append", required=True)
    args = parser.parse_args()
    try:
        if args.action == "materialize":
            if args.cache is None:
                raise ValueError("--cache is required")
            archive, receipt = materialize(args.candidate, args.cache, args.image)
        else:
            if args.archive is None or args.receipt is None:
                raise ValueError("--archive and --receipt are required")
            archive, receipt = args.archive, args.receipt
            verify(args.candidate, archive, receipt, args.image)
        print(json.dumps({"archive": str(archive), "receipt": str(receipt), "integrity": "PASS", "validation": "NOT_ASSERTED"}))
        return 0
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError, tarfile.TarError) as exc:
        print(f"INCOMPLETE: {exc}")
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
