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
E2B provider implementation.

This provider integrates with E2B Cloud for cloud-based code execution
using Firecracker microVMs.
"""

import uuid
from typing import Dict, Any, List

from .base import SandboxProvider, SandboxInstance, ExecutionResult


class E2BProvider(SandboxProvider):
    """
    E2B provider implementation.

    This provider uses E2B Cloud service for secure code execution
    in Firecracker microVMs.
    """

    def __init__(self):
        self.api_key: str = ""
        self.region: str = "us"
        self.timeout: int = 30
        self._initialized: bool = False

    def initialize(self, config: Dict[str, Any]) -> bool:
        """
        Initialize the provider with E2B credentials.

        Args:
            config: Configuration dictionary with keys:
                - api_key: E2B API key
                - region: Region (us, eu) (default: "us")
                - timeout: Request timeout in seconds (default: 30)

        Returns:
            True if initialization successful, False otherwise
        """
        self.api_key = config.get("api_key", "")
        self.region = config.get("region", "us")
        self.timeout = config.get("timeout", 30)

        # Validate required fields
        if not self.api_key:
            return False

        # TODO: Implement actual E2B API client initialization
        # For now, we'll mark as initialized but actual API calls will fail
        self._initialized = True
        return True

    def create_instance(self, template: str = "python") -> SandboxInstance:
        """
        Create a new sandbox instance in E2B.

        Args:
            template: Programming language template (python, nodejs, go, bash)

        Returns:
            SandboxInstance object

        Raises:
            RuntimeError: If instance creation fails
        """
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        # Normalize language
        language = self._normalize_language(template)

        # TODO: Implement actual E2B API call
        # POST /sandbox with template
        instance_id = str(uuid.uuid4())

        return SandboxInstance(
            instance_id=instance_id,
            provider="e2b",
            status="running",
            metadata={
                "language": language,
                "region": self.region,
            }
        )

    def execute_code(
        self,
        instance_id: str,
        code: str,
        language: str,
        timeout: int = 10
    ) -> ExecutionResult:
        """
        Execute code in the E2B instance.

        Args:
            instance_id: ID of the sandbox instance
            code: Source code to execute
            language: Programming language (python, nodejs, go, bash)
            timeout: Maximum execution time in seconds

        Returns:
            ExecutionResult containing stdout, stderr, exit_code, and metadata

        Raises:
            RuntimeError: If execution fails
            TimeoutError: If execution exceeds timeout
        """
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        # TODO: Implement actual E2B API call
        # POST /sandbox/{sandboxID}/execute

        raise RuntimeError(
            "E2B provider is not yet fully implemented. "
            "Please use the self-managed provider or implement the E2B API integration. "
            "See https://github.com/e2b-dev/e2b for API documentation."
        )

    def destroy_instance(self, instance_id: str) -> bool:
        """
        Destroy an E2B instance.

        Args:
            instance_id: ID of the instance to destroy

        Returns:
            True if destruction successful, False otherwise
        """
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        # TODO: Implement actual E2B API call
        # DELETE /sandbox/{sandboxID}
        return True

    def health_check(self) -> bool:
        """
        Check if the E2B service is accessible.

        Returns:
            True if provider is healthy, False otherwise
        """
        if not self._initialized:
            return False

        # TODO: Implement actual E2B health check API call
        # GET /healthz or similar
        # For now, return True if initialized with API key
        return bool(self.api_key)

    def get_supported_languages(self) -> List[str]:
        """
        Get list of supported programming languages.

        Returns:
            List of language identifiers
        """
        return ["python", "nodejs", "javascript", "go", "bash"]

    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        """
        Return configuration schema for E2B provider.

        Returns:
            Dictionary mapping field names to their schema definitions
        """
        return {
            "api_key": {
                "type": "string",
                "required": True,
                "label": "API Key",
                "placeholder": "e2b_sk_...",
                "description": "E2B API key for authentication",
                "secret": True,
            },
            "region": {
                "type": "string",
                "required": False,
                "label": "Region",
                "default": "us",
                "description": "E2B service region (us or eu)",
            },
            "timeout": {
                "type": "integer",
                "required": False,
                "label": "Request Timeout (seconds)",
                "default": 30,
                "min": 5,
                "max": 300,
                "description": "API request timeout for code execution",
            }
        }

    def _normalize_language(self, language: str) -> str:
        """
        Normalize language identifier to E2B template format.

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
