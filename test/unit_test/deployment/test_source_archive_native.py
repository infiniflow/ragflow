"""Exercise source packaging using disposable tiny trees, never the live checkout."""

import hashlib
import shutil
import subprocess
import sys
import tarfile
from pathlib import Path

import pytest


ROOT = Path(__file__).resolve().parents[3]


@pytest.fixture
def source_tree(tmp_path):
    source = tmp_path / "source"
    paths = [
        "deployment/linux-pg/build_archive.ps1",
        "deployment/linux-pg/install.sh",
        "deployment/linux-pg/upgrade.sh",
        "deployment/linux-pg/install_gvisor.sh",
        "deployment/linux-pg/docker-compose.release.yml",
        "deployment/linux-pg/seed_admin.py",
        "deployment/linux-pg/seed_asr.py",
        "deployment/linux-pg/env.template",
        "ragflow_deps/prepare_native.py",
        "ragflow_deps/native-deps.json",
        "ragflow_deps/download_deps.py",
        "ragflow_deps/download_go_deps.py",
        "ragflow_deps/Dockerfile",
        "go.mod",
    ]
    for name in paths:
        target = source / name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / name, target)
    for name in ("ragflow_deps/unexpected.bin", "ragflow_deps/cache/archive.tgz", "output/ignored.txt"):
        target = source / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text("must not be exported", encoding="utf-8")
    return source


def package(source, tmp_path):
    pwsh = shutil.which("pwsh")
    assert pwsh is not None, "PowerShell 7 is required to verify the source packager"
    assert shutil.which("tar") is not None, "tar is required to verify the source packager"
    output = tmp_path / "artifacts"
    result = subprocess.run(
        [pwsh, "-NoProfile", "-File", str(source / "deployment/linux-pg/build_archive.ps1"), "-ReleaseVersion", "v0.0.0-test", "-OutputDirectory", str(output)],
        capture_output=True,
        text=True,
        timeout=60,
    )
    return result, output / "ragflow-linux-pg-v0.0.0-test.tar.gz"


def test_archive_contains_importable_native_preparation_contract(source_tree, tmp_path):
    result, archive = package(source_tree, tmp_path)
    assert result.returncode == 0, result.stdout + result.stderr
    with tarfile.open(archive) as bundle:
        names = {name.removeprefix("./") for name in bundle.getnames()}
        assert {"ragflow_deps/prepare_native.py", "ragflow_deps/native-deps.json"} <= names
        assert "ragflow_deps/unexpected.bin" not in names
        assert "ragflow_deps/cache/archive.tgz" not in names
        assert "output/ignored.txt" not in names
        extracted = tmp_path / "extracted"
        bundle.extractall(extracted, filter="data")
    result = subprocess.run(
        [sys.executable, "-c", "from prepare_native import dependencies; assert len(dependencies()) == 3"],
        cwd=extracted / "ragflow_deps",
        capture_output=True,
        text=True,
        timeout=20,
    )
    assert result.returncode == 0, result.stdout + result.stderr
    expected_hash = hashlib.sha256(archive.read_bytes()).hexdigest()
    assert archive.with_name(archive.name + ".sha256").read_text().split()[0] == expected_hash


@pytest.mark.parametrize("missing", ["prepare_native.py", "native-deps.json"])
def test_missing_native_source_fails_before_archive_publication(source_tree, tmp_path, missing):
    (source_tree / "ragflow_deps" / missing).unlink()
    result, archive = package(source_tree, tmp_path)
    assert result.returncode != 0
    assert "Required deployment file is missing" in result.stdout + result.stderr
    assert not archive.exists()
    assert not archive.with_name(archive.name + ".sha256").exists()
