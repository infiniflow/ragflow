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
"""Regression tests for retrieval in api/apps/restful_apis/dify_retrieval_api.py.

Issue #15027: cross-tenant knowledge-base access via POST /api/v1/dify/retrieval.
The handler authenticated the caller and resolved tenant_id, but then fetched
the requested knowledge_id with no tenant filter, allowing any valid caller to
retrieve chunks from any other tenant's KB by id. The fix adds a
KnowledgebaseService.accessible(...) check immediately after the lookup.
"""

import asyncio
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _co():
            return self._value

        return _co().__await__()


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    # If `name` is a submodule, also overwrite the attribute on the parent
    # package. Otherwise `from <parent> import <child>` resolves to the
    # already-cached real submodule via attribute lookup, bypassing our
    # sys.modules entry and our stub.
    if "." in name:
        parent_name, _, child_name = name.rpartition(".")
        parent_mod = sys.modules.get(parent_name)
        if parent_mod is not None:
            monkeypatch.setattr(parent_mod, child_name, mod, raising=False)
    return mod


class _FakeRetriever:
    def __init__(self, chunks=None):
        self._chunks = chunks if chunks is not None else []
        self.retrieval_calls = []

    async def retrieval(self, question, embd_mdl, tenant_id, kb_ids, **kwargs):
        self.retrieval_calls.append({"question": question, "tenant_id": tenant_id, "kb_ids": list(kb_ids)})
        return {"chunks": list(self._chunks)}

    def retrieval_by_children(self, chunks, _tenant_ids):
        return chunks


class _FakeKGRetriever:
    async def retrieval(self, *_a, **_k):
        return {"content_with_weight": ""}


