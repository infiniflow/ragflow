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
"""Regression tests for tenant isolation on the chat / search-bot routes
(`api/apps/restful_apis/chat_api.py`, `api/apps/restful_apis/bot_api.py`)
and on `SearchService.accessible`.

Covered regressions:
- mindmap routes accepted arbitrary `kb_ids` (and search-app `kb_ids`) without
  a `KnowledgebaseService.accessible` check, leaking other tenants' chunks;
- `ask_about_embedded` did not validate the *effective* kb_ids that
  `async_ask` resolves after the search-app `search_config` override;
- search-app details were readable via `SearchService.get_detail` without a
  tenant check (`SearchService.accessible` now guards every consumer);
- the backward-compatible `chat_id` branch of `DELETE /chats` skipped
  `_ensure_owned_chat`, allowing cross-tenant chat deletion;
- `retrieval_test` granted access to private (non-TEAM) datasets of joined
  tenants because it only matched `tenant_id`, ignoring `permission`.
"""

import asyncio
import contextlib
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from typing import ClassVar

import pytest


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


async def _thread_pool_exec(fn, *args, **kwargs):
    return fn(*args, **kwargs)


def _quart_attrs(calls):
    def _response(*args, **kwargs):
        calls.setdefault("response", []).append(args[0] if args else None)
        return SimpleNamespace(headers=SimpleNamespace(add_header=lambda *a, **k: None))

    return {
        "Response": _response,
        "request": SimpleNamespace(),
    }


def _api_utils_attrs(request_payload):
    async def _request_json():
        return request_payload

    return {
        "add_tenant_id_to_kwargs": lambda func: func,
        "check_duplicate_ids": lambda ids, _kind: (list(dict.fromkeys(ids)), []),
        "get_data_error_result": lambda message="", **_k: {"code": 102, "message": message, "data": False},
        "get_error_data_result": lambda message="", **_k: {"code": 102, "message": message, "data": None},
        "get_json_result": lambda code=0, message="", data=None, **_k: {"code": code, "message": message, "data": data},
        "get_result": lambda **kwargs: {"code": 0, "data": kwargs.get("data")},
        "get_request_json": _request_json,
        "server_error_response": lambda exc: {"code": 500, "message": str(exc)},
        "token_required": lambda func: func,
        "validate_request": lambda *_a, **_k: lambda func: func,
    }


def _constants_attrs():
    return {
        "RetCode": SimpleNamespace(AUTHENTICATION_ERROR=401, OPERATING_ERROR=300, DATA_ERROR=102, ARGUMENT_ERROR=101),
        "LLMType": SimpleNamespace(),
        "StatusEnum": SimpleNamespace(VALID=SimpleNamespace(value="1"), INVALID=SimpleNamespace(value="0")),
    }


