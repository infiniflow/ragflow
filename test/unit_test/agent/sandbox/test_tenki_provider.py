import base64
import posixpath
from types import SimpleNamespace

import pytest

from agent.sandbox.providers.base import SandboxProviderConfigError
from agent.sandbox.providers.tenki import TenkiProvider
from agent.sandbox.result_protocol import RESULT_MARKER_PREFIX

pytestmark = pytest.mark.p3


class _FakeErrors:
    """Stand-in for tenki_sandbox.errors so tests need not install the SDK."""

    class QuotaExceededError(Exception):
        pass

    class RateLimitedError(Exception):
        pass

    class UnauthorizedError(Exception):
        pass

    class CommandTimeoutError(Exception):
        pass

    class PrimitiveTimeoutError(Exception):
        pass

    class FileNotFoundError(Exception):
        pass


class _FakeFS:
    def __init__(self):
        self.files: dict[str, bytes] = {}

    def write_text(self, path: str, content: str):
        self.files[path] = content.encode("utf-8")

    def read_bytes(self, path: str) -> bytes:
        return self.files[path]

    def list(self, path: str, *, include_hidden: bool = False):
        # Mirror the real SDK: entry.path is the basename within `path`,
        # not an absolute path.
        prefix = path.rstrip("/") + "/"
        entries = []
        seen_dirs: set[str] = set()
        for file_path, payload in self.files.items():
            if not file_path.startswith(prefix):
                continue
            relative = file_path[len(prefix):]
            head, _, tail = relative.partition("/")
            if tail:  # nested -> surface the intermediate directory once
                if head not in seen_dirs:
                    seen_dirs.add(head)
                    entries.append(SimpleNamespace(path=head, size=4096, mode=0o40755, is_dir=True, is_symlink=False, symlink_target=""))
                continue
            entries.append(
                SimpleNamespace(
                    path=head,
                    size=len(payload),
                    mode=0o100644,
                    is_dir=False,
                    is_symlink=False,
                    symlink_target="",
                )
            )
        return entries


class _FakeCommandResult:
    def __init__(self, exit_code: int, stdout: str = "", stderr: str = ""):
        self.exit_code = exit_code
        self.stdout_text = stdout
        self.stderr_text = stderr
        self.stdout = stdout.encode("utf-8")
        self.stderr = stderr.encode("utf-8")


class _FakeSandbox:
    def __init__(self, run_handler=None):
        self.id = "sbx-fake-1"
        self.state = "RUNNING"
        self.fs = _FakeFS()
        self.terminated = False
        self._run_handler = run_handler
        self.exec_calls: list[tuple] = []

    def exec(self, *argv, cwd=None, timeout=None, env=None, input=None, check=False, privileged=False):
        self.exec_calls.append((argv, cwd, timeout))
        if argv and argv[0] == "mkdir":
            return _FakeCommandResult(0)
        if self._run_handler is not None:
            return self._run_handler(self, argv, cwd)
        return _FakeCommandResult(0)

    def terminate(self):
        self.terminated = True


class _FakeClient:
    def __init__(self, sandbox: _FakeSandbox):
        self._sandbox = sandbox
        self.create_kwargs = None

    def who_am_i(self):
        return SimpleNamespace(owner_type="WORKSPACE")

    def create(self, **kwargs):
        self.create_kwargs = kwargs
        return self._sandbox


def _build_provider(sandbox: _FakeSandbox, monkeypatch) -> tuple[TenkiProvider, _FakeClient]:
    provider = TenkiProvider()
    provider.api_key = "tk_test"
    provider.project_id = "proj-1"
    provider.timeout = 30
    provider.max_output_bytes = 1024 * 1024
    provider.max_artifacts = 20
    provider.max_artifact_bytes = 1024 * 1024
    provider._initialized = True
    client = _FakeClient(sandbox)
    provider._client = client
    monkeypatch.setattr(provider, "_tenki_errors", lambda: _FakeErrors)
    return provider, client


def test_tenki_provider_executes_python_main_and_collects_artifacts(monkeypatch):
    def run_handler(sandbox, argv, cwd):
        # Simulate the script run: emit a structured result + write an artifact.
        sandbox.fs.files[posixpath.join(cwd, "artifacts", "chart.png")] = b"PNGDATA"
        payload = base64.b64encode(
            b'{"present":true,"value":{"message":"hello tenki"},"type":"json"}'
        ).decode("ascii")
        return _FakeCommandResult(0, stdout=f"debug line\n{RESULT_MARKER_PREFIX}{payload}\n")

    sandbox = _FakeSandbox(run_handler)
    provider, _ = _build_provider(sandbox, monkeypatch)

    instance = provider.create_instance("python")
    result = provider.execute_code(
        instance.instance_id,
        'def main() -> dict:\n    return {"message": "hello tenki"}\n',
        "python",
        timeout=5,
    )
    provider.destroy_instance(instance.instance_id)

    assert result.exit_code == 0
    assert result.stdout == "debug line\n"
    assert result.metadata["result_present"] is True
    assert result.metadata["result_value"] == {"message": "hello tenki"}
    assert result.metadata["artifacts"] == [
        {
            "name": "chart.png",
            "content_b64": base64.b64encode(b"PNGDATA").decode("ascii"),
            "mime_type": "image/png",
            "size": 7,
        }
    ]
    assert sandbox.terminated is True


