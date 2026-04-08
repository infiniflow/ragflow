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
from enum import Enum
from functools import wraps
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


class _DummyArgs(dict):
    def get(self, key, default=None):
        return super().get(key, default)

    def getlist(self, key):
        value = self.get(key, [])
        if value is None:
            return []
        if isinstance(value, list):
            return value
        return [value]


class _StubHeaders:
    def __init__(self):
        self._items = []

    def add_header(self, key, value):
        self._items.append((key, value))

    def get(self, key, default=None):
        for existing_key, value in reversed(self._items):
            if existing_key == key:
                return value
        return default


class _StubResponse:
    def __init__(self, body=None, mimetype=None, content_type=None):
        self.body = body
        self.mimetype = mimetype
        self.content_type = content_type
        self.headers = _StubHeaders()


def _passthrough_login_required(func):
    @wraps(func)
    async def _wrapper(*args, **kwargs):
        return await func(*args, **kwargs)

    return _wrapper


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


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


def _load_chat_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]
    module_name = "test_chat_restful_routes_unit_module"
    module_path = repo_root / "api" / "apps" / "restful_apis" / "chat_api.py"

    quart_mod = ModuleType("quart")
    quart_mod.request = SimpleNamespace(args=_DummyArgs())
    quart_mod.Response = _StubResponse
    monkeypatch.setitem(sys.modules, "quart", quart_mod)

    api_pkg = ModuleType("api")
    api_pkg.__path__ = [str(repo_root / "api")]
    monkeypatch.setitem(sys.modules, "api", api_pkg)

    apps_pkg = ModuleType("api.apps")
    apps_pkg.__path__ = [str(repo_root / "api" / "apps")]
    apps_pkg.current_user = SimpleNamespace(id="tenant-1")
    apps_pkg.login_required = _passthrough_login_required
    monkeypatch.setitem(sys.modules, "api.apps", apps_pkg)
    api_pkg.apps = apps_pkg

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    common_constants_mod = ModuleType("common.constants")

    class _StubLLMType(str, Enum):
        CHAT = "chat"
        IMAGE2TEXT = "image2text"
        RERANK = "rerank"

    class _StubRetCode(int, Enum):
        SUCCESS = 0
        DATA_ERROR = 102
        AUTHENTICATION_ERROR = 109

    class _StubStatusEnum(str, Enum):
        VALID = "1"
        INVALID = "0"

    common_constants_mod.LLMType = _StubLLMType
    common_constants_mod.RetCode = _StubRetCode
    common_constants_mod.StatusEnum = _StubStatusEnum
    monkeypatch.setitem(sys.modules, "common.constants", common_constants_mod)

    misc_utils_mod = ModuleType("common.misc_utils")
    misc_utils_mod.get_uuid = lambda: "generated-chat-id"
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_mod)

    dialog_service_mod = ModuleType("api.db.services.dialog_service")

    class _StubDialogService:
        model = SimpleNamespace(
            _meta=SimpleNamespace(
                fields={
                    "id": None,
                    "tenant_id": None,
                    "name": None,
                    "description": None,
                    "icon": None,
                    "kb_ids": None,
                    "llm_id": None,
                    "llm_setting": None,
                    "prompt_config": None,
                    "similarity_threshold": None,
                    "vector_similarity_weight": None,
                    "top_n": None,
                    "top_k": None,
                    "rerank_id": None,
                    "meta_data_filter": None,
                    "created_by": None,
                    "create_time": None,
                    "create_date": None,
                    "update_time": None,
                    "update_date": None,
                    "status": None,
                }
            )
        )

        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def save(**_kwargs):
            return True

        @staticmethod
        def get_by_id(_chat_id):
            return False, None

        @staticmethod
        def update_by_id(_chat_id, _payload):
            return True

        @staticmethod
        def get_by_tenant_ids(*_args, **_kwargs):
            return [], 0

    dialog_service_mod.DialogService = _StubDialogService
    dialog_service_mod.async_ask = lambda *_args, **_kwargs: None
    dialog_service_mod.async_chat = lambda *_args, **_kwargs: None
    dialog_service_mod.gen_mindmap = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "api.db.services.dialog_service", dialog_service_mod)

    conversation_service_mod = ModuleType("api.db.services.conversation_service")

    class _StubConversationService:
        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def get_list(*_args, **_kwargs):
            return []

        @staticmethod
        def get_by_id(_session_id):
            return False, None

        @staticmethod
        def update_by_id(_session_id, _payload):
            return True

        @staticmethod
        def delete_by_id(_session_id):
            return True

        @staticmethod
        def save(**_kwargs):
            return True

    conversation_service_mod.ConversationService = _StubConversationService
    conversation_service_mod.structure_answer = lambda *_args, **_kwargs: {}
    monkeypatch.setitem(sys.modules, "api.db.services.conversation_service", conversation_service_mod)

    kb_service_mod = ModuleType("api.db.services.knowledgebase_service")

    class _StubKnowledgebaseService:
        @staticmethod
        def accessible(**_kwargs):
            return []

        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def get_by_id(_kb_id):
            return False, None

    kb_service_mod.KnowledgebaseService = _StubKnowledgebaseService
    monkeypatch.setitem(sys.modules, "api.db.services.knowledgebase_service", kb_service_mod)

    tenant_llm_service_mod = ModuleType("api.db.services.tenant_llm_service")

    class _StubTenantLLMService:
        @staticmethod
        def split_model_name_and_factory(model_name):
            if model_name and "@" in model_name:
                return tuple(model_name.split("@", 1))
            return model_name, None

        @staticmethod
        def query(**_kwargs):
            return []

        @staticmethod
        def get_api_key(*_args, **_kwargs):
            return SimpleNamespace(id=1)

    tenant_llm_service_mod.TenantLLMService = _StubTenantLLMService
    monkeypatch.setitem(sys.modules, "api.db.services.tenant_llm_service", tenant_llm_service_mod)

    llm_service_mod = ModuleType("api.db.services.llm_service")

    class _StubLLMBundle:
        def __init__(self, *_args, **_kwargs):
            pass

    llm_service_mod.LLMBundle = _StubLLMBundle
    monkeypatch.setitem(sys.modules, "api.db.services.llm_service", llm_service_mod)

    search_service_mod = ModuleType("api.db.services.search_service")
    search_service_mod.SearchService = SimpleNamespace()
    monkeypatch.setitem(sys.modules, "api.db.services.search_service", search_service_mod)

    tenant_model_service_mod = ModuleType("api.db.joint_services.tenant_model_service")
    tenant_model_service_mod.get_model_config_by_type_and_name = lambda *_args, **_kwargs: {}
    tenant_model_service_mod.get_tenant_default_model_by_type = lambda *_args, **_kwargs: {}
    monkeypatch.setitem(sys.modules, "api.db.joint_services.tenant_model_service", tenant_model_service_mod)

    user_service_mod = ModuleType("api.db.services.user_service")

    class _StubTenantService:
        @staticmethod
        def get_by_id(_tenant_id):
            return True, SimpleNamespace(llm_id="glm-4")

    class _StubUserTenantService:
        @staticmethod
        def query(**_kwargs):
            return []

    user_service_mod.UserService = type("UserService", (), {})
    user_service_mod.TenantService = _StubTenantService
    user_service_mod.UserTenantService = _StubUserTenantService
    monkeypatch.setitem(sys.modules, "api.db.services.user_service", user_service_mod)

    chunk_feedback_service_mod = ModuleType("api.db.services.chunk_feedback_service")

    class _StubChunkFeedbackService:
        @staticmethod
        def apply_feedback(**_kwargs):
            return {"success_count": 0, "fail_count": 0, "chunk_ids": []}

    chunk_feedback_service_mod.ChunkFeedbackService = _StubChunkFeedbackService
    monkeypatch.setitem(sys.modules, "api.db.services.chunk_feedback_service", chunk_feedback_service_mod)

    api_utils_mod = ModuleType("api.utils.api_utils")

    def _check_duplicate_ids(ids, label):
        counts = {}
        for item in ids or []:
            counts[item] = counts.get(item, 0) + 1
        duplicate_messages = [f"Duplicate {label} ids: {item}" for item, count in counts.items() if count > 1]
        return list(set(ids or [])), duplicate_messages

    api_utils_mod.check_duplicate_ids = _check_duplicate_ids
    api_utils_mod.get_data_error_result = lambda message="": {"code": 102, "data": None, "message": message}
    api_utils_mod.get_json_result = lambda data=None, message="", code=0: {"code": code, "data": data, "message": message}
    api_utils_mod.get_request_json = lambda: _AwaitableValue({})
    api_utils_mod.server_error_response = lambda ex: {"code": 500, "data": None, "message": str(ex)}
    api_utils_mod.validate_request = lambda *_args, **_kwargs: (lambda func: func)
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils_mod)

    tenant_utils_mod = ModuleType("api.utils.tenant_utils")
    tenant_utils_mod.ensure_tenant_model_id_for_params = lambda _tenant_id, req: req
    monkeypatch.setitem(sys.modules, "api.utils.tenant_utils", tenant_utils_mod)

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = [str(repo_root / "rag")]
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)

    rag_prompts_pkg = ModuleType("rag.prompts")
    rag_prompts_pkg.__path__ = [str(repo_root / "rag" / "prompts")]
    monkeypatch.setitem(sys.modules, "rag.prompts", rag_prompts_pkg)

    rag_prompts_generator_mod = ModuleType("rag.prompts.generator")
    rag_prompts_generator_mod.chunks_format = lambda reference: reference.get("chunks", []) if isinstance(reference, dict) else []
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", rag_prompts_generator_mod)

    rag_prompts_template_mod = ModuleType("rag.prompts.template")
    rag_prompts_template_mod.load_prompt = lambda *_args, **_kwargs: ""
    monkeypatch.setitem(sys.modules, "rag.prompts.template", rag_prompts_template_mod)

    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
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
def test_update_chat_allows_knowledge_placeholder_without_sources(monkeypatch):
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
    updated = {}

    def _update(_chat_id, payload):
        updated.update(payload)
        return True

    monkeypatch.setattr(module.DialogService, "update_by_id", _update)

    res = _run(module.update_chat.__wrapped__("chat-1"))

    assert res["code"] == 0
    assert updated["prompt_config"]["system"] == "Answer with {knowledge}"


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