def _load_module(monkeypatch, module_name, relative_path, calls, request_payload):
    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / relative_path
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _load_chat_api(monkeypatch, *, kb_accessible, search_accessible, search_detail, request_payload, calls):
    """Load chat_api.py with stubbed services.

    `kb_accessible` / `search_accessible` are sets of ids that pass the
    corresponding `accessible` checks; everything else is denied.
    """

    def _kb_accessible(kb_id=None, user_id=None):
        calls.setdefault("kb_accessible", []).append((kb_id, user_id))
        return kb_id in kb_accessible

    def _search_accessible(search_id, user_id):
        calls.setdefault("search_accessible", []).append((search_id, user_id))
        return search_id in search_accessible

    def _search_get_detail(search_id):
        calls.setdefault("search_get_detail", []).append(search_id)
        return search_detail.get(search_id, {})

    async def _gen_mindmap(question, kb_ids, tenant_id, search_config=None, **_kwargs):
        calls.setdefault("gen_mindmap", []).append((question, sorted(kb_ids), tenant_id))
        return {"id": "mm"}

    class _DialogService:
        model = SimpleNamespace(_meta=SimpleNamespace(fields=set()))
        query_result: ClassVar[list] = []
        updated: ClassVar[list] = []

        @staticmethod
        def query(**_kwargs):
            return _DialogService.query_result

        @staticmethod
        def update_by_id(pid, payload):
            _DialogService.updated.append((pid, payload))
            return True

    _stub(monkeypatch, "quart", **_quart_attrs(calls))
    _stub(monkeypatch, "werkzeug.exceptions", BadRequest=type("BadRequest", (Exception,), {}))
    _stub(monkeypatch, "api.apps", current_user=SimpleNamespace(id="user-1"), login_required=lambda fn: fn, __path__=[])
    _stub(monkeypatch, "api.apps.restful_apis", __path__=[])
    _stub(monkeypatch, "api.apps.restful_apis._generation_params", merge_generation_config=lambda *_a, **_k: None, pop_generation_config=lambda *_a, **_k: {})
    _stub(monkeypatch, "api.db.services.llm_service", resolve_llm_setting=lambda s: s or {}, LLMBundle=lambda *_a, **_k: None)
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_api_key=lambda **_k: True,
        get_composite_model_name_by_id=lambda *_a, **_k: "model",
        get_model_config_by_id=lambda *_a, **_k: {},
        get_tenant_default_model_by_type=lambda *_a, **_k: {},
        resolve_model_config=lambda *_a, **_k: {},
        resolve_model_id=lambda *_a, **_k: "model",
    )
    _stub(monkeypatch, "api.db.services.chunk_feedback_service", ChunkFeedbackService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.conversation_service", ConversationService=SimpleNamespace(), structure_answer=lambda *_a, **_k: {})
    _stub(monkeypatch, "api.db.services.dialog_service", DialogService=_DialogService, gen_mindmap=_gen_mindmap, rag_agent=lambda *_a, **_k: None)
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(accessible=_kb_accessible),
        validate_dataset_embedding_models=lambda _kbs: None,
    )
    _stub(monkeypatch, "api.db.services.search_service", SearchService=SimpleNamespace(accessible=_search_accessible, get_detail=_search_get_detail))
    _stub(monkeypatch, "api.db.services.user_service", TenantService=SimpleNamespace(), UserTenantService=SimpleNamespace())
    _stub(monkeypatch, "api.utils.api_utils", **_api_utils_attrs(request_payload))
    _stub(monkeypatch, "common.constants", **_constants_attrs())
    _stub(monkeypatch, "common", settings=SimpleNamespace())
    _stub(monkeypatch, "common.settings", retriever=SimpleNamespace(), kg_retriever=SimpleNamespace())
    _stub(monkeypatch, "common.misc_utils", get_uuid=lambda: "uuid", thread_pool_exec=_thread_pool_exec)
    _stub(monkeypatch, "rag.prompts.generator", chunks_format=lambda *_a, **_k: [])
    _stub(monkeypatch, "rag.prompts.template", load_prompt=lambda *_a, **_k: "")

    return _load_module(monkeypatch, "test_chat_tenant_checks_chat_api", "api/apps/restful_apis/chat_api.py", calls, request_payload)


