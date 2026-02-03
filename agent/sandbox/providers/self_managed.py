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
        self.endpoint: str = "http://localhost:9385"
        self.timeout: int = 30
        self.max_retries: int = 3
        self.pool_size: int = 10
        self._initialized: bool = False

    def initialize(self, config: Dict[str, Any]) -> bool:
        """
        Initialize the provider with configuration.

        Args:
            config: Configuration dictionary with keys:
                - endpoint: HTTP endpoint (default: "http://localhost:9385")
                - timeout: Request timeout in seconds (default: 30)
                - max_retries: Maximum retry attempts (default: 3)
                - pool_size: Container pool size for info (default: 10)

        Returns:
            True if initialization successful, False otherwise
        """
        self.endpoint = config.get("endpoint", "http://localhost:9385")
        self.timeout = config.get("timeout", 30)
        self.max_retries = config.get("max_retries", 3)
        self.pool_size = config.get("pool_size", 10)

        # Validate endpoint is accessible
        if not self.health_check():
            # Try to fall back to SANDBOX_HOST from settings if we are using localhost
            if "localhost" in self.endpoint or "127.0.0.1" in self.endpoint:
                try:
                    from api import settings
                    if settings.SANDBOX_HOST and settings.SANDBOX_HOST not in self.endpoint:
                        original_endpoint = self.endpoint
                        self.endpoint = f"http://{settings.SANDBOX_HOST}:9385"
                        if self.health_check():
                            import logging
                            logging.warning(f"Sandbox self_managed: Connected using settings.SANDBOX_HOST fallback: {self.endpoint} (original: {original_endpoint})")
                            self._initialized = True
                            return True
                        else:
                            self.endpoint = original_endpoint # Restore if fallback also fails
                except ImportError:
                    pass

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
            }
        )

    def execute_code(
        self,
        instance_id: str,
        code: str,
        language: str,
        timeout: int = 10,
        arguments: Optional[Dict[str, Any]] = None
    ) -> ExecutionResult:
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
        payload = {
            "code_b64": code_b64,
            "language": normalized_lang,
            "arguments": arguments or {}
        }

        url = f"{self.endpoint}/run"
        exec_timeout = timeout or self.timeout

        start_time = time.time()

        try:
            response = requests.post(
                url,
                json=payload,
                timeout=exec_timeout,
                headers={"Content-Type": "application/json"}
            )

            execution_time = time.time() - start_time

            if response.status_code != 200:
                raise RuntimeError(
                    f"HTTP {response.status_code}: {response.text}"
                )

            result = response.json()

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
                }
            )

        except requests.Timeout:
            execution_time = time.time() - start_time
            raise TimeoutError(
                f"Execution timed out after {exec_timeout} seconds"
            )

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
                "placeholder": "http://localhost:9385",
                "default": "http://localhost:9385",
                "description": "HTTP endpoint of the executor_manager service"
            },
            "timeout": {
                "type": "integer",
                "required": False,
                "label": "Request Timeout (seconds)",
                "default": 30,
                "min": 5,
                "max": 300,
                "description": "HTTP request timeout for code execution"
            },
            "max_retries": {
                "type": "integer",
                "required": False,
                "label": "Max Retries",
                "default": 3,
                "min": 0,
                "max": 10,
                "description": "Maximum number of retry attempts for failed requests"
            },
            "pool_size": {
                "type": "integer",
                "required": False,
                "label": "Container Pool Size",
                "default": 10,
                "min": 1,
                "max": 100,
                "description": "Size of the container pool (configured in executor_manager)"
            }
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
            url_pattern = r'^(https?://|http://localhost|http://[\d\.]+:[a-z]+:[/]|http://[\w\.]+:)'
            if not re.match(url_pattern, endpoint):
                return False, f"Invalid endpoint format: {endpoint}. Must start with http:// or https://"

        # Validate pool_size is positive
        pool_size = config.get("pool_size", 10)
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
