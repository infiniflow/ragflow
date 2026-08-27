import base64
import posixpath
from types import SimpleNamespace

import pytest

from agent.sandbox.providers.base import SandboxProviderConfigError
from agent.sandbox.providers.ucloud_agent_sandbox import UCloudAgentSandboxProvider
from agent.sandbox.result_protocol import RESULT_MARKER_PREFIX

pytestmark = pytest.mark.p3


class _FakeFileType:
    FILE = "file"
    DIR = "dir"


class _AuthenticationException(Exception):
    pass


class _RateLimitException(Exception):
    pass


class _TimeoutException(Exception):
    pass


class _FileNotFoundException(Exception):
    pass


class _CommandExitException(Exception):
    def __init__(self, exit_code: int, stdout: str = "", stderr: str = "", error: str = ""):
        super().__init__(error or f"command exited with code {exit_code}")
        self.exit_code = exit_code
        self.stdout = stdout
        self.stderr = stderr
        self.error = error


class _FakeCommandResult:
    def __init__(self, exit_code: int = 0, stdout: str = "", stderr: str = ""):
        self.exit_code = exit_code
        self.stdout = stdout
        self.stderr = stderr


class _FakeFiles:
    def __init__(self):
        self.files: dict[str, bytes] = {}
        self.list_overrides: dict[str, list[SimpleNamespace]] = {}

    def write(self, path: str, data: str | bytes, request_timeout=None):
        self.files[path] = data.encode("utf-8") if isinstance(data, str) else data
        return SimpleNamespace(path=path)

    def read(self, path: str, format: str = "text", request_timeout=None):
        payload = self.files[path]
        if format == "bytes":
            return bytearray(payload)
        return payload.decode("utf-8")

    def list(self, path: str, depth: int = 1, request_timeout=None):
        if path in self.list_overrides:
            return self.list_overrides[path]

        prefix = path.rstrip("/") + "/"
        entries: list[SimpleNamespace] = []
        seen_dirs: set[str] = set()
        for file_path, payload in self.files.items():
            if not file_path.startswith(prefix):
                continue
            relative = file_path[len(prefix) :]
            head, _, tail = relative.partition("/")
            entry_path = posixpath.join(path, head)
            if tail:
                if head not in seen_dirs:
                    seen_dirs.add(head)
                    entries.append(
                        SimpleNamespace(
                            path=entry_path,
                            size=0,
                            type=_FakeFileType.DIR,
                            symlink_target=None,
                        )
                    )
                continue
            entries.append(
                SimpleNamespace(
                    path=entry_path,
                    size=len(payload),
                    type=_FakeFileType.FILE,
                    symlink_target=None,
                )
            )
        return entries


class _FakeCommands:
    def __init__(self, sandbox, run_handler=None):
        self._sandbox = sandbox
        self._run_handler = run_handler
        self.calls: list[dict] = []

    def run(self, cmd: str, cwd=None, timeout=None, request_timeout=None):
        self.calls.append(
            {
                "cmd": cmd,
                "cwd": cwd,
                "timeout": timeout,
                "request_timeout": request_timeout,
            }
        )
        if cmd.startswith("mkdir -p "):
            return _FakeCommandResult()
        if self._run_handler is not None:
            return self._run_handler(self._sandbox, cmd, cwd)
        return _FakeCommandResult()


class _FakeSandbox:
    def __init__(self, run_handler=None):
        self.sandbox_id = "sbx-fake-1"
        self.files = _FakeFiles()
        self.commands = _FakeCommands(self, run_handler)
        self.timeout_calls: list[tuple[int, int | None]] = []
        self.killed = False

    def set_timeout(self, timeout: int, request_timeout=None):
        self.timeout_calls.append((timeout, request_timeout))

    def kill(self, request_timeout=None):
        self.killed = True
        return True


class _FakeSDK:
    AuthenticationException = _AuthenticationException
    RateLimitException = _RateLimitException
    TimeoutException = _TimeoutException
    FileNotFoundException = _FileNotFoundException
    CommandExitException = _CommandExitException
    FileType = _FakeFileType

    def __init__(self, sandbox: _FakeSandbox):
        self._sandbox = sandbox
        self.create_kwargs: dict | None = None
        self.create_error: Exception | None = None
        self.Sandbox = SimpleNamespace(create=self._create)

    def _create(self, **kwargs):
        self.create_kwargs = kwargs
        if self.create_error is not None:
            raise self.create_error
        return self._sandbox


