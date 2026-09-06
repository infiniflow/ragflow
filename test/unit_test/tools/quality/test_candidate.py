"""Candidate byte-integrity contracts, not build/test attestations."""

import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

import pytest


ROOT = Path(__file__).resolve().parents[4]
TOOL = ROOT / "tools/quality/candidate.py"
spec = importlib.util.spec_from_file_location("candidate_under_test", TOOL)
candidate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(candidate)


def git(root, *args):
    return subprocess.run(["git", "-C", str(root), *args], capture_output=True, text=True, check=True)


@pytest.fixture
def repository(tmp_path):
    root = tmp_path / "repo"
    root.mkdir()
    git(root, "init", "-q")
    git(root, "config", "user.name", "Synthetic QA")
    git(root, "config", "user.email", "synthetic@example.invalid")
    git(root, "config", "core.autocrlf", "false")
    (root / "pyproject.toml").write_text('[project]\nname="fixture"\nversion="1.0.0"\n', encoding="utf-8")
    (root / "app.py").write_text("value = 1\n", encoding="utf-8")
    (root / ".gitignore").write_text("ignored.txt\n", encoding="utf-8")
    git(root, "add", ".")
    git(root, "commit", "-qm", "synthetic baseline")
    return root


def snapshot(root, destination, **kwargs):
    return candidate.snapshot(root, destination, kwargs.get("include", []), kwargs.get("allow_dirty", False), {"platform": "linux/amd64"})


def cli(*args):
    return subprocess.run([sys.executable, str(TOOL), *map(str, args)], capture_output=True, text=True, timeout=30)


def rewrite_manifest(destination, mutate):
    path = destination / "candidate.json"
    data = json.loads(path.read_bytes())
    mutate(data["identity"])
    data["source_id"] = candidate.digest(candidate.canonical(data["identity"]))
    path.write_bytes(candidate.canonical(data))


def test_clean_snapshot_cli_is_integrity_only(repository, tmp_path):
    profile = tmp_path / "build.json"
    profile.write_text('{"platform":"linux/amd64"}', encoding="utf-8")
    destination = tmp_path / "candidate"
    result = cli("snapshot", "--root", repository, "--candidate", destination, "--build-profile", profile)
    assert result.returncode == 0, result.stdout + result.stderr
    assert json.loads(result.stdout)["validation"] == "NOT_ASSERTED"
    manifest = candidate.verify(destination)
    assert not manifest["identity"]["dirty"]
    assert manifest["identity"]["version"] == "1.0.0"
    assert "app.py" in manifest["identity"]["files"]
    assert cli("verify", "--candidate", destination).returncode == 0


def test_dirty_refused_unless_explicit_and_new_untracked_not_implicit(repository, tmp_path):
    (repository / "app.py").write_text("changed", encoding="utf-8")
    (repository / "new.py").write_text("new", encoding="utf-8")
    with pytest.raises(ValueError, match="dirty"):
        snapshot(repository, tmp_path / "rejected")
    assert not (tmp_path / "rejected").exists()
    manifest = snapshot(repository, tmp_path / "accepted", allow_dirty=True)
    assert manifest["identity"]["dirty"]
    assert "new.py" not in manifest["identity"]["files"]


def test_exact_untracked_inclusion_and_tracked_deletion(repository, tmp_path):
    (repository / "new.py").write_text("new", encoding="utf-8")
    (repository / "app.py").unlink()
    manifest = snapshot(repository, tmp_path / "accepted", allow_dirty=True, include=["new.py"])
    assert manifest["identity"]["included_untracked"] == ["new.py"]
    assert "new.py" in manifest["identity"]["files"]
    assert "app.py" not in manifest["identity"]["files"]


@pytest.mark.parametrize("name", ["*.py", "app.py", "missing.py", "ignored.txt", "../outside.py"])
def test_nonexact_or_nonuntracked_include_rejected(repository, tmp_path, name):
    (repository / "ignored.txt").write_text("ignored", encoding="utf-8")
    with pytest.raises(ValueError, match="exact"):
        snapshot(repository, tmp_path / "rejected", allow_dirty=True, include=[name])


@pytest.mark.parametrize("mutation", ["modify", "delete", "add"])
def test_source_mutations_fail_verification(repository, tmp_path, mutation):
    destination = tmp_path / "candidate"
    snapshot(repository, destination)
    path = destination / "source/app.py"
    if mutation == "modify":
        path.write_text("different", encoding="utf-8")
    elif mutation == "delete":
        path.unlink()
    else:
        (destination / "source/new.py").write_text("unexpected build input", encoding="utf-8")
    with pytest.raises(ValueError):
        candidate.verify(destination)