@pytest.mark.p2
def test_chat_session_create_and_update_guard_matrix_unit(monkeypatch):
    module = _load_chat_module(monkeypatch)

    _set_request_json(monkeypatch, module, {"name": "session"})
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    res = _run(module.create_session.__wrapped__("chat-1"))
    assert res["message"] == "No authorization."

    dia = SimpleNamespace(prompt_config={"prologue": "hello"})
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [dia])
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _id: (True, dia))
    monkeypatch.setattr(module.ConversationService, "save", lambda **_kwargs: None)
    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (False, None))
    res = _run(module.create_session.__wrapped__("chat-1"))
    assert "Fail to create a session" in res["message"]

    _set_request_json(monkeypatch, module, {})
    monkeypatch.setattr(module.ConversationService, "query", lambda **_kwargs: [])
    res = _run(module.update_session.__wrapped__("chat-1", "session-1"))
    assert res["message"] == "Session not found!"

    monkeypatch.setattr(module.ConversationService, "query", lambda **_kwargs: [SimpleNamespace(id="session-1")])
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    res = _run(module.update_session.__wrapped__("chat-1", "session-1"))
    assert res["message"] == "No authorization."

    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    _set_request_json(monkeypatch, module, {"message": []})
    res = _run(module.update_session.__wrapped__("chat-1", "session-1"))
    assert "`messages` cannot be changed." in res["message"]

    _set_request_json(monkeypatch, module, {"reference": []})
    res = _run(module.update_session.__wrapped__("chat-1", "session-1"))
    assert "`reference` cannot be changed." in res["message"]

    _set_request_json(monkeypatch, module, {"name": ""})
    res = _run(module.update_session.__wrapped__("chat-1", "session-1"))
    assert "`name` can not be empty." in res["message"]

    _set_request_json(monkeypatch, module, {"name": "renamed"})
    monkeypatch.setattr(module.ConversationService, "update_by_id", lambda *_args, **_kwargs: False)
    res = _run(module.update_session.__wrapped__("chat-1", "session-1"))
    assert res["message"] == "Session not found!"


