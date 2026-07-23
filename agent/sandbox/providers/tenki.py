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

from __future__ import annotations

import base64
import json
import mimetypes
import os
import posixpath
import stat
import time
import uuid
from typing import Any, Dict, List, Optional

from agent.sandbox.result_protocol import (
    build_javascript_wrapper,
    build_python_wrapper,
    extract_structured_result,
)
from .base import (
    ExecutionResult,
    SandboxInstance,
    SandboxProvider,
    SandboxProviderConfigError,
)

ALLOWED_ARTIFACT_EXTENSIONS = {
    ".csv",
    ".html",
    ".jpeg",
    ".jpg",
    ".json",
    ".pdf",
    ".png",
    ".svg",
}

# Base directory inside the Tenki sandbox. The default image runs as the
# unprivileged "tenki" user whose home is writable.
SANDBOX_HOME = "/home/tenki"

# Maximum directory depth walked when collecting artifacts, guarding against
# deeply nested or symlink-looped trees produced by untrusted code.
MAX_ARTIFACT_DEPTH = 16


class TenkiProvider(SandboxProvider):
    """Execute code in a disposable Tenki microVM.

    Each create_instance() provisions a fresh sandbox, execute_code() runs a
    single script in it, and destroy_instance() terminates it. Only the stable
    create/exec/destroy path is used; no volumes or snapshots.
    """

    def __init__(self):
        self.api_key = ""
        self.project_id = ""
        self.base_url = ""
        self.image = ""
        self.allow_outbound = True
        self.timeout = 30
        self.max_lifetime = 3600
        self.cpu_cores = 0
        self.memory_mb = 0
        self.disk_size_gb = 0
        self.max_output_bytes = 1024 * 1024
        self.max_artifacts = 20
        self.max_artifact_bytes = 10 * 1024 * 1024
        self._initialized = False
        self._client = None
        self._instances: dict[str, dict[str, Any]] = {}

    def initialize(self, config: Dict[str, Any]) -> bool:
        self.api_key = str(config.get("api_key", "") or "").strip()
        self.project_id = str(config.get("project_id", "") or "").strip()
        self.base_url = str(config.get("base_url", "") or "").strip()
        self.image = str(config.get("image", "") or "").strip()
        self.allow_outbound = bool(config.get("allow_outbound", True))
        self.timeout = int(config.get("timeout", 30) or 30)
        self.max_lifetime = int(config.get("max_lifetime", 3600) or 3600)
        self.cpu_cores = int(config.get("cpu_cores", 0) or 0)
        self.memory_mb = int(config.get("memory_mb", 0) or 0)
        self.disk_size_gb = int(config.get("disk_size_gb", 0) or 0)
        self.max_output_bytes = int(config.get("max_output_bytes", 1024 * 1024) or 1024 * 1024)
        self.max_artifacts = int(config.get("max_artifacts", 20) or 20)
        self.max_artifact_bytes = int(config.get("max_artifact_bytes", 10 * 1024 * 1024) or 10 * 1024 * 1024)

        is_valid, error_message = self.validate_config(
            {
                "api_key": self.api_key,
                "project_id": self.project_id,
                "timeout": self.timeout,
                "max_lifetime": self.max_lifetime,
                "max_output_bytes": self.max_output_bytes,
                "max_artifacts": self.max_artifacts,
                "max_artifact_bytes": self.max_artifact_bytes,
            }
        )
        if not is_valid:
            raise SandboxProviderConfigError(error_message or "Invalid Tenki provider configuration.")

        self._client = self._create_client()
        self._assert_connectivity()

        self._initialized = True
        return True

    def create_instance(self, template: str = "python") -> SandboxInstance:
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        language = self._normalize_language(template)
        errors = self._tenki_errors()

        create_kwargs: dict[str, Any] = {
            "project_id": self.project_id,
            "allow_outbound": self.allow_outbound,
            "max_duration": self.max_lifetime,
            "metadata": {"source": "ragflow"},
        }
        if self.image:
            create_kwargs["image"] = self.image
        if self.cpu_cores > 0:
            create_kwargs["cpu_cores"] = self.cpu_cores
        if self.memory_mb > 0:
            create_kwargs["memory_mb"] = self.memory_mb
        if self.disk_size_gb > 0:
            create_kwargs["disk_size_gb"] = self.disk_size_gb

        try:
            sandbox = self._client.create(**create_kwargs)
        except errors.QuotaExceededError as exc:
            raise RuntimeError(f"Tenki quota exceeded: {exc}") from exc
        except errors.RateLimitedError as exc:
            raise RuntimeError(f"Tenki rate limited, please retry: {exc}") from exc
        except errors.UnauthorizedError as exc:
            raise SandboxProviderConfigError("Tenki authentication failed: check the API key.") from exc

        remote_work_dir = posixpath.join(SANDBOX_HOME, f"ragflow-codeexec-{uuid.uuid4().hex}")
        try:
            result = sandbox.exec(
                "mkdir", "-p", posixpath.join(remote_work_dir, "artifacts"),
                timeout=min(self.timeout, 10),
            )
            if result.exit_code != 0:
                raise RuntimeError(f"Failed to create sandbox workspace: {result.stderr_text or 'unknown error'}")
        except Exception:
            self._safe_terminate(sandbox)
            raise

        instance_id = str(uuid.uuid4())
        self._instances[instance_id] = {
            "sandbox": sandbox,
            "remote_work_dir": remote_work_dir,
            "language": language,
        }

        return SandboxInstance(
            instance_id=instance_id,
            provider="tenki",
            status="running",
            metadata={"language": language, "remote_work_dir": remote_work_dir, "sandbox_id": sandbox.id},
        )

    def execute_code(
        self,
        instance_id: str,
        code: str,
        language: str,
        timeout: int = 10,
        arguments: Optional[Dict[str, Any]] = None,
    ) -> ExecutionResult:
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")
        if instance_id not in self._instances:
            raise RuntimeError(f"Unknown Tenki sandbox instance: {instance_id}")

        normalized_lang = self._normalize_language(language)
        instance = self._instances[instance_id]
        sandbox = instance["sandbox"]
        remote_work_dir: str = instance["remote_work_dir"]
        errors = self._tenki_errors()

        args_json = json.dumps(arguments or {}, ensure_ascii=False)
        script_path, argv = self._prepare_script(sandbox, remote_work_dir, normalized_lang, code, args_json)

        requested_timeout = self.timeout if timeout is None else int(timeout)
        if requested_timeout <= 0:
            raise RuntimeError(f"Execution timeout must be greater than 0 seconds, got {requested_timeout}.")
        exec_timeout = min(requested_timeout, self.timeout)

        start_time = time.time()
        try:
            result = sandbox.exec(*argv, cwd=remote_work_dir, timeout=exec_timeout)
        except (errors.CommandTimeoutError, errors.PrimitiveTimeoutError) as exc:
            raise TimeoutError(f"Execution timed out after {exec_timeout} seconds") from exc
        except (errors.SessionTerminatedError, errors.SessionNotFoundError) as exc:
            # The sandbox was reclaimed (e.g. max_lifetime) or lost mid-run;
            # surface it as RuntimeError per the base contract, not an SDK type.
            raise RuntimeError(f"Tenki sandbox is no longer available: {exc}") from exc
        execution_time = time.time() - start_time

        stdout = result.stdout_text
        stderr = result.stderr_text
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
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")
        if instance_id not in self._instances:
            return True

        instance = self._instances.pop(instance_id)
        self._safe_terminate(instance["sandbox"])
        return True

    def health_check(self) -> bool:
        try:
            self._assert_connectivity()
            return True
        except Exception:
            return False

    def get_supported_languages(self) -> List[str]:
        return ["python", "javascript", "nodejs"]

    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        return {
            "api_key": {
                "type": "string",
                "required": True,
                "label": "API Key",
                "secret": True,
                "placeholder": "tk_...",
                "description": "Tenki API key. Create one at https://app.tenki.cloud under API Keys.",
            },
            "project_id": {
                "type": "string",
                "required": True,
                "label": "Project ID",
                "placeholder": "Tenki project UUID",
                "description": "Tenki project that sandboxes are created under.",
            },
            "base_url": {
                "type": "string",
                "required": False,
                "label": "API Endpoint",
                "placeholder": "https://api.tenki.cloud",
                "description": "Override the Tenki API endpoint. Leave empty for the default.",
            },
            "image": {
                "type": "string",
                "required": False,
                "label": "Sandbox Image",
                "description": "Base image for sandboxes. Empty uses the Tenki default image (includes python3 and node).",
            },
            "allow_outbound": {
                "type": "boolean",
                "required": False,
                "label": "Allow Outbound Network",
                "default": True,
                "description": "Security-relevant. Allow the sandbox to make outbound network connections (needed to install packages). Disable to run code without network access.",
            },
            "timeout": {
                "type": "integer",
                "required": False,
                "label": "Timeout (seconds)",
                "default": 30,
                "min": 1,
                "max": 600,
                "description": "Maximum execution time for a single run.",
            },
            "max_lifetime": {
                "type": "integer",
                "required": False,
                "label": "Max Sandbox Lifetime (seconds)",
                "default": 3600,
                "min": 60,
                "max": 86400,
                "description": "Tenki reclaims a sandbox after this, guarding against leaks if the run is interrupted.",
            },
            "cpu_cores": {
                "type": "integer",
                "required": False,
                "label": "vCPU Cores",
                "default": 0,
                "min": 0,
                "max": 16,
                "description": "vCPU per sandbox. 0 uses the Tenki default.",
            },
            "memory_mb": {
                "type": "integer",
                "required": False,
                "label": "Memory (MB)",
                "default": 0,
                "min": 0,
                "max": 65536,
                "description": "Memory per sandbox in MB. 0 uses the Tenki default.",
            },
            "disk_size_gb": {
                "type": "integer",
                "required": False,
                "label": "Disk Size (GB)",
                "default": 0,
                "min": 0,
                "max": 100,
                "description": "Disk size per sandbox in GB. 0 uses the Tenki default.",
            },
            "max_output_bytes": {
                "type": "integer",
                "required": False,
                "label": "Max Output Bytes",
                "default": 1048576,
                "min": 1024,
                "max": 10485760,
                "description": "Maximum combined stdout and stderr size.",
            },
            "max_artifacts": {
                "type": "integer",
                "required": False,
                "label": "Max Artifacts",
                "default": 20,
                "min": 0,
                "max": 100,
                "description": "Maximum number of files collected from the sandbox artifacts directory.",
            },
            "max_artifact_bytes": {
                "type": "integer",
                "required": False,
                "label": "Max Artifact Bytes",
                "default": 10485760,
                "min": 1024,
                "max": 104857600,
                "description": "Maximum size of a single artifact file in bytes.",
            },
        }

    def validate_config(self, config: Dict[str, Any]) -> tuple[bool, Optional[str]]:
        api_key = str(config.get("api_key", "") or "").strip()
        project_id = str(config.get("project_id", "") or "").strip()

        if not api_key:
            return False, "Tenki API key is required"
        if not project_id:
            return False, "Tenki project_id is required"

        for key in ("timeout", "max_lifetime", "max_output_bytes", "max_artifacts", "max_artifact_bytes"):
            try:
                value = int(config.get(key, 0) or 0)
            except (TypeError, ValueError):
                return False, f"{key} must be an integer"
            if key == "max_artifacts":
                if value < 0:
                    return False, "max_artifacts must be greater than or equal to 0"
            elif value <= 0:
                return False, f"{key} must be greater than 0"

        return True, None

    # -- internal helpers -------------------------------------------------

    def _create_client(self):
        tenki = _get_tenki_module()
        kwargs: dict[str, Any] = {"auth_token": self.api_key}
        if self.base_url:
            kwargs["base_url"] = self.base_url
        return tenki.Client(**kwargs)

    def _assert_connectivity(self) -> None:
        client = self._client or self._create_client()
        errors = self._tenki_errors()
        try:
            client.who_am_i()
        except errors.UnauthorizedError as exc:
            raise SandboxProviderConfigError("Tenki authentication failed: check the API key.") from exc
        except Exception as exc:
            raise SandboxProviderConfigError(f"Failed to reach Tenki API: {exc}") from exc

    def _prepare_script(self, sandbox, remote_work_dir: str, language: str, code: str, args_json: str) -> tuple[str, list[str]]:
        if language == "python":
            script_name = "main.py"
            script_content = build_python_wrapper(code, args_json)
            executable = "python3"
        elif language in {"javascript", "nodejs"}:
            script_name = "main.js"
            script_content = build_javascript_wrapper(code, args_json)
            executable = "node"
        else:
            raise RuntimeError(f"Unsupported language for Tenki provider: {language}")

        script_path = posixpath.join(remote_work_dir, script_name)
        sandbox.fs.write_text(script_path, script_content)
        return script_path, [executable, script_path]

    def _validate_output_size(self, stdout: str, stderr: str) -> None:
        output_size = len((stdout or "").encode("utf-8")) + len((stderr or "").encode("utf-8"))
        if output_size > self.max_output_bytes:
            raise RuntimeError(f"Tenki execution output exceeded {self.max_output_bytes} bytes.")

    def _collect_artifacts(self, sandbox, artifacts_dir: str) -> list[dict[str, Any]]:
        artifacts: list[dict[str, Any]] = []
        self._collect_artifacts_recursive(sandbox, artifacts_dir, "", artifacts, depth=0)
        return artifacts

    def _collect_artifacts_recursive(self, sandbox, current_dir: str, relative_dir: str, artifacts: list[dict[str, Any]], depth: int) -> None:
        if depth > MAX_ARTIFACT_DEPTH:
            raise RuntimeError(f"Artifact directory nesting exceeds {MAX_ARTIFACT_DEPTH} levels: {relative_dir}")

        errors = self._tenki_errors()
        try:
            entries = sandbox.fs.list(current_dir)
        except errors.FileNotFoundError:
            return
        except FileNotFoundError:
            return

        # fs.list returns each entry's basename in `.path`, not an absolute
        # path, so join it onto the directory being listed.
        for entry in sorted(entries, key=lambda item: item.path):
            name = posixpath.basename(entry.path)
            remote_path = posixpath.join(current_dir, name)
            relative_path = posixpath.join(relative_dir, name) if relative_dir else name

            # Reject symlinks. `is_symlink` is not populated by every SDK
            # release, so also inspect the stat mode bits as the reliable check.
            if getattr(entry, "is_symlink", False) or stat.S_ISLNK(entry.mode or 0):
                raise RuntimeError(f"Artifact symlinks are not allowed: {relative_path}")
            if entry.is_dir:
                self._collect_artifacts_recursive(sandbox, remote_path, relative_path, artifacts, depth + 1)
                continue

            if len(artifacts) >= self.max_artifacts:
                raise RuntimeError(f"Tenki execution produced more than {self.max_artifacts} artifacts.")

            size = int(entry.size or 0)
            if size > self.max_artifact_bytes:
                raise RuntimeError(f"Artifact exceeds {self.max_artifact_bytes} bytes: {relative_path}")

            ext = os.path.splitext(name)[1].lower()
            if ext not in ALLOWED_ARTIFACT_EXTENSIONS:
                raise RuntimeError(f"Unsupported artifact type: {relative_path}")

            content = sandbox.fs.read_bytes(remote_path)
            artifacts.append(
                {
                    "name": relative_path,
                    "content_b64": base64.b64encode(content).decode("ascii"),
                    "mime_type": mimetypes.guess_type(name)[0] or "application/octet-stream",
                    "size": size,
                }
            )

    def _safe_terminate(self, sandbox) -> None:
        try:
            sandbox.terminate()
        except Exception:
            pass

    def _tenki_errors(self):
        tenki = _get_tenki_module()
        return tenki.errors

    @staticmethod
    def _normalize_language(language: str) -> str:
        lang_lower = (language or "python").lower()
        if lang_lower in {"python", "python3"}:
            return "python"
        if lang_lower in {"javascript", "nodejs"}:
            return "nodejs"
        return lang_lower


def _get_tenki_module():
    try:
        import tenki_sandbox
    except ImportError as exc:
        # tenki-sandbox is an optional dependency: it requires protobuf>=6.31,
        # which conflicts with RAGFlow's pinned gRPC stack, so it is not a core
        # dependency. Install it into the runtime to enable this provider.
        raise SandboxProviderConfigError("tenki-sandbox is required for the Tenki sandbox provider. Install it with `pip install tenki-sandbox` (or `uv pip install tenki-sandbox`).") from exc
    return tenki_sandbox
