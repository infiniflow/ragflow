"""Offline layout integration with real packagers and a fake Docker executable."""

import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile

import pytest

from tools.quality import candidate, frontend_artifact
from test.unit_test.deployment import test_linux_pg_candidate_archive as source_fixtures


base_package = source_fixtures.package
PROFILE = source_fixtures.PROFILE


ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / "deployment/linux-pg/build_offline_archive.ps1"


@pytest.fixture
def offline(base_package, tmp_path):
    source = tmp_path / "repo"
    deployment = source / "deployment/linux-pg"
    (deployment / "install_offline.sh").write_text("#!/bin/sh\n# synthetic candidate installer\n")
    (deployment / "upgrade_offline.sh").write_text("#!/bin/sh\n# synthetic candidate upgrader\n")
    (deployment / "prepare_gvisor_bundle.ps1").write_text(
        "param([string]$Destination)\nNew-Item -ItemType Directory -Path $Destination -Force | Out-Null\n[System.IO.File]::WriteAllText((Join-Path $Destination 'fixture'), 'candidate-gvisor')\n"
    )
    (source / "services/asr-online-service").mkdir(parents=True)
    (source / "services/asr-online-service/Dockerfile").write_text("FROM scratch\n")
    for args in (("add", "."), ("commit", "-qm", "offline fixture")):
        subprocess.run(["git", "-C", str(source), *args], check=True, capture_output=True)
    snapshot = tmp_path / "offline-candidate"
    candidate.snapshot(source, snapshot, [], False, PROFILE)
    frontend = tmp_path / "offline-frontend.tar.gz"
    frontend_artifact.build(snapshot, tmp_path / "offline-work", frontend)
    fakebin = tmp_path / "fakebin"
    fakebin.mkdir()
    helper = fakebin / "docker_fake.py"
    helper.write_text(r"""
import hashlib, io, json, os, pathlib, sys, tarfile
args = sys.argv[1:]
with open(os.environ['OFFLINE_DOCKER_LOG'], 'a') as log:
    print(json.dumps(args), file=log)
config = b'{"architecture":"amd64","os":"linux"}'
image_id = 'sha256:' + hashlib.sha256(config).hexdigest()
if args[0] == 'build':
    assert args[-1].replace('\\', '/').endswith('source/services/asr-online-service')
elif args[:2] == ['image', 'inspect']:
    index = args.index('--format')
    fmt = args[index + 1]
    tags = args[2:index] + args[index + 2:]
    for tag in tags:
        print(image_id + ' linux/amd64' if '.Id' in fmt else 'linux/amd64')
elif args[:2] == ['image', 'save']:
    target = args[args.index('--output') + 1]
    tags = args[args.index('--output') + 2:]
    manifest = [{'Config': 'config.json', 'RepoTags': tags, 'Layers': ['layer.tar']}]
    with tarfile.open(target, 'w') as archive:
        for name, data in [('config.json', config), ('layer.tar', b'fixture layer'), ('manifest.json', json.dumps(manifest).encode())]:
            info = tarfile.TarInfo(name)
            info.size = len(data)
            archive.addfile(info, io.BytesIO(data))
else:
    raise AssertionError(args)
""")
    if os.name == "nt":
        (fakebin / "docker.cmd").write_text(f'@"{sys.executable}" "{helper}" %*\r\n')
    else:
        wrapper = fakebin / "docker"
        wrapper.write_text(f'#!/bin/sh\nexec "{sys.executable}" "{helper}" "$@"\n')
        wrapper.chmod(0o755)
    env = {**os.environ, "PATH": str(fakebin) + os.pathsep + os.environ["PATH"], "OFFLINE_DOCKER_LOG": str(tmp_path / "docker.log")}
    return snapshot, frontend, tmp_path / "bundle-cache", tmp_path / "offline-output", env


def invoke(offline, *extra):
    snapshot, frontend, cache, output, env = offline
    return subprocess.run(
        [
            "pwsh",
            "-NoProfile",
            "-File",
            str(SCRIPT),
            "-ReleaseVersion",
            "v1.2.3",
            "-OutputDirectory",
            str(output),
            "-CandidateDirectory",
            str(snapshot),
            "-FrontendArtifact",
            str(frontend),
            "-BundleCacheDirectory",
            str(cache),
            "-PythonCommand",
            sys.executable,
            *extra,
        ],
        cwd=ROOT,
        env=env,
        capture_output=True,
        text=True,
        timeout=60,
    )