@pytest.mark.p2
def test_chat_session_list_projection_unit(monkeypatch):
    module = _load_chat_module(monkeypatch)

    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(
            args=SimpleNamespace(
                get=lambda key, default=None: {
                    "page": 1,
                    "page_size": 30,
                    "orderby": "create_time",
                    "desc": "true",
                    "id": None,
                    "name": None,
                    "user_id": None,
                }.get(key, default)
            )
        ),
    )
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    monkeypatch.setattr(
        module.ConversationService,
        "get_list",
        lambda *_args, **_kwargs: [
            {
                "id": "session-1",
                "dialog_id": "chat-1",
                "message": [{"role": "assistant", "content": "hello"}],
                "reference": [],
            }
        ],
    )

    res = module.list_sessions.__wrapped__("chat-1")
    assert res["data"][0]["chat_id"] == "chat-1"
    assert res["data"][0]["messages"][0]["content"] == "hello"

    monkeypatch.setattr(
        module,
        "request",
        SimpleNamespace(
            args=SimpleNamespace(
                get=lambda key, default=None: {
                    "page": 1,
                    "page_size": 0,
                    "orderby": "create_time",
                    "desc": "true",
                    "id": None,
                    "name": None,
                    "user_id": None,
                }.get(key, default)
            )
        ),
    )
    res = module.list_sessions.__wrapped__("chat-1")
    assert res["data"] == []