def _build_provider(sandbox: _FakeSandbox, monkeypatch, **overrides) -> tuple[UCloudAgentSandboxProvider, _FakeSDK]:
    sdk = _FakeSDK(sandbox)
    monkeypatch.setattr("agent.sandbox.providers.ucloud_agent_sandbox._get_ucloud_sandbox_module", lambda: sdk)
    config = {
        "api_key": "ucloud-test-key",
        "region": "cn-wlcb",
        "template": "base",
        "timeout": 30,
        "sandbox_timeout": 300,
        "max_output_bytes": 1024 * 1024,
        "max_artifacts": 20,
        "max_artifact_bytes": 1024 * 1024,
    }
    config.update(overrides)
    provider = UCloudAgentSandboxProvider()
    assert provider.initialize(config) is True
    return provider, sdk


def test_ucloud_agent_sandbox_executes_python_and_collects_artifacts(monkeypatch):
    def run_handler(sandbox, cmd, cwd):
        sandbox.files.files[posixpath.join(cwd, "artifacts", "chart.png")] = b"PNGDATA"
        payload = base64.b64encode(b'{"present":true,"value":{"message":"hello ucloud"},"type":"json"}').decode("ascii")
        return _FakeCommandResult(stdout=f"debug line\n{RESULT_MARKER_PREFIX}{payload}\n")

    sandbox = _FakeSandbox(run_handler)
    provider, sdk = _build_provider(sandbox, monkeypatch)

    instance = provider.create_instance("python")
    result = provider.execute_code(
        instance.instance_id,
        'def main() -> dict:\n    return {"message": "hello ucloud"}\n',
        "python",
        timeout=5,
    )
    provider.destroy_instance(instance.instance_id)

    assert instance.provider == "ucloud_agent_sandbox"
    assert instance.metadata["sandbox_id"] == "sbx-fake-1"
    assert sdk.create_kwargs["template"] == "base"
    assert sdk.create_kwargs["allow_internet_access"] is False
    assert sdk.create_kwargs["secure"] is True
    assert sdk.create_kwargs["domain"] == "cn-wlcb.sandbox.ucloudai.com"
    assert result.exit_code == 0
    assert result.stdout == "debug line\n"
    assert result.metadata["result_present"] is True
    assert result.metadata["result_value"] == {"message": "hello ucloud"}
    assert result.metadata["artifacts"] == [
        {
            "name": "chart.png",
            "content_b64": base64.b64encode(b"PNGDATA").decode("ascii"),
            "mime_type": "image/png",
            "size": 7,
        }
    ]
    assert sandbox.timeout_calls == [(300, 30)]
    assert sandbox.killed is True


def test_ucloud_agent_sandbox_preserves_nonzero_exit(monkeypatch):
    sandbox = _FakeSandbox()
    provider, sdk = _build_provider(sandbox, monkeypatch)

    def run_handler(sandbox, cmd, cwd):
        raise sdk.CommandExitException(7, stderr="boom\n", error="failed")

    sandbox.commands._run_handler = run_handler
    instance = provider.create_instance("python")
    result = provider.execute_code(instance.instance_id, "def main():\n    raise SystemExit(7)\n", "python", timeout=5)

    assert result.exit_code == 7
    assert result.stderr == "boom\n"
    assert result.metadata["status"] == "error"
    assert result.metadata["result_present"] is False


def test_ucloud_agent_sandbox_maps_timeout(monkeypatch):
    sandbox = _FakeSandbox()
    provider, sdk = _build_provider(sandbox, monkeypatch)

    def run_handler(sandbox, cmd, cwd):
        raise sdk.TimeoutException("deadline exceeded")

    sandbox.commands._run_handler = run_handler
    instance = provider.create_instance("python")
    with pytest.raises(TimeoutError, match="timed out after 5 seconds"):
        provider.execute_code(instance.instance_id, "def main():\n    return 1\n", "python", timeout=5)


def test_ucloud_agent_sandbox_maps_create_errors(monkeypatch):
    sandbox = _FakeSandbox()
    provider, sdk = _build_provider(sandbox, monkeypatch)

    sdk.create_error = sdk.AuthenticationException("bad key")
    with pytest.raises(SandboxProviderConfigError, match="authentication failed"):
        provider.create_instance("python")

    sdk.create_error = sdk.RateLimitException("slow down")
    with pytest.raises(RuntimeError, match="rate limited"):
        provider.create_instance("python")


