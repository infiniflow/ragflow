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
from inspect import unwrap
from types import SimpleNamespace

import pytest

from test.testcases.test_sdk_api.test_message_management import conftest
from test.testcases.test_sdk_api.test_message_management import message_helpers


def lifecycle(monkeypatch, *, failing_id=None, wrong_tenant=False):
    state = {"unrelated": "tenant"}
    deleted = []
    finalizers = []

    def create(name, **kwargs):
        if name == "setup-failure":
            raise RuntimeError("creation failed")
        state[name] = "other" if wrong_tenant else "tenant"
        return SimpleNamespace(id=name, tenant_id=state[name])

    def delete(memory_id):
        deleted.append(memory_id)
        if memory_id == failing_id:
            raise RuntimeError("cleanup failed")
        del state[memory_id]

    rag = SimpleNamespace(create_memory=create, delete_memory=delete, get=lambda path: SimpleNamespace(json=lambda: {"code": 0 if path.split("/")[2] in state else 404}))
    monkeypatch.setattr(conftest, "RAGFlow", lambda *args: rag)
    settings = {"api_key": "key", "base_url": "unused", "tenant_id": "tenant"}
    client = unwrap(conftest.client)(settings, monkeypatch, SimpleNamespace(addfinalizer=finalizers.append))
    assert len(finalizers) == 1
    return client, finalizers[0], state, deleted


def test_cleanup_only_owned_ids_after_partial_setup(monkeypatch):
    client, cleanup, state, deleted = lifecycle(monkeypatch)
    client.create_memory(name="first")
    client.create_memory(name="second")
    with pytest.raises(RuntimeError, match="creation failed"):
        client.create_memory(name="setup-failure")
    cleanup()
    assert deleted == ["second", "first"]
    assert state == {"unrelated": "tenant"}


def test_cleanup_registered_before_ownership_assertion(monkeypatch):
    client, cleanup, state, deleted = lifecycle(monkeypatch, wrong_tenant=True)
    with pytest.raises(AssertionError, match="unexpected tenant"):
        client.create_memory(name="created")
    cleanup()
    assert deleted == ["created"]
    assert state == {"unrelated": "tenant"}


def test_cleanup_continues_and_reports_errors(monkeypatch):
    client, cleanup, state, deleted = lifecycle(monkeypatch, failing_id="second")
    client.create_memory(name="first")
    client.create_memory(name="second")
    with pytest.raises(AssertionError, match="second: cleanup failed"):
        cleanup()
    assert deleted == ["second", "first"]
    assert state == {"unrelated": "tenant", "second": "tenant"}


def test_cleanup_accepts_already_deleted_owned_id(monkeypatch):
    client, cleanup, state, deleted = lifecycle(monkeypatch)
    client.create_memory(name="first")
    client.delete_memory("first")
    cleanup()
    assert deleted == ["first"]
    assert state == {"unrelated": "tenant"}


def test_polling_waits_for_condition(monkeypatch):
    results = iter([[], [{"message_id": 7}]])
    monkeypatch.setattr(message_helpers.time, "sleep", lambda _: None)
    assert message_helpers.wait_until(lambda: next(results), bool) == [{"message_id": 7}]


def test_polling_has_a_deadline(monkeypatch):
    ticks = iter([0, 1])
    monkeypatch.setattr(message_helpers.time, "monotonic", lambda: next(ticks))
    with pytest.raises(AssertionError, match=r"last result: \[\]"):
        message_helpers.wait_until(lambda: [], bool, timeout=0.1)


def test_polling_does_not_hide_api_errors():
    def fail():
        raise RuntimeError("model unavailable")

    with pytest.raises(RuntimeError, match="model unavailable"):
        message_helpers.wait_until(fail, bool)


def test_unverified_integration_environment_fails_before_network(monkeypatch):
    monkeypatch.delenv("RAGFLOW_SDK_ISOLATED", raising=False)
    with pytest.raises(pytest.fail.Exception, match="verified disposable Go backend"):
        unwrap(conftest.integration_settings)()


def test_nonempty_tenant_is_rejected_without_mutation(monkeypatch):
    def get(url, **kwargs):
        assert kwargs["timeout"] == 30
        return SimpleNamespace(
            raise_for_status=lambda: None,
            json=lambda: {
                "code": 0,
                "data": {"total_count": 1, "memory_list": ["unrelated"]},
            },
        )

    monkeypatch.setattr(conftest.requests, "get", get)
    with pytest.raises(AssertionError, match="tenant has accessible memories"):
        unwrap(conftest.isolated_tenant)({"api_key": "key", "base_url": "https://unused.invalid"})
