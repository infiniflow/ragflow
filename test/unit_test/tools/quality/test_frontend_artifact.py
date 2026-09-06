"""Fake package executables exercise real child processes without network/builds."""

import json
import io
from pathlib import Path
import subprocess
import sys
import tarfile

import pytest

from tools.quality import candidate, frontend_artifact as frontend


PROFILE = {"platform": "linux/amd64", "node": "22.20.0", "package_manager": "pnpm@10.33.0", "minify": "esbuild", "sourcemap": False, "mode": "production"}


@pytest.fixture
def setup(tmp_path, monkeypatch):
    source = tmp_path / "source-repo"
    (source / "web/src").mkdir(parents=True)
    (source / "pyproject.toml").write_text('[project]\nversion="1.0.0"\n', encoding="utf-8")
    (source / "web/package.json").write_text('{"private":true}', encoding="utf-8")
    (source / "web/pnpm-lock.yaml").write_text("lockfileVersion: '9.0'\n", encoding="utf-8")
    (source / "web/src/index.ts").write_text("export const value = 1;", encoding="utf-8")
    for args in (
        ["init", "-q"],
        ["config", "user.name", "Synthetic"],
        ["config", "user.email", "qa@example.invalid"],
        ["config", "core.autocrlf", "false"],
        ["add", "."],
        ["commit", "-qm", "baseline"],
    ):
        subprocess.run(["git", "-C", str(source), *args], check=True, capture_output=True)
    snapshot = tmp_path / "candidate"
    candidate.snapshot(source, snapshot, [], False, PROFILE.copy())
    helper = tmp_path / "fake_tools.py"
    helper.write_text(
        """
import json, os, pathlib, sys
kind, *args = sys.argv[1:]
if args == ["--version"]:
    print("v22.20.0" if kind == "node" else "10.33.0")
elif args[0] == "install":
    assert "--frozen-lockfile" in args and "--ignore-scripts" in args and "--prod=false" in args
    assert "SYNTHETIC_SECRET" not in os.environ
    assert os.environ["VITE_MINIFY"] == "esbuild"
    assert os.environ["VITE_BUILD_SOURCEMAP"] == "false"
    pathlib.Path("node_modules").mkdir()
    pathlib.Path("node_modules/install-proof").write_text("installed")
elif args == ["exec", "vite", "build", "--mode", "production"]:
    assert pathlib.Path("node_modules/install-proof").exists()
    pathlib.Path("dist/assets").mkdir(parents=True)
    pathlib.Path("dist/index.html").write_text("<html>built</html>")
    pathlib.Path("dist/assets/app.js").write_text("console.log('built');")
else:
    raise AssertionError(args)
""",
        encoding="utf-8",
    )
    monkeypatch.setattr(frontend, "_tools", lambda pnpm_cli=None: ([sys.executable, str(helper), "node"], [sys.executable, str(helper), "pnpm"]))
    monkeypatch.setenv("SYNTHETIC_SECRET", "must-not-reach-build")
    return snapshot, tmp_path / "work", tmp_path / "frontend.tar.gz", helper


def test_build_isolated_artifact_and_cli_verification(setup):
    snapshot, work, archive, _ = setup
    before = candidate.verify(snapshot)
    receipt = frontend.build(snapshot, work, archive)
    assert receipt["source_id"] == before["source_id"]
    assert receipt["validation"] == "NOT_ASSERTED"
    assert set(receipt["files"]) == {"index.html", "assets/app.js"}
    assert receipt["toolchain"] == {"node": "22.20.0", "pnpm": "10.33.0"}
    assert not (snapshot / "source/web/dist").exists()
    assert candidate.verify(snapshot) == before
    result = subprocess.run(
        [sys.executable, "-m", "tools.quality.frontend_artifact", "verify", "--candidate", str(snapshot), "--archive", str(archive)],
        capture_output=True,
        text=True,
        timeout=20,
    )
    assert result.returncode == 0, result.stdout + result.stderr
    assert json.loads(result.stdout)["validation"] == "NOT_ASSERTED"


@pytest.mark.parametrize("change", ["inputs", "new-input", "version", "empty-output", "failure"])
def test_failed_or_mutating_build_has_no_completion_receipt(setup, change):
    snapshot, work, archive, helper = setup
    text = helper.read_text()
    if change == "inputs":
        text += '\nif args and args[0] == "install": pathlib.Path("src/index.ts").write_text("mutated")\n'
    elif change == "new-input":
        text += '\nif args and args[0] == "install": pathlib.Path(".env.local").write_text("injected")\n'
    elif change == "version":
        text = text.replace("v22.20.0", "v20.0.0")
    elif change == "empty-output":
        text = text.replace("<html>built</html>", "")
    else:
        text += '\nif args and args[0] == "install": raise SystemExit(12)\n'
    helper.write_text(text, encoding="utf-8")
    with pytest.raises(ValueError):
        frontend.build(snapshot, work, archive)
    assert not Path(str(archive) + ".json").exists()
    candidate.verify(snapshot)


