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
import io
import json
import mimetypes
import os
import posixpath
import shlex
import stat
import time
import uuid
from typing import TYPE_CHECKING, Any, Dict, List, Optional

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

if TYPE_CHECKING:
    import paramiko


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


class SSHProvider(SandboxProvider):
    """Execute code on a remote host through SSH."""

    def __init__(self):
        self.host = ""
        self.port = 22
        self.username = ""
        self.password = ""
        self.private_key = ""
        self.passphrase = ""
        self.python_bin = "python3"
        self.node_bin = "node"
        self.work_dir = "/tmp"
        self.timeout = 30
        self.max_output_bytes = 1024 * 1024
        self.max_artifacts = 20
        self.max_artifact_bytes = 10 * 1024 * 1024
        self._initialized = False
        self._instances: dict[str, dict[str, Any]] = {}

    def initialize(self, config: Dict[str, Any]) -> bool:
        self.host = str(config.get("host", "")).strip()
        self.port = int(config.get("port", 22) or 22)
        self.username = str(config.get("username", "")).strip()
        self.password = str(config.get("password", "") or "")
        self.private_key = str(config.get("private_key", "") or "")
        self.passphrase = str(config.get("passphrase", "") or "")
        self.python_bin = str(config.get("python_bin", "python3") or "python3").strip() or "python3"
        self.node_bin = str(config.get("node_bin", "node") or "node").strip() or "node"
        self.work_dir = str(config.get("work_dir", "/tmp") or "/tmp").strip() or "/tmp"
        self.timeout = int(config.get("timeout", 30) or 30)
        self.max_output_bytes = int(config.get("max_output_bytes", 1024 * 1024) or 1024 * 1024)
        self.max_artifacts = int(config.get("max_artifacts", 20) or 20)
        self.max_artifact_bytes = int(config.get("max_artifact_bytes", 10 * 1024 * 1024) or 10 * 1024 * 1024)

        is_valid, error_message = self.validate_config(
            {
                "host": self.host,
                "port": self.port,
                "username": self.username,
                "password": self.password,
                "private_key": self.private_key,
                "passphrase": self.passphrase,
                "python_bin": self.python_bin,
                "node_bin": self.node_bin,
                "work_dir": self.work_dir,
                "timeout": self.timeout,
                "max_output_bytes": self.max_output_bytes,
                "max_artifacts": self.max_artifacts,
                "max_artifact_bytes": self.max_artifact_bytes,
            }
        )
        if not is_valid:
            raise SandboxProviderConfigError(error_message or "Invalid SSH provider configuration.")

        self._assert_connectivity()

        self._initialized = True
        return True

    def create_instance(self, template: str = "python") -> SandboxInstance:
        if not self._initialized:
            raise RuntimeError("Provider not initialized. Call initialize() first.")

        language = self._normalize_language(template)
        client = self._create_ssh_client()
        sftp = client.open_sftp()

        try:
            remote_work_dir = self._create_remote_workspace(client)
            stdout, stderr, exit_code = self._run_remote_command(
                client,
                f"mkdir -p {shlex.quote(posixpath.join(remote_work_dir, 'artifacts'))}",
                timeout=min(self.timeout, 10),
            )
            if exit_code != 0:
                raise RuntimeError(
                    f"Failed to create remote artifacts directory: {stderr or stdout or 'unknown error'}"
                )
        except Exception:
            sftp.close()
            client.close()
            raise

        instance_id = str(uuid.uuid4())
        self._instances[instance_id] = {
            "client": client,
            "sftp": sftp,
            "remote_work_dir": remote_work_dir,
            "language": language,
        }

        return SandboxInstance(
            instance_id=instance_id,
            provider="ssh",
            status="running",
            metadata={"language": language, "remote_work_dir": remote_work_dir},
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
            raise RuntimeError(f"Unknown SSH sandbox instance: {instance_id}")

        normalized_lang = self._normalize_language(language)
        instance = self._instances[instance_id]
        client: paramiko.SSHClient = instance["client"]
        sftp: paramiko.SFTPClient = instance["sftp"]
        remote_work_dir: str = instance["remote_work_dir"]

        args_json = json.dumps(arguments or {}, ensure_ascii=False)
        remote_script_path, command = self._upload_script(
            sftp=sftp,
            remote_work_dir=remote_work_dir,
            language=normalized_lang,
            code=code,
            args_json=args_json,
        )

        requested_timeout = self.timeout if timeout is None else int(timeout)
        if requested_timeout <= 0:
            raise RuntimeError(f"Execution timeout must be greater than 0 seconds, got {requested_timeout}.")
        exec_timeout = min(requested_timeout, self.timeout)

        start_time = time.time()
        stdout, stderr, exit_code = self._run_remote_command(client, command, timeout=exec_timeout)
        execution_time = time.time() - start_time

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
                "script_path": remote_script_path,
                "remote_work_dir": remote_work_dir,
                "status": "ok" if exit_code == 0 else "error",
                "timeout": exec_timeout,
                "command": command,
                "artifacts": self._collect_artifacts(
                    sftp, posixpath.join(remote_work_dir, "artifacts")
                ),
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
        client: paramiko.SSHClient = instance["client"]
        sftp: paramiko.SFTPClient = instance["sftp"]
        remote_work_dir: str = instance["remote_work_dir"]

        cleanup_error: Optional[Exception] = None
        try:
            stdout, stderr, exit_code = self._run_remote_command(
                client,
                f"rm -rf {shlex.quote(remote_work_dir)}",
                timeout=min(self.timeout, 10),
            )
            if exit_code != 0:
                raise RuntimeError(stderr or stdout or "unknown error")
        except Exception as exc:
            cleanup_error = exc
        finally:
            try:
                sftp.close()
            finally:
                client.close()

        if cleanup_error is not None:
            raise RuntimeError(f"Failed to clean remote workspace {remote_work_dir}: {cleanup_error}")
        return True

    def health_check(self) -> bool:
        try:
            self._assert_connectivity()
            return True
        except Exception:
            return False

    def _assert_connectivity(self) -> None:
        try:
            client = self._create_ssh_client()
            try:
                _, stderr, exit_code = self._run_remote_command(
                    client,
                    "true",
                    timeout=min(self.timeout, 10),
                )
                if exit_code != 0:
                    raise SandboxProviderConfigError(
                        f"SSH connectivity check failed on {self.username}@{self.host}:{self.port}: "
                        f"{stderr or 'remote command returned non-zero exit status'}"
                    )
            finally:
                client.close()
        except SandboxProviderConfigError:
            raise
        except Exception as exc:
            raise SandboxProviderConfigError(
                f"Failed to connect to SSH host {self.username}@{self.host}:{self.port}: {exc}"
            ) from exc

    def get_supported_languages(self) -> List[str]:
        return ["python", "javascript", "nodejs"]

    @staticmethod
    def get_config_schema() -> Dict[str, Dict]:
        return {
            "host": {
                "type": "string",
                "required": True,
                "label": "SSH Host",
                "placeholder": "192.168.1.10",
                "description": "Remote host that will execute generated code.",
            },
            "port": {
                "type": "integer",
                "required": True,
                "label": "SSH Port",
                "default": 22,
                "min": 1,
                "max": 65535,
                "description": "SSH port on the remote host.",
            },
            "username": {
                "type": "string",
                "required": True,
                "label": "SSH Username",
                "placeholder": "ragflow",
                "description": "Username used to connect to the remote host.",
            },
            "password": {
                "type": "string",
                "required": False,
                "label": "SSH Password",
                "secret": True,
                "placeholder": "Optional when using a private key",
                "description": "Password-based SSH authentication.",
            },
            "private_key": {
                "type": "string",
                "required": False,
                "label": "SSH Private Key",
                "secret": True,
                "multiline": True,
                "placeholder": "Paste PEM content or enter a local file path",
                "description": "Private key PEM content or a readable private key path on the RAGFlow host.",
            },
            "passphrase": {
                "type": "string",
                "required": False,
                "label": "Private Key Passphrase",
                "secret": True,
                "placeholder": "Optional",
                "description": "Passphrase for the private key if it is encrypted.",
            },
            "python_bin": {
                "type": "string",
                "required": False,
                "default": "python3",
                "label": "Python Binary",
                "description": "Python executable used for remote code execution.",
            },
            "node_bin": {
                "type": "string",
                "required": False,
                "default": "node",
                "label": "Node.js Binary",
                "description": "Node.js executable used for remote JavaScript execution.",
            },
            "work_dir": {
                "type": "string",
                "required": False,
                "label": "Remote Workspace Root",
                "default": "/tmp",
                "placeholder": "/tmp",
                "description": "Writable remote directory used to create a temporary workspace.",
            },
            "timeout": {
                "type": "integer",
                "required": False,
                "label": "Timeout (seconds)",
                "default": 30,
                "min": 1,
                "max": 600,
                "description": "Maximum SSH execution time for a single run.",
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
                "description": "Maximum number of files collected from the remote artifacts directory.",
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
        host = str(config.get("host", "") or "").strip()
        username = str(config.get("username", "") or "").strip()
        password = str(config.get("password", "") or "")
        private_key = str(config.get("private_key", "") or "")
        python_bin = str(config.get("python_bin", "python3") or "python3").strip()
        node_bin = str(config.get("node_bin", "node") or "node").strip()

        if not host:
            return False, "SSH host is required"
        if not username:
            return False, "SSH username is required"
        if not password and not private_key:
            return False, "Either password or private_key must be provided"
        if not python_bin:
            return False, "Python binary is required"
        if not node_bin:
            return False, "Node.js binary is required"

        try:
            port = int(config.get("port", 22) or 22)
        except (TypeError, ValueError):
            return False, "SSH port must be an integer"
        if port <= 0 or port > 65535:
            return False, "SSH port must be between 1 and 65535"

        for key in ("timeout", "max_output_bytes", "max_artifacts", "max_artifact_bytes"):
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

    def _create_ssh_client(self) -> paramiko.SSHClient:
        paramiko = _get_paramiko_module()
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())

        connect_kwargs: dict[str, Any] = {
            "hostname": self.host,
            "port": self.port,
            "username": self.username,
            "timeout": self.timeout,
            "banner_timeout": self.timeout,
            "auth_timeout": self.timeout,
            "look_for_keys": False,
            "allow_agent": False,
        }
        if self.private_key:
            connect_kwargs["pkey"] = self._load_private_key()
        if self.password:
            connect_kwargs["password"] = self.password

        client.connect(**connect_kwargs)
        return client

    def _load_private_key(self) -> paramiko.PKey:
        paramiko = _get_paramiko_module()
        loaders = (
            paramiko.RSAKey,
            paramiko.Ed25519Key,
            paramiko.ECDSAKey,
            paramiko.DSSKey,
        )
        errors: list[str] = []
        private_key_value = self.private_key.strip()
        passphrase = self.passphrase or None

        if os.path.exists(private_key_value):
            for key_cls in loaders:
                try:
                    return key_cls.from_private_key_file(private_key_value, password=passphrase)
                except Exception as exc:
                    errors.append(str(exc))
        else:
            for key_cls in loaders:
                try:
                    return key_cls.from_private_key(io.StringIO(private_key_value), password=passphrase)
                except Exception as exc:
                    errors.append(str(exc))

        raise SandboxProviderConfigError(
            "Failed to load SSH private key. " + "; ".join(error for error in errors if error)
        )

    def _create_remote_workspace(self, client: paramiko.SSHClient) -> str:
        base_dir = self.work_dir.rstrip("/") or "/tmp"
        template = posixpath.join(base_dir, "ragflow-codeexec.XXXXXX")
        stdout, stderr, exit_code = self._run_remote_command(
            client,
            f"mkdir -p {shlex.quote(base_dir)} && mktemp -d {shlex.quote(template)}",
            timeout=min(self.timeout, 10),
        )
        if exit_code != 0:
            raise RuntimeError(
                f"Failed to create remote workspace on {self.host}: {stderr or stdout or 'unknown error'}"
            )

        remote_work_dir = stdout.strip().splitlines()[-1] if stdout.strip() else ""
        if not remote_work_dir:
            raise RuntimeError("Remote workspace creation did not return a path.")
        return remote_work_dir

    def _upload_script(
        self,
        sftp: paramiko.SFTPClient,
        remote_work_dir: str,
        language: str,
        code: str,
        args_json: str,
    ) -> tuple[str, str]:
        if language == "python":
            script_name = "main.py"
            script_content = build_python_wrapper(code, args_json)
        elif language in {"javascript", "nodejs"}:
            script_name = "main.js"
            script_content = build_javascript_wrapper(code, args_json)
        else:
            raise RuntimeError(f"Unsupported language for SSH provider: {language}")

        remote_script_path = posixpath.join(remote_work_dir, script_name)
        with sftp.file(remote_script_path, "w") as remote_file:
            remote_file.write(script_content)

        command = self._build_execution_command(remote_work_dir, remote_script_path, language)
        return remote_script_path, command

    def _build_execution_command(self, remote_work_dir: str, remote_script_path: str, language: str) -> str:
        normalized_lang = self._normalize_language(language)
        if normalized_lang == "python":
            executable = self.python_bin
        elif normalized_lang == "nodejs":
            executable = self.node_bin
        else:
            raise RuntimeError(f"Unsupported language for SSH provider: {language}")

        return (
            f"cd {shlex.quote(remote_work_dir)} && "
            f"{shlex.quote(executable)} {shlex.quote(remote_script_path)}"
        )

    def _run_remote_command(
        self,
        client: paramiko.SSHClient,
        command: str,
        timeout: int,
    ) -> tuple[str, str, int]:
        stdin, stdout_stream, stderr_stream = client.exec_command(command, timeout=timeout)
        stdin.close()
        channel = stdout_stream.channel

        stdout_chunks: list[bytes] = []
        stderr_chunks: list[bytes] = []
        deadline = time.time() + timeout

        while True:
            while channel.recv_ready():
                stdout_chunks.append(channel.recv(65536))
            while channel.recv_stderr_ready():
                stderr_chunks.append(channel.recv_stderr(65536))

            if channel.exit_status_ready():
                break
            if time.time() > deadline:
                channel.close()
                raise TimeoutError(f"Execution timed out after {timeout} seconds")
            time.sleep(0.1)

        while channel.recv_ready():
            stdout_chunks.append(channel.recv(65536))
        while channel.recv_stderr_ready():
            stderr_chunks.append(channel.recv_stderr(65536))

        exit_code = channel.recv_exit_status()
        stdout = b"".join(stdout_chunks).decode("utf-8", errors="replace")
        stderr = b"".join(stderr_chunks).decode("utf-8", errors="replace")
        return stdout, stderr, exit_code

    def _validate_output_size(self, stdout: str, stderr: str) -> None:
        output_size = len((stdout or "").encode("utf-8")) + len((stderr or "").encode("utf-8"))
        if output_size > self.max_output_bytes:
            raise RuntimeError(f"SSH execution output exceeded {self.max_output_bytes} bytes.")

    def _collect_artifacts(
        self,
        sftp: paramiko.SFTPClient,
        artifacts_dir: str,
    ) -> list[dict[str, Any]]:
        artifacts: list[dict[str, Any]] = []
        self._collect_artifacts_recursive(sftp, artifacts_dir, "", artifacts)
        return artifacts

    def _collect_artifacts_recursive(
        self,
        sftp: paramiko.SFTPClient,
        current_dir: str,
        relative_dir: str,
        artifacts: list[dict[str, Any]],
    ) -> None:
        try:
            entries = sftp.listdir_attr(current_dir)
        except FileNotFoundError:
            return

        for entry in sorted(entries, key=lambda item: item.filename):
            name = entry.filename
            remote_path = posixpath.join(current_dir, name)
            relative_path = posixpath.join(relative_dir, name) if relative_dir else name
            mode = entry.st_mode
            if mode is None:
                mode = sftp.lstat(remote_path).st_mode
            if mode is None:
                raise RuntimeError(f"Unable to determine artifact entry type: {relative_path}")

            if stat.S_ISLNK(mode):
                raise RuntimeError(f"Artifact symlinks are not allowed: {relative_path}")
            if stat.S_ISDIR(mode):
                self._collect_artifacts_recursive(sftp, remote_path, relative_path, artifacts)
                continue
            if not stat.S_ISREG(mode):
                raise RuntimeError(f"Unsupported artifact entry: {relative_path}")

            if len(artifacts) >= self.max_artifacts:
                raise RuntimeError(f"SSH execution produced more than {self.max_artifacts} artifacts.")

            size = int(entry.st_size or 0)
            if size > self.max_artifact_bytes:
                raise RuntimeError(f"Artifact exceeds {self.max_artifact_bytes} bytes: {relative_path}")

            ext = os.path.splitext(name)[1].lower()
            if ext not in ALLOWED_ARTIFACT_EXTENSIONS:
                raise RuntimeError(f"Unsupported artifact type: {relative_path}")

            with sftp.file(remote_path, "rb") as artifact_file:
                content = artifact_file.read()

            artifacts.append(
                {
                    "name": relative_path,
                    "content_b64": base64.b64encode(content).decode("ascii"),
                    "mime_type": mimetypes.guess_type(name)[0] or "application/octet-stream",
                    "size": size,
                }
            )

    @staticmethod
    def _normalize_language(language: str) -> str:
        lang_lower = (language or "python").lower()
        if lang_lower in {"python", "python3"}:
            return "python"
        if lang_lower in {"javascript", "nodejs"}:
            return "nodejs"
        return lang_lower


def _get_paramiko_module():
    try:
        import paramiko
    except ImportError as exc:
        raise SandboxProviderConfigError(
            "paramiko is required for the SSH sandbox provider. Install the project dependencies to enable it."
        ) from exc
    return paramiko