def _load_bot_api(monkeypatch, *, kb_accessible, search_accessible, search_detail, request_payload, calls):
    """Load bot_api.py with stubbed services (same contract as chat loader)."""

    def _kb_accessible(kb_id=None, user_id=None):
        calls.setdefault("kb_accessible", []).append((kb_id, user_id))
        return kb_id in kb_accessible

    def _search_accessible(search_id, user_id):
        calls.setdefault("search_accessible", []).append((search_id, user_id))
        return search_id in search_accessible

    def _search_get_detail(search_id):
        calls.setdefault("search_get_detail", []).append(search_id)
        return search_detail.get(search_id, {})

    async def _gen_mindmap(question, kb_ids, tenant_id, search_config=None, **_kwargs):
        calls.setdefault("gen_mindmap", []).append((question, sorted(kb_ids), tenant_id))
        return {"id": "mm"}

    async def _async_ask(question, kb_ids, tenant_id, **kwargs):
        calls.setdefault("async_ask", []).append((question, list(kb_ids), tenant_id, kwargs))
        yield {"answer": "ok", "final": True}

    _stub(monkeypatch, "quart", **_quart_attrs(calls))
    _stub(monkeypatch, "api.apps", AUTH_BETA="beta", login_required=lambda *_a, **_k: lambda func: func, __path__=[])
    _stub(monkeypatch, "api.apps.restful_apis", __path__=[])
    _stub(monkeypatch, "agent.canvas", Canvas=lambda *_a, **_k: SimpleNamespace())
    _stub(monkeypatch, "api.db.services.api_service", API4ConversationService=SimpleNamespace())
    _stub(monkeypatch, "api.db.services.canvas_service", UserCanvasService=SimpleNamespace(), completion=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.conversation_service", async_iframe_completion=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.dialog_service", DialogService=SimpleNamespace(), async_ask=_async_ask, gen_mindmap=_gen_mindmap)
    _stub(monkeypatch, "api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(accessible=_kb_accessible, get_by_ids=lambda ids, **_k: [], get_by_id=lambda kb_id: (False, None)),
    )
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=lambda *_a, **_k: None, resolve_llm_setting=lambda s: s or {})
    _stub(monkeypatch, "api.db.services.search_service", SearchService=SimpleNamespace(accessible=_search_accessible, get_detail=_search_get_detail))
    _stub(
        monkeypatch,
        "api.db.services.user_service",
        TenantService=SimpleNamespace(get_by_id=lambda _uid: (True, SimpleNamespace(llm_id="llm"))),
        UserTenantService=SimpleNamespace(),
    )
    _stub(monkeypatch, "api.db.joint_services.tenant_model_service", get_tenant_default_model_by_type=lambda *_a, **_k: {}, resolve_model_config=lambda *_a, **_k: {})
    _stub(monkeypatch, "common.metadata_utils", apply_meta_data_filter=lambda *_a, **_k: None)
    _stub(monkeypatch, "common.misc_utils", get_uuid=lambda: "uuid", thread_pool_exec=_thread_pool_exec)
    _stub(monkeypatch, "api.utils.api_utils", **_api_utils_attrs(request_payload))
    _stub(monkeypatch, "rag.app.tag", label_question=lambda *_a, **_k: None)
    _stub(monkeypatch, "rag.prompts.template", load_prompt=lambda *_a, **_k: "")
    _stub(monkeypatch, "rag.prompts.generator", cross_languages=lambda *_a, **_k: None, keyword_extraction=lambda *_a, **_k: None)
    _stub(monkeypatch, "rag.utils.web_search_conn", has_web_search_provider=lambda *_a, **_k: False)
    _stub(monkeypatch, "common.constants", **_constants_attrs())
    _stub(monkeypatch, "common", settings=SimpleNamespace())
    _stub(monkeypatch, "common.settings", retriever=SimpleNamespace(), kg_retriever=SimpleNamespace())
    _stub(monkeypatch, "api.utils.reference_metadata_utils", enrich_chunks_with_document_metadata=lambda *_a, **_k: None, resolve_reference_metadata_preferences=lambda *_a, **_k: (False, []))

    return _load_module(monkeypatch, "test_chat_tenant_checks_bot_api", "api/apps/restful_apis/bot_api.py", calls, request_payload)


def _consume_stream(stream):
    async def _consume():
        async for _chunk in stream:
            pass

    asyncio.run(_consume())


