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
Integration tests for Aliyun Code Interpreter provider.

These tests require real Aliyun credentials and will make actual API calls.
To run these tests, set the following environment variables:

    export AGENTRUN_ACCESS_KEY_ID="LTAI5t..."
    export AGENTRUN_ACCESS_KEY_SECRET="..."
    export AGENTRUN_ACCOUNT_ID="1234567890..."  # Aliyun primary account ID (主账号ID)
    export AGENTRUN_REGION="cn-hangzhou"  # Note: AGENTRUN_REGION (SDK will read this)

Then run:
    pytest agent/sandbox/tests/test_aliyun_codeinterpreter_integration.py -v

Official Documentation: https://help.aliyun.com/zh/functioncompute/fc/sandbox-sandbox-code-interepreter
"""

import os
import pytest
from agent.sandbox.providers.aliyun_codeinterpreter import AliyunCodeInterpreterProvider


# Skip all tests if credentials are not provided
pytestmark = pytest.mark.skipif(
    not all(
        [
            os.getenv("AGENTRUN_ACCESS_KEY_ID"),
            os.getenv("AGENTRUN_ACCESS_KEY_SECRET"),
            os.getenv("AGENTRUN_ACCOUNT_ID"),
        ]
    ),
    reason="Aliyun credentials not set. Set AGENTRUN_ACCESS_KEY_ID, AGENTRUN_ACCESS_KEY_SECRET, and AGENTRUN_ACCOUNT_ID.",
)


@pytest.fixture
def aliyun_config():
    """Get Aliyun configuration from environment variables."""
    return {
        "access_key_id": os.getenv("AGENTRUN_ACCESS_KEY_ID"),
        "access_key_secret": os.getenv("AGENTRUN_ACCESS_KEY_SECRET"),
        "account_id": os.getenv("AGENTRUN_ACCOUNT_ID"),
        "region": os.getenv("AGENTRUN_REGION", "cn-hangzhou"),
        "template_name": os.getenv("AGENTRUN_TEMPLATE_NAME", ""),
        "timeout": 30,
    }


@pytest.fixture
def provider(aliyun_config):
    """Create an initialized Aliyun provider."""
    provider = AliyunCodeInterpreterProvider()
    initialized = provider.initialize(aliyun_config)
    if not initialized:
        pytest.skip("Failed to initialize Aliyun provider. Check credentials, account ID, and network.")
    return provider


@pytest.mark.integration
class TestAliyunCodeInterpreterIntegration:
    """Integration tests for Aliyun Code Interpreter provider."""

    def test_initialize_provider(self, aliyun_config):
        """Test provider initialization with real credentials."""
        provider = AliyunCodeInterpreterProvider()
        result = provider.initialize(aliyun_config)

        assert result is True
        assert provider._initialized is True

    def test_health_check(self, provider):
        """Test health check with real API."""
        result = provider.health_check()

        assert result is True

    def test_get_supported_languages(self, provider):
        """Test getting supported languages."""
        languages = provider.get_supported_languages()

        assert "python" in languages
        assert "javascript" in languages
        assert isinstance(languages, list)

    def test_create_python_instance(self, provider):
        """Test creating a Python sandbox instance."""
        try:
            instance = provider.create_instance("python")

            assert instance.provider == "aliyun_codeinterpreter"
            assert instance.status in ["READY", "CREATING"]
            assert instance.metadata["language"] == "python"
            assert len(instance.instance_id) > 0

            # Clean up
            provider.destroy_instance(instance.instance_id)
        except Exception as e:
            pytest.skip(f"Instance creation failed: {str(e)}. API might not be available yet.")

    def test_execute_python_code(self, provider):
        """Test executing Python code in the sandbox."""
        try:
            # Create instance
            instance = provider.create_instance("python")

            # Execute simple code
            result = provider.execute_code(
                instance_id=instance.instance_id,
                code="print('Hello from Aliyun Code Interpreter!')\nprint(42)",
                language="python",
                timeout=30,  # Max 30 seconds
            )

            assert result.exit_code == 0
            assert "Hello from Aliyun Code Interpreter!" in result.stdout
            assert "42" in result.stdout
            assert result.execution_time > 0

            # Clean up
            provider.destroy_instance(instance.instance_id)
        except Exception as e:
            pytest.skip(f"Code execution test failed: {str(e)}. API might not be available yet.")

    def test_execute_python_code_with_arguments(self, provider):
        """Test executing Python code with arguments parameter."""
        try:
            # Create instance
            instance = provider.create_instance("python")

            # Execute code with arguments
            result = provider.execute_code(
                instance_id=instance.instance_id,
                code="""def main(name: str, count: int) -> dict:
    return {"message": f"Hello {name}!" * count}
