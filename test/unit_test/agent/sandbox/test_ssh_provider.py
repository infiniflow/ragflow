import base64
from types import SimpleNamespace

import pytest

from agent.sandbox.providers.ssh import SSHProvider
from agent.sandbox.result_protocol import RESULT_MARKER_PREFIX

pytestmark = pytest.mark.p3


class _FakeWritableFile:
    def __init__(self, sftp, path: str):
        self._sftp = sftp
        self._path = path
        self._chunks: list[str] = []

    def write(self, content: str):
        self._chunks.append(content)

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, tb):
        self._sftp.files[self._path] = "".join(self._chunks).encode("utf-8")
        return False


class _FakeReadableFile:
    def __init__(self, payload: bytes):
        self._payload = payload

    def read(self):
        return self._payload

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, tb):
        return False


class _FakeSFTP:
    def __init__(self):
        self.files: dict[str, bytes] = {}
        self.closed = False

    def file(self, path: str, mode: str):
        if "w" in mode:
            return _FakeWritableFile(self, path)
        return _FakeReadableFile(self.files[path])

    def listdir_attr(self, path: str):
        prefix = path.rstrip("/") + "/"
        names = []
        for file_path, payload in self.files.items():
            if not file_path.startswith(prefix):
                continue
            relative = file_path[len(prefix):]
            if "/" in relative:
                continue
            names.append(
                SimpleNamespace(
                    filename=relative,
                    st_mode=0o100644,
                    st_size=len(payload),
                )
            )
        return names

    def close(self):
        self.closed = True


class _FakeClient:
    def __init__(self, sftp: _FakeSFTP):
        self._sftp = sftp
        self.closed = False

    def open_sftp(self):
        return self._sftp

    def close(self):
        self.closed = True


def _build_provider():
    provider = SSHProvider()
    provider.host = "example.com"
    provider.port = 22
    provider.username = "ragflow"
    provider.password = "secret"
    provider.work_dir = "/tmp"
    provider.command_template = "cd {workspace} && python3 {script_path}"
    provider.timeout = 5
    provider.max_output_bytes = 1024 * 1024
    provider.max_artifacts = 20
    provider.max_artifact_bytes = 1024 * 1024
    provider._initialized = True
    return provider


def test_ssh_provider_executes_python_main_and_collects_artifacts(monkeypatch):
    provider = _build_provider()
    fake_sftp = _FakeSFTP()
    fake_client = _FakeClient(fake_sftp)
    executed_commands: list[str] = []

    monkeypatch.setattr(provider, "_create_ssh_client", lambda: fake_client)
    monkeypatch.setattr(provider, "_create_remote_workspace", lambda client: "/tmp/ws-123")

    def _run_remote_command(client, command: str, timeout: int):
        executed_commands.append(command)
        if command.startswith("mkdir -p "):
            return "", "", 0
        if command.startswith("cd /tmp/ws-123 && python3 /tmp/ws-123/main.py"):
            fake_sftp.files["/tmp/ws-123/artifacts/chart.png"] = b"PNGDATA"
            payload = base64.b64encode(
                b'{"present":true,"value":{"message":"hello ssh"},"type":"json"}'
            ).decode("ascii")
            return f"debug line\n{RESULT_MARKER_PREFIX}{payload}\n", "", 0
        if command.startswith("rm -rf "):
            return "", "", 0
        raise AssertionError(f"Unexpected command: {command}")

    monkeypatch.setattr(provider, "_run_remote_command", _run_remote_command)

    instance = provider.create_instance("python")
    result = provider.execute_code(
        instance.instance_id,
        'def main() -> dict:\n    return {"message": "hello ssh"}\n',
        "python",
        timeout=5,
    )
    provider.destroy_instance(instance.instance_id)

    assert result.exit_code == 0
    assert result.stdout == "debug line\n"
    assert result.metadata["result_present"] is True
    assert result.metadata["result_value"] == {"message": "hello ssh"}
    assert result.metadata["artifacts"] == [
        {
            "name": "chart.png",
            "content_b64": base64.b64encode(b"PNGDATA").decode("ascii"),
            "mime_type": "image/png",
            "size": 7,
        }
    ]
    assert "cd /tmp/ws-123 && python3 /tmp/ws-123/main.py" in executed_commands
    assert fake_sftp.closed is True
    assert fake_client.closed is True


def test_ssh_provider_propagates_timeouts():
    provider = _build_provider()
    provider._instances["instance-1"] = {
        "client": object(),
        "sftp": _FakeSFTP(),
        "remote_work_dir": "/tmp/ws-123",
        "language": "python",
    }

    def _timeout(*args, **kwargs):
        raise TimeoutError("Execution timed out after 5 seconds")

    provider._run_remote_command = _timeout  # type: ignore[method-assign]

    with pytest.raises(TimeoutError, match="Execution timed out"):
        provider.execute_code(
            "instance-1",
            'def main() -> dict:\n    return {"ok": True}\n',
            "python",
            timeout=5,
        )
