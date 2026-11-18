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

"""Service-free checks for the integration suite's resource boundaries."""

from types import SimpleNamespace

from test.testcases.test_sdk_api.test_dataset_mangement import conftest as fixtures
import pytest


@pytest.mark.parametrize("isolated", [None, "0", "1"])
def test_integration_requires_explicit_environment_before_network(monkeypatch, isolated):
    monkeypatch.delenv("RAGFLOW_SDK_BASE_URL", raising=False)
    monkeypatch.delenv("RAGFLOW_SDK_API_KEY", raising=False)
    if isolated is None:
        monkeypatch.delenv("RAGFLOW_SDK_ISOLATED", raising=False)
    else:
        monkeypatch.setenv("RAGFLOW_SDK_ISOLATED", isolated)

    def no_network(*args, **kwargs):
        pytest.fail("Missing integration prerequisites must not open a connection")

    monkeypatch.setattr(fixtures, "RAGFlow", no_network)
    with pytest.raises(pytest.fail.Exception, match="Integration environment required"):
        fixtures.integration_settings.__wrapped__()


def test_integration_refuses_nonempty_tenant(monkeypatch):
    monkeypatch.setenv("RAGFLOW_SDK_ISOLATED", "1")
    monkeypatch.setenv("RAGFLOW_SDK_BASE_URL", "http://example.invalid")
    monkeypatch.setenv("RAGFLOW_SDK_API_KEY", "test-only-key")
    monkeypatch.setattr(fixtures, "RAGFlow", lambda *args: SimpleNamespace(list_datasets=lambda **kwargs: [object()]))
    with pytest.raises(AssertionError, match="test tenant is not empty"):
        fixtures.integration_settings.__wrapped__()


def test_cleanup_only_deletes_created_survivors_after_failure(monkeypatch):
    datasets = {"unrelated": SimpleNamespace(id="unrelated")}
    deleted = []

    class FakeClient:
        def create_dataset(self, name):
            if name == "failure":
                raise RuntimeError("create failed")
            datasets[name] = SimpleNamespace(id=name)
            return datasets[name]

        def list_datasets(self, *, ids, page_size):
            assert page_size == 100
            return [datasets[key] for key in ids if key in datasets]

        def delete_datasets(self, *, ids):
            deleted.extend(ids)
            for key in ids:
                del datasets[key]

    monkeypatch.setattr(fixtures, "RAGFlow", lambda *args: FakeClient())
    fixture = fixtures.client.__wrapped__({"api_key": "test", "base_url": "http://example.invalid"}, monkeypatch)
    client = next(fixture)
    client.create_dataset("survivor")
    client.create_dataset("already-deleted")
    del datasets["already-deleted"]
    with pytest.raises(RuntimeError, match="create failed"):
        client.create_dataset("failure")
    with pytest.raises(StopIteration):
        next(fixture)
    assert deleted == ["survivor"]
    assert set(datasets) == {"unrelated"}


@pytest.fixture
def sdk_fixtures(monkeypatch):
    # Standalone contract runs do not load the ancestor's credential configuration.
    for key in ("ZHIPU_AI_API_KEY", "SILICONFLOW_API_KEY"):
        if not fixtures.os.getenv(key):
            monkeypatch.setenv(key, "test-only")
    from test.testcases.test_sdk_api import conftest

    return conftest


@pytest.mark.parametrize("engine", ["infinity", "elasticsearch"])
@pytest.mark.p2
def test_ci_provisions_owned_tenants_models_and_pipeline(monkeypatch, sdk_fixtures, engine):

    for key, value in {
        "RAGFLOW_SDK_CI": "1",
        "CI": "true",
        "HOST_ADDRESS": "http://example.invalid",
        "DOC_ENGINE": engine,
        "ZHIPU_AI_API_KEY": "test-chat-key",
        "SILICONFLOW_API_KEY": "test-rerank-key",
    }.items():
        monkeypatch.setenv(key, value)
    calls = []
    accounts = []
    sessions = []

    class Session:
        def __init__(self):
            self.headers = {}
            self.closed = False
            sessions.append(self)

        def request(self, method, url, *, json, timeout):
            assert timeout == 120
            path = url.removeprefix("http://example.invalid/api/v1")
            calls.append((method, path, json))
            data, headers = None, {}
            if path == "/users":
                accounts.append(json["email"])
                self.identity = str(len(accounts))
            elif path == "/auth/login":
                data, headers = {"id": self.identity}, {"Authorization": f"jwt-{self.identity}"}
            elif path == "/system/tokens":
                assert self.headers["Authorization"] == f"jwt-{self.identity}"
                data = {"token": f"key-{self.identity}"}
            elif path == "/pipelines/general":
                data = {"dsl": {"components": {}}}
            elif path == "/agents":
                data = {"id": "pipeline"}
            elif path == "/agents/pipeline/publish":
                data = {"id": "version"}
            return SimpleNamespace(status_code=200, headers=headers, json=lambda: {"code": 0, "data": data})

        def close(self):
            self.closed = True

    monkeypatch.setattr(sdk_fixtures.requests, "Session", Session)
    env = sdk_fixtures._prepare_ci_environment("test_dataset_mangement")
    assert len(set(accounts)) == 2
    assert all(email.startswith("sdk-") for email in accounts)
    assert env["RAGFLOW_SDK_ISOLATED"] == "1"
    assert env["RAGFLOW_SDK_API_KEY"] == "key-1"
    assert env["RAGFLOW_SDK_OTHER_API_KEY"] == "key-2"
    assert env["RAGFLOW_SDK_TENANT_ID"] != env["RAGFLOW_SDK_OTHER_TENANT_ID"]
    assert env["RAGFLOW_SDK_DOC_ENGINE"] == engine
    assert env["RAGFLOW_SDK_PIPELINE_ID"] == "pipeline"
    assert env["RAGFLOW_SDK_PIPELINE_VERSION_ID"] == "version"
    assert env["RAGFLOW_SDK_MEMORY_LLM_ID"] != env["RAGFLOW_SDK_MEMORY_UPDATE_LLM_ID"]
    assert [body["model_type"] for method, path, body in calls if path == "/models/default"] == ["chat", "embedding", "rerank"]
    assert all(session.closed for session in sessions)
    assert not any(method == "DELETE" for method, _, _ in calls)


