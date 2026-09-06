"""Package tiny synthetic candidates with the real PowerShell/tar entrypoint."""

import json
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile

import pytest

from tools.quality import candidate, frontend_artifact


ROOT = Path(__file__).resolve().parents[3]
SCRIPT = ROOT / "deployment/linux-pg/build_archive.ps1"
PROFILE = {"platform": "linux/amd64", "node": "22.20.0", "package_manager": "pnpm@10.33.0", "minify": "esbuild", "sourcemap": False, "mode": "production"}
REQUIRED = (
    "deployment/linux-pg/install.sh",
    "deployment/linux-pg/upgrade.sh",
    "deployment/linux-pg/install_gvisor.sh",
    "deployment/linux-pg/docker-compose.release.yml",
    "deployment/linux-pg/seed_admin.py",
    "deployment/linux-pg/seed_asr.py",
    "deployment/linux-pg/env.template",
    "ragflow_deps/prepare_native.py",
    "ragflow_deps/native-deps.json",
)


@pytest.fixture
def package(tmp_path, monkeypatch):
    assert shutil.which("pwsh"), "PowerShell prerequisite missing; archive validation is incomplete"
    source = tmp_path / "repo"
    source.mkdir()
    for relative in REQUIRED:
        path = source / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("# synthetic fixture\n", encoding="utf-8")
    (source / "web").mkdir()
    (source / "web/package.json").write_text('{"private":true}')
    (source / "web/pnpm-lock.yaml").write_text("lockfileVersion: '9.0'\n")
    (source / "pyproject.toml").write_text('[project]\nversion="1.2.3"\n')
    for args in (("init", "-q"), ("config", "user.name", "Fixture"), ("config", "user.email", "qa@example.invalid"), ("config", "core.autocrlf", "false"), ("add", "."), ("commit", "-qm", "fixture")):
        subprocess.run(["git", "-C", str(source), *args], check=True, capture_output=True)
    snapshot = tmp_path / "candidate"
    candidate.snapshot(source, snapshot, [], False, PROFILE)
    fake = tmp_path / "fake_build.py"
    fake.write_text(
        "import pathlib,sys\nkind,*args=sys.argv[1:]\n"
        "if args == ['--version']: print('v22.20.0' if kind == 'node' else '10.33.0')\n"
        "elif args[0] == 'install': pathlib.Path('node_modules').mkdir()\n"
        "elif args[0] == 'exec':\n"
        " pathlib.Path('dist/assets').mkdir(parents=True)\n"
        " pathlib.Path('dist/index.html').write_text('<html>synthetic built SPA</html>')\n"
        " pathlib.Path('dist/assets/app.js').write_text('synthetic')\n"
        " pathlib.Path('dist/assets/example.sh').write_bytes(b'example\\r\\n')\n"
        "else: raise AssertionError(args)\n"
    )
    monkeypatch.setattr(frontend_artifact, "_tools", lambda _pnpm_cli=None: ([sys.executable, str(fake), "node"], [sys.executable, str(fake), "pnpm"]))
    frontend = tmp_path / "frontend.tar.gz"
    frontend_artifact.build(snapshot, tmp_path / "work", frontend)
    return snapshot, frontend, tmp_path / "output"


def invoke(package, *extra, version="v1.2.3", include_frontend=True):
    snapshot, frontend, output = package
    command = ["pwsh", "-NoProfile", "-File", str(SCRIPT), "-ReleaseVersion", version, "-OutputDirectory", str(output), "-CandidateDirectory", str(snapshot), "-PythonCommand", sys.executable]
    if include_frontend:
        command.extend(["-UseExistingFrontend", "-FrontendArtifact", str(frontend)])
    return subprocess.run([*command, *extra], cwd=ROOT, capture_output=True, text=True, timeout=45)


