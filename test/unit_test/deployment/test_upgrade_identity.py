"""Real jq identity rules in isolated Bash; never executes the upgrade itself.

When native Bash lacks jq, set JQ_TEST_BINARY to a checksum-verified Linux jq
and JQ_TEST_IMAGE to an existing Bash image for a networkless disposable runner.
Missing prerequisites are explicit skipped/incomplete evidence, never success.
"""

import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess

import pytest


ROOT = Path(__file__).resolve().parents[3]
SOURCE = (ROOT / "deployment/linux-pg/upgrade.sh").read_text()
FUNCTION = SOURCE[SOURCE.index("candidate_source_id() {") : SOURCE.index("\nenv_value() {")]
PREFLIGHT = SOURCE[SOURCE.index("INCOMING_CANDIDATE=") : SOURCE.index('\nSTAGE_DIR="')]
DELIVERY_START = SOURCE.index("if [[ -n ${INCOMING_SOURCE_ID} ]]; then", SOURCE.index("postgres-schema-after.sql"))
DELIVERY = SOURCE[DELIVERY_START : SOURCE.index('\nsudo install -d -m 0700 "${SECRETS_DIR}"', DELIVERY_START)]


@pytest.fixture(scope="module")
def shell():
    bash = shutil.which("bash")
    if bash and subprocess.run([bash, "-c", "command -v jq"], capture_output=True).returncode == 0:
        return [bash, "-c", "bash -s"], ""
    binary = os.environ.get("JQ_TEST_BINARY")
    image = os.environ.get("JQ_TEST_IMAGE")
    if not binary or not image or not shutil.which("docker"):
        pytest.skip("INCOMPLETE: real jq requires native Bash+jq or explicit JQ_TEST_BINARY/JQ_TEST_IMAGE")
    path = Path(binary).resolve()
    assert path.is_file()
    assert hashlib.sha256(path.read_bytes()).hexdigest() == "5942c9b0934e510ee61eb3e30273f1b3fe2590df93933a93d7c58b81d19c8ff5", "Pinned jq 1.7.1 dependency checksum mismatch"
    return [
        "docker",
        "run",
        "--rm",
        "-i",
        "--network",
        "none",
        "--mount",
        f"type=bind,source={path},target=/fixture-jq,readonly",
        "--entrypoint",
        "bash",
        image,
        "-s",
    ], "cp /fixture-jq /tmp/jq\nchmod 0700 /tmp/jq\nexport PATH=/tmp:$PATH\n"


def manifest(version="1.2.3", source_id="a" * 64):
    return {"source_id": source_id, "identity": {"version": version}}


def run(shell, incoming, installed, current="v1.2.3", target="v1.2.3", delivery=False):
    command, prefix = shell
    harness = r"""
set -Eeuo pipefail
fixture=$(mktemp -d /tmp/ragflow-identity-test.XXXXXX)
trap 'rm -rf -- "$fixture"' EXIT
SOURCE_ROOT=$fixture/incoming
INSTALL_DIR=$fixture/installed
mkdir -p "$SOURCE_ROOT" "$INSTALL_DIR"
RAGFLOW_PORT=80
ALLOW_DOWNGRADE=0
die() { echo "$*" >&2; exit 1; }
upgrade_fail() { echo "$*" >&2; return 1; }
curl() { echo '{"status":"ok","db":"ok","redis":"ok","doc_engine":"ok","storage":"ok"}'; }
verify_installed_audit_table() { return 0; }
"""
    script = prefix + harness + FUNCTION + f"\nFROM_VERSION={shlex.quote(current)}\nRELEASE_VERSION={shlex.quote(target)}\n"
    for root, value in (("SOURCE_ROOT", incoming), ("INSTALL_DIR", installed)):
        if value is not None:
            script += f"printf '%s' {shlex.quote(json.dumps(value))} > \"${root}/DEPLOYMENT-CANDIDATE.json\"\n"
    if delivery:
        script += "INCOMING_CANDIDATE=$SOURCE_ROOT/DEPLOYMENT-CANDIDATE.json\nINSTALLED_CANDIDATE=$INSTALL_DIR/DEPLOYMENT-CANDIDATE.json\n"
        script += f"INCOMING_SOURCE_ID={shlex.quote(incoming['source_id'])}\n" + DELIVERY
    else:
        script += PREFLIGHT
    script += "\necho IDENTITY_ACCEPTED\n"
    result = subprocess.run(command, input=script.encode(), capture_output=True, timeout=30)
    result.stdout = result.stdout.decode(errors="replace")
    result.stderr = result.stderr.decode(errors="replace")
    return result


def test_same_version_requires_same_present_identity(shell):
    result = run(shell, manifest(), manifest())
    assert result.returncode == 0, result.stderr
    assert "already installed and healthy" in result.stdout
    for incoming, installed in ((manifest(source_id="b" * 64), manifest()), (manifest(), None), (None, manifest())):
        result = run(shell, incoming, installed)
        assert result.returncode != 0
        assert "already installed" not in result.stdout


def test_legacy_noop_warns_that_identity_is_unverified(shell):
    result = run(shell, None, None)
    assert result.returncode == 0, result.stderr
    assert "unverified source identity" in result.stderr


def test_new_version_allows_legacy_transition_but_not_identity_downgrade(shell):
    allowed = run(shell, manifest(version="1.2.4"), None, target="v1.2.4")
    assert allowed.returncode == 0, allowed.stderr
    assert "IDENTITY_ACCEPTED" in allowed.stdout
    denied = run(shell, None, manifest(), target="v1.2.4")
    assert denied.returncode != 0
    assert "identity downgrade" in denied.stderr


@pytest.mark.parametrize("invalid", [[], {"source_id": 123, "identity": {"version": "1.2.3"}}, manifest(source_id="short"), manifest(version="9.9.9"), manifest(version="1.2.3; echo unsafe")])
def test_invalid_json_shape_id_or_version_is_rejected(shell, invalid):
    result = run(shell, invalid, None)
    assert result.returncode != 0
    assert "Invalid candidate" in result.stderr
    assert "IDENTITY_ACCEPTED" not in result.stdout


def test_installed_version_must_match_known_deployed_version(shell):
    result = run(shell, manifest(version="1.2.4"), manifest(version="1.0.0"), target="v1.2.4")
    assert result.returncode != 0
    assert "Invalid candidate" in result.stderr


def test_delivery_compares_installed_bytes_without_asserting_runtime_identity(shell):
    result = run(shell, manifest(), manifest(), delivery=True)
    assert result.returncode == 0, result.stderr
    changed = {**manifest(), "extra": "different bytes"}
    result = run(shell, manifest(), changed, delivery=True)
    assert result.returncode != 0
    assert "manifest bytes differ" in result.stderr


def test_identity_checks_are_before_stop_and_before_acceptance_marker():
    assert SOURCE.index("INCOMING_CANDIDATE=") < SOURCE.index('APP_STOPPED=1\ncompose "${INSTALL_DIR}" stop ragflow-cpu')
    assert DELIVERY_START < SOURCE.index("UPGRADE-COMPLETED.env")
    assert 'candidate_source_id "${INSTALLED_CANDIDATE}" "${RELEASE_VERSION}"' in DELIVERY