@pytest.mark.parametrize("missing", ["RAGFLOW_SDK_CI", "HOST_ADDRESS", "DOC_ENGINE", "ZHIPU_AI_API_KEY", "SILICONFLOW_API_KEY"])
@pytest.mark.p2
def test_ci_setup_rejects_missing_prerequisites_before_network(monkeypatch, sdk_fixtures, missing):

    for key, value in {"RAGFLOW_SDK_CI": "1", "CI": "true", "HOST_ADDRESS": "http://example.invalid", "DOC_ENGINE": "infinity", "ZHIPU_AI_API_KEY": "test", "SILICONFLOW_API_KEY": "test"}.items():
        monkeypatch.setenv(key, value)
    monkeypatch.delenv(missing)
    monkeypatch.setattr(sdk_fixtures.requests, "Session", lambda: pytest.fail("Invalid CI setup must not open a connection"))
    with pytest.raises(RuntimeError, match="requires|require credentials"):
        sdk_fixtures._prepare_ci_environment("test_dataset_mangement")


@pytest.mark.p2
def test_ci_hook_reuses_folder_accounts_and_leaves_contract_tests_offline(monkeypatch, sdk_fixtures):
    from pathlib import Path

    monkeypatch.setenv("RAGFLOW_SDK_CI", "1")
    prepared = []

    def prepare(folder):
        prepared.append(folder)
        return {"RAGFLOW_SDK_API_KEY": folder}

    monkeypatch.setattr(sdk_fixtures, "_prepare_ci_environment", prepare)
    config = SimpleNamespace(_sdk_ci_environments={}, _sdk_ci_environment=monkeypatch)
    item = SimpleNamespace(path=Path("test_dataset_mangement/test_example.py"), config=config, get_closest_marker=lambda name: object() if name == "integration" else None)
    sdk_fixtures.pytest_runtest_setup(item)
    monkeypatch.setenv("RAGFLOW_SDK_API_KEY", "other-folder")
    sdk_fixtures.pytest_runtest_setup(item)
    assert prepared == ["test_dataset_mangement"]
    assert sdk_fixtures.os.environ["RAGFLOW_SDK_API_KEY"] == "test_dataset_mangement"
    item.path = Path("test_agent_management/test_example.py")
    sdk_fixtures.pytest_runtest_setup(item)
    assert prepared == ["test_dataset_mangement", "test_agent_management"]
    item.get_closest_marker = lambda name: None
    sdk_fixtures.pytest_runtest_setup(item)
    assert len(prepared) == 2


@pytest.mark.parametrize("http_status,code", [(500, 0), (200, 102)])
@pytest.mark.p2
def test_ci_setup_propagates_errors_without_response_secrets(monkeypatch, sdk_fixtures, http_status, code):

    session = SimpleNamespace(request=lambda *args, **kwargs: SimpleNamespace(status_code=http_status, headers={}, json=lambda: {"code": code, "message": "secret-provider-key"}))
    monkeypatch.setattr(sdk_fixtures.requests, "Session", lambda: session)
    with pytest.raises(RuntimeError, match="SDK environment setup failed") as error:
        sdk_fixtures._API("http://example.invalid").call("POST", "/providers/example/instances", {"api_key": "secret-provider-key"})
    assert "secret-provider-key" not in str(error.value)


@pytest.mark.p2
def test_ci_hook_stops_run_after_bootstrap_failure(monkeypatch, sdk_fixtures):
    from pathlib import Path

    monkeypatch.setenv("RAGFLOW_SDK_CI", "1")

    def fail_setup(folder):
        raise RuntimeError("SDK environment setup failed: POST /providers/example/instances")

    monkeypatch.setattr(sdk_fixtures, "_prepare_ci_environment", fail_setup)
    config = SimpleNamespace(_sdk_ci_environments={}, _sdk_ci_environment=monkeypatch)
    item = SimpleNamespace(path=Path("test_dataset_mangement/test_example.py"), config=config, get_closest_marker=lambda name: object() if name == "integration" else None)
    with pytest.raises(pytest.exit.Exception, match="SDK environment setup failed") as error:
        sdk_fixtures.pytest_runtest_setup(item)
    assert error.value.returncode == 1
    assert config._sdk_ci_environments == {}