@pytest.mark.p1
class TestChatMindmapTenantChecks:
    def test_foreign_kb_rejected(self, monkeypatch):
        calls = {}
        module = _load_chat_api(
            monkeypatch,
            kb_accessible={"kb-own"},
            search_accessible=set(),
            search_detail={},
            request_payload={"question": "q", "kb_ids": ["kb-foreign"]},
            calls=calls,
        )

        res = asyncio.run(module.mindmap())

        assert res["code"] == 102, res
        assert res["message"] == "You don't own the dataset kb-foreign", res
        assert "gen_mindmap" not in calls

    def test_foreign_search_app_rejected(self, monkeypatch):
        calls = {}
        module = _load_chat_api(
            monkeypatch,
            kb_accessible={"kb-own"},
            search_accessible=set(),
            search_detail={},
            request_payload={"question": "q", "kb_ids": ["kb-own"], "search_id": "srch-foreign"},
            calls=calls,
        )

        res = asyncio.run(module.mindmap())

        assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res
        assert res["message"] == "no authorization", res
        assert "search_get_detail" not in calls
        assert "gen_mindmap" not in calls

    def test_accessible_kb_and_search_app_merged_and_forwarded(self, monkeypatch):
        calls = {}
        module = _load_chat_api(
            monkeypatch,
            kb_accessible={"kb-own", "kb-search"},
            search_accessible={"srch-own"},
            search_detail={"srch-own": {"tenant_id": "user-1", "search_config": {"kb_ids": ["kb-search"]}}},
            request_payload={"question": "q", "kb_ids": ["kb-own"], "search_id": "srch-own"},
            calls=calls,
        )

        res = asyncio.run(module.mindmap())

        assert res["code"] == 0, res
        question, kb_ids, tenant_id = calls["gen_mindmap"][0]
        assert question == "q"
        assert kb_ids == ["kb-own", "kb-search"]
        assert tenant_id == "user-1"
        # every kb that reaches gen_mindmap must have passed the accessible check
        assert {kb for kb, _user in calls["kb_accessible"]} == {"kb-own", "kb-search"}
        assert all(user == "user-1" for _kb, user in calls["kb_accessible"])


@pytest.mark.p1
class TestChatRecommendationTenantChecks:
    def test_foreign_search_app_rejected(self, monkeypatch):
        calls = {}
        module = _load_chat_api(
            monkeypatch,
            kb_accessible=set(),
            search_accessible=set(),
            search_detail={},
            request_payload={"question": "q", "search_id": "srch-foreign"},
            calls=calls,
        )

        res = asyncio.run(module.recommendation())

        assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res
        assert res["message"] == "no authorization", res
        assert "search_get_detail" not in calls


@pytest.mark.p1
class TestBulkDeleteChatsTenantChecks:
    def test_chat_id_of_foreign_tenant_rejected(self, monkeypatch):
        calls = {}
        module = _load_chat_api(
            monkeypatch,
            kb_accessible=set(),
            search_accessible=set(),
            search_detail={},
            request_payload={"chat_id": "chat-victim"},
            calls=calls,
        )
        module.DialogService.query_result = []  # _ensure_owned_chat finds nothing

        res = asyncio.run(module.bulk_delete_chats())

        assert res["code"] == module.RetCode.AUTHENTICATION_ERROR, res
        assert res["message"] == "no authorization", res
        assert module.DialogService.updated == []

    def test_owned_chat_id_deleted(self, monkeypatch):
        calls = {}
        module = _load_chat_api(
            monkeypatch,
            kb_accessible=set(),
            search_accessible=set(),
            search_detail={},
            request_payload={"chat_id": "chat-mine"},
            calls=calls,
        )
        module.DialogService.query_result = [SimpleNamespace(id="chat-mine")]

        res = asyncio.run(module.bulk_delete_chats())

        assert res["code"] == 0 and res["data"] is True, res
        assert module.DialogService.updated == [("chat-mine", {"status": module.StatusEnum.INVALID.value})]


@pytest.mark.p1
class TestAskAboutEmbeddedTenantChecks:
    def test_foreign_request_kb_rejected(self, monkeypatch):
        calls = {}
        module = _load_bot_api(
            monkeypatch,
            kb_accessible={"kb-own"},
            search_accessible=set(),
            search_detail={},
            request_payload={"question": "q", "kb_ids": ["kb-foreign"]},
            calls=calls,
        )

        res = asyncio.run(module.ask_about_embedded(tenant_id="tenant-1"))

        assert res["code"] == 102, res
        assert res["message"] == "You don't own the dataset kb-foreign", res
        assert "async_ask" not in calls

    def test_search_config_override_kb_rejected(self, monkeypatch):
        """The effective kb_ids after the search_config override must be checked."""
        calls = {}
        module = _load_bot_api(
            monkeypatch,
            kb_accessible={"kb-own"},
            search_accessible={"srch-own"},
            search_detail={"srch-own": {"search_config": {"kb_ids": ["kb-hidden-foreign"]}}},
            request_payload={"question": "q", "kb_ids": ["kb-own"], "search_id": "srch-own"},
            calls=calls,
        )

        res = asyncio.run(module.ask_about_embedded(tenant_id="tenant-1"))

        assert res["code"] == 102, res
        assert res["message"] == "You don't own the dataset kb-hidden-foreign", res
        assert "async_ask" not in calls

    def test_accessible_kbs_stream_and_forwarded(self, monkeypatch):
        calls = {}
        module = _load_bot_api(
            monkeypatch,
            kb_accessible={"kb-own"},
            search_accessible=set(),
            search_detail={},
            request_payload={"question": "q", "kb_ids": ["kb-own"], "stream": True},
            calls=calls,
        )

        asyncio.run(module.ask_about_embedded(tenant_id="tenant-1"))

        _consume_stream(calls["response"][0])
        question, kb_ids, tenant_id, kwargs = calls["async_ask"][0]
        assert question == "q"
        assert kb_ids == ["kb-own"]
        assert tenant_id == "tenant-1"
        assert kwargs.get("search_config") == {}