@pytest.mark.parametrize("tamper", ["archive", "receipt", "candidate", "profile", "file-map", "toolchain"])
def test_reuse_rejects_tamper(setup, tamper):
    snapshot, work, archive, _ = setup
    frontend.build(snapshot, work, archive)
    receipt = Path(str(archive) + ".json")
    if tamper == "archive":
        archive.write_bytes(archive.read_bytes() + b"unexpected")
    elif tamper == "candidate":
        (snapshot / "source/web/src/index.ts").write_text("changed")
    else:
        data = json.loads(receipt.read_bytes())
        if tamper == "receipt":
            data["source_id"] = "0" * 64
        elif tamper == "profile":
            data["profile"]["minify"] = "terser"
        elif tamper == "toolchain":
            data["toolchain"]["node"] = "0.0.0"
        else:
            del data["files"]["assets/app.js"]
        if tamper != "receipt":
            data["receipt_id"] = candidate.digest(candidate.canonical({k: v for k, v in data.items() if k != "receipt_id"}))
        receipt.write_bytes(candidate.canonical(data))
    with pytest.raises(ValueError):
        frontend.verify(snapshot, archive, receipt)


def test_fresh_work_and_separate_output_required(setup):
    snapshot, work, archive, _ = setup
    work.mkdir()
    with pytest.raises(ValueError, match="fresh"):
        frontend.build(snapshot, work, archive)
    with pytest.raises(ValueError, match="separate"):
        frontend.build(snapshot, snapshot / "work", archive)


@pytest.mark.parametrize("field,value", [("package_manager", "npm@10.0.0"), ("node", "latest"), ("sourcemap", "false"), ("mode", "development"), ("minify", "other")])
def test_profile_is_explicit_and_supported(field, value):
    profile = {**PROFILE, field: value}
    with pytest.raises(ValueError):
        frontend._profile(profile)


def test_timeout_stops_owned_child(tmp_path):
    with pytest.raises(subprocess.TimeoutExpired):
        frontend._run([sys.executable, "-c", "import time;time.sleep(60)"], tmp_path, dict(), tmp_path / "timeout.log", 1)


def test_explicit_pnpm_cli_uses_selected_node_without_wrapper(tmp_path, monkeypatch):
    entrypoint = tmp_path / "pnpm.cjs"
    entrypoint.write_text("// synthetic entrypoint", encoding="utf-8")
    monkeypatch.setattr(frontend.shutil, "which", lambda name: "selected-node" if name == "node" else None)
    node, pnpm = frontend._tools(entrypoint)
    assert node == ["selected-node"]
    assert pnpm == ["selected-node", str(entrypoint.resolve())]


@pytest.mark.parametrize("name,exists", [("missing.cjs", False), ("pnpm.cmd", True), ("directory.js", True)])
def test_invalid_explicit_pnpm_cli_rejected(tmp_path, monkeypatch, name, exists):
    entrypoint = tmp_path / name
    if name == "directory.js":
        entrypoint.mkdir()
    elif exists:
        entrypoint.write_text("invalid", encoding="utf-8")
    monkeypatch.setattr(frontend.shutil, "which", lambda _: "selected-node")
    with pytest.raises(ValueError, match="regular"):
        frontend._tools(entrypoint)


@pytest.mark.parametrize("entry", ["../escape", "duplicate", "symlink"])
def test_archive_parser_rejects_unsafe_entries_even_with_updated_archive_hash(tmp_path, entry):
    archive = tmp_path / "bad.tar.gz"
    with tarfile.open(archive, "w:gz") as bundle:
        member = tarfile.TarInfo("../escape" if entry == "../escape" else "index.html")
        if entry == "symlink":
            member.type = tarfile.SYMTYPE
            member.linkname = "outside"
            bundle.addfile(member)
        else:
            member.size = 1
            bundle.addfile(member, io.BytesIO(b"x"))
            if entry == "duplicate":
                bundle.addfile(member, io.BytesIO(b"x"))
    receipt = {"archive": {"sha256": candidate.file_digest(archive), "size": archive.stat().st_size}, "files": {}}
    with pytest.raises(ValueError):
        frontend._verify_archive(archive, receipt)
