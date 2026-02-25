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
from types import ModuleType, SimpleNamespace

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
    def __init__(self, embd_id="embd@factory", chunk_num=1):
        self.embd_id = embd_id
        self.chunk_num = chunk_num

    def to_json(self):
        return {"id": "kb-1"}


class _DummyDialogRecord:
    def __init__(self):
        self._data = {
            "id": "chat-1",
            "name": "chat-name",
            "prompt_config": {
                "system": "Answer with {knowledge}",
                "parameters": [{"key": "knowledge", "optional": False}],
                "prologue": "hello",
                "quote": True,
            },
            "llm_setting": {"temperature": 0.1},
            "llm_id": "glm-4",
            "similarity_threshold": 0.2,
            "vector_similarity_weight": 0.3,
            "top_n": 6,
            "rerank_id": "",
            "top_k": 1024,
            "kb_ids": ["kb-1"],
            "icon": "icon.png",
        }

    def to_json(self):
        return deepcopy(self._data)


def _run(coro):
    return asyncio.run(coro)


def _load_chat_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    deepdoc_pkg = ModuleType("deepdoc")
    deepdoc_parser_pkg = ModuleType("deepdoc.parser")
    deepdoc_parser_pkg.__path__ = []

    class _StubPdfParser:
        pass

    class _StubExcelParser:
        pass

    class _StubDocxParser:
        pass

    deepdoc_parser_pkg.PdfParser = _StubPdfParser
    deepdoc_parser_pkg.ExcelParser = _StubExcelParser
    deepdoc_parser_pkg.DocxParser = _StubDocxParser
    deepdoc_pkg.parser = deepdoc_parser_pkg
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser", deepdoc_parser_pkg)

    deepdoc_excel_module = ModuleType("deepdoc.parser.excel_parser")
    deepdoc_excel_module.RAGFlowExcelParser = _StubExcelParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.excel_parser", deepdoc_excel_module)

    deepdoc_parser_utils = ModuleType("deepdoc.parser.utils")
    deepdoc_parser_utils.get_text = lambda *_args, **_kwargs: ""
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", deepdoc_parser_utils)
    monkeypatch.setitem(sys.modules, "xgboost", ModuleType("xgboost"))

    module_name = "test_chat_sdk_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "sdk" / "chat.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _set_request_json(monkeypatch, module, payload):
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(deepcopy(payload)))


@pytest.mark.p2
def test_create_internal_failure_paths(monkeypatch):
    module = _load_chat_module(monkeypatch)

    _set_request_json(monkeypatch, module, {"name": "chat-a", "dataset_ids": ["kb-1", "kb-2"]})
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: [SimpleNamespace(id="kb")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [_DummyKB(chunk_num=1)])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_ids", lambda _ids: [_DummyKB(embd_id="embd-a@x"), _DummyKB(embd_id="embd-b@y")])
    monkeypatch.setattr(module.TenantLLMService, "split_model_name_and_factory", lambda model: (model.split("@")[0], "factory"))
    res = _run(module.create.__wrapped__("tenant-1"))
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert "different embedding models" in res["message"]

    _set_request_json(monkeypatch, module, {"name": "chat-a", "dataset_ids": []})
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (False, None))
    res = _run(module.create.__wrapped__("tenant-1"))
    assert res["message"] == "Tenant not found!"

    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    monkeypatch.setattr(module.DialogService, "save", lambda **_kwargs: False)
    res = _run(module.create.__wrapped__("tenant-1"))
    assert res["message"] == "Fail to new a chat!"

    monkeypatch.setattr(module.DialogService, "save", lambda **_kwargs: True)
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (False, None))
    res = _run(module.create.__wrapped__("tenant-1"))
    assert res["message"] == "Fail to new a chat!"

    _set_request_json(
        monkeypatch,
        module,
        {"name": "chat-rerank", "dataset_ids": [], "prompt": {"rerank_model": "unknown-rerank-model"}},
    )
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(llm_id="glm-4")))
    rerank_query_calls = []

    def _mock_tenant_llm_query(**kwargs):
        rerank_query_calls.append(kwargs)
        return False

    monkeypatch.setattr(module.TenantLLMService, "query", _mock_tenant_llm_query)
    res = _run(module.create.__wrapped__("tenant-1"))
    assert "`rerank_model` unknown-rerank-model doesn't exist" in res["message"]
    assert rerank_query_calls[-1]["model_type"] == "rerank"
    assert rerank_query_calls[-1]["llm_name"] == "unknown-rerank-model"

    _set_request_json(monkeypatch, module, {"name": "chat-tenant", "dataset_ids": [], "tenant_id": "tenant-forbidden"})
    res = _run(module.create.__wrapped__("tenant-1"))
    assert res["message"] == "`tenant_id` must not be provided."


