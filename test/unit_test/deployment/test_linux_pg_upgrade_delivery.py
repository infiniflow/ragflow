from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
DEPLOYMENT = ROOT / "deployment" / "linux-pg"


def _read(name: str) -> str:
    return (DEPLOYMENT / name).read_text(encoding="utf-8")


def test_upgrade_shell_scripts_are_syntactically_valid() -> None:
    bash = shutil.which("bash")
    if bash is None:
        return

    subprocess.run(
        [
            bash,
            "-n",
            "deployment/linux-pg/upgrade.sh",
            "deployment/linux-pg/upgrade_offline.sh",
            "deployment/linux-pg/upgrade_registry.sh",
        ],
        check=True,
        cwd=ROOT,
    )


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
