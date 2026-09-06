"""Docker-save cache checks against real fake-CLI subprocesses, no Docker daemon."""

import hashlib
import io
import json
import sys
import tarfile

import pytest

from tools.quality import candidate, docker_bundle as bundle


TAGS = ["fixture/api:1", "fixture/db:2"]


def saved_tar(path, configs, *, change=None):
    manifest = []
    with tarfile.open(path, "w") as archive:

        def add(name, data):
            entry = tarfile.TarInfo(name)
            entry.size = len(data)
            archive.addfile(entry, io.BytesIO(data))

        for index, (tag, config) in enumerate(configs.items()):
            data = candidate.canonical(config)
            name = hashlib.sha256(data).hexdigest() + ".json"
            if change == "config" and index == 0:
                data += b" "
            add(name, data)
            manifest.append({"Config": name, "RepoTags": [tag], "Layers": []})
        if change == "tag":
            manifest[0]["RepoTags"] = ["fixture/unexpected:1"]
        if change == "missing-layer":
            manifest[0]["Layers"] = ["missing/layer.tar"]
        if change == "traversal":
            add("../escape", b"bad")
        if change == "symlink":
            link = tarfile.TarInfo("outside")
            link.type = tarfile.SYMTYPE
            link.linkname = "../../outside"
            archive.addfile(link)
        add("manifest.json", candidate.canonical(manifest))
        if change == "duplicate":
            add("manifest.json", candidate.canonical(manifest))


def oci_saved_tar(path, configs, *, change=None):
    """Containerd inspect ID is the root OCI index digest, not config digest."""
    blobs, legacy, roots, inventory = {}, [], [], {}

    def blob(value):
        raw = candidate.canonical(value)
        digest = "sha256:" + candidate.digest(raw)
        blobs["blobs/sha256/" + digest.removeprefix("sha256:")] = raw
        return {"digest": digest, "size": len(raw)}

    for number, (tag, config) in enumerate(configs.items()):
        config_descriptor = blob(config)
        child = {"schemaVersion": 2, "config": config_descriptor.copy(), "layers": []}
        if change == "wrong-config" and number == 0:
            child["config"]["digest"] = "sha256:" + "0" * 64
        child_descriptor = blob(child)
        child_path = "blobs/sha256/" + child_descriptor["digest"].removeprefix("sha256:")
        if change == "child-tamper" and number == 0:
            blobs[child_path] += b" "
        if change == "missing-child" and number == 0:
            del blobs[child_path]
        child_descriptor["platform"] = {"os": "linux", "architecture": "arm64" if change == "wrong-platform" and number == 0 else "amd64"}
        children = [child_descriptor, {"digest": "sha256:" + "9" * 64, "size": 10, "platform": {"os": "linux", "architecture": "arm64"}}]
        if change == "duplicate-platform" and number == 0:
            children.append(child_descriptor.copy())
        root_descriptor = blob({"schemaVersion": 2, "manifests": children})
        if change == "root-tamper" and number == 0:
            blobs["blobs/sha256/" + root_descriptor["digest"].removeprefix("sha256:")] += b" "
        inventory[tag] = {"id": root_descriptor["digest"], "platform": "linux/amd64"}
        root_descriptor["annotations"] = {"io.containerd.image.name": "docker.io/" + ("fixture/other:1" if change == "wrong-tag" and number == 0 else tag)}
        roots.append(root_descriptor)
        legacy.append({"Config": "blobs/sha256/" + config_descriptor["digest"].removeprefix("sha256:"), "RepoTags": [tag], "Layers": []})
    blobs["manifest.json"] = candidate.canonical(legacy)
    blobs["index.json"] = candidate.canonical({"schemaVersion": 2, "manifests": roots})
    with tarfile.open(path, "w") as archive:
        for name, data in blobs.items():
            member = tarfile.TarInfo(name)
            member.size = len(data)
            archive.addfile(member, io.BytesIO(data))
    return inventory


