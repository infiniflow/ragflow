"""Publication ordering contracts; shell probes never contact a registry or Git remote."""

from pathlib import Path
import shutil
import subprocess

import pytest
import yaml


ROOT = Path(__file__).resolve().parents[4]
RELEASE = ROOT / ".github/workflows/release.yml"
CHECKS = {
    "regression": "tests.yml",
    "separated_regression": "sep-tests.yml",
    "browser_regression": "browser-regression.yml",
}


def _workflow(path=RELEASE):
    return yaml.safe_load(path.read_text(encoding="utf-8"))


def _step(name):
    return next(step for step in _workflow()["jobs"]["release"]["steps"] if step.get("name") == name)


def test_publication_requires_same_run_regressions_and_build():
    workflow = _workflow()
    jobs = workflow["jobs"]
    assert set(jobs["release"]["needs"]) == {*CHECKS, "prepare", "build_cli"}
    assert "if" not in jobs["release"]  # Default success() rejects failed/skipped dependencies.
    assert "continue-on-error" not in jobs["release"]
    assert "release" in jobs["publish_cli_assets"]["needs"]
    assert "if" not in jobs["publish_cli_assets"]
    for job, filename in CHECKS.items():
        assert jobs[job]["uses"] == f"./.github/workflows/{filename}"
        assert "if" not in jobs[job]
        called = _workflow(ROOT / ".github/workflows" / filename)
        # PyYAML's YAML 1.1 loader represents the Actions 'on' key as True.
        assert "workflow_call" in called[True]
        assert called["concurrency"]["group"].startswith(filename.removesuffix(".yml") + "-")
        assert called["concurrency"]["group"] != workflow["concurrency"]["group"]


def test_tag_moves_only_after_image_build_and_before_publication():
    jobs = _workflow()["jobs"]
    prepare = "\n".join(step.get("run", "") for step in jobs["prepare"]["steps"])
    assert "git push" not in prepare
    assert "git tag" not in prepare
    steps = jobs["release"]["steps"]
    names = [step.get("name") for step in steps]
    assert names.index("Build release image") < names.index("Move the existing mutable tag") < names.index("Push release image")
    build = _step("Build release image")
    assert "docker build" in build["run"]
    assert "docker push" not in build["run"]
    assert "continue-on-error" not in build
    assert _step("Move the existing mutable tag")["if"] == "github.event_name == 'schedule'"


def test_release_checkouts_pin_source_and_publication_is_serialized():
    workflow = _workflow()
    assert workflow["concurrency"] == {"group": "release-publication", "cancel-in-progress": False}
    for job in workflow["jobs"].values():
        for step in job.get("steps", []):
            if step.get("uses", "").startswith("actions/checkout@"):
                assert step["with"]["ref"] == "${{ github.sha }}"


@pytest.mark.parametrize("prerelease,latest", [("true", False), ("false", True)])
def test_image_publish_does_not_promote_nightly_to_latest(prerelease, latest):
    bash = shutil.which("bash")
    assert bash, "Bash is required to exercise the actual release shell"
    script = f"PRERELEASE={prerelease}\nRELEASE_TAG={'nightly' if prerelease == 'true' else 'v1.2.3'}\nDOCKERHUB_TOKEN=fixture-only\n"
    script += 'sudo() { printf "%s\\n" "$*"; };\n' + _step("Push release image")["run"]
    result = subprocess.run(
        [bash, "-c", "bash -s"],
        input=script.encode(),
        capture_output=True,
        timeout=15,
    )
    assert result.returncode == 0, result.stderr
    assert (b"docker push infiniflow/ragflow:latest" in result.stdout) == latest
    assert b"fixture-only" not in result.stdout


def test_failed_registry_login_prevents_push():
    script = 'PRERELEASE=false\nRELEASE_TAG=v1.2.3\nDOCKERHUB_TOKEN=fixture-only\nsudo() { printf "%s\\n" "$*"; return 7; };\n' + _step("Push release image")["run"]
    result = subprocess.run(
        ["bash", "-c", "bash -s"],
        input=script.encode(),
        capture_output=True,
        timeout=15,
    )
    assert result.returncode == 7
    assert b"docker push" not in result.stdout
