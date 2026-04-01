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

import asyncio
import importlib.util
import sys
from copy import deepcopy
from pathlib import Path
from types import SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _co():
            return self._value

        return _co().__await__()


class _DummyKB:
    def __init__(self, kid="kb-1", embd_id="embd@factory", chunk_num=1, name="Dataset A", status="1"):
        self.id = kid
        self.embd_id = embd_id
        self.chunk_num = chunk_num
        self.name = name
        self.status = status


class _DummyDialogRecord:
    def __init__(self, data=None):
        self._data = data or {
            "id": "chat-1",
            "name": "chat-name",
            "description": "desc",
            "icon": "icon.png",
            "kb_ids": ["kb-1"],
            "llm_id": "glm-4",
            "llm_setting": {"temperature": 0.1},
            "prompt_config": {
                "system": "Answer with {knowledge}",
                "parameters": [{"key": "knowledge", "optional": False}],
                "prologue": "hello",
                "quote": True,
            },
            "similarity_threshold": 0.2,
            "vector_similarity_weight": 0.3,
            "top_n": 6,
            "top_k": 1024,
            "rerank_id": "",
            "meta_data_filter": {},
            "tenant_id": "tenant-1",
        }

    def to_dict(self):
        return deepcopy(self._data)


def _run(coro):
    return asyncio.run(coro)


def _load_chat_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]
    module_name = "test_chat_restful_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "chat_api.py"

    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    monkeypatch.setattr(module, "current_user", SimpleNamespace(id="tenant-1"))
    return module


def _set_request_json(monkeypatch, module, payload):
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(deepcopy(payload)))


@pytest.mark.p2
def test_create_chat_uses_direct_chat_fields(monkeypatch):
    module = _load_chat_module(monkeypatch)
    saved = {}

    _set_request_json(
        monkeypatch,
        module,
        {
            "name": "chat-a",
            "icon": "icon.png",
            "dataset_ids": ["kb-1"],
            "llm_id": "glm-4",
            "llm_setting": {"temperature": 0.8},
            "prompt_config": {
                "system": "Answer with {knowledge}",
                "parameters": [{"key": "knowledge", "optional": False}],
                "prologue": "Hi",
            },
            "vector_similarity_weight": 0.25,
        },
    )
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: [SimpleNamespace(id="kb-1")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [_DummyKB()])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, _DummyKB()))
    monkeypatch.setattr(module.TenantLLMService, "split_model_name_and_factory", lambda model: (model.split("@")[0], "factory"))
    monkeypatch.setattr(module.TenantLLMService, "query", lambda **_kwargs: [SimpleNamespace(id="llm-1")])

    def _save(**kwargs):
        saved.update(kwargs)
        return True

    monkeypatch.setattr(module.DialogService, "save", _save)
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(saved)))

    res = _run(module.create.__wrapped__())

    assert res["code"] == 0
    assert saved["kb_ids"] == ["kb-1"]
    assert saved["prompt_config"]["prologue"] == "Hi"
    assert saved["llm_id"] == "glm-4"
    assert saved["llm_setting"]["temperature"] == 0.8
    assert res["data"]["dataset_ids"] == ["kb-1"]
    assert res["data"]["kb_names"] == ["Dataset A"]
    assert "kb_ids" not in res["data"]
    assert "prompt" not in res["data"]
    assert "llm" not in res["data"]
    assert "avatar" not in res["data"]


@pytest.mark.p2
def test_create_chat_blank_name_is_treated_as_missing(monkeypatch):
    module = _load_chat_module(monkeypatch)

    _set_request_json(
        monkeypatch,
        module,
        {
            "name": "   ",
            "dataset_ids": [],
        },
    )
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))

    res = _run(module.create.__wrapped__())

    assert res["code"] == 102
    assert res["message"] == "`name` is required."


@pytest.mark.p1
def test_create_chat_accepts_provider_scoped_rerank_id(monkeypatch):
    module = _load_chat_module(monkeypatch)
    saved = {}
    query_calls = []

    _set_request_json(
        monkeypatch,
        module,
        {
            "name": "chat-a",
            "icon": "icon.png",
            "dataset_ids": ["kb-1"],
            "llm_id": "glm-4@ZHIPU-AI",
            "llm_setting": {"temperature": 0.8},
            "prompt_config": {
                "system": "Answer with {knowledge}",
                "parameters": [{"key": "knowledge", "optional": False}],
                "prologue": "Hi",
            },
            "rerank_id": "custom-reranker@OpenAI",
            "vector_similarity_weight": 0.25,
        },
    )
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4@ZHIPU-AI")))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: [SimpleNamespace(id="kb-1")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [_DummyKB()])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, _DummyKB()))

    def _split_model_name_and_factory(model_name):
        return {
            "glm-4@ZHIPU-AI": ("glm-4", "ZHIPU-AI"),
            "custom-reranker@OpenAI": ("custom-reranker", "OpenAI"),
        }.get(model_name, (model_name, None))

    def _query(**kwargs):
        query_calls.append(kwargs)
        if kwargs == {
            "tenant_id": "tenant-1",
            "llm_name": "glm-4",
            "llm_factory": "ZHIPU-AI",
            "model_type": "chat",
        }:
            return [SimpleNamespace(id="llm-1")]
        if kwargs == {
            "tenant_id": "tenant-1",
            "llm_name": "custom-reranker",
            "llm_factory": "OpenAI",
            "model_type": "rerank",
        }:
            return [SimpleNamespace(id="rerank-1")]
        return []

    monkeypatch.setattr(module.TenantLLMService, "split_model_name_and_factory", _split_model_name_and_factory)
    monkeypatch.setattr(module.TenantLLMService, "query", _query)

    def _save(**kwargs):
        saved.update(kwargs)
        return True

    monkeypatch.setattr(module.DialogService, "save", _save)
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(saved)))

    res = _run(module.create.__wrapped__())

    assert res["code"] == 0
    assert saved["rerank_id"] == "custom-reranker@OpenAI"
    assert {
        "tenant_id": "tenant-1",
        "llm_name": "custom-reranker",
        "llm_factory": "OpenAI",
        "model_type": "rerank",
    } in query_calls