def _load_dify_retrieval(monkeypatch, *, kb, accessible, request_body, tenant_id, chunks=None):
    """Load dify_retrieval_api.py with minimum stubs to exercise the retrieval handler."""

    def _add_tenant_id_to_kwargs(func):
        async def wrapper(**kwargs):
            kwargs["tenant_id"] = tenant_id
            return await func(**kwargs)

        return wrapper

    _stub(
        monkeypatch,
        "api.apps",
        current_user=SimpleNamespace(id=tenant_id),
        login_required=lambda func: func,
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        add_tenant_id_to_kwargs=_add_tenant_id_to_kwargs,
        build_error_result=lambda message="", code=0, data=False: {"code": code, "message": message, "data": data},
        get_request_json=lambda: _AwaitableValue(request_body),
        get_json_result=lambda code=0, message="", data=None: {"code": code, "message": message, "data": data},
    )

    _stub(
        monkeypatch,
        "api.db.services.document_service",
        DocumentService=SimpleNamespace(
            get_by_id=lambda _id: (True, SimpleNamespace(id=_id, meta_fields={})),
            get_by_ids=lambda ids, cols=None: [SimpleNamespace(id=doc_id, meta_fields={}) for doc_id in ids],
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.doc_metadata_service",
        DocMetadataService=SimpleNamespace(get_flatted_meta_by_kbs=lambda _ids: {}),
    )

    acc_fn = accessible if callable(accessible) else (lambda *_a, **_k: accessible)
    _stub(
        monkeypatch,
        "api.db.services.knowledgebase_service",
        KnowledgebaseService=SimpleNamespace(get_by_id=lambda _id: kb, accessible=acc_fn),
    )

    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=lambda *_a, **_k: SimpleNamespace())

    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=lambda *_a, **_k: {},
        get_model_config_from_provider_instance=lambda *_a, **_k: {},
    )

    _stub(
        monkeypatch,
        "common.metadata_utils",
        meta_filter=lambda *_a, **_k: [],
        convert_conditions=lambda c: c,
    )

    _stub(monkeypatch, "rag.app.tag", label_question=lambda *_a, **_k: {})

    fake_retriever = _FakeRetriever(chunks=chunks)
    _stub(
        monkeypatch,
        "common.settings",
        retriever=fake_retriever,
        kg_retriever=_FakeKGRetriever(),
    )

    quart_stub = ModuleType("quart")
    quart_stub.request = SimpleNamespace(method="POST", args={})
    quart_stub.jsonify = lambda payload: payload
    monkeypatch.setitem(sys.modules, "quart", quart_stub)

    repo_root = Path(__file__).resolve().parents[5]
    module_path = repo_root / "api" / "apps" / "restful_apis" / "dify_retrieval_api.py"
    spec = importlib.util.spec_from_file_location("test_dify_retrieval_module", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_dify_retrieval_module", module)
    spec.loader.exec_module(module)
    module._fake_retriever = fake_retriever
    return module


@pytest.mark.p1
class TestDifyRetrievalTenantCheck:
    """Regression for #15027: cross-tenant KB exposure via /dify/retrieval."""

    @pytest.mark.p1
    def test_cross_tenant_request_is_rejected(self, monkeypatch, caplog):
        """A caller whose tenant does NOT own the requested KB must be denied.

        Also verifies that the denial is recorded via the module logger so
        operators can audit cross-tenant access attempts after the fact.
        """
        import logging

        owner_kb = SimpleNamespace(id="kb-victim", tenant_id="tenant-owner", tenant_embd_id="", embd_id="bge")
        request_body = {
            "knowledge_id": "kb-victim",
            "query": "VICTIM_SECRET",
            "retrieval_setting": {"top_k": 10, "score_threshold": 0.0},
        }

        def _accessible_only_for_owner(kb_id, user_id):
            return user_id == "tenant-owner"

        module = _load_dify_retrieval(
            monkeypatch,
            kb=(True, owner_kb),
            accessible=_accessible_only_for_owner,
            request_body=request_body,
            tenant_id="tenant-attacker",
            chunks=[{"doc_id": "d1", "content_with_weight": "VICTIM_SECRET ...", "similarity": 0.9, "docnm_kwd": "doc.txt"}],
        )

        caplog.set_level(logging.WARNING, logger=module.__name__)
        result = asyncio.run(module.retrieval())

        assert result["code"] == 109, f"expected AUTHENTICATION_ERROR (109), got {result}"
        msg = result["message"].lower()
        assert "authorization" in msg or "authentication" in msg
        assert "records" not in result, "cross-tenant request leaked records"
        assert module._fake_retriever.retrieval_calls == [], "retriever invoked despite denial"

        denial_logs = [r for r in caplog.records if r.levelno == logging.WARNING and "cross-tenant" in r.getMessage()]
        assert denial_logs, "denial branch must emit a WARNING audit log"
        rendered = denial_logs[0].getMessage()
        assert "tenant-attacker" in rendered, "caller tenant must appear in the audit log"
        assert "kb-victim" in rendered, "denied knowledge_id must appear in the audit log"
        assert "VICTIM_SECRET" not in rendered, "audit log must not leak request payload contents"

    @pytest.mark.p1
    def test_same_tenant_request_succeeds(self, monkeypatch):
        """When the caller's tenant owns the KB, retrieval proceeds normally."""
        owner_kb = SimpleNamespace(id="kb-owner", tenant_id="tenant-owner", tenant_embd_id="", embd_id="bge")
        request_body = {
            "knowledge_id": "kb-owner",
            "query": "hello",
            "retrieval_setting": {"top_k": 5, "score_threshold": 0.0},
        }

        module = _load_dify_retrieval(
            monkeypatch,
            kb=(True, owner_kb),
            accessible=lambda _id, _u: True,
            request_body=request_body,
            tenant_id="tenant-owner",
            chunks=[{"doc_id": "d1", "content_with_weight": "hello world", "similarity": 0.8, "docnm_kwd": "doc.txt"}],
        )

        result = asyncio.run(module.retrieval())

        assert "records" in result
        assert len(result["records"]) == 1
        assert result["records"][0]["content"] == "hello world"
        assert module._fake_retriever.retrieval_calls, "retriever was not called on legitimate request"

    @pytest.mark.p1
    def test_missing_knowledge_base_returns_not_found(self, monkeypatch):
        """KB id that does not exist returns 404 before the access check fires."""
        request_body = {"knowledge_id": "kb-does-not-exist", "query": "hello"}

        def _accessible_should_not_be_called(*_a, **_k):
            raise AssertionError("accessible() must not be called for a missing KB")

        module = _load_dify_retrieval(
            monkeypatch,
            kb=(False, None),
            accessible=_accessible_should_not_be_called,
            request_body=request_body,
            tenant_id="tenant-attacker",
        )

        result = asyncio.run(module.retrieval())

        assert result["code"] == 404
        assert "not found" in result["message"].lower()