@pytest.mark.p2
def test_chat_session_delete_routes_partial_duplicate_unit(monkeypatch):
    module = _load_chat_module(monkeypatch)

    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    _set_request_json(monkeypatch, module, {})
    res = _run(module.delete_sessions.__wrapped__("chat-1"))
    assert res["code"] == 0

    monkeypatch.setattr(module.ConversationService, "delete_by_id", lambda *_args, **_kwargs: True)

    def _conversation_query(**kwargs):
        if "dialog_id" in kwargs and "id" not in kwargs:
            return [SimpleNamespace(id="seed")]
        if kwargs.get("id") == "ok":
            return [SimpleNamespace(id="ok")]
        return []

    monkeypatch.setattr(module.ConversationService, "query", _conversation_query)

    _set_request_json(monkeypatch, module, {"ids": ["ok", "bad"]})
    monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
    res = _run(module.delete_sessions.__wrapped__("chat-1"))
    assert res["code"] == 0
    assert res["data"]["success_count"] == 1
    assert res["data"]["errors"] == ["The chat doesn't own the session bad"]

    _set_request_json(monkeypatch, module, {"ids": ["bad"]})
    monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
    res = _run(module.delete_sessions.__wrapped__("chat-1"))
    assert res["message"] == "The chat doesn't own the session bad"

    _set_request_json(monkeypatch, module, {"ids": ["ok", "ok"]})
    monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (["ok"], ["Duplicate session ids: ok"]))
    res = _run(module.delete_sessions.__wrapped__("chat-1"))
    assert res["code"] == 0
    assert res["data"]["success_count"] == 1
    assert res["data"]["errors"] == ["Duplicate session ids: ok"]