def test_verified_candidate_and_frontend_are_packaged_and_integrity_recorded(package):
    result = invoke(package)
    assert result.returncode == 0, result.stdout + result.stderr
    snapshot, _, output = package
    archive = output / "ragflow-linux-pg-v1.2.3.tar.gz"
    receipt = candidate.verify_artifact(snapshot, archive, "linux-pg-source")
    assert "status" not in receipt and "validation" not in receipt
    with tarfile.open(archive) as bundle:
        assert bundle.extractfile("./web/dist/index.html").read() == b"<html>synthetic built SPA</html>"
        assert bundle.extractfile("./web/dist/assets/example.sh").read() == b"example\r\n"
        assert json.loads(bundle.extractfile("./DEPLOYMENT-CANDIDATE.json").read()) == candidate.verify(snapshot)
        deployment = bundle.extractfile("./DEPLOYMENT-SOURCE.env").read().decode()
        assert "FRONTEND_INTEGRITY=verified-receipt" in deployment
        assert "CANDIDATE_VALIDATION=NOT_ASSERTED" in deployment
        assert "SOURCE_ID=" + receipt["source_id"] in deployment
        assert all("node_modules" not in item.name and ".git/" not in item.name for item in bundle)


def test_candidate_source_only_does_not_claim_frontend_verification(package):
    result = invoke(package, include_frontend=False)
    assert result.returncode == 0, result.stdout + result.stderr
    with tarfile.open(package[2] / "ragflow-linux-pg-v1.2.3.tar.gz") as bundle:
        deployment = bundle.extractfile("./DEPLOYMENT-SOURCE.env").read().decode()
        assert "FRONTEND_INTEGRITY=NOT_ASSERTED" in deployment
        assert "./web/dist/index.html" not in bundle.getnames()


def test_canonical_candidate_archive_is_never_overwritten_or_rebuilt(package):
    first = invoke(package, include_frontend=False)
    assert first.returncode == 0, first.stdout + first.stderr
    snapshot, frontend, output = package
    archive = output / "ragflow-linux-pg-v1.2.3.tar.gz"
    before = archive.read_bytes()
    overwritten = invoke(package, "-Overwrite", include_frontend=False)
    assert overwritten.returncode != 0
    assert "overwritten or rebuilt" in overwritten.stderr
    assert archive.read_bytes() == before
    candidate.verify_artifact(snapshot, archive, "linux-pg-source")
    elsewhere = output.parent / "another-output"
    rebuilt = invoke((snapshot, frontend, elsewhere), include_frontend=False)
    assert rebuilt.returncode != 0
    assert "overwritten or rebuilt" in rebuilt.stderr
    assert not list(elsewhere.glob("*.tar.gz"))


@pytest.mark.parametrize("tamper", ["version", "version-prefix", "source", "archive", "receipt"])
def test_invalid_identity_or_artifact_fails_without_publishing_archive(package, tamper):
    snapshot, frontend, output = package
    version = "v9.9.9" if tamper == "version" else "v1.2.3"
    if tamper == "version-prefix":
        version = "vv1.2.3"
    if tamper == "source":
        (snapshot / "source/web/package.json").write_text("changed")
    elif tamper == "archive":
        frontend.write_bytes(frontend.read_bytes() + b"corrupt")
    elif tamper == "receipt":
        Path(str(frontend) + ".json").write_text("{}")
    result = invoke(package, version=version)
    assert result.returncode != 0
    assert not list(output.glob("*.tar.gz"))
    assert not (snapshot / "artifact-linux-pg-source.json").exists()


def test_output_cannot_mutate_candidate_source(package):
    snapshot, frontend, _ = package
    output = snapshot / "source/package-output"
    result = invoke((snapshot, frontend, output))
    assert result.returncode != 0
    assert "must not modify candidate source" in result.stderr
    assert not output.exists()
    candidate.verify(snapshot)


def test_frontend_artifact_requires_explicit_candidate_and_prebuilt_switch(tmp_path):
    for arguments in (("-FrontendArtifact", "unused"), ("-FrontendArtifact", "unused", "-CandidateDirectory", "unused")):
        result = subprocess.run(["pwsh", "-NoProfile", "-File", str(SCRIPT), "-ReleaseVersion", "v1.2.3", "-OutputDirectory", str(tmp_path), *arguments], capture_output=True, text=True, timeout=15)
        assert result.returncode != 0
        assert "requires CandidateDirectory and UseExistingFrontend" in result.stderr
    assert not list(tmp_path.iterdir())


def test_candidate_is_reverified_immediately_before_archive_creation():
    text = SCRIPT.read_text()
    tar_position = text.index("Invoke-Tar -ArgumentList @('-czf'")
    previous_verify = text.rindex("Invoke-QualityTool 'candidate.py' @('verify'", 0, tar_position)
    assert previous_verify > text.index("Forbidden release files were exported")
    assert "Candidate identity changed during packaging" in text[previous_verify:tar_position]
