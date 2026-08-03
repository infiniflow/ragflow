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
Novita Agent Sandbox provider implementation.

This provider integrates with Novita AI's Agent Sandbox Code Interpreter
service for cloud-based code execution using the official novita-sandbox SDK.

Official Documentation: https://novita.ai/docs/guides/sandbox-overview
Official SDK: https://pypi.org/project/novita-sandbox/
"""

import logging
import time
from typing import Any, Dict, List, Optional

from agent.sandbox.result_protocol import build_javascript_wrapper, build_python_wrapper, extract_structured_result

from .base import ExecutionResult, SandboxInstance, SandboxProvider, SandboxProviderConfigError

logger = logging.getLogger(__name__)


class NovitaSandboxProvider(SandboxProvider):
    """
    Novita Agent Sandbox provider implementation.

    This provider uses the official novita-sandbox SDK's code_interpreter
    module to run code in Novita's cloud sandbox environments.
    """

    def __init__(self):
        self.api_key: str = ""
        self.domain: str = ""
        self.timeout: int = 30
        self._initialized: bool = False

    def initialize(self, config: Dict[str, Any]) -> bool:
        """
        Initialize the provider with Novita credentials.

        Args:
            config: Configuration dictionary with keys:
                - api_key: Novita API key
                - domain: Optional Novita sandbox domain override
                - timeout: Request timeout in seconds (default: 30)

        Returns:
            True if initialization successful, False otherwise
        """
        self.api_key = str(config.get("api_key", "") or "").strip()
        self.domain = str(config.get("domain", "") or "").strip()
        self.timeout = int(config.get("timeout", 30) or 30)

        if not self.api_key:
            logger.error("Novita Sandbox: Missing api_key")
            return False

        # novita-sandbox is an optional dependency: pull in httpx/pydantic/protobuf
        # versions that may differ from RAGFlow's pinned stack, so it is not a
        # core dependency. Fail loudly rather than silently no-op.
        _get_novita_module()

        self._initialized = True
        return True

    def create_instance(self, template: str = "python") -> SandboxInstance:
        """
        Create a new Novita sandbox instance.

        Args:
            template: Programming language template (python, nodejs)

        Returns:
            SandboxInstance object

        Raises:
            RuntimeError: If instance creation fails
        """
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        language = self._normalize_language(template)

        try:
            sandbox = self._create_sandbox()
        except SandboxProviderConfigError:
            raise
        except Exception as exc:
            raise RuntimeError(f"Failed to create Novita sandbox: {exc}") from exc

        return SandboxInstance(
            instance_id=sandbox.sandbox_id,
            provider="novita",
            status="running",
            metadata={"language": language},
        )

    def execute_code(self, instance_id: str, code: str, language: str, timeout: int = 10, arguments: Optional[Dict[str, Any]] = None) -> ExecutionResult:
        """
        Execute code in the Novita sandbox instance.

        Args:
            instance_id: ID of the sandbox instance
            code: Source code to execute
            language: Programming language (python, javascript, nodejs)
            timeout: Maximum execution time in seconds
            arguments: Optional arguments dict to pass to main() function

        Returns:
            ExecutionResult containing stdout, stderr, exit_code, and metadata

        Raises:
            RuntimeError: If execution fails
            TimeoutError: If execution exceeds timeout
        """
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        novita = _get_novita_module()
        normalized_lang = self._normalize_language(language)

        import json

        args_json = json.dumps(arguments or {})
        if normalized_lang == "python":
            wrapped_code = build_python_wrapper(code, args_json)
        else:
            wrapped_code = build_javascript_wrapper(code, args_json)

        exec_timeout = timeout or self.timeout

        try:
            sandbox = novita.code_interpreter.Sandbox.connect(
                instance_id,
                api_key=self.api_key,
                domain=self.domain or None,
            )

            start_time = time.time()
            run_language = None if normalized_lang == "python" else "javascript"
            execution = sandbox.run_code(wrapped_code, language=run_language, timeout=exec_timeout)
            execution_time = time.time() - start_time

            stdout = "".join(execution.logs.stdout)
            stderr = "".join(execution.logs.stderr)
            exit_code = 0

            if execution.error is not None:
                stderr = f"{stderr}\n{execution.error.name}: {execution.error.value}".strip()
                exit_code = 1

            stdout, structured_result = extract_structured_result(stdout)

            return ExecutionResult(
                stdout=stdout,
                stderr=stderr,
                exit_code=exit_code,
                execution_time=execution_time,
                metadata={
                    "instance_id": instance_id,
                    "language": normalized_lang,
                    "result_present": structured_result.get("present", False),
                    "result_value": structured_result.get("value"),
                    "result_type": structured_result.get("type"),
                },
            )
        except novita.SandboxException as exc:
            if "timeout" in str(exc).lower():
                raise TimeoutError(f"Execution timed out after {exec_timeout} seconds") from exc
            raise RuntimeError(f"Failed to execute code: {exc}") from exc
        except Exception as exc:
            raise RuntimeError(f"Unexpected error during execution: {exc}") from exc

    def destroy_instance(self, instance_id: str) -> bool:
        """
        Destroy a Novita sandbox instance.

        Args:
            instance_id: ID of the instance to destroy

        Returns:
            True if destruction successful, False otherwise
        """
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        novita = _get_novita_module()

        try:
            return novita.code_interpreter.Sandbox.kill(instance_id, api_key=self.api_key, domain=self.domain or None)
        except Exception as exc:
            logger.warning(f"Failed to destroy Novita sandbox instance {instance_id}: {exc}")
            return False

    def health_check(self) -> bool:
        """
        Check if the Novita Sandbox service is accessible.

        Returns:
            True if provider is healthy, False otherwise
        """
        if not self._initialized:
            return False

        try:
            sandbox = self._create_sandbox()
            sandbox.kill()
            return True
        except Exception as exc:
            logger.warning(f"Novita Sandbox health check failed: {exc}")
            return False

    def get_supported_languages(self) -> List[str]:
        """
        Get list of supported programming languages.

        Returns:
            List of language identifiers
        """
        return ["python", "nodejs", "javascript"]

    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        """
        Return configuration schema for Novita Sandbox provider.

        Returns:
            Dictionary mapping field names to their schema definitions
        """
        return {
            "api_key": {
                "type": "string",
                "required": True,
                "label": "API Key",
                "placeholder": "sk_...",
                "description": "Novita API key for authentication",
                "secret": True,
            },
            "domain": {
                "type": "string",
                "required": False,
                "label": "Domain",
                "placeholder": "us-phx-1.sandbox.novita.ai",
                "description": "Optional Novita sandbox domain override",
            },
            "timeout": {
                "type": "integer",
                "required": False,
                "label": "Request Timeout (seconds)",
                "default": 30,
                "min": 5,
                "max": 300,
                "description": "API request timeout for code execution",
            },
        }

    def validate_config(self, config: Dict[str, Any]) -> tuple[bool, Optional[str]]:
        """
        Validate Novita-specific configuration.

        Args:
            config: Configuration dictionary to validate

        Returns:
            Tuple of (is_valid, error_message)
        """
        api_key = config.get("api_key", "")
        if not api_key:
            return False, "API key is required"

        timeout = config.get("timeout", 30)
        if isinstance(timeout, int) and (timeout < 1 or timeout > 300):
            return False, "Timeout must be between 1 and 300 seconds"

        return True, None

    def _create_sandbox(self):
        novita = _get_novita_module()
        return novita.code_interpreter.Sandbox.create(
            api_key=self.api_key,
            domain=self.domain or None,
            timeout=self.timeout,
        )

    def _normalize_language(self, language: str) -> str:
        """
        Normalize language identifier to Novita Sandbox format.

        Args:
            language: Language identifier

        Returns:
            Normalized language identifier
        """
        if not language:
            return "python"

        lang_lower = language.lower()
        if lang_lower in ("python", "python3"):
            return "python"
        elif lang_lower in ("javascript", "nodejs"):
            return "nodejs"
        else:
            return language


def _get_novita_module():
    try:
        import novita_sandbox
        import novita_sandbox.code_interpreter  # noqa: F401
    except ImportError as exc:
        # novita-sandbox is an optional dependency: it pulls in httpx/pydantic/
        # protobuf version ranges that may not match RAGFlow's pinned stack, so
        # it is not a core dependency. Install it into the runtime to enable
        # this provider.
        raise SandboxProviderConfigError("novita-sandbox is required for the Novita Sandbox provider. Install it with `pip install novita-sandbox` (or `uv pip install novita-sandbox`).") from exc
    return novita_sandbox
