from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[3]
DEPLOYMENT = ROOT / "deployment" / "linux-pg"


def _read(name: str) -> str:
    return (DEPLOYMENT / name).read_text(encoding="utf-8")


@pytest.mark.parametrize("script_name", ["install.sh", "install_gvisor.sh", "upgrade.sh", "upgrade_offline.sh", "upgrade_registry.sh"])
def test_upgrade_shell_scripts_are_syntactically_valid(script_name) -> None:
    bash = shutil.which("bash")
    assert bash is not None, "Bash prerequisite missing; shell syntax validation is incomplete"

    subprocess.run(
        [
            bash,
            "-n",
            f"deployment/linux-pg/{script_name}",
        ],
        check=True,
        cwd=ROOT,
    )


@pytest.mark.parametrize("script_name", ["install.sh", "upgrade.sh"])
@pytest.mark.parametrize(
    ("environment", "expected_message"),
    [
        ({"OFFLINE_INSTALL": "unexpected"}, "OFFLINE_INSTALL must be 0 or 1."),
        ({"REGISTRY_INSTALL": "unexpected"}, "REGISTRY_INSTALL must be 0 or 1."),
        (
            {"OFFLINE_INSTALL": "1", "REGISTRY_INSTALL": "1"},
            "OFFLINE_INSTALL and REGISTRY_INSTALL cannot both be enabled.",
        ),
    ],
)
def test_deployment_modes_fail_closed_before_host_mutation(script_name, environment, expected_message) -> None:
    bash = shutil.which("bash")
    assert bash is not None, "Bash prerequisite missing; deployment mode validation is incomplete"

    assignments = " ".join(f"{key}={value}" for key, value in environment.items())
    result = subprocess.run(
        [bash, "-c", f"{assignments} bash deployment/linux-pg/{script_name}"],
        cwd=ROOT,
        env=os.environ,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        check=False,
    )

    assert result.returncode == 1
    assert expected_message in result.stderr


def test_upgrade_has_data_and_rollback_gates() -> None:
    script = _read("upgrade.sh")

    required_fragments = (
        "flock -n",
        "pg_dump",
        "pg_restore --list",
        "minio-data.tar.gz",
        "SHA256SUMS",
        "system_audit_event",
        "PREVIOUS_DIR",
        "wait_for_health",
        "DB_TYPE",
        "DOC_ENGINE",
        "seed_asr.py",
    )
    for fragment in required_fragments:
        assert fragment in script

    assert "docker compose down" not in script
    assert "down -v" not in script
    assert "docker volume rm" not in script
    assert "is already installed and healthy; no upgrade is required" in script
    assert "Target ${RELEASE_VERSION} is older than installed" in script
    assert 'SANDBOX_LOGS=$(sudo docker logs "${SANDBOX_CONTAINER}" 2>&1)' in script
    assert 'docker logs "${SANDBOX_CONTAINER}" 2>&1 | grep -Eq' not in script


def test_upgrade_preflight_checks_runtime_and_offline_images_before_success() -> None:
    script = _read("upgrade.sh")
    preflight = script[script.index('validate_compose "${STAGE_DIR}" 1') :]

    check_only = preflight.index('if [[ ${CHECK_ONLY} == "1" ]]')
    assert preflight.index("verify_runsc_runtime") < check_only
    assert preflight.index('verify_offline_images "${STAGE_DIR}"') < check_only
    assert "docker info --format '{{json .Runtimes}}' | grep -q" not in script


def test_install_acceptance_does_not_short_circuit_docker_log_producer() -> None:
    installer = _read("install.sh")
    gvisor_installer = _read("install_gvisor.sh")

    assert 'SANDBOX_LOGS=$(sudo docker logs "${SANDBOX_CONTAINER}" 2>&1)' in installer
    assert 'docker logs "${SANDBOX_CONTAINER}" 2>&1 | grep -Eq' not in installer
    assert "docker info --format '{{json .Runtimes}}' | grep -q" not in installer
    assert "docker info --format '{{json .Runtimes}}' | grep -q" not in gvisor_installer


def test_offline_upgrade_preflights_before_loading_images() -> None:
    script = _read("upgrade_offline.sh")

    assert script.index("run_preflight") < script.index("sudo docker load")
    assert "Docker images were not reloaded" in script


