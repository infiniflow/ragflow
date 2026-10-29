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

from test.testcases.test_sdk_api.test_memory_management import conftest


def fake_client(monkeypatch, *, fail_cleanup=False):
    state = {"unrelated": {"id": "unrelated"}}
    deleted = []

    def create(name, **kwargs):
        if name == "setup-failure":
            raise RuntimeError("creation failed")
        state[name] = {"id": name, "tenant_id": "tenant"}
        return SimpleNamespace(**state[name])

    def delete(memory_id):
        deleted.append(memory_id)
        if fail_cleanup and memory_id == "first":
            raise RuntimeError("cleanup failed")
        del state[memory_id]

    rag = SimpleNamespace(
        create_memory=create,
        delete_memory=delete,
        get=lambda path: SimpleNamespace(json=lambda: {"code": 0 if path.split("/")[2] in state else 404}),
    )
    monkeypatch.setattr(conftest, "RAGFlow", lambda *args: rag)
    lifecycle = unwrap(getattr(conftest, "client"))({"api_key": "key", "base_url": "unused", "tenant_id": "tenant"}, monkeypatch)
    return next(lifecycle), lifecycle, state, deleted


def test_cleanup_owns_only_created_ids_after_partial_setup(monkeypatch):
    rag, lifecycle, state, deleted = fake_client(monkeypatch)
    rag.create_memory(name="first")
    rag.create_memory(name="second")
    with pytest.raises(RuntimeError, match="creation failed"):
        rag.create_memory(name="setup-failure")
    with pytest.raises(StopIteration):
        next(lifecycle)
    assert deleted == ["first", "second"]
    assert set(state) == {"unrelated"}


def test_cleanup_reports_failure_and_continues(monkeypatch):
    rag, lifecycle, state, deleted = fake_client(monkeypatch, fail_cleanup=True)
    rag.create_memory(name="first")
    rag.create_memory(name="second")
    with pytest.raises(AssertionError, match="first: cleanup failed"):
        next(lifecycle)
    assert deleted == ["first", "second"]
    assert set(state) == {"unrelated", "first"}


def test_cleanup_accepts_already_deleted_owned_memory(monkeypatch):
    rag, lifecycle, state, deleted = fake_client(monkeypatch)
    memory = rag.create_memory(name="first")
    rag.delete_memory(memory.id)
    with pytest.raises(StopIteration):
        next(lifecycle)
    assert deleted == ["first"]
    assert set(state) == {"unrelated"}
