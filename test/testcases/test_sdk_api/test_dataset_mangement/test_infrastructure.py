"""Service-free checks for the integration suite's resource boundaries."""

from types import SimpleNamespace

import conftest as fixtures
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
