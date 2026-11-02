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

"""Service-free checks for the document suite's resource boundaries."""

from types import SimpleNamespace

from test.testcases.test_sdk_api.test_file_management_within_dataset import conftest as fixtures
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


@pytest.mark.parametrize("persist_before_failure", [False, True])
def test_cleanup_only_deletes_created_survivors_after_failure(monkeypatch, persist_before_failure):
    datasets = {"unrelated": SimpleNamespace(id="unrelated")}
    deleted = []
    failed_name = "sdk-document-" + "a" * 32

    class FakeClient:
        def create_dataset(self, name):
            if name == failed_name:
                if persist_before_failure:
                    datasets[name] = SimpleNamespace(id=name, name=name)
                raise RuntimeError("create failed")
            datasets[name] = SimpleNamespace(id=name, name=name)
            return datasets[name]

        def list_datasets(self, *, ids=None, name=None, page_size):
            assert page_size == 100
            if name is not None:
                return [dataset for dataset in datasets.values() if getattr(dataset, "name", None) == name]
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
        client.create_dataset(failed_name)
    with pytest.raises(StopIteration):
        next(fixture)
    assert deleted == ["survivor"] + ([failed_name] if persist_before_failure else [])
    assert set(datasets) == {"unrelated"}


@pytest.mark.parametrize("timeout", [None, 12])
def test_live_http_calls_have_bounded_timeouts(monkeypatch, timeout):
    calls = []

    def transport(session, *args, **kwargs):
        calls.append(kwargs)
        return object()

    monkeypatch.setattr(fixtures.requests.sessions.Session, "request", transport)
    request = SimpleNamespace(node=SimpleNamespace(get_closest_marker=lambda name: object()))
    fixtures.network_boundary.__wrapped__(request, monkeypatch)
    kwargs = {} if timeout is None else {"timeout": timeout}
    fixtures.requests.Session().request("GET", "http://example.invalid", **kwargs)
    assert calls == [{"timeout": (5, 60) if timeout is None else timeout}]


def test_service_free_network_boundary_rejects_connections(monkeypatch):
    request = SimpleNamespace(node=SimpleNamespace(get_closest_marker=lambda name: None))
    fixtures.network_boundary.__wrapped__(request, monkeypatch)
    with pytest.raises(pytest.fail.Exception, match="must not open a network connection"):
        fixtures.requests.Session().request("GET", "http://example.invalid")