def test_ucloud_agent_sandbox_executes_javascript(monkeypatch):
    def run_handler(sandbox, cmd, cwd):
        payload = base64.b64encode(b'{"present":true,"value":42,"type":"json"}').decode("ascii")
        return _FakeCommandResult(stdout=f"{RESULT_MARKER_PREFIX}{payload}\n")

    sandbox = _FakeSandbox(run_handler)
    provider, _ = _build_provider(sandbox, monkeypatch)
    instance = provider.create_instance("javascript")
    result = provider.execute_code(instance.instance_id, "function main(args) { return 42; }", "javascript", timeout=5)

    assert instance.metadata["language"] == "nodejs"
    assert any(path.endswith("main.js") for path in sandbox.files.files)
    assert any(call["cmd"].startswith("node ") for call in sandbox.commands.calls)
    assert result.metadata["result_value"] == 42


def test_ucloud_agent_sandbox_config_schema_and_validation():
    schema = UCloudAgentSandboxProvider.get_config_schema()
    assert schema["api_key"]["required"] is True
    assert schema["api_key"]["secret"] is True
    assert schema["template"]["default"] == "base"
    assert schema["allow_internet_access"]["default"] is False
    assert schema["region"]["default"] == "cn-wlcb"

    provider = UCloudAgentSandboxProvider()
    ok, message = provider.validate_config(
        {
            "api_key": "key",
            "template": "base",
            "timeout": 30,
            "sandbox_timeout": 300,
            "max_output_bytes": 1024,
            "max_artifacts": 5,
            "max_artifact_bytes": 1024,
        }
    )
    assert ok is True
    assert message is None

    ok, message = provider.validate_config({"api_key": ""})
    assert ok is False
    assert "API key" in message


def test_ucloud_agent_sandbox_enforces_output_limit(monkeypatch):
    sandbox = _FakeSandbox(lambda sandbox, cmd, cwd: _FakeCommandResult(stdout="x" * 5000))
    provider, _ = _build_provider(sandbox, monkeypatch, max_output_bytes=100)
    instance = provider.create_instance("python")

    with pytest.raises(RuntimeError, match="output exceeded"):
        provider.execute_code(instance.instance_id, "def main():\n    return 1\n", "python", timeout=5)


def test_ucloud_agent_sandbox_rejects_disallowed_artifact(monkeypatch):
    def run_handler(sandbox, cmd, cwd):
        sandbox.files.files[posixpath.join(cwd, "artifacts", "malware.exe")] = b"MZ"
        return _FakeCommandResult()

    sandbox = _FakeSandbox(run_handler)
    provider, _ = _build_provider(sandbox, monkeypatch)
    instance = provider.create_instance("python")

    with pytest.raises(RuntimeError, match="Unsupported artifact type"):
        provider.execute_code(instance.instance_id, "def main():\n    return 1\n", "python", timeout=5)


def test_ucloud_agent_sandbox_rejects_symlink_artifact(monkeypatch):
    def run_handler(sandbox, cmd, cwd):
        artifacts_dir = posixpath.join(cwd, "artifacts")
        sandbox.files.list_overrides[artifacts_dir] = [
            SimpleNamespace(
                path=posixpath.join(artifacts_dir, "evil.json"),
                size=10,
                type=_FakeFileType.FILE,
                symlink_target="/etc/passwd",
            )
        ]
        return _FakeCommandResult()

    sandbox = _FakeSandbox(run_handler)
    provider, _ = _build_provider(sandbox, monkeypatch)
    instance = provider.create_instance("python")

    with pytest.raises(RuntimeError, match="symlinks are not allowed"):
        provider.execute_code(instance.instance_id, "def main():\n    return 1\n", "python", timeout=5)


def test_ucloud_agent_sandbox_destroy_is_idempotent(monkeypatch):
    sandbox = _FakeSandbox()
    provider, _ = _build_provider(sandbox, monkeypatch)

    assert provider.destroy_instance("already-gone") is True


def test_ucloud_agent_sandbox_supported_languages():
    assert UCloudAgentSandboxProvider().get_supported_languages() == ["python", "javascript"]