def test_tenki_provider_reports_nonzero_exit(monkeypatch):
    def run_handler(sandbox, argv, cwd):
        return _FakeCommandResult(7, stdout="", stderr="boom\n")

    sandbox = _FakeSandbox(run_handler)
    provider, _ = _build_provider(sandbox, monkeypatch)

    instance = provider.create_instance("python")
    result = provider.execute_code(instance.instance_id, "def main():\n    raise SystemExit(7)\n", "python", timeout=5)

    assert result.exit_code == 7
    assert result.stderr == "boom\n"
    assert result.metadata["status"] == "error"
    assert result.metadata["result_present"] is False


def test_tenki_provider_propagates_timeout(monkeypatch):
    def run_handler(sandbox, argv, cwd):
        raise _FakeErrors.CommandTimeoutError("timed out")

    sandbox = _FakeSandbox(run_handler)
    provider, _ = _build_provider(sandbox, monkeypatch)

    instance = provider.create_instance("python")
    with pytest.raises(TimeoutError, match="timed out after"):
        provider.execute_code(instance.instance_id, "def main():\n    return 1\n", "python", timeout=5)


def test_tenki_provider_create_maps_quota_error(monkeypatch):
    sandbox = _FakeSandbox()
    provider, client = _build_provider(sandbox, monkeypatch)

    def _raise_quota(**kwargs):
        raise _FakeErrors.QuotaExceededError("no credits")

    client.create = _raise_quota
    with pytest.raises(RuntimeError, match="quota exceeded"):
        provider.create_instance("python")


def test_tenki_provider_destroy_is_idempotent(monkeypatch):
    sandbox = _FakeSandbox()
    provider, _ = _build_provider(sandbox, monkeypatch)
    # Destroying an unknown instance returns True without error.
    assert provider.destroy_instance("nonexistent") is True


def test_tenki_provider_create_passes_project_and_outbound(monkeypatch):
    sandbox = _FakeSandbox()
    provider, client = _build_provider(sandbox, monkeypatch)
    provider.image = "my-image"
    provider.cpu_cores = 4

    provider.create_instance("python")

    assert client.create_kwargs["project_id"] == "proj-1"
    assert client.create_kwargs["allow_outbound"] is True
    assert client.create_kwargs["image"] == "my-image"
    assert client.create_kwargs["cpu_cores"] == 4


def test_tenki_provider_config_schema_and_validation():
    schema = TenkiProvider.get_config_schema()
    assert schema["api_key"]["required"] is True
    assert schema["api_key"]["secret"] is True
    assert schema["project_id"]["required"] is True

    provider = TenkiProvider()
    ok, _ = provider.validate_config({"api_key": "tk", "project_id": "p", "timeout": 30, "max_lifetime": 3600, "max_output_bytes": 1024, "max_artifacts": 5, "max_artifact_bytes": 1024})
    assert ok is True
    bad, msg = provider.validate_config({"api_key": "", "project_id": "p"})
    assert bad is False
    assert "API key" in msg


def test_tenki_provider_supported_languages():
    assert TenkiProvider().get_supported_languages() == ["python", "javascript", "nodejs"]


def test_tenki_provider_initialize_maps_auth_error(monkeypatch):
    provider = TenkiProvider()
    monkeypatch.setattr(provider, "_tenki_errors", lambda: _FakeErrors)

    class _UnauthorizedClient:
        def who_am_i(self):
            raise _FakeErrors.UnauthorizedError("bad token")

    monkeypatch.setattr(provider, "_create_client", lambda: _UnauthorizedClient())

    with pytest.raises(SandboxProviderConfigError, match="authentication failed"):
        provider.initialize({"api_key": "tk_bad", "project_id": "proj-1"})


def test_tenki_provider_instances_are_independent(monkeypatch):
    sandboxes: list[_FakeSandbox] = []

    class _MultiClient(_FakeClient):
        def create(self, **kwargs):
            sb = _FakeSandbox()
            sb.id = f"sbx-{len(sandboxes)}"
            sandboxes.append(sb)
            return sb

    provider = TenkiProvider()
    provider.api_key = "tk_test"
    provider.project_id = "proj-1"
    provider.timeout = 30
    provider._initialized = True
    provider._client = _MultiClient(None)
    monkeypatch.setattr(provider, "_tenki_errors", lambda: _FakeErrors)

    a = provider.create_instance("python")
    b = provider.create_instance("python")

    assert a.instance_id != b.instance_id
    assert a.metadata["sandbox_id"] != b.metadata["sandbox_id"]

    provider.destroy_instance(a.instance_id)
    assert sandboxes[0].terminated is True
    assert sandboxes[1].terminated is False  # destroying one leaves the other running

    provider.destroy_instance(b.instance_id)
    assert sandboxes[1].terminated is True