@pytest.mark.p1
def test_create_chat_allows_default_knowledge_placeholder_without_sources(monkeypatch):
    module = _load_chat_module(monkeypatch)
    saved = {}

    _set_request_json(monkeypatch, module, {"name": "chat-a"})
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module.TenantLLMService, "get_api_key", lambda *_args, **_kwargs: SimpleNamespace(id=1))

    def _save(**kwargs):
        saved.update(kwargs)
        return True

    monkeypatch.setattr(module.DialogService, "save", _save)
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(saved)))

    res = _run(module.create.__wrapped__())

    assert res["code"] == 0
    assert saved["kb_ids"] == []
    assert saved["prompt_config"]["system"].find("{knowledge}") >= 0
    assert saved["prompt_config"]["parameters"] == [{"key": "knowledge", "optional": False}]


@pytest.mark.p1
def test_create_chat_uses_tenant_default_llm_when_llm_id_is_null(monkeypatch):
    module = _load_chat_module(monkeypatch)
    saved = {}

    _set_request_json(
        monkeypatch,
        module,
        {
            "name": "chat-a",
            "dataset_ids": ["kb-1"],
            "llm_id": None,
            "llm_setting": {"temperature": 0.8},
            "prompt_config": {
                "system": "Answer with {knowledge}",
                "parameters": [{"key": "knowledge", "optional": False}],
            },
        },
    )
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: [SimpleNamespace(id="kb-1")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [_DummyKB()])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, _DummyKB()))
    monkeypatch.setattr(module.TenantLLMService, "get_api_key", lambda *_args, **_kwargs: SimpleNamespace(id=1))

    def _save(**kwargs):
        saved.update(kwargs)
        return True

    monkeypatch.setattr(module.DialogService, "save", _save)
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(saved)))

    res = _run(module.create.__wrapped__())

    assert res["code"] == 0
    assert saved["llm_id"] == "glm-4"
    assert saved["llm_setting"]["temperature"] == 0.8


@pytest.mark.p2
def test_patch_chat_merges_prompt_and_llm_settings(monkeypatch):
    module = _load_chat_module(monkeypatch)
    updated = {}
    existing = _DummyDialogRecord().to_dict()

    _set_request_json(
        monkeypatch,
        module,
        {
            "prompt_config": {"prologue": "updated opener"},
            "llm_setting": {"temperature": 0.9},
        },
    )
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(existing)))
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))

    def _update(_chat_id, payload):
        updated.update(payload)
        return True

    monkeypatch.setattr(module.DialogService, "update_by_id", _update)

    res = _run(module.patch_chat.__wrapped__("chat-1"))

    assert res["code"] == 0
    assert updated["prompt_config"]["system"] == "Answer with {knowledge}"
    assert updated["prompt_config"]["prologue"] == "updated opener"
    assert updated["llm_setting"]["temperature"] == 0.9


@pytest.mark.p2
def test_patch_chat_drops_response_only_fields_before_update(monkeypatch):
    module = _load_chat_module(monkeypatch)
    updated = {}
    existing = _DummyDialogRecord().to_dict()
    payload = {
        "name": "renamed-chat",
        "description": existing["description"],
        "icon": existing["icon"],
        "dataset_ids": existing["kb_ids"],
        "kb_names": ["Dataset A"],
        "llm_id": existing["llm_id"],
        "llm_setting": existing["llm_setting"],
        "prompt_config": existing["prompt_config"],
        "similarity_threshold": existing["similarity_threshold"],
        "vector_similarity_weight": existing["vector_similarity_weight"],
        "top_n": existing["top_n"],
        "top_k": existing["top_k"],
        "rerank_id": existing["rerank_id"],
    }

    _set_request_json(monkeypatch, module, payload)
    monkeypatch.setattr(
        module.DialogService,
        "query",
        lambda **kwargs: [] if "name" in kwargs else [SimpleNamespace(id="chat-1")],
    )
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(existing)))
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: [SimpleNamespace(id="kb-1")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [_DummyKB()])
    monkeypatch.setattr(module.TenantLLMService, "split_model_name_and_factory", lambda model: (model.split("@")[0], "factory"))
    monkeypatch.setattr(module.TenantLLMService, "query", lambda **_kwargs: [SimpleNamespace(id="llm-1")])

    def _update(_chat_id, req):
        updated.update(req)
        return True

    monkeypatch.setattr(module.DialogService, "update_by_id", _update)

    res = _run(module.patch_chat.__wrapped__("chat-1"))

    assert res["code"] == 0
    assert updated["name"] == "renamed-chat"
    assert "kb_names" not in updated


