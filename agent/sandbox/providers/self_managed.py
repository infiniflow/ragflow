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
Self-managed sandbox provider implementation.

This provider wraps the existing executor_manager HTTP API which manages
a pool of Docker containers with gVisor for secure code execution.
"""

import base64
import os
import time
import uuid
from typing import Dict, Any, List, Optional

import requests

from .base import SandboxProvider, SandboxInstance, ExecutionResult


class SelfManagedProvider(SandboxProvider):
    """
    Self-managed sandbox provider using Daytona/Docker.

    This provider communicates with the executor_manager HTTP API
    which manages a pool of containers for code execution.
    """

    def __init__(self):
        self.endpoint: str = "http://sandbox-executor-manager:9385"
        self.timeout: int = 30
        self.max_retries: int = 3
        self.pool_size: int = 3
        self._initialized: bool = False

    def initialize(self, config: Dict[str, Any]) -> bool:
        """
        Initialize the provider with configuration.

        Args:
            config: Configuration dictionary with keys:
                - endpoint: HTTP endpoint (default: "http://sandbox-executor-manager:9385")
                - timeout: Request timeout in seconds (default: 30)
                - max_retries: Maximum retry attempts (default: 3)
                - pool_size: Container pool size for info (default: 10)

        Returns:
            True if initialization successful, False otherwise
        """
        self.endpoint = config.get("endpoint", "http://sandbox-executor-manager:9385")
        self.timeout = config.get("timeout", 30)
        self.max_retries = config.get("max_retries", 3)
        self.pool_size = config.get("executor_manager_pool_size", config.get("pool_size", 3))

        # Validate endpoint is accessible
        if not self.health_check():
            return False

        self._initialized = True
        return True

    def create_instance(self, template: str = "python") -> SandboxInstance:
        """
        Create a new sandbox instance.

        Note: For self-managed provider, instances are managed internally
        by the executor_manager's container pool. This method returns
        a logical instance handle.

        Args:
            template: Programming language (python, nodejs)

        Returns:
            SandboxInstance object

        Raises:
            RuntimeError: If instance creation fails
        """
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        # Normalize language
        language = self._normalize_language(template)

        # The executor_manager manages instances internally via container pool
        # We create a logical instance ID for tracking
        instance_id = str(uuid.uuid4())

        return SandboxInstance(
            instance_id=instance_id,
            provider="self_managed",
            status="running",
            metadata={
                "language": language,
                "endpoint": self.endpoint,
                "pool_size": self.pool_size,
            },
        )

    def execute_code(self, instance_id: str, code: str, language: str, timeout: int = 10, arguments: Optional[Dict[str, Any]] = None) -> ExecutionResult:
        """
        Execute code in the sandbox.

        Args:
            instance_id: ID of the sandbox instance (not used for self-managed)
            code: Source code to execute
            language: Programming language (python, nodejs, javascript)
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

        # Normalize language
        normalized_lang = self._normalize_language(language)

        # Prepare request
        code_b64 = base64.b64encode(code.encode("utf-8")).decode("utf-8")
        payload = {"code_b64": code_b64, "language": normalized_lang, "arguments": arguments or {}}

        url = f"{self.endpoint}/run"
        exec_timeout = timeout or self.timeout

        start_time = time.time()

        try:
            response = requests.post(url, json=payload, timeout=exec_timeout, headers={"Content-Type": "application/json"})

            execution_time = time.time() - start_time

            if response.status_code != 200:
                raise RuntimeError(f"HTTP {response.status_code}: {response.text}")

            result = response.json()
            structured_result = result.get("result") or {}

            return ExecutionResult(
                stdout=result.get("stdout", ""),
                stderr=result.get("stderr", ""),
                exit_code=result.get("exit_code", 0),
                execution_time=execution_time,
                metadata={
                    "status": result.get("status"),
                    "time_used_ms": result.get("time_used_ms"),
                    "memory_used_kb": result.get("memory_used_kb"),
                    "detail": result.get("detail"),
                    "instance_id": instance_id,
                    "artifacts": result.get("artifacts", []),
                    "result_present": structured_result.get("present", False),
                    "result_value": structured_result.get("value"),
                    "result_type": structured_result.get("type"),
                },
            )

        except requests.Timeout:
            execution_time = time.time() - start_time
            raise TimeoutError(f"Execution timed out after {exec_timeout} seconds")

        except requests.RequestException as e:
            raise RuntimeError(f"HTTP request failed: {str(e)}")

    def destroy_instance(self, instance_id: str) -> bool:
        """
        Destroy a sandbox instance.

        Note: For self-managed provider, instances are returned to the
        internal pool automatically by executor_manager after execution.
        This is a no-op for tracking purposes.

        Args:
            instance_id: ID of the instance to destroy

        Returns:
            True (always succeeds for self-managed)
        """
        # The executor_manager manages container lifecycle internally
        # Container is returned to pool after execution
        return True

    def health_check(self) -> bool:
        """
        Check if the provider is healthy and accessible.

        Returns:
            True if provider is healthy, False otherwise
        """
        try:
            url = f"{self.endpoint}/healthz"
            response = requests.get(url, timeout=5)
            return response.status_code == 200
        except Exception:
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
        Return configuration schema for self-managed provider.

        Returns:
            Dictionary mapping field names to their schema definitions
        """
        return {
            "endpoint": {
                "type": "string",
                "required": True,
                "label": "Executor Manager Endpoint",
                "placeholder": "http://sandbox-executor-manager:9385",
                "default": "http://sandbox-executor-manager:9385",
                "description": "HTTP endpoint used by RAGFlow to call sandbox-executor-manager.",
                "scope": "runtime",
                "readonly": False,
            },
            "timeout": {
                "type": "integer",
                "required": False,
                "label": "Request Timeout (seconds)",
                "default": 30,
                "min": 5,
                "max": 300,
                "description": "Maximum request time for a single code execution call. Unit: seconds.",
                "scope": "runtime",
                "readonly": False,
            },
            "executor_manager_image": {
                "type": "string",
                "required": False,
                "label": "Executor Manager Image",
                "default": os.getenv("SANDBOX_EXECUTOR_MANAGER_IMAGE", "infiniflow/sandbox-executor-manager:latest"),
                "description": "Docker image used by sandbox-executor-manager.",
                "scope": "deployment",
                "readonly": True,
            },
            "executor_manager_pool_size": {
                "type": "integer",
                "required": False,
                "label": "Container Pool Size",
                "default": int(os.getenv("SANDBOX_EXECUTOR_MANAGER_POOL_SIZE", "3")),
                "min": 1,
                "max": 100,
                "description": "Container pool size used by sandbox-executor-manager.",
                "scope": "deployment",
                "readonly": True,
            },
            "base_python_image": {
                "type": "string",
                "required": False,
                "label": "Base Python Image",
                "default": os.getenv("SANDBOX_BASE_PYTHON_IMAGE", "infiniflow/sandbox-base-python:latest"),
                "description": "Python runtime image used by executor-managed containers.",
                "scope": "deployment",
                "readonly": True,
            },
            "base_nodejs_image": {
                "type": "string",
                "required": False,
                "label": "Base Node.js Image",
                "default": os.getenv("SANDBOX_BASE_NODEJS_IMAGE", "infiniflow/sandbox-base-nodejs:latest"),
                "description": "Node.js runtime image used by executor-managed containers.",
                "scope": "deployment",
                "readonly": True,
            },
            "executor_manager_port": {
                "type": "integer",
                "required": False,
                "label": "Executor Manager Port",
                "default": int(os.getenv("SANDBOX_EXECUTOR_MANAGER_PORT", "9385")),
                "min": 1,
                "max": 65535,
                "description": "Host port exposed by sandbox-executor-manager.",
                "scope": "deployment",
                "readonly": True,
            },
            "enable_seccomp": {
                "type": "boolean",
                "required": False,
                "label": "Enable Seccomp",
                "default": os.getenv("SANDBOX_ENABLE_SECCOMP", "false").lower() == "true",
                "description": "Whether sandbox-executor-manager starts containers with seccomp enabled.",
                "scope": "deployment",
                "readonly": True,
            },
            "max_memory": {
                "type": "string",
                "required": False,
                "label": "Max Memory",
                "default": os.getenv("SANDBOX_MAX_MEMORY", "256m"),
                "description": "Memory limit applied to each sandbox container. Common format: 256m or 1g.",
                "scope": "deployment",
                "readonly": True,
            },
            "sandbox_timeout": {
                "type": "string",
                "required": False,
                "label": "Sandbox Timeout",
                "default": os.getenv("SANDBOX_TIMEOUT", "10s"),
                "description": "Executor-manager container timeout for each sandbox run. Common format: 10s or 1m.",
                "scope": "deployment",
                "readonly": True,
            },
        }

    def _normalize_language(self, language: str) -> str:
        """
        Normalize language identifier to executor_manager format.

        Args:
            language: Language identifier (python, python3, nodejs, javascript)

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

    def validate_config(self, config: dict) -> tuple[bool, Optional[str]]:
        """
        Validate self-managed provider configuration.

        Performs custom validation beyond the basic schema validation,
        such as checking URL format.

        Args:
            config: Configuration dictionary to validate

        Returns:
            Tuple of (is_valid, error_message)
        """
        # Validate endpoint URL format
        endpoint = config.get("endpoint", "")
        if endpoint:
            # Check if it's a valid HTTP/HTTPS URL or localhost
            import re

            url_pattern = r"^(https?://|http://localhost|http://[\d\.]+:[a-z]+:[/]|http://[\w\.]+:)"
            if not re.match(url_pattern, endpoint):
                return False, f"Invalid endpoint format: {endpoint}. Must start with http:// or https://"

        # Validate pool_size is positive
        pool_size = config.get("executor_manager_pool_size", config.get("pool_size", 3))
        if isinstance(pool_size, int) and pool_size <= 0:
            return False, "Pool size must be greater than 0"

        # Validate timeout is reasonable
        timeout = config.get("timeout", 30)
        if isinstance(timeout, int) and (timeout < 1 or timeout > 600):
            return False, "Timeout must be between 1 and 600 seconds"

        # Validate max_retries
        max_retries = config.get("max_retries", 3)
        if isinstance(max_retries, int) and (max_retries < 0 or max_retries > 10):
            return False, "Max retries must be between 0 and 10"

        return True, None
