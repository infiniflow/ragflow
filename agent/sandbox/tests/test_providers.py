#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
Unit tests for sandbox provider abstraction layer.
"""

import pytest
from unittest.mock import Mock, patch
import requests

from agent.sandbox.providers.base import SandboxProvider, SandboxInstance, ExecutionResult
from agent.sandbox.providers.manager import ProviderManager
from agent.sandbox.providers.self_managed import SelfManagedProvider


class TestSandboxDataclasses:
    """Test sandbox dataclasses."""

    def test_sandbox_instance_creation(self):
        """Test SandboxInstance dataclass creation."""
        instance = SandboxInstance(
            instance_id="test-123",
            provider="self_managed",
            status="running",
            metadata={"language": "python"}
        )

        assert instance.instance_id == "test-123"
        assert instance.provider == "self_managed"
        assert instance.status == "running"
        assert instance.metadata == {"language": "python"}

    def test_sandbox_instance_default_metadata(self):
        """Test SandboxInstance with None metadata."""
        instance = SandboxInstance(
            instance_id="test-123",
            provider="self_managed",
            status="running",
            metadata=None
        )

        assert instance.metadata == {}

    def test_execution_result_creation(self):
        """Test ExecutionResult dataclass creation."""
        result = ExecutionResult(
            stdout="Hello, World!",
            stderr="",
            exit_code=0,
            execution_time=1.5,
            metadata={"status": "success"}
        )

        assert result.stdout == "Hello, World!"
        assert result.stderr == ""
        assert result.exit_code == 0
        assert result.execution_time == 1.5
        assert result.metadata == {"status": "success"}

    def test_execution_result_default_metadata(self):
        """Test ExecutionResult with None metadata."""
        result = ExecutionResult(
            stdout="output",
            stderr="error",
            exit_code=1,
            execution_time=0.5,
            metadata=None
        )

        assert result.metadata == {}


class TestProviderManager:
    """Test ProviderManager functionality."""

    def test_manager_initialization(self):
        """Test ProviderManager initialization."""
        manager = ProviderManager()

        assert manager.current_provider is None
        assert manager.current_provider_name is None
        assert not manager.is_configured()

    def test_set_provider(self):
        """Test setting a provider."""
        manager = ProviderManager()
        mock_provider = Mock(spec=SandboxProvider)

        manager.set_provider("self_managed", mock_provider)

        assert manager.current_provider == mock_provider
        assert manager.current_provider_name == "self_managed"
        assert manager.is_configured()

    def test_get_provider(self):
        """Test getting the current provider."""
        manager = ProviderManager()
        mock_provider = Mock(spec=SandboxProvider)

        manager.set_provider("self_managed", mock_provider)

        assert manager.get_provider() == mock_provider

    def test_get_provider_name(self):
        """Test getting the current provider name."""
        manager = ProviderManager()
        mock_provider = Mock(spec=SandboxProvider)

        manager.set_provider("self_managed", mock_provider)

        assert manager.get_provider_name() == "self_managed"

    def test_get_provider_when_not_set(self):
        """Test getting provider when none is set."""
        manager = ProviderManager()

        assert manager.get_provider() is None
        assert manager.get_provider_name() is None


class TestSelfManagedProvider:
    """Test SelfManagedProvider implementation."""

    def test_provider_initialization(self):
        """Test provider initialization."""
        provider = SelfManagedProvider()

        assert provider.endpoint == "http://localhost:9385"
        assert provider.timeout == 30
        assert provider.max_retries == 3
        assert provider.pool_size == 10
        assert not provider._initialized

    @patch('requests.get')
    def test_initialize_success(self, mock_get):
        """Test successful initialization."""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_get.return_value = mock_response

        provider = SelfManagedProvider()
        result = provider.initialize({
            "endpoint": "http://test-endpoint:9385",
            "timeout": 60,
            "max_retries": 5,
            "pool_size": 20
        })

        assert result is True
        assert provider.endpoint == "http://test-endpoint:9385"
        assert provider.timeout == 60
        assert provider.max_retries == 5
        assert provider.pool_size == 20
        assert provider._initialized
        mock_get.assert_called_once_with("http://test-endpoint:9385/healthz", timeout=5)

    @patch('requests.get')
    def test_initialize_failure(self, mock_get):
        """Test initialization failure."""
        mock_get.side_effect = Exception("Connection error")

        provider = SelfManagedProvider()
        result = provider.initialize({"endpoint": "http://invalid:9385"})

        assert result is False
        assert not provider._initialized

    def test_initialize_default_config(self):
        """Test initialization with default config."""
        with patch('requests.get') as mock_get:
            mock_response = Mock()
            mock_response.status_code = 200
            mock_get.return_value = mock_response

            provider = SelfManagedProvider()
            result = provider.initialize({})

            assert result is True
            assert provider.endpoint == "http://localhost:9385"
            assert provider.timeout == 30

    def test_create_instance_python(self):
        """Test creating a Python instance."""
        provider = SelfManagedProvider()
        provider._initialized = True

        instance = provider.create_instance("python")

        assert instance.provider == "self_managed"
        assert instance.status == "running"
        assert instance.metadata["language"] == "python"
        assert instance.metadata["endpoint"] == "http://localhost:9385"
        assert len(instance.instance_id) > 0  # Verify instance_id exists

    def test_create_instance_nodejs(self):
        """Test creating a Node.js instance."""
        provider = SelfManagedProvider()
        provider._initialized = True

        instance = provider.create_instance("nodejs")

        assert instance.metadata["language"] == "nodejs"

    def test_create_instance_not_initialized(self):
        """Test creating instance when provider not initialized."""
        provider = SelfManagedProvider()

        with pytest.raises(RuntimeError, match="Provider not initialized"):
            provider.create_instance("python")

    @patch('requests.post')
    def test_execute_code_success(self, mock_post):
        """Test successful code execution."""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "status": "success",
            "stdout": '{"result": 42}',
            "stderr": "",
            "exit_code": 0,
            "time_used_ms": 100.0,
            "memory_used_kb": 1024.0
        }
        mock_post.return_value = mock_response

        provider = SelfManagedProvider()
        provider._initialized = True

        result = provider.execute_code(
            instance_id="test-123",
            code="def main(): return {'result': 42}",
            language="python",
            timeout=10
        )

        assert result.stdout == '{"result": 42}'
        assert result.stderr == ""
        assert result.exit_code == 0
        assert result.execution_time > 0
        assert result.metadata["status"] == "success"
        assert result.metadata["instance_id"] == "test-123"

    @patch('requests.post')
    def test_execute_code_timeout(self, mock_post):
        """Test code execution timeout."""
        mock_post.side_effect = requests.Timeout()

        provider = SelfManagedProvider()
        provider._initialized = True

        with pytest.raises(TimeoutError, match="Execution timed out"):
            provider.execute_code(
                instance_id="test-123",
                code="while True: pass",
                language="python",
                timeout=5
            )

    @patch('requests.post')
    def test_execute_code_http_error(self, mock_post):
        """Test code execution with HTTP error."""
        mock_response = Mock()
        mock_response.status_code = 500
        mock_response.text = "Internal Server Error"
        mock_post.return_value = mock_response

        provider = SelfManagedProvider()
        provider._initialized = True

        with pytest.raises(RuntimeError, match="HTTP 500"):
            provider.execute_code(
                instance_id="test-123",
                code="invalid code",
                language="python"
            )

    def test_execute_code_not_initialized(self):
        """Test executing code when provider not initialized."""
        provider = SelfManagedProvider()

        with pytest.raises(RuntimeError, match="Provider not initialized"):
            provider.execute_code(
                instance_id="test-123",
                code="print('hello')",
                language="python"
            )

    def test_destroy_instance(self):
        """Test destroying an instance (no-op for self-managed)."""
        provider = SelfManagedProvider()
        provider._initialized = True

        # For self-managed, destroy_instance is a no-op
        result = provider.destroy_instance("test-123")

        assert result is True

    @patch('requests.get')
    def test_health_check_success(self, mock_get):
        """Test successful health check."""
        mock_response = Mock()
        mock_response.status_code = 200
        mock_get.return_value = mock_response

        provider = SelfManagedProvider()

        result = provider.health_check()

        assert result is True
        mock_get.assert_called_once_with("http://localhost:9385/healthz", timeout=5)

    @patch('requests.get')
    def test_health_check_failure(self, mock_get):
        """Test health check failure."""
        mock_get.side_effect = Exception("Connection error")

        provider = SelfManagedProvider()

        result = provider.health_check()

        assert result is False

    def test_get_supported_languages(self):
        """Test getting supported languages."""
        provider = SelfManagedProvider()

        languages = provider.get_supported_languages()

        assert "python" in languages
        assert "nodejs" in languages
        assert "javascript" in languages

    def test_get_config_schema(self):
        """Test getting configuration schema."""
        schema = SelfManagedProvider.get_config_schema()

        assert "endpoint" in schema
        assert schema["endpoint"]["type"] == "string"
        assert schema["endpoint"]["required"] is True
        assert schema["endpoint"]["default"] == "http://localhost:9385"

        assert "timeout" in schema
        assert schema["timeout"]["type"] == "integer"
        assert schema["timeout"]["default"] == 30

        assert "max_retries" in schema
        assert schema["max_retries"]["type"] == "integer"

        assert "pool_size" in schema
        assert schema["pool_size"]["type"] == "integer"

    def test_normalize_language_python(self):
        """Test normalizing Python language identifier."""
        provider = SelfManagedProvider()

        assert provider._normalize_language("python") == "python"
        assert provider._normalize_language("python3") == "python"
        assert provider._normalize_language("PYTHON") == "python"
        assert provider._normalize_language("Python3") == "python"

    def test_normalize_language_javascript(self):
        """Test normalizing JavaScript language identifier."""
        provider = SelfManagedProvider()

        assert provider._normalize_language("javascript") == "nodejs"
        assert provider._normalize_language("nodejs") == "nodejs"
        assert provider._normalize_language("JavaScript") == "nodejs"
        assert provider._normalize_language("NodeJS") == "nodejs"

    def test_normalize_language_default(self):
        """Test language normalization with empty/unknown input."""
        provider = SelfManagedProvider()

        assert provider._normalize_language("") == "python"
        assert provider._normalize_language(None) == "python"
        assert provider._normalize_language("unknown") == "unknown"


class TestProviderInterface:
    """Test that providers correctly implement the interface."""

    def test_self_managed_provider_is_abstract(self):
        """Test that SelfManagedProvider is a SandboxProvider."""
        provider = SelfManagedProvider()

        assert isinstance(provider, SandboxProvider)

    def test_self_managed_provider_has_abstract_methods(self):
        """Test that SelfManagedProvider implements all abstract methods."""
        provider = SelfManagedProvider()

        # Check all abstract methods are implemented
        assert hasattr(provider, 'initialize')
        assert callable(provider.initialize)

        assert hasattr(provider, 'create_instance')
        assert callable(provider.create_instance)

        assert hasattr(provider, 'execute_code')
        assert callable(provider.execute_code)

        assert hasattr(provider, 'destroy_instance')
        assert callable(provider.destroy_instance)

        assert hasattr(provider, 'health_check')
        assert callable(provider.health_check)

        assert hasattr(provider, 'get_supported_languages')
        assert callable(provider.get_supported_languages)