@pytest.mark.p2
def test_update_chat_rejects_knowledge_placeholder_without_sources(monkeypatch):
    module = _load_chat_module(monkeypatch)
    existing = _DummyDialogRecord().to_dict()

    _set_request_json(
        monkeypatch,
        module,
        {
            "name": "chat-name",
            "description": "desc",
            "icon": "icon.png",
            "dataset_ids": [],
            "llm_id": "glm-4",
            "llm_setting": {"temperature": 0.1},
            "prompt_config": {
                "system": "Answer with {knowledge}",
                "parameters": [{"key": "knowledge", "optional": False}],
                "prologue": "hello",
                "quote": True,
            },
            "similarity_threshold": 0.2,
            "vector_similarity_weight": 0.3,
            "top_n": 6,
            "top_k": 1024,
            "rerank_id": "",
        },
    )
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord(existing)))
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))
    monkeypatch.setattr(module.TenantLLMService, "split_model_name_and_factory", lambda model: (model.split("@")[0], "factory"))
    monkeypatch.setattr(module.TenantLLMService, "query", lambda **_kwargs: [SimpleNamespace(id="llm-1")])

    res = _run(module.update_chat.__wrapped__("chat-1"))

    assert res["code"] == 102
    assert res["message"] == "Please remove `{knowledge}` in system prompt since no dataset / Tavily used here."


@pytest.mark.p2
def test_list_chats_returns_old_business_fields(monkeypatch):
    module = _load_chat_module(monkeypatch)
    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(
            args=SimpleNamespace(
                get=lambda key, default=None: {
                    "keywords": "",
                    "page": 1,
                    "page_size": 20,
                    "orderby": "create_time",
                    "desc": "true",
                }.get(key, default),
                getlist=lambda _key: [],
            )
        ),
    )
    monkeypatch.setattr(
        module.DialogService,
        "get_by_tenant_ids",
        lambda *_args, **_kwargs: (
            [_DummyDialogRecord().to_dict()],
            1,
        ),
    )
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, _DummyKB()))

    res = module.list_chats.__wrapped__()

    assert res["code"] == 0
    chat = res["data"]["chats"][0]
    assert chat["icon"] == "icon.png"
    assert chat["dataset_ids"] == ["kb-1"]
    assert chat["kb_names"] == ["Dataset A"]
    assert "kb_ids" not in chat
    assert chat["prompt_config"]["prologue"] == "hello"
    assert "dataset_names" not in chat
    assert "prompt" not in chat
    assert "llm" not in chat


@pytest.mark.p2
def test_list_chats_keeps_zero_pagination_semantics(monkeypatch):
    module = _load_chat_module(monkeypatch)
    calls = []

    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(
            args=SimpleNamespace(
                get=lambda key, default=None: {
                    "keywords": "",
                    "page": 0,
                    "page_size": 0,
                    "orderby": "create_time",
                    "desc": "true",
                }.get(key, default),
                getlist=lambda _key: [],
            )
        ),
    )

    def _get_by_tenant_ids(_owner_ids, _user_id, page_number, items_per_page, *_args, **_kwargs):
        calls.append((page_number, items_per_page))
        return ([_DummyDialogRecord().to_dict()], 1)

    monkeypatch.setattr(module.DialogService, "get_by_tenant_ids", _get_by_tenant_ids)
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _id: (True, _DummyKB()))

    res = module.list_chats.__wrapped__()

    assert res["code"] == 0
    assert calls[-1] == (0, 0)
    assert len(res["data"]["chats"]) == 1

    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(
            args=SimpleNamespace(
                get=lambda key, default=None: {
                    "keywords": "",
                    "page": 0,
                    "page_size": 2,
                    "orderby": "create_time",
                    "desc": "true",
                }.get(key, default),
                getlist=lambda _key: [],
            )
        ),
    )

    res = module.list_chats.__wrapped__()

    assert res["code"] == 0
    assert calls[-1] == (0, 2)
    assert len(res["data"]["chats"]) == 1

    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(
            args=SimpleNamespace(
                get=lambda key, default=None: {
                    "keywords": "",
                    "page_size": 2,
                    "orderby": "create_time",
                    "desc": "true",
                }.get(key, default),
                getlist=lambda _key: [],
            )
        ),
    )

    res = module.list_chats.__wrapped__()

    assert res["code"] == 0
    assert calls[-1] == (0, 2)
    assert len(res["data"]["chats"]) == 1