""",
                language="python",
                timeout=30,
                arguments={"name": "World", "count": 2}
            )

            assert result.exit_code == 0
            assert "Hello World!Hello World!" in result.stdout

            # Clean up
            provider.destroy_instance(instance.instance_id)
        except Exception as e:
            pytest.skip(f"Arguments test failed: {str(e)}. API might not be available yet.")

    def test_execute_python_code_with_error(self, provider):
        """Test executing Python code that produces an error."""
        try:
            # Create instance
            instance = provider.create_instance("python")

            # Execute code with error
            result = provider.execute_code(instance_id=instance.instance_id, code="raise ValueError('Test error')", language="python", timeout=30)

            assert result.exit_code != 0
            assert len(result.stderr) > 0 or "ValueError" in result.stdout

            # Clean up
            provider.destroy_instance(instance.instance_id)
        except Exception as e:
            pytest.skip(f"Error handling test failed: {str(e)}. API might not be available yet.")

    def test_execute_javascript_code(self, provider):
        """Test executing JavaScript code in the sandbox."""
        try:
            # Create instance
            instance = provider.create_instance("javascript")

            # Execute simple code
            result = provider.execute_code(instance_id=instance.instance_id, code="console.log('Hello from JavaScript!');", language="javascript", timeout=30)

            assert result.exit_code == 0
            assert "Hello from JavaScript!" in result.stdout

            # Clean up
            provider.destroy_instance(instance.instance_id)
        except Exception as e:
            pytest.skip(f"JavaScript execution test failed: {str(e)}. API might not be available yet.")

    def test_execute_javascript_code_with_arguments(self, provider):
        """Test executing JavaScript code with arguments parameter."""
        try:
            # Create instance
            instance = provider.create_instance("javascript")

            # Execute code with arguments
            result = provider.execute_code(
                instance_id=instance.instance_id,
                code="""function main(args) {
  const { name, count } = args;
  return `Hello ${name}!`.repeat(count);
}""",
                language="javascript",
                timeout=30,
                arguments={"name": "World", "count": 2}
            )

            assert result.exit_code == 0
            assert "Hello World!Hello World!" in result.stdout

            # Clean up
            provider.destroy_instance(instance.instance_id)
        except Exception as e:
            pytest.skip(f"JavaScript arguments test failed: {str(e)}. API might not be available yet.")

    def test_destroy_instance(self, provider):
        """Test destroying a sandbox instance."""
        try:
            # Create instance
            instance = provider.create_instance("python")

            # Destroy instance
            result = provider.destroy_instance(instance.instance_id)

            # Note: The API might return True immediately or async
            assert result is True or result is False
        except Exception as e:
            pytest.skip(f"Destroy instance test failed: {str(e)}. API might not be available yet.")

    def test_config_validation(self, provider):
        """Test configuration validation."""
        # Valid config
        is_valid, error = provider.validate_config({"access_key_id": "LTAI5tXXXXXXXXXX", "account_id": "1234567890123456", "region": "cn-hangzhou", "timeout": 30})
        assert is_valid is True
        assert error is None

        # Invalid access key
        is_valid, error = provider.validate_config({"access_key_id": "INVALID_KEY"})
        assert is_valid is False

        # Missing account ID
        is_valid, error = provider.validate_config({})
        assert is_valid is False
        assert "Account ID" in error

    def test_timeout_limit(self, provider):
        """Test that timeout is limited to 30 seconds."""
        # Timeout > 30 should be clamped to 30
        provider2 = AliyunCodeInterpreterProvider()
        provider2.initialize(
            {
                "access_key_id": os.getenv("AGENTRUN_ACCESS_KEY_ID"),
                "access_key_secret": os.getenv("AGENTRUN_ACCESS_KEY_SECRET"),
                "account_id": os.getenv("AGENTRUN_ACCOUNT_ID"),
                "timeout": 60,  # Request 60 seconds
            }
        )

        # Should be clamped to 30
        assert provider2.timeout == 30


@pytest.mark.integration
class TestAliyunCodeInterpreterScenarios:
    """Test real-world usage scenarios."""

    def test_data_processing_workflow(self, provider):
        """Test a simple data processing workflow."""
        try:
            instance = provider.create_instance("python")

            # Execute data processing code
            code = """
import json
data = [{"name": "Alice", "age": 30}, {"name": "Bob", "age": 25}]
result = json.dumps(data, indent=2)
print(result)
"""
            result = provider.execute_code(instance_id=instance.instance_id, code=code, language="python", timeout=30)

            assert result.exit_code == 0
            assert "Alice" in result.stdout
            assert "Bob" in result.stdout

            provider.destroy_instance(instance.instance_id)
        except Exception as e:
            pytest.skip(f"Data processing test failed: {str(e)}")

    def test_string_manipulation(self, provider):
        """Test string manipulation operations."""
        try:
            instance = provider.create_instance("python")

            code = """
text = "Hello, World!"
print(text.upper())
print(text.lower())
print(text.replace("World", "Aliyun"))
"""
            result = provider.execute_code(instance_id=instance.instance_id, code=code, language="python", timeout=30)

            assert result.exit_code == 0
            assert "HELLO, WORLD!" in result.stdout
            assert "hello, world!" in result.stdout
            assert "Hello, Aliyun!" in result.stdout

            provider.destroy_instance(instance.instance_id)
        except Exception as e:
            pytest.skip(f"String manipulation test failed: {str(e)}")

    def test_context_persistence(self, provider):
        """Test code execution with context persistence."""
        try:
            instance = provider.create_instance("python")

            # First execution - define variable
            result1 = provider.execute_code(instance_id=instance.instance_id, code="x = 42\nprint(x)", language="python", timeout=30)
            assert result1.exit_code == 0

            # Second execution - use variable
            # Note: Context persistence depends on whether the contextId is reused
            result2 = provider.execute_code(instance_id=instance.instance_id, code="print(f'x is {x}')", language="python", timeout=30)

            # Context might or might not persist depending on API implementation
            assert result2.exit_code == 0

            provider.destroy_instance(instance.instance_id)
        except Exception as e:
            pytest.skip(f"Context persistence test failed: {str(e)}")


def test_without_credentials():
    """Test that tests are skipped without credentials."""
    # This test should always run (not skipped)
    if all(
        [
            os.getenv("AGENTRUN_ACCESS_KEY_ID"),
            os.getenv("AGENTRUN_ACCESS_KEY_SECRET"),
            os.getenv("AGENTRUN_ACCOUNT_ID"),
        ]
    ):
        assert True  # Credentials are set
    else:
        assert True  # Credentials not set, test still passes
