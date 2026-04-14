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
Base interface for sandbox providers.

Each sandbox provider (self-managed, SaaS) implements this interface
to provide code execution capabilities.
"""

from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Dict, Any, Optional, List


@dataclass
class SandboxInstance:
    """Represents a sandbox execution instance"""
    instance_id: str
    provider: str
    status: str  # running, stopped, error
    metadata: Dict[str, Any]

    def __post_init__(self):
        if self.metadata is None:
            self.metadata = {}


@dataclass
class ExecutionResult:
    """Result of code execution in a sandbox"""
    stdout: str
    stderr: str
    exit_code: int
    execution_time: float  # in seconds
    metadata: Dict[str, Any]

    def __post_init__(self):
        if self.metadata is None:
            self.metadata = {}


class SandboxProvider(ABC):
    """
    Base interface for all sandbox providers.

    Each provider implementation (self-managed, Aliyun OpenSandbox, E2B, etc.)
    must implement these methods to provide code execution capabilities.
    """

    @abstractmethod
    def initialize(self, config: Dict[str, Any]) -> bool:
        """
        Initialize the provider with configuration.

        Args:
            config: Provider-specific configuration dictionary

        Returns:
            True if initialization successful, False otherwise
        """
        pass

    @abstractmethod
    def create_instance(self, template: str = "python") -> SandboxInstance:
        """
        Create a new sandbox instance.

        Args:
            template: Programming language/template for the instance
                     (e.g., "python", "nodejs", "bash")

        Returns:
            SandboxInstance object representing the created instance

        Raises:
            RuntimeError: If instance creation fails
        """
        pass

    @abstractmethod
    def execute_code(
        self,
        instance_id: str,
        code: str,
        language: str,
        timeout: int = 10,
        arguments: Optional[Dict[str, Any]] = None
    ) -> ExecutionResult:
        """
        Execute code in a sandbox instance.

        Args:
            instance_id: ID of the sandbox instance
            code: Source code to execute
            language: Programming language (python, javascript, etc.)
            timeout: Maximum execution time in seconds
            arguments: Optional arguments dict to pass to main() function

        Returns:
            ExecutionResult containing stdout, stderr, exit_code, and metadata

        Raises:
            RuntimeError: If execution fails
            TimeoutError: If execution exceeds timeout
        """
        pass

    @abstractmethod
    def destroy_instance(self, instance_id: str) -> bool:
        """
        Destroy a sandbox instance.

        Args:
            instance_id: ID of the instance to destroy

        Returns:
            True if destruction successful, False otherwise

        Raises:
            RuntimeError: If destruction fails
        """
        pass

    @abstractmethod
    def health_check(self) -> bool:
        """
        Check if the provider is healthy and accessible.

        Returns:
            True if provider is healthy, False otherwise
        """
        pass

    @abstractmethod
    def get_supported_languages(self) -> List[str]:
        """
        Get list of supported programming languages.

        Returns:
            List of language identifiers (e.g., ["python", "javascript", "go"])
        """
        pass

    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        """
        Return configuration schema for this provider.

        The schema defines what configuration fields are required/optional,
        their types, validation rules, and UI labels.

        Returns:
            Dictionary mapping field names to their schema definitions.

        Example:
            {
                "endpoint": {
                    "type": "string",
                    "required": True,
                    "label": "API Endpoint",
                    "placeholder": "http://localhost:9385"
                },
                "timeout": {
                    "type": "integer",
                    "default": 30,
                    "label": "Timeout (seconds)",
                    "min": 5,
                    "max": 300
                }
            }
        """
        return {}

    def validate_config(self, config: Dict[str, Any]) -> tuple[bool, Optional[str]]:
        """
        Validate provider-specific configuration.

        This method allows providers to implement custom validation logic beyond
        the basic schema validation. Override this method to add provider-specific
        checks like URL format validation, API key format validation, etc.

        Args:
            config: Configuration dictionary to validate

        Returns:
            Tuple of (is_valid, error_message):
                - is_valid: True if configuration is valid, False otherwise
                - error_message: Error message if invalid, None if valid

        Example:
            >>> def validate_config(self, config):
            >>>     endpoint = config.get("endpoint", "")
            >>>     if not endpoint.startswith(("http://", "https://")):
            >>>         return False, "Endpoint must start with http:// or https://"
            >>>     return True, None
        """
        # Default implementation: no custom validation
        return True, None