@pytest.mark.p2
def test_update_internal_failure_paths(monkeypatch):
    module = _load_chat_module(monkeypatch)

    _set_request_json(monkeypatch, module, {"name": "anything"})
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert res["message"] == "You do not own the chat"

    _set_request_json(monkeypatch, module, {"name": "chat-name"})
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (False, None))
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert res["message"] == "Tenant not found!"

    _set_request_json(monkeypatch, module, {"dataset_ids": ["kb-1", "kb-2"]})
    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(id="tenant-1")))
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: [SimpleNamespace(id="kb")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [_DummyKB(chunk_num=1)])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_ids", lambda _ids: [_DummyKB(embd_id="embd-a@x"), _DummyKB(embd_id="embd-b@y")])
    monkeypatch.setattr(module.TenantLLMService, "split_model_name_and_factory", lambda model: (model.split("@")[0], "factory"))
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert res["code"] == module.RetCode.AUTHENTICATION_ERROR
    assert "different embedding models" in res["message"]

    _set_request_json(monkeypatch, module, {"avatar": "new-avatar"})
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord()))
    monkeypatch.setattr(module.DialogService, "update_by_id", lambda *_args, **_kwargs: False)
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert res["message"] == "Chat not found!"

    monkeypatch.setattr(module.TenantService, "get_by_id", lambda _tid: (True, SimpleNamespace(id="tenant-1")))
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, _DummyDialogRecord()))
    monkeypatch.setattr(module.DialogService, "update_by_id", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(
        module.DialogService,
        "query",
        lambda **kwargs: (
            [SimpleNamespace(id="chat-1")]
            if kwargs.get("id") == "chat-1"
            else ([SimpleNamespace(id="dup")] if kwargs.get("name") == "dup-name" else [])
        ),
    )
    monkeypatch.setattr(
        module.TenantLLMService,
        "split_model_name_and_factory",
        lambda model: (model.split("@")[0], "factory"),
    )
    monkeypatch.setattr(
        module.TenantLLMService,
        "query",
        lambda **kwargs: kwargs.get("llm_name") in {"glm-4", "allowed-rerank"},
    )

    _set_request_json(monkeypatch, module, {"show_quotation": True})
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert res["code"] == 0

    _set_request_json(monkeypatch, module, {"dataset_ids": ["kb-no-owner"]})
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: [])
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert "You don't own the dataset kb-no-owner" in res["message"]

    _set_request_json(monkeypatch, module, {"dataset_ids": ["kb-unparsed"]})
    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda **_kwargs: [SimpleNamespace(id="kb-unparsed")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [_DummyKB(chunk_num=0)])
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert "doesn't own parsed file" in res["message"]

    _set_request_json(monkeypatch, module, {"llm": {"model_name": "unknown-model", "model_type": "unsupported"}})
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert "`model_name` unknown-model doesn't exist" in res["message"]

    _set_request_json(
        monkeypatch,
        module,
        {"prompt": {"prompt": "No placeholder", "variables": [{"key": "knowledge", "optional": False}], "rerank_model": "unknown-rerank"}},
    )
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert "`rerank_model` unknown-rerank doesn't exist" in res["message"]

    _set_request_json(
        monkeypatch,
        module,
        {"prompt": {"prompt": "No placeholder", "variables": [{"key": "knowledge", "optional": False}]}},
    )
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert "Parameter 'knowledge' is not used" in res["message"]

    _set_request_json(
        monkeypatch,
        module,
        {"prompt": {"prompt": "Optional-only prompt", "variables": [{"key": "maybe", "optional": True}]}},
    )
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert res["code"] == 0

    _set_request_json(monkeypatch, module, {"name": ""})
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert res["message"] == "`name` cannot be empty."

    _set_request_json(monkeypatch, module, {"name": "dup-name"})
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert res["message"] == "Duplicated chat name in updating chat."

    _set_request_json(monkeypatch, module, {"llm": {"model_name": "glm-4", "temperature": 0.9}})
    res = _run(module.update.__wrapped__("tenant-1", "chat-1"))
    assert res["code"] == 0


@pytest.mark.p2
def test_delete_duplicate_no_success_path(monkeypatch):
    module = _load_chat_module(monkeypatch)

    _set_request_json(monkeypatch, module, {"ids": ["chat-1", "chat-1"]})
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    monkeypatch.setattr(module.DialogService, "update_by_id", lambda *_args, **_kwargs: 0)
    res = _run(module.delete_chats.__wrapped__("tenant-1"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "Duplicate assistant ids: chat-1" in res["message"]

    _set_request_json(monkeypatch, module, {"ids": ["missing-chat"]})
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    res = _run(module.delete_chats.__wrapped__("tenant-1"))
    assert res["code"] == module.RetCode.DATA_ERROR
    assert "Assistant(missing-chat) not found." in res["message"]

    _set_request_json(monkeypatch, module, {"ids": ["chat-1", "chat-1"]})
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    monkeypatch.setattr(module.DialogService, "update_by_id", lambda *_args, **_kwargs: 1)
    res = _run(module.delete_chats.__wrapped__("tenant-1"))
    assert res["code"] == 0
    assert res["data"]["success_count"] == 1


@pytest.mark.p2
def test_list_missing_kb_warning_and_desc_false(monkeypatch, caplog):
    module = _load_chat_module(monkeypatch)

    monkeypatch.setattr(module, "request", SimpleNamespace(args={"desc": "False"}))
    monkeypatch.setattr(module.DialogService, "get_list", lambda *_args, **_kwargs: [
        {
            "id": "chat-1",
            "name": "chat-name",
            "prompt_config": {"system": "Answer with {knowledge}", "parameters": [{"key": "knowledge", "optional": False}], "do_refer": True},
            "similarity_threshold": 0.2,
            "vector_similarity_weight": 0.3,
            "top_n": 6,
            "rerank_id": "",
            "llm_setting": {"temperature": 0.1},
            "llm_id": "glm-4",
            "kb_ids": ["missing-kb"],
            "icon": "icon.png",
        }
    ])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [])

    with caplog.at_level("WARNING"):
        res = module.list_chat.__wrapped__("tenant-1")

    assert res["code"] == 0
    assert res["data"][0]["datasets"] == []
    assert res["data"][0]["avatar"] == "icon.png"
    assert "does not exist" in caplog.text