@pytest.fixture
def setup(tmp_path, monkeypatch):
    snapshot = tmp_path / "candidate"
    source = snapshot / "source"
    source.mkdir(parents=True)
    pyproject = source / "pyproject.toml"
    pyproject.write_text('[project]\nversion="1.0.0"\n', encoding="utf-8")
    identity = {
        "schema": 1,
        "version": "1.0.0",
        "head": "a" * 40,
        "dirty": False,
        "build": {"platform": "linux/amd64"},
        "files": {"pyproject.toml": {"sha256": candidate.file_digest(pyproject), "size": pyproject.stat().st_size}},
    }
    (snapshot / "candidate.json").write_bytes(candidate.canonical({"identity": identity, "source_id": candidate.digest(candidate.canonical(identity))}))
    configs = {tag: {"os": "linux", "architecture": "amd64", "config": {"Labels": {"fixture": tag}}} for tag in TAGS}
    fixture_tar = tmp_path / "saved.tar"
    saved_tar(fixture_tar, configs)
    state_path = tmp_path / "docker-state.json"
    state = {"images": {tag: {"id": "sha256:" + candidate.digest(candidate.canonical(config)), "platform": "linux/amd64"} for tag, config in configs.items()}, "saved": str(fixture_tar), "saves": 0}
    state_path.write_text(json.dumps(state), encoding="utf-8")
    script = tmp_path / "fake_docker.py"
    script.write_text(
        """
import json, pathlib, shutil, sys
path = pathlib.Path(sys.argv[1])
state = json.loads(path.read_text())
args = sys.argv[2:]
if args[:2] == ["image", "inspect"]:
    assert args[2:4] == ["--format", "{{.Id}} {{.Os}}/{{.Architecture}}"]
    for tag in args[4:]:
        image = state["images"][tag]
        print(image["id"], image["platform"])
elif args[:3] == ["image", "save", "--output"]:
    assert args[4:] == sorted(state["images"])
    shutil.copyfile(state["saved"], args[3])
    state["saves"] += 1
    if state.get("retag_after_save"):
        state["images"][args[4]]["id"] = "sha256:" + "0" * 64
    path.write_text(json.dumps(state))
else:
    raise AssertionError(args)
""",
        encoding="utf-8",
    )
    monkeypatch.setattr(bundle, "_docker_command", lambda: [sys.executable, str(script), str(state_path)])
    return snapshot, tmp_path / "cache", state_path, fixture_tar, configs


def test_save_preserves_tags_and_retry_reuses_without_save(setup):
    snapshot, cache, state, _, _ = setup
    archive, receipt = bundle.materialize(snapshot, cache, TAGS)
    data = bundle.verify(snapshot, archive, receipt, TAGS)
    assert set(data["images"]) == set(TAGS)
    assert data["validation"] == "NOT_ASSERTED"
    assert bundle.materialize(snapshot, cache, list(reversed(TAGS))) == (archive, receipt)
    assert json.loads(state.read_text())["saves"] == 1


def test_corrupt_cache_is_not_overwritten_or_resaved(setup):
    snapshot, cache, state, _, _ = setup
    archive, _ = bundle.materialize(snapshot, cache, TAGS)
    archive.write_bytes(archive.read_bytes() + b"corrupt")
    with pytest.raises(ValueError, match="integrity"):
        bundle.materialize(snapshot, cache, TAGS)
    assert json.loads(state.read_text())["saves"] == 1
    assert archive.read_bytes().endswith(b"corrupt")


def test_retagged_current_image_rejects_old_receipt(setup):
    snapshot, cache, state_path, _, _ = setup
    archive, receipt = bundle.materialize(snapshot, cache, TAGS)
    state = json.loads(state_path.read_text())
    state["images"][TAGS[0]]["id"] = "sha256:" + "0" * 64
    state_path.write_text(json.dumps(state), encoding="utf-8")
    with pytest.raises(ValueError, match="current tags changed"):
        bundle.verify(snapshot, archive, receipt, TAGS)


