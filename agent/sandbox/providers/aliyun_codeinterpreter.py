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
Aliyun Code Interpreter provider implementation.

This provider integrates with Aliyun Function Compute Code Interpreter service
for secure code execution in serverless microVMs using the official agentrun-sdk.

Official Documentation: https://help.aliyun.com/zh/functioncompute/fc/sandbox-sandbox-code-interepreter
Official SDK: https://github.com/Serverless-Devs/agentrun-sdk-python

https://api.aliyun.com/api/AgentRun/2025-09-10/CreateTemplate?lang=PYTHON
https://api.aliyun.com/api/AgentRun/2025-09-10/CreateSandbox?lang=PYTHON
"""

import logging
import os
import time
from typing import Dict, Any, List, Optional
from datetime import datetime, timezone

from agentrun.sandbox import TemplateType, CodeLanguage, Template, TemplateInput, Sandbox
from agentrun.utils.config import Config
from agentrun.utils.exception import ServerError

from .base import SandboxProvider, SandboxInstance, ExecutionResult

logger = logging.getLogger(__name__)


class AliyunCodeInterpreterProvider(SandboxProvider):
    """
    Aliyun Code Interpreter provider implementation.

    This provider uses the official agentrun-sdk to interact with
    Aliyun Function Compute's Code Interpreter service.
    """

    def __init__(self):
        self.access_key_id: Optional[str] = None
        self.access_key_secret: Optional[str] = None
        self.account_id: Optional[str] = None
        self.region: str = "cn-hangzhou"
        self.template_name: str = ""
        self.timeout: int = 30
        self._initialized: bool = False
        self._config: Optional[Config] = None

    def initialize(self, config: Dict[str, Any]) -> bool:
        """
        Initialize the provider with Aliyun credentials.

        Args:
            config: Configuration dictionary with keys:
                - access_key_id: Aliyun AccessKey ID
                - access_key_secret: Aliyun AccessKey Secret
                - account_id: Aliyun primary account ID (主账号ID)
                - region: Region (default: "cn-hangzhou")
                - template_name: Optional sandbox template name
                - timeout: Request timeout in seconds (default: 30, max 30)

        Returns:
            True if initialization successful, False otherwise
        """
        # Get values from config or environment variables
        access_key_id = config.get("access_key_id") or os.getenv("AGENTRUN_ACCESS_KEY_ID")
        access_key_secret = config.get("access_key_secret") or os.getenv("AGENTRUN_ACCESS_KEY_SECRET")
        account_id = config.get("account_id") or os.getenv("AGENTRUN_ACCOUNT_ID")
        region = config.get("region") or os.getenv("AGENTRUN_REGION", "cn-hangzhou")

        self.access_key_id = access_key_id
        self.access_key_secret = access_key_secret
        self.account_id = account_id
        self.region = region
        self.template_name = config.get("template_name", "")
        self.timeout = min(config.get("timeout", 30), 30)  # Max 30 seconds

        logger.info(f"Aliyun Code Interpreter: Initializing with account_id={self.account_id}, region={self.region}")

        # Validate required fields
        if not self.access_key_id or not self.access_key_secret:
            logger.error("Aliyun Code Interpreter: Missing access_key_id or access_key_secret")
            return False

        if not self.account_id:
            logger.error("Aliyun Code Interpreter: Missing account_id (主账号ID)")
            return False

        # Create SDK configuration
        try:
            logger.info(f"Aliyun Code Interpreter: Creating Config object with account_id={self.account_id}")
            self._config = Config(
                access_key_id=self.access_key_id,
                access_key_secret=self.access_key_secret,
                account_id=self.account_id,
                region_id=self.region,
                timeout=self.timeout,
            )
            logger.info("Aliyun Code Interpreter: Config object created successfully")

            # Verify connection with health check
            if not self.health_check():
                logger.error(f"Aliyun Code Interpreter: Health check failed for region {self.region}")
                return False

            self._initialized = True
            logger.info(f"Aliyun Code Interpreter: Initialized successfully for region {self.region}")
            return True

        except Exception as e:
            logger.error(f"Aliyun Code Interpreter: Initialization failed - {str(e)}")
            return False

    def create_instance(self, template: str = "python") -> SandboxInstance:
        """
        Create a new sandbox instance in Aliyun Code Interpreter.

        Args:
            template: Programming language (python, javascript)

        Returns:
            SandboxInstance object

        Raises:
            RuntimeError: If instance creation fails
        """
        if not self._initialized or not self._config:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        # Normalize language
        language = self._normalize_language(template)

        try:
            # Get or create template
            from agentrun.sandbox import Sandbox

            if self.template_name:
                # Use existing template
                template_name = self.template_name
            else:
                # Try to get default template, or create one if it doesn't exist
                default_template_name = f"ragflow-{language}-default"
                try:
                    # Check if template exists
                    Template.get_by_name(default_template_name, config=self._config)
                    template_name = default_template_name
                except Exception:
                    # Create default template if it doesn't exist
                    template_input = TemplateInput(
                        template_name=default_template_name,
                        template_type=TemplateType.CODE_INTERPRETER,
                    )
                    Template.create(template_input, config=self._config)
                    template_name = default_template_name

            # Create sandbox directly
            sandbox = Sandbox.create(
                template_type=TemplateType.CODE_INTERPRETER,
                template_name=template_name,
                sandbox_idle_timeout_seconds=self.timeout,
                config=self._config,
            )

            instance_id = sandbox.sandbox_id

            return SandboxInstance(
                instance_id=instance_id,
                provider="aliyun_codeinterpreter",
                status="READY",
                metadata={
                    "language": language,
                    "region": self.region,
                    "account_id": self.account_id,
                    "template_name": template_name,
                    "created_at": datetime.now(timezone.utc).isoformat(),
                },
            )

        except ServerError as e:
            raise RuntimeError(f"Failed to create sandbox instance: {str(e)}")
        except Exception as e:
            raise RuntimeError(f"Unexpected error creating instance: {str(e)}")

    def execute_code(self, instance_id: str, code: str, language: str, timeout: int = 10, arguments: Optional[Dict[str, Any]] = None) -> ExecutionResult:
        """
        Execute code in the Aliyun Code Interpreter instance.

        Args:
            instance_id: ID of the sandbox instance
            code: Source code to execute
            language: Programming language (python, javascript)
            timeout: Maximum execution time in seconds (max 30)
            arguments: Optional arguments dict to pass to main() function

        Returns:
            ExecutionResult containing stdout, stderr, exit_code, and metadata

        Raises:
            RuntimeError: If execution fails
            TimeoutError: If execution exceeds timeout
        """
        if not self._initialized or not self._config:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        # Normalize language
        normalized_lang = self._normalize_language(language)

        # Enforce 30-second hard limit
        timeout = min(timeout or self.timeout, 30)

        try:
            # Connect to existing sandbox instance
            sandbox = Sandbox.connect(sandbox_id=instance_id, config=self._config)

            # Convert language string to CodeLanguage enum
            code_language = CodeLanguage.PYTHON if normalized_lang == "python" else CodeLanguage.JAVASCRIPT

            # Wrap code to call main() function
            # Matches self_managed provider behavior: call main(**arguments)
            if normalized_lang == "python":
                # Build arguments string for main() call
                if arguments:
                    import json as json_module
                    args_json = json_module.dumps(arguments)
                    wrapped_code = f'''{code}

if __name__ == "__main__":
    import json
    result = main(**{args_json})
    print(json.dumps(result) if isinstance(result, dict) else result)
'''
                else:
                    wrapped_code = f'''{code}

if __name__ == "__main__":
    import json
    result = main()
    print(json.dumps(result) if isinstance(result, dict) else result)
'''
            else:  # javascript
                if arguments:
                    import json as json_module
                    args_json = json_module.dumps(arguments)
                    wrapped_code = f'''{code}

// Call main and output result
const result = main({args_json});
console.log(typeof result === 'object' ? JSON.stringify(result) : String(result));
'''
                else:
                    wrapped_code = f'''{code}

// Call main and output result
const result = main();
console.log(typeof result === 'object' ? JSON.stringify(result) : String(result));
'''
            logger.debug(f"Aliyun Code Interpreter: Wrapped code (first 200 chars): {wrapped_code[:200]}")

            start_time = time.time()

            # Execute code using SDK's simplified execute endpoint
            logger.info(f"Aliyun Code Interpreter: Executing code (language={normalized_lang}, timeout={timeout})")
            logger.debug(f"Aliyun Code Interpreter: Original code (first 200 chars): {code[:200]}")
            result = sandbox.context.execute(
                code=wrapped_code,
                language=code_language,
                timeout=timeout,
            )

            execution_time = time.time() - start_time
            logger.info(f"Aliyun Code Interpreter: Execution completed in {execution_time:.2f}s")
            logger.debug(f"Aliyun Code Interpreter: Raw SDK result: {result}")

            # Parse execution result
            results = result.get("results", []) if isinstance(result, dict) else []
            logger.info(f"Aliyun Code Interpreter: Parsed {len(results)} result items")

            # Extract stdout and stderr from results
            stdout_parts = []
            stderr_parts = []
            exit_code = 0
            execution_status = "ok"

            for item in results:
                result_type = item.get("type", "")
                text = item.get("text", "")

                if result_type == "stdout":
                    stdout_parts.append(text)
                elif result_type == "stderr":
                    stderr_parts.append(text)
                    exit_code = 1  # Error occurred
                elif result_type == "endOfExecution":
                    execution_status = item.get("status", "ok")
                    if execution_status != "ok":
                        exit_code = 1
                elif result_type == "error":
                    stderr_parts.append(text)
                    exit_code = 1

            stdout = "\n".join(stdout_parts)
            stderr = "\n".join(stderr_parts)

            logger.info(f"Aliyun Code Interpreter: stdout length={len(stdout)}, stderr length={len(stderr)}, exit_code={exit_code}")
            if stdout:
                logger.debug(f"Aliyun Code Interpreter: stdout (first 200 chars): {stdout[:200]}")
            if stderr:
                logger.debug(f"Aliyun Code Interpreter: stderr (first 200 chars): {stderr[:200]}")

            return ExecutionResult(
                stdout=stdout,
                stderr=stderr,
                exit_code=exit_code,
                execution_time=execution_time,
                metadata={
                    "instance_id": instance_id,
                    "language": normalized_lang,
                    "context_id": result.get("contextId") if isinstance(result, dict) else None,
                    "timeout": timeout,
                },
            )

        except ServerError as e:
            if "timeout" in str(e).lower():
                raise TimeoutError(f"Execution timed out after {timeout} seconds")
            raise RuntimeError(f"Failed to execute code: {str(e)}")
        except Exception as e:
            raise RuntimeError(f"Unexpected error during execution: {str(e)}")

    def destroy_instance(self, instance_id: str) -> bool:
        """
        Destroy an Aliyun Code Interpreter instance.

        Args:
            instance_id: ID of the instance to destroy

        Returns:
            True if destruction successful, False otherwise
        """
        if not self._initialized or not self._config:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        try:
            # Delete sandbox by ID directly
            Sandbox.delete_by_id(sandbox_id=instance_id)

            logger.info(f"Successfully destroyed sandbox instance {instance_id}")
            return True

        except ServerError as e:
            logger.error(f"Failed to destroy instance {instance_id}: {str(e)}")
            return False
        except Exception as e:
            logger.error(f"Unexpected error destroying instance {instance_id}: {str(e)}")
            return False

    def health_check(self) -> bool:
        """
        Check if the Aliyun Code Interpreter service is accessible.

        Returns:
            True if provider is healthy, False otherwise
        """
        if not self._initialized and not (self.access_key_id and self.account_id):
            return False

        try:
            # Try to list templates to verify connection
            from agentrun.sandbox import Template

            templates = Template.list(config=self._config)
            return templates is not None

        except Exception as e:
            logger.warning(f"Aliyun Code Interpreter health check failed: {str(e)}")
            # If we get any response (even an error), the service is reachable
            return "connection" not in str(e).lower()

    def get_supported_languages(self) -> List[str]:
        """
        Get list of supported programming languages.

        Returns:
            List of language identifiers
        """
        return ["python", "javascript"]

    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        """
        Return configuration schema for Aliyun Code Interpreter provider.

        Returns:
            Dictionary mapping field names to their schema definitions
        """
        return {
            "access_key_id": {
                "type": "string",
                "required": True,
                "label": "Access Key ID",
                "placeholder": "LTAI5t...",
                "description": "Aliyun AccessKey ID for authentication",
                "secret": False,
            },
            "access_key_secret": {
                "type": "string",
                "required": True,
                "label": "Access Key Secret",
                "placeholder": "••••••••••••••••",
                "description": "Aliyun AccessKey Secret for authentication",
                "secret": True,
            },
            "account_id": {
                "type": "string",
                "required": True,
                "label": "Account ID",
                "placeholder": "1234567890...",
                "description": "Aliyun primary account ID (主账号ID), required for API calls",
            },
            "region": {
                "type": "string",
                "required": False,
                "label": "Region",
                "default": "cn-hangzhou",
                "description": "Aliyun region for Code Interpreter service",
                "options": ["cn-hangzhou", "cn-beijing", "cn-shanghai", "cn-shenzhen", "cn-guangzhou"],
            },
            "template_name": {
                "type": "string",
                "required": False,
                "label": "Template Name",
                "placeholder": "my-interpreter",
                "description": "Optional sandbox template name for pre-configured environments",
            },
            "timeout": {
                "type": "integer",
                "required": False,
                "label": "Execution Timeout (seconds)",
                "default": 30,
                "min": 1,
                "max": 30,
                "description": "Code execution timeout (max 30 seconds - hard limit)",
            },
        }

    def validate_config(self, config: Dict[str, Any]) -> tuple[bool, Optional[str]]:
        """
        Validate Aliyun-specific configuration.

        Args:
            config: Configuration dictionary to validate

        Returns:
            Tuple of (is_valid, error_message)
        """
        # Validate access key format
        access_key_id = config.get("access_key_id", "")
        if access_key_id and not access_key_id.startswith("LTAI"):
            return False, "Invalid AccessKey ID format (should start with 'LTAI')"

        # Validate account ID
        account_id = config.get("account_id", "")
        if not account_id:
            return False, "Account ID is required"

        # Validate region
        valid_regions = ["cn-hangzhou", "cn-beijing", "cn-shanghai", "cn-shenzhen", "cn-guangzhou"]
        region = config.get("region", "cn-hangzhou")
        if region and region not in valid_regions:
            return False, f"Invalid region. Must be one of: {', '.join(valid_regions)}"

        # Validate timeout range (max 30 seconds)
        timeout = config.get("timeout", 30)
        if isinstance(timeout, int) and (timeout < 1 or timeout > 30):
            return False, "Timeout must be between 1 and 30 seconds"

        return True, None

    def _normalize_language(self, language: str) -> str:
        """
        Normalize language identifier to Aliyun format.

        Args:
            language: Language identifier (python, python3, javascript, nodejs)

        Returns:
            Normalized language identifier
        """
        if not language:
            return "python"

        lang_lower = language.lower()
        if lang_lower in ("python", "python3"):
            return "python"
        elif lang_lower in ("javascript", "nodejs"):
            return "javascript"
        else:
            return language