def test_gvisor_registration_restarts_only_when_runtime_path_changes() -> None:
    script = _read("install_gvisor.sh")
    assert 'if [[ ${runtime_path} != "/usr/local/bin/runsc" ]]; then' in script
    assert "sudo systemctl restart docker" in script
    assert "systemctl kill" not in script
    assert script.index("runtime_path=") < script.index("sudo /usr/local/bin/runsc install")


def test_registry_upgrade_preflights_before_pulling_images() -> None:
    script = _read("upgrade_registry.sh")

    assert script.index("run_preflight") < script.index("sudo docker login")
    assert "images were not pulled" in script


def test_release_builders_include_upgrade_entrypoints() -> None:
    source_builder = _read("build_archive.ps1")
    assert "upgrade.sh" in source_builder
    assert "$item.Name -eq '.git'" in source_builder
    assert "Linux scripts contain CR characters" in source_builder
    assert "[switch]$UseExistingFrontend" in source_builder
    assert "UseExistingFrontend requires web/dist/index.html" in source_builder
    assert "FRONTEND_MODE=$(if ($UseExistingFrontend) { 'prebuilt' } else { 'excluded' })" in source_builder
    assert "^deployment/linux-pg/release-[^/]+(/|$)" in source_builder
    assert "$item.Name -like 'cmake-build-*'" in source_builder
    assert "'coverage'" in source_builder
    assert "^[0-9a-f]{32,64}$" in source_builder
    assert "'rag/res/deepdoc'" in source_builder
    assert "^agent/business_requirements/document_constructor_" in source_builder
    offline_builder = _read("build_offline_archive.ps1")
    assert "upgrade_offline.sh" in offline_builder
    assert "GetRelativePath($payloadRoot" in offline_builder
    assert "$packageRoot, '.'" not in offline_builder
    registry_builder = _read("build_registry_archive.ps1")
    assert "upgrade_registry.sh" in registry_builder
    assert "GetRelativePath($payloadRoot" in registry_builder
    assert "$packageRoot, '.'" not in registry_builder


def test_full_server_stack_is_required_and_packaged() -> None:
    release = _read("docker-compose.release.yml")
    installer = _read("install.sh")
    upgrader = _read("upgrade.sh")
    offline_builder = _read("build_offline_archive.ps1")

    for service in (
        "t-one-asr",
        "sandbox-executor-manager",
        "otel-collector",
        "tempo",
        "loki",
        "prometheus",
        "grafana",
    ):
        assert f"  {service}:" in release
        assert service in installer
        assert service in upgrader

    assert "docker-compose.observability.yml" in installer
    assert "docker-compose.observability.yml" in upgrader
    assert "DOCKER_IMAGE_COUNT=$($dockerImages.Count)" in offline_builder
    assert "infiniflow/sandbox-base-python:latest" in offline_builder
    assert "infiniflow/sandbox-base-nodejs:latest" in offline_builder
    assert "prepare_gvisor_bundle.ps1" in offline_builder


def test_asr_transcription_has_domain_audit_without_content_payload() -> None:
    source = (ROOT / "api" / "apps" / "restful_apis" / "chat_api.py").read_text(encoding="utf-8")
    start = source.index("async def transcription():")
    end = source.index("\n\n@manager.route", start)
    transcription = source[start:end]

    assert 'action="asr.transcription"' in transcription
    assert 'reason_code="MISSING_FILE"' in transcription
    assert 'reason_code="UNSUPPORTED_FORMAT"' in transcription
    assert 'reason_code="MODEL_CONFIG_ERROR"' in transcription
    assert 'reason_code="TRANSCRIPTION_ERROR"' in transcription
    assert '"file_size_bytes": file_size_bytes' in transcription
    assert '"audio_format": suffix' in transcription
    assert '"text": text' not in transcription.split("metadata={", 1)[1].split("}", 1)[0]


def test_package_wrappers_verify_checksums_before_upgrade() -> None:
    for wrapper in ("upgrade_offline.sh", "upgrade_registry.sh"):
        script = _read(wrapper)
        checksum_position = script.index("sha256sum -c")
        upgrade_position = script.rindex("deployment/linux-pg/upgrade.sh")
        assert checksum_position < upgrade_position
        assert "--check" in script