def test_verified_offline_layout_uses_candidate_frontend_and_cached_docker_bytes(offline):
    snapshot, _, cache, output, env = offline
    result = invoke(offline)
    assert result.returncode == 0, result.stdout + result.stderr
    assert not (snapshot / "source/web/dist").exists()
    archive = output / "ragflow-linux-pg-v1.2.3-offline.tar.gz"
    with tarfile.open(archive) as bundle:
        names = bundle.getnames()
        assert "install_offline.sh" in names and "upgrade_offline.sh" in names
        assert b"synthetic candidate installer" in bundle.extractfile("install_offline.sh").read()
        assert bundle.extractfile("payload/gvisor/fixture").read() == b"candidate-gvisor"
        with tarfile.open(fileobj=io.BytesIO(bundle.extractfile("payload/web-dist.tar.gz").read())) as web:
            assert web.extractfile("dist/index.html").read() == b"<html>synthetic built SPA</html>"
            assert web.extractfile("dist/assets/example.sh").read() == b"example\r\n"
        with tarfile.open(fileobj=io.BytesIO(bundle.extractfile("payload/ragflow-linux-pg-v1.2.3.tar.gz").read())) as source:
            assert "./DEPLOYMENT-CANDIDATE.json" in source.getnames()
            assert "./web/dist/index.html" not in source.getnames()
        receipt = json.loads(bundle.extractfile("payload/docker-images.json").read())
        assert receipt["validation"] == "NOT_ASSERTED"
        assert receipt["source_id"] == candidate.verify(snapshot)["source_id"]
        assert len(bundle.extractfile("payload/docker-images.txt").read().splitlines()) == 15
        manifest = bundle.extractfile("OFFLINE-PACKAGE.env").read()
        assert b"CANDIDATE_VALIDATION=NOT_ASSERTED" in manifest
        hashes = bundle.extractfile("SHA256SUMS").read()
        assert b"payload/docker-images.json" in hashes and b"payload/frontend-build.json" in hashes
    operations = [json.loads(line) for line in Path(env["OFFLINE_DOCKER_LOG"]).read_text().splitlines()]
    assert len([args for args in operations if args[:2] == ["image", "save"]]) == 1
    assert len(list(cache.glob("*/bundle.json"))) == 1
    assert all(args[0] not in ("run", "load", "compose", "push") for args in operations)
    cached_source = next(cache.glob("source-*/*.tar.gz"))
    before = (cached_source.read_bytes(), cached_source.stat().st_mtime_ns)
    repeated = invoke(offline, "-Overwrite")
    assert repeated.returncode == 0, repeated.stdout + repeated.stderr
    assert "Reusing verified source cache" in repeated.stdout
    assert (cached_source.read_bytes(), cached_source.stat().st_mtime_ns) == before
    operations = [json.loads(line) for line in Path(env["OFFLINE_DOCKER_LOG"]).read_text().splitlines()]
    assert len([args for args in operations if args[:2] == ["image", "save"]]) == 1


@pytest.mark.parametrize("corrupt", ["source-missing", "source-checksum", "docker-bytes"])
def test_incomplete_or_corrupt_cache_never_replaces_existing_envelope(offline, corrupt):
    _, _, cache, output, _ = offline
    result = invoke(offline)
    assert result.returncode == 0, result.stdout + result.stderr
    archive = output / "ragflow-linux-pg-v1.2.3-offline.tar.gz"
    before = archive.read_bytes()
    source = next(cache.glob("source-*/*.tar.gz"))
    if corrupt == "source-missing":
        source.unlink()
    elif corrupt == "source-checksum":
        Path(str(source) + ".sha256").write_text("corrupt")
    else:
        docker_archive = next(cache.glob("*/docker-images.tar"))
        docker_archive.write_bytes(docker_archive.read_bytes() + b"corrupt")
    result = invoke(offline, "-Overwrite")
    assert result.returncode != 0
    assert archive.read_bytes() == before


def test_corrupt_frontend_fails_before_any_docker_operation(offline):
    _, frontend, _, output, env = offline
    frontend.write_bytes(frontend.read_bytes() + b"corrupt")
    result = invoke(offline)
    assert result.returncode != 0
    assert not Path(env["OFFLINE_DOCKER_LOG"]).exists()
    assert not list(output.glob("*.tar.gz"))


@pytest.mark.parametrize("argument", ["-CandidateDirectory", "-FrontendArtifact", "-BundleCacheDirectory"])
def test_verified_mode_requires_all_three_inputs_before_mutation(tmp_path, argument):
    result = subprocess.run(
        ["pwsh", "-NoProfile", "-File", str(SCRIPT), "-ReleaseVersion", "v1.2.3", "-OutputDirectory", str(tmp_path), argument, "unused"], capture_output=True, text=True, timeout=15
    )
    assert result.returncode != 0
    assert "requires CandidateDirectory" in result.stderr
    assert "BundleCacheDirectory together" in result.stderr
    assert not list(tmp_path.iterdir())


def test_verified_mode_rechecks_inputs_at_final_packaging_boundary():
    text = SCRIPT.read_text()
    tar = text.index("Invoke-Tar -ArgumentList (@('-czf', $archivePath")
    verify = text.rindex("Invoke-QualityTool 'candidate.py' @('verify'", 0, tar)
    assert "Invoke-QualityTool 'frontend_artifact.py'" in text[verify:tar]
    assert "Invoke-QualityTool 'docker_bundle.py'" in text[verify:tar]