@pytest.mark.p1
class TestBotMindmapTenantChecks:
    def test_foreign_kb_rejected(self, monkeypatch):
        calls = {}
        module = _load_bot_api(
            monkeypatch,
            kb_accessible=set(),
            search_accessible=set(),
            search_detail={},
            request_payload={"question": "q", "kb_ids": ["kb-foreign"]},
            calls=calls,
        )

        res = asyncio.run(module.mindmap(tenant_id="tenant-1"))

        assert res["code"] == 102, res
        assert res["message"] == "You don't own the dataset kb-foreign", res
        assert "gen_mindmap" not in calls

    def test_foreign_search_app_rejected(self, monkeypatch):
        calls = {}
        module = _load_bot_api(
            monkeypatch,
            kb_accessible={"kb-own"},
            search_accessible=set(),
            search_detail={},
            request_payload={"question": "q", "kb_ids": ["kb-own"], "search_id": "srch-foreign"},
            calls=calls,
        )

        res = asyncio.run(module.mindmap(tenant_id="tenant-1"))

        assert res["code"] == module.RetCode.OPERATING_ERROR, res
        assert res["message"] == "Has no permission for this operation.", res
        assert "search_get_detail" not in calls
        assert "gen_mindmap" not in calls

    def test_accessible_kb_forwarded(self, monkeypatch):
        calls = {}
        module = _load_bot_api(
            monkeypatch,
            kb_accessible={"kb-own"},
            search_accessible=set(),
            search_detail={},
            request_payload={"question": "q", "kb_ids": ["kb-own"]},
            calls=calls,
        )

        res = asyncio.run(module.mindmap(tenant_id="tenant-1"))

        assert res["code"] == 0, res
        assert calls["gen_mindmap"][0][1] == ["kb-own"]
        assert all(user == "tenant-1" for _kb, user in calls["kb_accessible"])


@pytest.mark.p1
class TestRelatedQuestionsTenantChecks:
    def test_foreign_search_app_rejected(self, monkeypatch):
        calls = {}
        module = _load_bot_api(
            monkeypatch,
            kb_accessible=set(),
            search_accessible=set(),
            search_detail={},
            request_payload={"question": "q", "search_id": "srch-foreign"},
            calls=calls,
        )

        res = asyncio.run(module.related_questions_embedded(tenant_id="tenant-1"))

        assert res["code"] == module.RetCode.OPERATING_ERROR, res
        assert res["message"] == "Has no permission for this operation.", res
        assert "search_get_detail" not in calls


