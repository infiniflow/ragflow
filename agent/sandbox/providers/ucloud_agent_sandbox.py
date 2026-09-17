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

"""UCloud Agent Sandbox provider for remote code execution."""

from __future__ import annotations

import base64
import json
import logging
import mimetypes
import os
import posixpath
import shlex
import time
import uuid
from typing import Any

from agent.sandbox.result_protocol import build_javascript_wrapper, build_python_wrapper, extract_structured_result

from .base import ExecutionResult, SandboxInstance, SandboxProvider, SandboxProviderConfigError

logger = logging.getLogger(__name__)

ALLOWED_ARTIFACT_EXTENSIONS = {".csv", ".html", ".jpeg", ".jpg", ".json", ".pdf", ".png", ".svg"}
DEFAULT_REGION = "cn-wlcb"
DOMAIN_SUFFIX = "sandbox.ucloudai.com"
SANDBOX_HOME = "/home/user"
MAX_ARTIFACT_DEPTH = 16


class UCloudAgentSandboxProvider(SandboxProvider):
    """Execute Python and JavaScript in disposable UCloud Agent Sandboxes."""

    def __init__(self):
        """Initialize the provider with safe defaults and no active instances."""
        self.api_key = ""
        self.region = DEFAULT_REGION
        self.domain = ""
        self.api_url = ""
        self.template = "base"
        self.allow_internet_access = False
        self.insecure_http = False
        self.timeout = 30
        self.sandbox_timeout = 300
        self.max_output_bytes = 1024 * 1024
        self.max_artifacts = 20
        self.max_artifact_bytes = 10 * 1024 * 1024
        self._initialized = False
        self._instances: dict[str, dict[str, Any]] = {}

    def initialize(self, config: dict[str, Any]) -> bool:
        """Validate and apply provider configuration.

        Args:
            config: Provider credentials, endpoint options, and execution limits.

        Returns:
            True when the provider is ready to create sandboxes.

        Raises:
            SandboxProviderConfigError: If the configuration or SDK is invalid.
        """
        self.api_key = str(config.get("api_key", "") or "").strip()
        self.region = str(config.get("region", DEFAULT_REGION) or DEFAULT_REGION).strip()
        self.domain = str(config.get("domain", "") or "").strip()
        self.api_url = str(config.get("api_url", "") or "").strip()
        self.template = str(config.get("template", "base") or "base").strip()
        self.allow_internet_access = bool(config.get("allow_internet_access", False))
        self.insecure_http = bool(config.get("insecure_http", False))
        self.timeout = int(config.get("timeout", 30) or 30)
        self.sandbox_timeout = int(config.get("sandbox_timeout", 300) or 300)
        self.max_output_bytes = int(config.get("max_output_bytes", 1024 * 1024) or 1024 * 1024)
        self.max_artifacts = int(config.get("max_artifacts", 20) or 20)
        self.max_artifact_bytes = int(config.get("max_artifact_bytes", 10 * 1024 * 1024) or 10 * 1024 * 1024)

        is_valid, error_message = self.validate_config(config | {"api_key": self.api_key})
        if not is_valid:
            raise SandboxProviderConfigError(error_message or "Invalid UCloud Agent Sandbox configuration.")

        _get_ucloud_sandbox_module()
        self._initialized = True
        logger.info("UCloud Agent Sandbox provider initialized")
        return True

    def create_instance(self, template: str = "python") -> SandboxInstance:
        """Create a disposable sandbox and its isolated execution workspace.

        Args:
            template: Requested language identifier used to validate the runtime.

        Returns:
            A RAGFlow sandbox instance handle.
        """
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        language = self._normalize_language(template)
        if language not in {"python", "nodejs"}:
            raise RuntimeError(f"Unsupported language for UCloud Agent Sandbox provider: {template}")

        sdk = _get_ucloud_sandbox_module()
        try:
            sandbox = sdk.Sandbox.create(
                template=self.template,
                timeout=self.sandbox_timeout,
                metadata={"source": "ragflow"},
                secure=True,
                allow_internet_access=self.allow_internet_access,
                **self._api_options(),
            )
        except sdk.AuthenticationException as exc:
            raise SandboxProviderConfigError("UCloud Agent Sandbox authentication failed: check the API key.") from exc
        except sdk.RateLimitException as exc:
            raise RuntimeError(f"UCloud Agent Sandbox rate limited, please retry: {exc}") from exc
        except sdk.TimeoutException as exc:
            raise TimeoutError("Timed out while creating a UCloud Agent Sandbox.") from exc
        except Exception as exc:
            raise RuntimeError(f"Failed to create UCloud Agent Sandbox: {exc}") from exc

        remote_work_dir = posixpath.join(SANDBOX_HOME, f"ragflow-codeexec-{uuid.uuid4().hex}")
        try:
            sandbox.commands.run(
                f"mkdir -p {shlex.quote(posixpath.join(remote_work_dir, 'artifacts'))}",
                timeout=min(self.timeout, 10),
                request_timeout=self.timeout,
            )
        except Exception:
            self._safe_kill(sandbox)
            raise

        instance_id = str(uuid.uuid4())
        self._instances[instance_id] = {"sandbox": sandbox, "remote_work_dir": remote_work_dir, "language": language}
        return SandboxInstance(
            instance_id=instance_id,
            provider="ucloud_agent_sandbox",
            status="running",
            metadata={
                "language": language,
                "remote_work_dir": remote_work_dir,
                "sandbox_id": sandbox.sandbox_id,
                "template": self.template,
            },
        )

    def execute_code(
        self,
        instance_id: str,
        code: str,
        language: str,
        timeout: int = 10,
        arguments: dict[str, Any] | None = None,
    ) -> ExecutionResult:
        """Execute wrapped code in an existing UCloud sandbox.

        Args:
            instance_id: RAGFlow instance identifier returned by create_instance.
            code: User-provided source code defining a main function.
            language: Python or JavaScript language identifier.
            timeout: Maximum execution duration in seconds.
            arguments: Values passed to the user-defined main function.

        Returns:
            Captured output, structured result metadata, and allowed artifacts.
        """
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")
        if instance_id not in self._instances:
            raise RuntimeError(f"Unknown UCloud Agent Sandbox instance: {instance_id}")

        normalized_lang = self._normalize_language(language)
        instance = self._instances[instance_id]
        sandbox = instance["sandbox"]
        remote_work_dir: str = instance["remote_work_dir"]
        script_path, executable = self._prepare_script(sandbox, remote_work_dir, normalized_lang, code, arguments or {})

        requested_timeout = self.timeout if timeout is None else int(timeout)
        if requested_timeout <= 0:
            raise RuntimeError(f"Execution timeout must be greater than 0 seconds, got {requested_timeout}.")
        exec_timeout = min(requested_timeout, self.timeout)
        sdk = _get_ucloud_sandbox_module()

        start_time = time.time()
        try:
            sandbox.set_timeout(max(self.sandbox_timeout, exec_timeout + 30), request_timeout=self.timeout)
            result = sandbox.commands.run(
                f"{executable} {shlex.quote(script_path)}",
                cwd=remote_work_dir,
                timeout=exec_timeout,
                request_timeout=max(self.timeout, exec_timeout),
            )
        except sdk.CommandExitException as exc:
            result = exc
        except sdk.TimeoutException as exc:
            raise TimeoutError(f"Execution timed out after {exec_timeout} seconds") from exc
        except Exception as exc:
            raise RuntimeError(f"UCloud Agent Sandbox execution failed: {exc}") from exc
        execution_time = time.time() - start_time

        stdout = result.stdout or ""
        stderr = result.stderr or ""
        exit_code = int(result.exit_code)
        self._validate_output_size(stdout, stderr)
        stdout, structured_result = extract_structured_result(stdout)

        return ExecutionResult(
            stdout=stdout,
            stderr=stderr,
            exit_code=exit_code,
            execution_time=execution_time,
            metadata={
                "instance_id": instance_id,
                "sandbox_id": sandbox.sandbox_id,
                "language": normalized_lang,
                "script_path": script_path,
                "remote_work_dir": remote_work_dir,
                "status": "ok" if exit_code == 0 else "error",
                "timeout": exec_timeout,
                "artifacts": self._collect_artifacts(sandbox, posixpath.join(remote_work_dir, "artifacts")),
                "result_present": structured_result.get("present", False),
                "result_value": structured_result.get("value"),
                "result_type": structured_result.get("type"),
            },
        )

    def destroy_instance(self, instance_id: str) -> bool:
        """Destroy a sandbox instance if it is still tracked by the provider."""
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")
        instance = self._instances.pop(instance_id, None)
        if instance is None:
            return True
        self._safe_kill(instance["sandbox"])
        return True

    def health_check(self) -> bool:
        """Return whether the provider is initialized with an API key."""
        return self._initialized and bool(self.api_key)

    def get_supported_languages(self) -> list[str]:
        """Return the language identifiers accepted by this provider."""
        return ["python", "javascript"]

    @staticmethod
    def get_config_schema() -> dict[str, dict]:
        """Return the Admin UI configuration schema for this provider."""
        return {
            "api_key": {
                "type": "string",
                "required": True,
                "label": "API Key",
                "secret": True,
                "description": "UCloud Agent Sandbox API key.",
            },
            "region": {
                "type": "string",
                "required": False,
                "label": "Region",
                "default": DEFAULT_REGION,
                "description": "UCloud Agent Sandbox region, for example cn-wlcb or us-ca.",
            },
            "domain": {
                "type": "string",
                "required": False,
                "label": "Domain",
                "description": "Override the sandbox domain. Leave empty to derive it from Region.",
            },
            "api_url": {
                "type": "string",
                "required": False,
                "label": "API URL",
                "description": "Override the UCloud Agent Sandbox control-plane API URL.",
            },
            "template": {
                "type": "string",
                "required": False,
                "label": "Template",
                "default": "base",
                "description": "Sandbox template. The base template includes Python and Node.js.",
            },
            "allow_internet_access": {
                "type": "boolean",
                "required": False,
                "label": "Allow Internet Access",
                "default": False,
                "description": "Allow sandboxed code to access the internet. Disabled by default.",
            },
            "insecure_http": {
                "type": "boolean",
                "required": False,
                "label": "Use Insecure HTTP",
                "default": False,
                "description": "Use HTTP instead of HTTPS. Enable only for trusted private deployments.",
            },
            "timeout": {
                "type": "integer",
                "required": False,
                "label": "Execution Timeout (seconds)",
                "default": 30,
                "min": 1,
                "max": 600,
            },
            "sandbox_timeout": {
                "type": "integer",
                "required": False,
                "label": "Sandbox Lifetime (seconds)",
                "default": 300,
                "min": 60,
                "max": 86400,
            },
            "max_output_bytes": {
                "type": "integer",
                "required": False,
                "label": "Max Output Bytes",
                "default": 1048576,
                "min": 1024,
                "max": 10485760,
            },
            "max_artifacts": {
                "type": "integer",
                "required": False,
                "label": "Max Artifacts",
                "default": 20,
                "min": 0,
                "max": 100,
            },
            "max_artifact_bytes": {
                "type": "integer",
                "required": False,
                "label": "Max Artifact Bytes",
                "default": 10485760,
                "min": 1024,
                "max": 104857600,
            },
        }

    def validate_config(self, config: dict[str, Any]) -> tuple[bool, str | None]:
        """Validate required credentials and numeric execution limits."""
        if not str(config.get("api_key", "") or "").strip():
            return False, "UCloud Agent Sandbox API key is required"
        if not str(config.get("template", "base") or "").strip():
            return False, "template is required"
        for key in ("timeout", "sandbox_timeout", "max_output_bytes", "max_artifact_bytes"):
            try:
                value = int(config.get(key, self.get_config_schema()[key]["default"]) or 0)
            except (TypeError, ValueError):
                return False, f"{key} must be an integer"
            if value <= 0:
                return False, f"{key} must be greater than 0"
        try:
            max_artifacts = int(config.get("max_artifacts", 20) or 0)
        except (TypeError, ValueError):
            return False, "max_artifacts must be an integer"
        if max_artifacts < 0:
            return False, "max_artifacts must be greater than or equal to 0"
        return True, None

    def _api_options(self) -> dict[str, Any]:
        """Build keyword arguments shared by UCloud SDK API calls."""
        options: dict[str, Any] = {
            "api_key": self.api_key,
            "domain": self.domain or f"{self.region}.{DOMAIN_SUFFIX}",
            "insecure_http": self.insecure_http,
            "request_timeout": float(self.timeout),
            "integration": "ragflow",
        }
        if self.api_url:
            options["api_url"] = self.api_url
        return options

    def _prepare_script(self, sandbox, remote_work_dir: str, language: str, code: str, arguments: dict[str, Any]) -> tuple[str, str]:
        """Wrap user code, upload it, and return its path and executable."""
        args_json = json.dumps(arguments, ensure_ascii=False)
        if language == "python":
            script_name = "main.py"
            script_content = build_python_wrapper(code, args_json)
            executable = "python3"
        elif language == "nodejs":
            script_name = "main.js"
            script_content = build_javascript_wrapper(code, args_json)
            executable = "node"
        else:
            raise RuntimeError(f"Unsupported language for UCloud Agent Sandbox provider: {language}")
        script_path = posixpath.join(remote_work_dir, script_name)
        sandbox.files.write(script_path, script_content, request_timeout=self.timeout)
        return script_path, executable

    def _validate_output_size(self, stdout: str, stderr: str) -> None:
        """Reject combined standard output that exceeds the configured limit."""
        output_size = len(stdout.encode("utf-8")) + len(stderr.encode("utf-8"))
        if output_size > self.max_output_bytes:
            raise RuntimeError(f"UCloud Agent Sandbox execution output exceeded {self.max_output_bytes} bytes.")

    def _collect_artifacts(self, sandbox, artifacts_dir: str) -> list[dict[str, Any]]:
        """Collect allowed files from the execution artifact directory."""
        artifacts: list[dict[str, Any]] = []
        self._collect_artifacts_recursive(sandbox, artifacts_dir, "", artifacts, depth=0)
        return artifacts

    def _collect_artifacts_recursive(self, sandbox, current_dir: str, relative_dir: str, artifacts: list[dict[str, Any]], depth: int) -> None:
        """Traverse artifact directories while enforcing type, size, and depth limits."""
        if depth > MAX_ARTIFACT_DEPTH:
            raise RuntimeError(f"Artifact directory nesting exceeds {MAX_ARTIFACT_DEPTH} levels: {relative_dir}")
        sdk = _get_ucloud_sandbox_module()
        try:
            entries = sandbox.files.list(current_dir, depth=1, request_timeout=self.timeout)
        except sdk.FileNotFoundException:
            return
        for entry in sorted(entries, key=lambda item: item.path):
            name = posixpath.basename(entry.path)
            relative_path = posixpath.join(relative_dir, name) if relative_dir else name
            if entry.symlink_target is not None:
                raise RuntimeError(f"Artifact symlinks are not allowed: {relative_path}")
            if entry.type == sdk.FileType.DIR:
                self._collect_artifacts_recursive(sandbox, entry.path, relative_path, artifacts, depth + 1)
                continue
            if len(artifacts) >= self.max_artifacts:
                raise RuntimeError(f"UCloud Agent Sandbox execution produced more than {self.max_artifacts} artifacts.")
            if entry.size > self.max_artifact_bytes:
                raise RuntimeError(f"Artifact exceeds {self.max_artifact_bytes} bytes: {relative_path}")
            extension = os.path.splitext(name)[1].lower()
            if extension not in ALLOWED_ARTIFACT_EXTENSIONS:
                raise RuntimeError(f"Unsupported artifact type: {relative_path}")
            content = bytes(sandbox.files.read(entry.path, format="bytes", request_timeout=self.timeout))
            artifacts.append(
                {
                    "name": relative_path,
                    "content_b64": base64.b64encode(content).decode("ascii"),
                    "mime_type": mimetypes.guess_type(name)[0] or "application/octet-stream",
                    "size": entry.size,
                }
            )

    def _safe_kill(self, sandbox) -> None:
        """Best-effort terminate a remote sandbox during cleanup."""
        try:
            sandbox.kill(request_timeout=self.timeout)
        except Exception as exc:  # noqa: BLE001 - cleanup is deliberately best-effort
            logger.warning("Failed to kill UCloud Agent Sandbox %s: %s", sandbox.sandbox_id, exc)

    @staticmethod
    def _normalize_language(language: str) -> str:
        """Normalize supported language aliases to provider runtime names."""
        value = (language or "python").lower()
        if value in {"python", "python3"}:
            return "python"
        if value in {"javascript", "js", "node", "nodejs"}:
            return "nodejs"
        return value


def _get_ucloud_sandbox_module():
    """Import and return the UCloud SDK with a provider-specific error."""
    try:
        import ucloud_sandbox
    except ImportError as exc:
        raise SandboxProviderConfigError("ucloud-sandbox is required for the UCloud Agent Sandbox provider.") from exc
    return ucloud_sandbox