def test_manifest_tamper_and_version_conflict(repository, tmp_path):
    destination = tmp_path / "candidate"
    snapshot(repository, destination)
    manifest = json.loads((destination / "candidate.json").read_bytes())
    manifest["identity"]["version"] = "2.0.0"
    (destination / "candidate.json").write_bytes(candidate.canonical(manifest))
    with pytest.raises(ValueError, match="identity mismatch"):
        candidate.verify(destination)
    rewrite_manifest(destination, lambda identity: identity.update(version="2.0.0"))
    with pytest.raises(ValueError, match="version mismatch"):
        candidate.verify(destination)


def test_artifact_repeat_conflict_tamper_and_crosscandidate(repository, tmp_path):
    first = tmp_path / "first"
    snapshot(repository, first)
    artifact = tmp_path / "build.tar"
    artifact.write_bytes(b"built bytes")
    result = cli("record-artifact", "--candidate", first, "--artifact", artifact, "--name", "image")
    assert result.returncode == 0, result.stdout + result.stderr
    assert json.loads(result.stdout)["validation"] == "NOT_ASSERTED"
    candidate.attest_artifact(first, artifact, "image")
    assert candidate.verify_artifact(first, artifact, "image")["sha256"]
    artifact.write_bytes(b"changed bytes")
    with pytest.raises(ValueError, match="integrity"):
        candidate.verify_artifact(first, artifact, "image")
    with pytest.raises(ValueError, match="conflict"):
        candidate.attest_artifact(first, artifact, "image")
    (repository / "app.py").write_text("next build", encoding="utf-8")
    second = tmp_path / "second"
    snapshot(repository, second, allow_dirty=True)
    shutil.copy2(first / "artifact-image.json", second / "artifact-image.json")
    with pytest.raises(ValueError, match="another candidate"):
        candidate.verify_artifact(second, artifact, "image")


def test_same_version_different_content_conflicts(repository, tmp_path):
    first = snapshot(repository, tmp_path / "first")
    assert candidate.compare_installed(first, first) == "same"
    (repository / "app.py").write_text("next", encoding="utf-8")
    second = snapshot(repository, tmp_path / "second", allow_dirty=True)
    with pytest.raises(ValueError, match="same version"):
        candidate.compare_installed(second, first)


@pytest.mark.parametrize("name", ["../escape", "/absolute", "C:/absolute", "x\\y", "stream:ads"])
def test_unsafe_manifest_paths_rejected(repository, tmp_path, name):
    destination = tmp_path / "candidate"
    snapshot(repository, destination)
    rewrite_manifest(destination, lambda identity: identity["files"].update({name: {"sha256": "0" * 64, "size": 0}}))
    with pytest.raises(ValueError, match="unsafe"):
        candidate.safe_path(destination / "source", name)
    with pytest.raises(ValueError):
        candidate.verify(destination)


@pytest.mark.parametrize("name", [".env", "credentials.key", "web/.env.local", "web/.env.production.local"])
def test_secret_paths_rejected(repository, tmp_path, name):
    path = repository / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("synthetic secret marker", encoding="utf-8")
    with pytest.raises(ValueError, match="private"):
        snapshot(repository, tmp_path / "candidate", allow_dirty=True, include=[name])


def test_docker_env_omitted_and_recorded(repository, tmp_path):
    path = repository / "docker/.env"
    path.parent.mkdir()
    path.write_text("synthetic secret marker", encoding="utf-8")
    git(repository, "add", "docker/.env")
    git(repository, "commit", "-qm", "synthetic env exclusion")
    destination = tmp_path / "candidate"
    manifest = snapshot(repository, destination)
    assert manifest["identity"]["omitted"] == ["docker/.env"]
    assert not (destination / "source/docker/.env").exists()


def link_directory(link, target):
    if os.name == "nt":
        # Junctions require no admin privileges, unlike Windows symlinks.
        subprocess.run(["cmd", "/c", "mklink", "/J", str(link), str(target)], capture_output=True, check=True)
    else:
        link.symlink_to(target, target_is_directory=True)


def test_linked_source_root_rejected(repository, tmp_path):
    destination = tmp_path / "candidate"
    snapshot(repository, destination)
    source = destination / "source"
    real_source = tmp_path / "linked-source"
    source.rename(real_source)
    link_directory(source, real_source)
    try:
        with pytest.raises(ValueError, match="link"):
            candidate.verify(destination)
    finally:
        source.rmdir() if os.name == "nt" else source.unlink()


