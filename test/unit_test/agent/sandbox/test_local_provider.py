import base64
import sys

import pytest

from agent.sandbox.providers.local import LocalProvider


def _make_provider(tmp_path, **overrides):
    config = {
        "python_bin": sys.executable,
        "work_dir": str(tmp_path),
        "timeout": 5,
        "max_memory_mb": 512,
        "max_output_bytes": 1024 * 1024,
        "max_artifacts": 20,
        "max_artifact_bytes": 1024 * 1024,
    }
    config.update(overrides)
    provider = LocalProvider()
    provider.initialize(config)
    return provider


def test_local_provider_initializes_from_config(tmp_path):
    provider = LocalProvider()
    provider.initialize({"python_bin": sys.executable, "work_dir": str(tmp_path)})
    assert provider.health_check() is True


def test_local_provider_executes_python_main(tmp_path):
    provider = _make_provider(tmp_path)
    instance = provider.create_instance("python")

    try:
        result = provider.execute_code(
            instance.instance_id,
            'def main(name: str) -> dict:\n    return {"message": "hello " + name}\n',
            "python",
            timeout=5,
            arguments={"name": "ragflow"},
        )
    finally:
        provider.destroy_instance(instance.instance_id)

    assert result.exit_code == 0
    assert result.stdout == ""
    assert result.metadata["result_present"] is True
    assert result.metadata["result_value"] == {"message": "hello ragflow"}


def test_local_provider_collects_artifacts(tmp_path):
    provider = _make_provider(tmp_path)
    instance = provider.create_instance("python")

    try:
        result = provider.execute_code(
            instance.instance_id,
            ("from pathlib import Path\ndef main() -> dict:\n    Path('artifacts/chart.png').write_bytes(b'PNGDATA')\n    return {'ok': True}\n"),
            "python",
            timeout=5,
        )
    finally:
        provider.destroy_instance(instance.instance_id)

    assert result.metadata["artifacts"] == [
        {
            "name": "chart.png",
            "content_b64": base64.b64encode(b"PNGDATA").decode("ascii"),
            "mime_type": "image/png",
            "size": 7,
        }
    ]


def test_local_provider_times_out(tmp_path):
    provider = _make_provider(tmp_path, timeout=1)
    instance = provider.create_instance("python")

    try:
        with pytest.raises(TimeoutError):
            provider.execute_code(
                instance.instance_id,
                "import time\n\ndef main() -> dict:\n    time.sleep(5)\n    return {'ok': True}\n",
                "python",
                timeout=1,
            )
    finally:
        provider.destroy_instance(instance.instance_id)
