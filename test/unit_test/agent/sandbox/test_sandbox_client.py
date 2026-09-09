import pytest
from agent.sandbox import client as sandbox_client
from agent.sandbox.providers.self_managed import SelfManagedProvider

pytestmark = pytest.mark.p2


def test_client_defaults_to_self_managed(monkeypatch):
    class FakeSettingsService:
        @staticmethod
        def get_by_name(name):
            return []

    monkeypatch.setattr(sandbox_client, "SystemSettingsService", FakeSettingsService)
    monkeypatch.setattr(SelfManagedProvider, "initialize", lambda self, config: True)
    monkeypatch.setattr(sandbox_client, "_provider_manager", None)

    provider_manager = sandbox_client.get_provider_manager()

    assert provider_manager.get_provider_name() == "self_managed"
    assert isinstance(provider_manager.get_provider(), SelfManagedProvider)


def test_self_managed_schema_uses_env_for_deployment_defaults(monkeypatch):
    monkeypatch.setenv("SANDBOX_EXECUTOR_MANAGER_IMAGE", "custom-executor:latest")
    monkeypatch.setenv("SANDBOX_EXECUTOR_MANAGER_POOL_SIZE", "7")
    monkeypatch.setenv("SANDBOX_BASE_PYTHON_IMAGE", "custom-python:latest")
    monkeypatch.setenv("SANDBOX_BASE_NODEJS_IMAGE", "custom-node:latest")
    monkeypatch.setenv("SANDBOX_EXECUTOR_MANAGER_PORT", "19485")
    monkeypatch.setenv("SANDBOX_ENABLE_SECCOMP", "true")
    monkeypatch.setenv("SANDBOX_MAX_MEMORY", "512m")
    monkeypatch.setenv("SANDBOX_TIMEOUT", "25s")

    schema = SelfManagedProvider.get_config_schema()

    assert schema["executor_manager_image"]["default"] == "custom-executor:latest"
    assert schema["executor_manager_pool_size"]["default"] == 7
    assert schema["base_python_image"]["default"] == "custom-python:latest"
    assert schema["base_nodejs_image"]["default"] == "custom-node:latest"
    assert schema["executor_manager_port"]["default"] == 19485
    assert schema["enable_seccomp"]["default"] is True
    assert schema["max_memory"]["default"] == "512m"
    assert schema["sandbox_timeout"]["default"] == "25s"