def test_linked_artifact_parent_rejected(repository, tmp_path):
    destination = tmp_path / "candidate"
    snapshot(repository, destination)
    actual = tmp_path / "actual-artifacts"
    actual.mkdir()
    (actual / "image.tar").write_bytes(b"artifact")
    linked = tmp_path / "linked-artifacts"
    link_directory(linked, actual)
    try:
        with pytest.raises(ValueError, match="link|regular"):
            candidate.attest_artifact(destination, linked / "image.tar", "image")
    finally:
        linked.rmdir() if os.name == "nt" else linked.unlink()


def test_source_changes_during_capture_rejected(repository, tmp_path, monkeypatch):
    original = candidate.file_digest
    changed = False

    def mutate_after_hash(path):
        nonlocal changed
        value = original(path)
        if path == repository / "app.py" and not changed:
            changed = True
            path.write_text("modified during capture", encoding="utf-8")
        return value

    monkeypatch.setattr(candidate, "file_digest", mutate_after_hash)
    destination = tmp_path / "candidate"
    with pytest.raises(ValueError, match="changed"):
        snapshot(repository, destination)
    assert not (destination / "candidate.json").exists()


def test_nested_destination_and_unknown_build_parameters_rejected(repository, tmp_path):
    with pytest.raises(ValueError, match="outside"):
        snapshot(repository, repository / "candidate")
    with pytest.raises(ValueError, match="allowlisted"):
        candidate.snapshot(repository, tmp_path / "candidate", [], False, {"platform": "linux/amd64", "SECRET_TOKEN": "synthetic"})


def test_linked_tracked_source_parent_rejected(repository, tmp_path):
    folder = repository / "pkg"
    folder.mkdir()
    (folder / "module.py").write_text("value = 1", encoding="utf-8")
    git(repository, "add", "pkg/module.py")
    git(repository, "commit", "-qm", "synthetic package")
    target = tmp_path / "linked-package"
    folder.rename(target)
    link_directory(folder, target)
    try:
        with pytest.raises(ValueError, match="link"):
            snapshot(repository, tmp_path / "candidate", allow_dirty=True)
    finally:
        folder.rmdir() if os.name == "nt" else folder.unlink()


def test_tree_files_matches_reference_hashes_and_detects_changed_bytes(tmp_path):
    root = tmp_path / "tree"
    (root / "nested/deeper").mkdir(parents=True)
    contents = {"empty.txt": b"", "nested/deeper/data.bin": bytes(range(256)), "nested/unicode.txt": "проверка".encode()}
    for name, value in contents.items():
        (root / name).write_bytes(value)
    expected = {name: {"sha256": candidate.digest(value), "size": len(value)} for name, value in contents.items()}
    assert candidate.tree_files(root) == expected
    (root / "nested/deeper/data.bin").write_bytes(b"different")
    assert candidate.tree_files(root) != expected


@pytest.mark.parametrize("field,value", [("size", 999), ("sha256", "0" * 64)])
def test_rehashed_manifest_wrong_file_metadata_rejected(repository, tmp_path, field, value):
    destination = tmp_path / "candidate"
    snapshot(repository, destination)
    rewrite_manifest(destination, lambda identity: identity["files"]["app.py"].update({field: value}))
    with pytest.raises(ValueError, match="integrity"):
        candidate.verify(destination)


def test_tree_files_prunes_only_exact_generated_directories(tmp_path):
    root = tmp_path / "tree"
    (root / "web/nested/dist").mkdir(parents=True)
    (root / "web/dist").mkdir()
    (root / "web/dist/built.js").write_text("built")
    (root / "web/nested/dist/source.txt").write_text("source")
    dependency = tmp_path / "dependencies"
    dependency.mkdir()
    linked = root / "web/node_modules"
    link_directory(linked, dependency)
    try:
        result = candidate.tree_files(root, frozenset({"web/node_modules", "web/dist"}))
        assert set(result) == {"web/nested/dist/source.txt"}
        with pytest.raises(ValueError, match="link"):
            candidate.tree_files(root)
    finally:
        linked.rmdir() if os.name == "nt" else linked.unlink()


def test_tree_files_rejects_nonregular_file_before_hash(tmp_path, monkeypatch):
    import stat
    from types import SimpleNamespace

    root = tmp_path / "tree"
    root.mkdir()
    path = root / "pipe"
    path.write_text("placeholder")
    original = Path.lstat

    def report_fifo(self):
        return SimpleNamespace(st_mode=stat.S_IFIFO, st_reparse_tag=0) if self == path else original(self)

    monkeypatch.setattr(Path, "lstat", report_fifo)
    with pytest.raises(ValueError, match="nonregular"):
        candidate.tree_files(root)
