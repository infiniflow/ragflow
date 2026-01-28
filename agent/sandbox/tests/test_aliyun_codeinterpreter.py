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
Unit tests for Aliyun Code Interpreter provider.

These tests use mocks and don't require real Aliyun credentials.

Official Documentation: https://help.aliyun.com/zh/functioncompute/fc/sandbox-sandbox-code-interepreter
Official SDK: https://github.com/Serverless-Devs/agentrun-sdk-python
"""

import pytest
from unittest.mock import patch, MagicMock

from agent.sandbox.providers.base import SandboxProvider
from agent.sandbox.providers.aliyun_codeinterpreter import AliyunCodeInterpreterProvider


class TestAliyunCodeInterpreterProvider:
    """Test AliyunCodeInterpreterProvider implementation."""

    def test_provider_initialization(self):
        """Test provider initialization."""
        provider = AliyunCodeInterpreterProvider()

        assert provider.access_key_id == ""
        assert provider.access_key_secret == ""
        assert provider.account_id == ""
        assert provider.region == "cn-hangzhou"
        assert provider.template_name == ""
        assert provider.timeout == 30
        assert not provider._initialized

    @patch("agent.sandbox.providers.aliyun_codeinterpreter.Template")
    def test_initialize_success(self, mock_template):
        """Test successful initialization."""
        # Mock health check response
        mock_template.list.return_value = []

        provider = AliyunCodeInterpreterProvider()
        result = provider.initialize(
            {
                "access_key_id": "LTAI5tXXXXXXXXXX",
                "access_key_secret": "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
                "account_id": "1234567890123456",
                "region": "cn-hangzhou",
                "template_name": "python-sandbox",
                "timeout": 20,
            }
        )

        assert result is True
        assert provider.access_key_id == "LTAI5tXXXXXXXXXX"
        assert provider.access_key_secret == "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
        assert provider.account_id == "1234567890123456"
        assert provider.region == "cn-hangzhou"
        assert provider.template_name == "python-sandbox"
        assert provider.timeout == 20
        assert provider._initialized

    def test_initialize_missing_credentials(self):
        """Test initialization with missing credentials."""
        provider = AliyunCodeInterpreterProvider()

        # Missing access_key_id
        result = provider.initialize({"access_key_secret": "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"})
        assert result is False

        # Missing access_key_secret
        result = provider.initialize({"access_key_id": "LTAI5tXXXXXXXXXX"})
        assert result is False

        # Missing account_id
        provider2 = AliyunCodeInterpreterProvider()
        result = provider2.initialize({"access_key_id": "LTAI5tXXXXXXXXXX", "access_key_secret": "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"})
        assert result is False

    @patch("agent.sandbox.providers.aliyun_codeinterpreter.Template")
    def test_initialize_default_config(self, mock_template):
        """Test initialization with default config."""
        mock_template.list.return_value = []

        provider = AliyunCodeInterpreterProvider()
        result = provider.initialize({"access_key_id": "LTAI5tXXXXXXXXXX", "access_key_secret": "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX", "account_id": "1234567890123456"})

        assert result is True
        assert provider.region == "cn-hangzhou"
        assert provider.template_name == ""

    @patch("agent.sandbox.providers.aliyun_codeinterpreter.CodeInterpreterSandbox")
    def test_create_instance_python(self, mock_sandbox_class):
        """Test creating a Python instance."""
        # Mock successful instance creation
        mock_sandbox = MagicMock()
        mock_sandbox.sandbox_id = "01JCED8Z9Y6XQVK8M2NRST5WXY"
        mock_sandbox_class.return_value = mock_sandbox

        provider = AliyunCodeInterpreterProvider()
        provider._initialized = True
        provider._config = MagicMock()

        instance = provider.create_instance("python")

        assert instance.provider == "aliyun_codeinterpreter"
        assert instance.status == "READY"
        assert instance.metadata["language"] == "python"

    @patch("agent.sandbox.providers.aliyun_codeinterpreter.CodeInterpreterSandbox")
    def test_create_instance_javascript(self, mock_sandbox_class):
        """Test creating a JavaScript instance."""
        mock_sandbox = MagicMock()
        mock_sandbox.sandbox_id = "01JCED8Z9Y6XQVK8M2NRST5WXY"
        mock_sandbox_class.return_value = mock_sandbox

        provider = AliyunCodeInterpreterProvider()
        provider._initialized = True
        provider._config = MagicMock()

        instance = provider.create_instance("javascript")

        assert instance.metadata["language"] == "javascript"

    def test_create_instance_not_initialized(self):
        """Test creating instance when provider not initialized."""
        provider = AliyunCodeInterpreterProvider()

        with pytest.raises(RuntimeError, match="Provider not initialized"):
            provider.create_instance("python")

    @patch("agent.sandbox.providers.aliyun_codeinterpreter.CodeInterpreterSandbox")
    def test_execute_code_success(self, mock_sandbox_class):
        """Test successful code execution."""
        # Mock sandbox instance
        mock_sandbox = MagicMock()
        mock_sandbox.context.execute.return_value = {
            "results": [{"type": "stdout", "text": "Hello, World!"}, {"type": "result", "text": "None"}, {"type": "endOfExecution", "status": "ok"}],
            "contextId": "kernel-12345-67890",
        }
        mock_sandbox_class.return_value = mock_sandbox

        provider = AliyunCodeInterpreterProvider()
        provider._initialized = True
        provider._config = MagicMock()

        result = provider.execute_code(instance_id="01JCED8Z9Y6XQVK8M2NRST5WXY", code="print('Hello, World!')", language="python", timeout=10)

        assert result.stdout == "Hello, World!"
        assert result.stderr == ""
        assert result.exit_code == 0
        assert result.execution_time > 0

    @patch("agent.sandbox.providers.aliyun_codeinterpreter.CodeInterpreterSandbox")
    def test_execute_code_timeout(self, mock_sandbox_class):
        """Test code execution timeout."""
        from agentrun.utils.exception import ServerError

        mock_sandbox = MagicMock()
        mock_sandbox.context.execute.side_effect = ServerError(408, "Request timeout")
        mock_sandbox_class.return_value = mock_sandbox

        provider = AliyunCodeInterpreterProvider()
        provider._initialized = True
        provider._config = MagicMock()

        with pytest.raises(TimeoutError, match="Execution timed out"):
            provider.execute_code(instance_id="01JCED8Z9Y6XQVK8M2NRST5WXY", code="while True: pass", language="python", timeout=5)

    @patch("agent.sandbox.providers.aliyun_codeinterpreter.CodeInterpreterSandbox")
    def test_execute_code_with_error(self, mock_sandbox_class):
        """Test code execution with error."""
        mock_sandbox = MagicMock()
        mock_sandbox.context.execute.return_value = {
            "results": [{"type": "stderr", "text": "Traceback..."}, {"type": "error", "text": "NameError: name 'x' is not defined"}, {"type": "endOfExecution", "status": "error"}]
        }
        mock_sandbox_class.return_value = mock_sandbox

        provider = AliyunCodeInterpreterProvider()
        provider._initialized = True
        provider._config = MagicMock()

        result = provider.execute_code(instance_id="01JCED8Z9Y6XQVK8M2NRST5WXY", code="print(x)", language="python")

        assert result.exit_code != 0
        assert len(result.stderr) > 0

    def test_get_supported_languages(self):
        """Test getting supported languages."""
        provider = AliyunCodeInterpreterProvider()

        languages = provider.get_supported_languages()

        assert "python" in languages
        assert "javascript" in languages

    def test_get_config_schema(self):
        """Test getting configuration schema."""
        schema = AliyunCodeInterpreterProvider.get_config_schema()

        assert "access_key_id" in schema
        assert schema["access_key_id"]["required"] is True

        assert "access_key_secret" in schema
        assert schema["access_key_secret"]["required"] is True

        assert "account_id" in schema
        assert schema["account_id"]["required"] is True

        assert "region" in schema
        assert "template_name" in schema
        assert "timeout" in schema

    def test_validate_config_success(self):
        """Test successful configuration validation."""
        provider = AliyunCodeInterpreterProvider()

        is_valid, error_msg = provider.validate_config({"access_key_id": "LTAI5tXXXXXXXXXX", "account_id": "1234567890123456", "region": "cn-hangzhou"})

        assert is_valid is True
        assert error_msg is None

    def test_validate_config_invalid_access_key(self):
        """Test validation with invalid access key format."""
        provider = AliyunCodeInterpreterProvider()

        is_valid, error_msg = provider.validate_config({"access_key_id": "INVALID_KEY"})

        assert is_valid is False
        assert "AccessKey ID format" in error_msg

    def test_validate_config_missing_account_id(self):
        """Test validation with missing account ID."""
        provider = AliyunCodeInterpreterProvider()

        is_valid, error_msg = provider.validate_config({})

        assert is_valid is False
        assert "Account ID" in error_msg

    def test_validate_config_invalid_region(self):
        """Test validation with invalid region."""
        provider = AliyunCodeInterpreterProvider()

        is_valid, error_msg = provider.validate_config(
            {
                "access_key_id": "LTAI5tXXXXXXXXXX",
                "account_id": "1234567890123456",  # Provide required field
                "region": "us-west-1",
            }
        )

        assert is_valid is False
        assert "Invalid region" in error_msg

    def test_validate_config_invalid_timeout(self):
        """Test validation with invalid timeout (> 30 seconds)."""
        provider = AliyunCodeInterpreterProvider()

        is_valid, error_msg = provider.validate_config(
            {
                "access_key_id": "LTAI5tXXXXXXXXXX",
                "account_id": "1234567890123456",  # Provide required field
                "timeout": 60,
            }
        )

        assert is_valid is False
        assert "Timeout must be between 1 and 30 seconds" in error_msg

    def test_normalize_language_python(self):
        """Test normalizing Python language identifier."""
        provider = AliyunCodeInterpreterProvider()

        assert provider._normalize_language("python") == "python"
        assert provider._normalize_language("python3") == "python"
        assert provider._normalize_language("PYTHON") == "python"

    def test_normalize_language_javascript(self):
        """Test normalizing JavaScript language identifier."""
        provider = AliyunCodeInterpreterProvider()

        assert provider._normalize_language("javascript") == "javascript"
        assert provider._normalize_language("nodejs") == "javascript"
        assert provider._normalize_language("JavaScript") == "javascript"


class TestAliyunCodeInterpreterInterface:
    """Test that Aliyun provider correctly implements the interface."""

    def test_aliyun_provider_is_abstract(self):
        """Test that AliyunCodeInterpreterProvider is a SandboxProvider."""
        provider = AliyunCodeInterpreterProvider()

        assert isinstance(provider, SandboxProvider)

    def test_aliyun_provider_has_abstract_methods(self):
        """Test that AliyunCodeInterpreterProvider implements all abstract methods."""
        provider = AliyunCodeInterpreterProvider()

        assert hasattr(provider, "initialize")
        assert callable(provider.initialize)

        assert hasattr(provider, "create_instance")
        assert callable(provider.create_instance)

        assert hasattr(provider, "execute_code")
        assert callable(provider.execute_code)

        assert hasattr(provider, "destroy_instance")
        assert callable(provider.destroy_instance)

        assert hasattr(provider, "health_check")
        assert callable(provider.health_check)

        assert hasattr(provider, "get_supported_languages")
        assert callable(provider.get_supported_languages)
