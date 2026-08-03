#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""
Unit tests for Novita Sandbox provider.

These tests use fakes and don't require the real novita-sandbox SDK or
credentials, since novita-sandbox is an optional dependency.

Official Documentation: https://novita.ai/docs/guides/sandbox-overview
Official SDK: https://pypi.org/project/novita-sandbox/
"""

from types import SimpleNamespace

import pytest

from agent.sandbox.providers.base import SandboxProvider, SandboxProviderConfigError
from agent.sandbox.providers.novita import NovitaSandboxProvider


class _FakeLogs:
    def __init__(self, stdout=None, stderr=None):
        self.stdout = stdout or []
        self.stderr = stderr or []


class _FakeExecution:
    def __init__(self, stdout=None, stderr=None, error=None):
        self.logs = _FakeLogs(stdout, stderr)
        self.error = error


class _FakeSandbox:
    def __init__(self, sandbox_id="sbx-fake-1", run_handler=None):
        self.sandbox_id = sandbox_id
        self._run_handler = run_handler
        self.killed = False
        self.run_calls: list[tuple] = []

    def run_code(self, code, language=None, timeout=None):
        self.run_calls.append((code, language, timeout))
        if self._run_handler is not None:
            return self._run_handler(code, language, timeout)
        return _FakeExecution(stdout=["ok\n"])

    def kill(self):
        self.killed = True
        return True


class _FakeSandboxClass:
    def __init__(self, sandbox: _FakeSandbox):
        self._sandbox = sandbox
        self.create_kwargs = None
        self.connect_kwargs = None
        self.kill_kwargs = None

    def create(self, **kwargs):
        self.create_kwargs = kwargs
        return self._sandbox

    def connect(self, sandbox_id, **kwargs):
        self.connect_kwargs = (sandbox_id, kwargs)
        return self._sandbox

    def kill(self, sandbox_id, **kwargs):
        self.kill_kwargs = (sandbox_id, kwargs)
        return True


class _FakeNovitaModule:
    """Stand-in for the novita_sandbox module so tests need not install the SDK."""

    class SandboxException(Exception):
        pass

    def __init__(self, sandbox: _FakeSandbox):
        self.code_interpreter = SimpleNamespace(Sandbox=_FakeSandboxClass(sandbox))


def _build_provider(sandbox: _FakeSandbox, monkeypatch) -> tuple[NovitaSandboxProvider, _FakeNovitaModule]:
    provider = NovitaSandboxProvider()
    provider.api_key = "sk_test"
    provider.timeout = 30
    provider._initialized = True
    fake_module = _FakeNovitaModule(sandbox)
    monkeypatch.setattr("agent.sandbox.providers.novita._get_novita_module", lambda: fake_module)
    return provider, fake_module


class TestNovitaSandboxProviderInit:
    """Test NovitaSandboxProvider initialization."""

    def test_provider_initialization_defaults(self):
        provider = NovitaSandboxProvider()

        assert provider.api_key == ""
        assert provider.domain == ""
        assert provider.timeout == 30
        assert not provider._initialized

    def test_initialize_missing_api_key(self):
        provider = NovitaSandboxProvider()

        result = provider.initialize({})

        assert result is False
        assert not provider._initialized

    def test_initialize_success(self, monkeypatch):
        provider = NovitaSandboxProvider()
        monkeypatch.setattr("agent.sandbox.providers.novita._get_novita_module", lambda: _FakeNovitaModule(_FakeSandbox()))

        result = provider.initialize({"api_key": "sk_test", "domain": "us-phx-1.sandbox.novita.ai", "timeout": 20})

        assert result is True
        assert provider.api_key == "sk_test"
        assert provider.domain == "us-phx-1.sandbox.novita.ai"
        assert provider.timeout == 20
        assert provider._initialized

    def test_initialize_raises_when_sdk_missing(self, monkeypatch):
        provider = NovitaSandboxProvider()

        def _raise():
            raise SandboxProviderConfigError("novita-sandbox is required for the Novita Sandbox provider. Install it with `pip install novita-sandbox` (or `uv pip install novita-sandbox`).")

        monkeypatch.setattr("agent.sandbox.providers.novita._get_novita_module", _raise)

        with pytest.raises(SandboxProviderConfigError):
            provider.initialize({"api_key": "sk_test"})


class TestNovitaSandboxProviderExecution:
    """Test NovitaSandboxProvider create/execute/destroy."""

    def test_create_instance_returns_sandbox_id(self, monkeypatch):
        sandbox = _FakeSandbox(sandbox_id="sbx-abc")
        provider, fake_module = _build_provider(sandbox, monkeypatch)

        instance = provider.create_instance("python")

        assert instance.instance_id == "sbx-abc"
        assert instance.provider == "novita"
        assert instance.status == "running"
        assert instance.metadata["language"] == "python"

    def test_execute_code_success(self, monkeypatch):
        def run_handler(code, language, timeout):
            assert language is None  # python maps to default context
            return _FakeExecution(stdout=["hello\n"])

        sandbox = _FakeSandbox(run_handler=run_handler)
        provider, _ = _build_provider(sandbox, monkeypatch)

        result = provider.execute_code("sbx-abc", 'print("hello")', "python", timeout=5)

        assert result.exit_code == 0
        assert result.stdout == "hello\n"
        assert result.stderr == ""

    def test_execute_code_javascript_language_mapping(self, monkeypatch):
        captured = {}

        def run_handler(code, language, timeout):
            captured["language"] = language
            return _FakeExecution(stdout=["ok\n"])

        sandbox = _FakeSandbox(run_handler=run_handler)
        provider, _ = _build_provider(sandbox, monkeypatch)

        provider.execute_code("sbx-abc", "console.log('ok')", "javascript", timeout=5)

        assert captured["language"] == "javascript"

    def test_execute_code_reports_error(self, monkeypatch):
        error = SimpleNamespace(name="ValueError", value="boom")

        def run_handler(code, language, timeout):
            return _FakeExecution(stdout=[], stderr=["traceback\n"], error=error)

        sandbox = _FakeSandbox(run_handler=run_handler)
        provider, _ = _build_provider(sandbox, monkeypatch)

        result = provider.execute_code("sbx-abc", "raise ValueError()", "python", timeout=5)

        assert result.exit_code == 1
        assert "ValueError: boom" in result.stderr

    def test_destroy_instance_calls_kill(self, monkeypatch):
        sandbox = _FakeSandbox()
        provider, fake_module = _build_provider(sandbox, monkeypatch)

        result = provider.destroy_instance("sbx-abc")

        assert result is True
        assert fake_module.code_interpreter.Sandbox.kill_kwargs[0] == "sbx-abc"

    def test_execute_code_before_initialize_raises(self):
        provider = NovitaSandboxProvider()

        with pytest.raises(RuntimeError):
            provider.execute_code("sbx-abc", "print(1)", "python")


class TestNovitaSandboxProviderConfig:
    """Test config schema and validation."""

    def test_get_config_schema_has_required_fields(self):
        schema = NovitaSandboxProvider.get_config_schema()

        assert "api_key" in schema
        assert schema["api_key"]["required"] is True
        assert "domain" in schema
        assert "timeout" in schema

    def test_validate_config_requires_api_key(self):
        provider = NovitaSandboxProvider()

        is_valid, error = provider.validate_config({})

        assert is_valid is False
        assert error

    def test_validate_config_rejects_out_of_range_timeout(self):
        provider = NovitaSandboxProvider()

        is_valid, error = provider.validate_config({"api_key": "sk_test", "timeout": 1000})

        assert is_valid is False

    def test_validate_config_accepts_valid_config(self):
        provider = NovitaSandboxProvider()

        is_valid, error = provider.validate_config({"api_key": "sk_test", "timeout": 30})

        assert is_valid is True
        assert error is None

    def test_normalize_language(self):
        provider = NovitaSandboxProvider()

        assert provider._normalize_language("python") == "python"
        assert provider._normalize_language("python3") == "python"
        assert provider._normalize_language("javascript") == "nodejs"
        assert provider._normalize_language("nodejs") == "nodejs"
        assert provider._normalize_language("") == "python"


class TestNovitaSandboxProviderInterface:
    """Test that NovitaSandboxProvider correctly implements the interface."""

    def test_novita_provider_is_sandbox_provider(self):
        provider = NovitaSandboxProvider()

        assert isinstance(provider, SandboxProvider)

    def test_novita_provider_has_abstract_methods(self):
        provider = NovitaSandboxProvider()

        assert callable(provider.initialize)
        assert callable(provider.create_instance)
        assert callable(provider.execute_code)
        assert callable(provider.destroy_instance)
        assert callable(provider.health_check)
        assert callable(provider.get_supported_languages)
