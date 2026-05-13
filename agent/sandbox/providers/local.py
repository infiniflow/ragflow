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

import base64
import json
import mimetypes
import os
import shutil
import signal
import subprocess
import time
import uuid
from pathlib import Path
from typing import Any, Dict, List, Optional

from agent.sandbox.result_protocol import build_javascript_wrapper, build_python_wrapper, extract_structured_result
from .base import ExecutionResult, SandboxInstance, SandboxProvider, SandboxProviderConfigError


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

LOCAL_PYTHON_THREAD_ENV_VARS = (
    "OPENBLAS_NUM_THREADS",
    "OMP_NUM_THREADS",
    "MKL_NUM_THREADS",
    "NUMEXPR_NUM_THREADS",
    "BLIS_NUM_THREADS",
    "VECLIB_MAXIMUM_THREADS",
)


def _env_enabled(name: str) -> bool:
    return os.environ.get(name, "").strip().lower() in {"1", "true", "yes", "on"}


class LocalProvider(SandboxProvider):
    """
    Execute code as a local child process.

    This provider is intentionally gated by SANDBOX_LOCAL_ENABLED because it is
    not a sandbox boundary. Use a low-privilege runtime account.
    """

    def __init__(self):
        self.python_bin = "python3"
        self.node_bin = "node"
        self.work_dir = Path("/tmp/ragflow-codeexec")
        self.timeout = 30
        self.max_memory_mb = 512
        self.max_output_bytes = 1024 * 1024
        self.max_artifacts = 20
        self.max_artifact_bytes = 10 * 1024 * 1024
        self._initialized = False
        self._instances: dict[str, Path] = {}

    def initialize(self, config: Dict[str, Any]) -> bool:
        if not _env_enabled("SANDBOX_LOCAL_ENABLED"):
            raise SandboxProviderConfigError("Local code execution is disabled. Set SANDBOX_LOCAL_ENABLED=true to enable it.")

        self.python_bin = str(self._resolve_config_value(config, "python_bin", "SANDBOX_LOCAL_PYTHON_BIN", "python3"))
        self.node_bin = str(self._resolve_config_value(config, "node_bin", "SANDBOX_LOCAL_NODE_BIN", "node"))
        self.work_dir = Path(self._resolve_config_value(config, "work_dir", "SANDBOX_LOCAL_WORK_DIR", "/tmp/ragflow-codeexec")).resolve()
        self.timeout = int(self._resolve_config_value(config, "timeout", "SANDBOX_LOCAL_TIMEOUT", 30))
        self.max_memory_mb = int(self._resolve_config_value(config, "max_memory_mb", "SANDBOX_LOCAL_MAX_MEMORY_MB", 512))
        self.max_output_bytes = int(self._resolve_config_value(config, "max_output_bytes", "SANDBOX_LOCAL_MAX_OUTPUT_BYTES", 1024 * 1024))
        self.max_artifacts = int(self._resolve_config_value(config, "max_artifacts", "SANDBOX_LOCAL_MAX_ARTIFACTS", 20))
        self.max_artifact_bytes = int(self._resolve_config_value(config, "max_artifact_bytes", "SANDBOX_LOCAL_MAX_ARTIFACT_BYTES", 10 * 1024 * 1024))

        self._validate_limits()
        self.work_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
        self._initialized = True
        return True

    def create_instance(self, template: str = "python") -> SandboxInstance:
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        language = self._normalize_language(template)
        instance_id = str(uuid.uuid4())
        instance_dir = self.work_dir / instance_id
        instance_dir.mkdir(mode=0o700)
        (instance_dir / "artifacts").mkdir(mode=0o700)
        self._instances[instance_id] = instance_dir

        return SandboxInstance(
            instance_id=instance_id,
            provider="local",
            status="running",
            metadata={"language": language, "work_dir": str(instance_dir)},
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

        normalized_lang = self._normalize_language(language)
        instance_dir = self._instances[instance_id]
        args_json = json.dumps(arguments or {}, ensure_ascii=False)
        command, script_path = self._prepare_script(instance_dir, normalized_lang, code, args_json)
        requested_timeout = self.timeout if timeout is None else int(timeout)
        if requested_timeout <= 0:
            raise RuntimeError(f"Execution timeout must be greater than 0 seconds, got {requested_timeout}.")
        exec_timeout = min(requested_timeout, self.timeout)

        start_time = time.time()
        process = subprocess.Popen(
            command,
            cwd=instance_dir,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            encoding="utf-8",
            errors="replace",
            env=self._build_child_env(instance_dir),
            preexec_fn=self._limit_child_process if os.name == "posix" else None,
            start_new_session=os.name == "posix",
        )

        try:
            stdout, stderr = process.communicate(timeout=exec_timeout)
        except subprocess.TimeoutExpired:
            if os.name == "posix":
                os.killpg(process.pid, signal.SIGKILL)
            else:
                process.kill()
            process.communicate()
            raise TimeoutError(f"Execution timed out after {exec_timeout} seconds")

        execution_time = time.time() - start_time
        self._validate_output_size(stdout, stderr)
        stdout, structured_result = extract_structured_result(stdout)

        return ExecutionResult(
            stdout=stdout,
            stderr=stderr,
            exit_code=process.returncode,
            execution_time=execution_time,
            metadata={
                "instance_id": instance_id,
                "language": normalized_lang,
                "script_path": str(script_path),
                "status": "ok" if process.returncode == 0 else "error",
                "timeout": exec_timeout,
                "artifacts": self._collect_artifacts(instance_dir / "artifacts"),
                "result_present": structured_result.get("present", False),
                "result_value": structured_result.get("value"),
                "result_type": structured_result.get("type"),
            },
        )

    def destroy_instance(self, instance_id: str) -> bool:
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        instance_dir = self._instances.pop(instance_id)
        shutil.rmtree(instance_dir)
        return True

    def health_check(self) -> bool:
        return self._initialized and self.work_dir.exists() and os.access(self.work_dir, os.W_OK)

    def get_supported_languages(self) -> List[str]:
        return ["python", "javascript", "nodejs"]

    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        return {
            "python_bin": {"type": "string", "required": False, "default": "python3"},
            "node_bin": {"type": "string", "required": False, "default": "node"},
            "work_dir": {"type": "string", "required": False, "default": "/tmp/ragflow-codeexec"},
            "timeout": {"type": "integer", "required": False, "default": 30},
            "max_memory_mb": {"type": "integer", "required": False, "default": 512},
            "max_output_bytes": {"type": "integer", "required": False, "default": 1048576},
            "max_artifacts": {"type": "integer", "required": False, "default": 20},
            "max_artifact_bytes": {"type": "integer", "required": False, "default": 10485760},
        }

    def _validate_limits(self) -> None:
        if self.timeout <= 0:
            raise SandboxProviderConfigError("SANDBOX_LOCAL_TIMEOUT must be greater than 0.")
        if self.max_memory_mb <= 0:
            raise SandboxProviderConfigError("SANDBOX_LOCAL_MAX_MEMORY_MB must be greater than 0.")
        if self.max_output_bytes <= 0:
            raise SandboxProviderConfigError("SANDBOX_LOCAL_MAX_OUTPUT_BYTES must be greater than 0.")
        if self.max_artifacts < 0:
            raise SandboxProviderConfigError("SANDBOX_LOCAL_MAX_ARTIFACTS must be greater than or equal to 0.")
        if self.max_artifact_bytes <= 0:
            raise SandboxProviderConfigError("SANDBOX_LOCAL_MAX_ARTIFACT_BYTES must be greater than 0.")

    def _prepare_script(self, instance_dir: Path, language: str, code: str, args_json: str) -> tuple[list[str], Path]:
        if language == "python":
            script_path = instance_dir / "main.py"
            script_path.write_text(build_python_wrapper(code, args_json), encoding="utf-8")
            return [self.python_bin, str(script_path)], script_path
        if language in {"javascript", "nodejs"}:
            script_path = instance_dir / "main.js"
            script_path.write_text(build_javascript_wrapper(code, args_json), encoding="utf-8")
            return [self.node_bin, str(script_path)], script_path
        raise RuntimeError(f"Unsupported language for local provider: {language}")

    @staticmethod
    def _resolve_config_value(config: Dict[str, Any], key: str, env_name: str, default: Any) -> Any:
        value = config.get(key)
        if value is not None:
            return value
        return os.environ.get(env_name, default)

    def _build_child_env(self, instance_dir: Path) -> dict[str, str]:
        env = {
            "HOME": str(instance_dir),
            "MPLBACKEND": "Agg",
            "PATH": os.environ.get("PATH", ""),
            "PYTHONUNBUFFERED": "1",
            "TMPDIR": str(instance_dir),
        }
        for name in LOCAL_PYTHON_THREAD_ENV_VARS:
            value = os.environ.get(name)
            if value is not None:
                env[name] = value
        return env

    def _limit_child_process(self) -> None:
        import resource

        self._set_resource_limit(resource.RLIMIT_CPU, self.timeout + 1)
        self._set_resource_limit(resource.RLIMIT_AS, self.max_memory_mb * 1024 * 1024)
        self._set_resource_limit(resource.RLIMIT_FSIZE, self.max_artifact_bytes)
        self._set_resource_limit(resource.RLIMIT_NOFILE, 64)

    @staticmethod
    def _set_resource_limit(kind: int, value: int) -> None:
        import resource

        _, hard = resource.getrlimit(kind)
        limit = value if hard == resource.RLIM_INFINITY else min(value, hard)
        resource.setrlimit(kind, (limit, limit))

    def _validate_output_size(self, stdout: str, stderr: str) -> None:
        output_size = len((stdout or "").encode("utf-8")) + len((stderr or "").encode("utf-8"))
        if output_size > self.max_output_bytes:
            raise RuntimeError(f"Local execution output exceeded {self.max_output_bytes} bytes.")

    def _collect_artifacts(self, artifacts_dir: Path) -> list[dict[str, Any]]:
        artifacts: list[dict[str, Any]] = []
        for path in sorted(artifacts_dir.rglob("*")):
            if path.is_symlink():
                raise RuntimeError(f"Artifact symlinks are not allowed: {path.name}")
            if path.is_dir():
                continue
            if not path.is_file():
                raise RuntimeError(f"Unsupported artifact entry: {path.name}")

            if len(artifacts) >= self.max_artifacts:
                raise RuntimeError(f"Local execution produced more than {self.max_artifacts} artifacts.")

            size = path.stat().st_size
            if size > self.max_artifact_bytes:
                raise RuntimeError(f"Artifact exceeds {self.max_artifact_bytes} bytes: {path.name}")

            ext = path.suffix.lower()
            if ext not in ALLOWED_ARTIFACT_EXTENSIONS:
                raise RuntimeError(f"Unsupported artifact type: {path.name}")

            artifacts.append(
                {
                    "name": path.relative_to(artifacts_dir).as_posix(),
                    "content_b64": base64.b64encode(path.read_bytes()).decode("ascii"),
                    "mime_type": mimetypes.guess_type(path.name)[0] or "application/octet-stream",
                    "size": size,
                }
            )
        return artifacts

    @staticmethod
    def _normalize_language(language: str) -> str:
        lang_lower = (language or "python").lower()
        if lang_lower in {"python", "python3"}:
            return "python"
        if lang_lower in {"javascript", "nodejs"}:
            return "nodejs"
        return lang_lower