def test_new_inventory_gets_new_key_without_overwriting_old_cache(setup):
    snapshot, cache, state_path, fixture_tar, configs = setup
    previous, _ = bundle.materialize(snapshot, cache, TAGS)
    original_hash = candidate.file_digest(previous)
    configs[TAGS[0]]["config"]["Labels"]["revision"] = "new"
    saved_tar(fixture_tar, configs)
    state = json.loads(state_path.read_text())
    state["images"][TAGS[0]]["id"] = "sha256:" + candidate.digest(candidate.canonical(configs[TAGS[0]]))
    state_path.write_text(json.dumps(state), encoding="utf-8")
    current, _ = bundle.materialize(snapshot, cache, TAGS)
    assert current != previous
    assert candidate.file_digest(previous) == original_hash
    assert json.loads(state_path.read_text())["saves"] == 2


def test_retag_during_save_never_publishes_cache(setup):
    snapshot, cache, state_path, _, _ = setup
    state = json.loads(state_path.read_text())
    state["retag_after_save"] = True
    state_path.write_text(json.dumps(state), encoding="utf-8")
    with pytest.raises(ValueError, match="changed during save"):
        bundle.materialize(snapshot, cache, TAGS)
    assert not list(cache.glob("*/bundle.json"))


@pytest.mark.parametrize("change", ["config", "tag", "missing-layer", "traversal", "symlink", "duplicate"])
def test_invalid_saved_archive_not_published(setup, change):
    snapshot, cache, _, archive, configs = setup
    saved_tar(archive, configs, change=change)
    with pytest.raises(ValueError):
        bundle.materialize(snapshot, cache, TAGS)
    assert not list(cache.glob("*/bundle.json"))


def test_platform_mismatch_rejected_before_save(setup):
    snapshot, cache, state_path, _, _ = setup
    state = json.loads(state_path.read_text())
    state["images"][TAGS[0]]["platform"] = "linux/arm64"
    state_path.write_text(json.dumps(state), encoding="utf-8")
    with pytest.raises(ValueError, match="linux/amd64"):
        bundle.materialize(snapshot, cache, TAGS)
    assert json.loads(state_path.read_text())["saves"] == 0


@pytest.mark.parametrize("change", [None, "wrong-config", "child-tamper", "missing-child", "wrong-platform", "duplicate-platform", "root-tamper", "wrong-tag"])
def test_containerd_oci_chain_verification(setup, change):
    snapshot, cache, state_path, fixture_tar, configs = setup
    inventory = oci_saved_tar(fixture_tar, configs, change=change)
    state = json.loads(state_path.read_text())
    state["images"] = inventory
    state_path.write_text(json.dumps(state), encoding="utf-8")
    if change:
        with pytest.raises(ValueError):
            bundle.materialize(snapshot, cache, TAGS)
        assert not list(cache.glob("*/bundle.json"))
    else:
        archive, receipt = bundle.materialize(snapshot, cache, TAGS)
        assert bundle.verify(snapshot, archive, receipt, TAGS)["images"] == inventory
        bundle.materialize(snapshot, cache, TAGS)
        assert json.loads(state_path.read_text())["saves"] == 1


def test_receipt_cannot_cross_candidate_or_claim_tests(setup):
    snapshot, cache, _, _, _ = setup
    archive, receipt = bundle.materialize(snapshot, cache, TAGS)
    original = json.loads(receipt.read_bytes())
    for field, value in (("source_id", "0" * 64), ("validation", "PASS")):
        data = {**original, field: value}
        data["receipt_id"] = candidate.digest(candidate.canonical({key: value for key, value in data.items() if key != "receipt_id"}))
        receipt.write_bytes(candidate.canonical(data))
        with pytest.raises(ValueError):
            bundle.verify(snapshot, archive, receipt, TAGS)


@pytest.mark.parametrize("tags", [[], ["missing-tag"], ["--option:bad"], ["foo@sha256:abcd"], ["a:1", "a:1"]])
def test_bad_tags_fail_without_docker(monkeypatch, tags):
    monkeypatch.setattr(bundle, "_docker", lambda *_args: pytest.fail("must reject before Docker"))
    with pytest.raises(ValueError):
        bundle.inspect(tags)