@pytest.mark.p1
class TestRetrievalTestTenantChecks:
    def test_foreign_search_app_rejected(self, monkeypatch):
        calls = {}
        module = _load_bot_api(
            monkeypatch,
            kb_accessible={"kb-own"},
            search_accessible=set(),
            search_detail={},
            request_payload={"question": "q", "kb_id": "kb-own", "search_id": "srch-foreign"},
            calls=calls,
        )

        res = asyncio.run(module.retrieval_test_embedded(tenant_id="tenant-1"))

        assert res["code"] == module.RetCode.OPERATING_ERROR, res
        assert res["message"] == "Has no permission for this operation.", res
        assert "search_get_detail" not in calls

    def test_kb_failing_accessible_check_rejected(self, monkeypatch):
        """A private dataset of a joined tenant must be denied by the accessible check."""
        calls = {}
        module = _load_bot_api(
            monkeypatch,
            kb_accessible=set(),
            search_accessible=set(),
            search_detail={},
            request_payload={"question": "q", "kb_id": ["kb-private-of-joined-tenant"]},
            calls=calls,
        )

        res = asyncio.run(module.retrieval_test_embedded(tenant_id="tenant-1"))

        assert res["code"] == module.RetCode.OPERATING_ERROR, res
        assert res["message"] == "Only owner of dataset authorized for this operation.", res
        assert calls["kb_accessible"] == [("kb-private-of-joined-tenant", "tenant-1")]


class _Cond:
    def __init__(self, *parts):
        self.parts = parts

    def __and__(self, other):
        return _Cond(*self.parts, *getattr(other, "parts", ()))

    def __repr__(self):
        return "&".join(repr(part) for part in self.parts)


class _FakeField:
    def __init__(self, name):
        self.name = name

    def __eq__(self, other):
        return _Cond((self.name, "==", getattr(other, "name", other)))


class _FakeSelectQuery:
    def __init__(self, state):
        self._state = state
        self.joins = []
        self.wheres = []

    def join(self, model, on=None):
        self.joins.append((model, on))
        return self

    def where(self, *conds):
        self.wheres.extend(conds)
        return self

    def first(self):
        return self._state["first"]


def _fake_model(state, name):
    class _Model:
        id = _FakeField("id")
        tenant_id = _FakeField("tenant_id")
        user_id = _FakeField("user_id")
        status = _FakeField("status")

        @classmethod
        def select(cls, *_fields):
            query = _FakeSelectQuery(state)
            state["last_query"] = query
            return query

    _Model.__name__ = name
    return _Model


class _FakeDB:
    @staticmethod
    def connection_context():
        @contextlib.contextmanager
        def _ctx():
            yield

        return _ctx()


class TestSearchServiceAccessible:
    def _load(self, monkeypatch, state):
        repo_root = Path(__file__).resolve().parents[5]

        _stub(monkeypatch, "api.db.db_models", DB=_FakeDB, Search=_fake_model(state, "Search"), User=_fake_model(state, "User"), UserTenant=_fake_model(state, "UserTenant"))
        _stub(monkeypatch, "api.db.services.common_service", CommonService=object)
        _stub(monkeypatch, "common.constants", StatusEnum=SimpleNamespace(VALID=SimpleNamespace(value="1")))
        _stub(monkeypatch, "common.time_utils", current_timestamp=lambda: 0, datetime_format=lambda _dt: "")

        module_path = repo_root / "api" / "db" / "services" / "search_service.py"
        spec = importlib.util.spec_from_file_location("test_chat_tenant_checks_search_service", module_path)
        module = importlib.util.module_from_spec(spec)
        monkeypatch.setitem(sys.modules, "test_chat_tenant_checks_search_service", module)
        spec.loader.exec_module(module)
        return module

    def test_returns_false_when_not_found_or_not_a_member(self, monkeypatch):
        state = {"first": None}
        module = self._load(monkeypatch, state)

        assert module.SearchService.accessible("srch-1", "user-1") is False

        query = state["last_query"]
        join_model, join_on = query.joins[0]
        assert join_model.__name__ == "UserTenant"
        # membership must be scoped to VALID rows, else invalid memberships pass
        # (peewee renders the join condition as tuples of bare column names)
        assert "status" in str(join_on)
        # the join must scope the search app to a tenant the user belongs to
        assert "tenant_id" in repr(join_on) and "user_id" in repr(join_on), repr(join_on)
        # the search app itself must be valid
        assert "status" in repr(query.wheres[0]) and "id" in repr(query.wheres[0]), repr(query.wheres)

    def test_returns_true_for_tenant_member(self, monkeypatch):
        state = {"first": SimpleNamespace(id="srch-1")}
        module = self._load(monkeypatch, state)

        assert module.SearchService.accessible("srch-1", "user-1") is True
