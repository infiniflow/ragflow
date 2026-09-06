"""VERSION compatibility without copying Git history into a Docker context."""

from pathlib import Path
import subprocess
import sys

import pytest
import yaml

from tools.quality.write_build_identity import write


ROOT = Path(__file__).resolve().parents[4]


@pytest.fixture
def project(tmp_path):
    (tmp_path / "pyproject.toml").write_text('[project]\nversion="1.2.3"\n', encoding="utf-8")
    return tmp_path


def test_standalone_metadata_explicitly_unverified(project):
    assert write(project) == ("1.2.3-unverified", "unverified")
    assert (project / "VERSION").read_bytes() == b"1.2.3-unverified\n"
    assert (project / "SOURCE_REVISION").read_bytes() == b"unverified\n"


def test_ci_supplied_git_describe_value_preserved(project):
    for args in (["init", "-q"], ["config", "user.name", "Synthetic"], ["config", "user.email", "qa@example.invalid"], ["add", "."], ["commit", "-qm", "baseline"], ["tag", "v1.2.3"]):
        subprocess.run(["git", "-C", str(project), *args], check=True, capture_output=True)
    version = subprocess.check_output(["git", "-C", str(project), "describe", "--tags", "--match=v*", "--first-parent", "--always"], text=True).strip()
    revision = subprocess.check_output(["git", "-C", str(project), "rev-parse", "HEAD"], text=True).strip()
    assert write(project, version, revision) == (version, revision)
    assert (project / "VERSION").read_text().strip() == "v1.2.3"


@pytest.mark.parametrize("version", ["v1.2.3", "v1.2.3-4-gabcdef12", "abcdef12", "1.2.3+build.1", "a" * 128])
def test_allowed_version_forms(project, version):
    assert write(project, version, "A" * 40) == (version, "a" * 40)


@pytest.mark.parametrize("version", ["bad\nversion", "../version", "$(secret)", "v 1", "-bad", "версия", "x" * 129])
def test_invalid_version_refused_before_writes(project, version):
    with pytest.raises(ValueError, match="version"):
        write(project, version)
    assert not (project / "VERSION").exists()
    assert not (project / "SOURCE_REVISION").exists()


@pytest.mark.parametrize("revision", ["abc123", "g" * 40, "a" * 41, "a" * 40 + "\n"])
def test_invalid_revision_refused_before_writes(project, revision):
    with pytest.raises(ValueError, match="revision"):
        write(project, "v1.2.3", revision)
    assert not (project / "VERSION").exists()


def test_cli_default_without_git(project):
    result = subprocess.run([sys.executable, str(ROOT / "tools/quality/write_build_identity.py"), "--root", str(project)], capture_output=True, text=True, timeout=15)
    assert result.returncode == 0, result.stdout + result.stderr
    assert "metadata only" in result.stdout
    assert "1.2.3-unverified" in result.stdout


def test_dockerfile_preserves_runtime_files_without_git():
    dockerfile = (ROOT / "Dockerfile").read_text()
    assert "COPY .git" not in dockerfile
    assert "git describe" not in dockerfile
    assert "ARG RAGFLOW_BUILD_VERSION" in dockerfile
    assert "ARG RAGFLOW_SOURCE_REVISION" in dockerfile
    assert "COPY --from=builder /ragflow/VERSION /ragflow/VERSION" in dockerfile
    assert "COPY --from=builder /ragflow/SOURCE_REVISION /ragflow/SOURCE_REVISION" in dockerfile
    assert ".git" in (ROOT / ".dockerignore").read_text().splitlines()


@pytest.mark.parametrize("workflow,count", [("tests.yml", 2), ("sep-tests.yml", 2), ("release.yml", 1)])
def test_all_main_image_ci_build_callers_supply_checkout_identity(workflow, count):
    document = yaml.safe_load((ROOT / ".github/workflows" / workflow).read_text())
    builds = []
    for job in document["jobs"].values():
        for step in job.get("steps", []):
            script = step.get("run", "")
            if "docker build " in script and "-f Dockerfile " in script:
                builds.append(script)
                assert "git describe --tags --match='v*' --first-parent --always" in script
                assert "git rev-parse HEAD" in script
                assert '--build-arg "RAGFLOW_BUILD_VERSION=${build_version}"' in script
                assert '--build-arg "RAGFLOW_SOURCE_REVISION=${source_revision}"' in script
    assert len(builds) == count
