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
import sys
import types
import warnings
from types import SimpleNamespace

import pytest

# xgboost imports pkg_resources and emits a deprecation warning that is promoted
# to error in our pytest configuration; ignore it for this unit test module.
warnings.filterwarnings(
    "ignore",
    message="pkg_resources is deprecated as an API.*",
    category=UserWarning,
)


def _install_cv2_stub_if_unavailable():
    try:
        import cv2  # noqa: F401

        return
    except Exception:
        pass

    stub = types.ModuleType("cv2")

    # Constants referenced by deepdoc import-time defaults.
    stub.INTER_LINEAR = 1
    stub.INTER_CUBIC = 2
    stub.BORDER_CONSTANT = 0
    stub.BORDER_REPLICATE = 1
    stub.COLOR_BGR2RGB = 0
    stub.COLOR_BGR2GRAY = 1
    stub.COLOR_GRAY2BGR = 2
    stub.IMREAD_IGNORE_ORIENTATION = 128
    stub.IMREAD_COLOR = 1
    stub.RETR_LIST = 1
    stub.CHAIN_APPROX_SIMPLE = 2

    def _missing(*_args, **_kwargs):
        raise RuntimeError("cv2 runtime call is unavailable in this test environment")

    def _module_getattr(name):
        if name.isupper():
            return 0
        return _missing

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


_install_cv2_stub_if_unavailable()

from api.db.services import dialog_service


class _StubChatModel:
    def __init__(self, outputs):
        self._outputs = outputs
        self.calls = []

    async def async_chat(self, system_prompt, messages, llm_setting):
        idx = len(self.calls)
        if idx >= len(self._outputs):
            raise AssertionError("async_chat called more times than expected")
        self.calls.append(
            {
                "system_prompt": system_prompt,
                "message": messages[0]["content"],
                "llm_setting": llm_setting,
            }
        )
        return self._outputs[idx]


class _StubRetriever:
    def __init__(self, results):
        self._results = results
        self.sql_calls = []

    def sql_retrieval(self, sql, format="json"):
        assert format == "json"
        idx = len(self.sql_calls)
        if idx >= len(self._results):
            raise AssertionError("sql_retrieval called more times than expected")
        self.sql_calls.append(sql)
        return self._results[idx]


class _StubAsyncRetriever:
    def __init__(self, result):
        self.result = result
        self.calls = []

    async def retrieval(self, *args, **kwargs):
        self.calls.append({"args": args, "kwargs": kwargs})
        return self.result

    def retrieval_by_children(self, chunks, tenant_ids):
        return chunks


@pytest.fixture
def force_es_engine(monkeypatch):
    monkeypatch.setattr(dialog_service.settings, "DOC_ENGINE_INFINITY", False)
    monkeypatch.setattr(dialog_service.settings, "DOC_ENGINE_OCEANBASE", False)


@pytest.mark.p2
def test_use_sql_repairs_missing_source_columns_for_non_aggregate(monkeypatch, force_es_engine):
    retriever = _StubRetriever(
        [
            {
                "columns": [{"name": "product"}],
                "rows": [["desk"], ["monitor"]],
            },
            {
                "columns": [{"name": "doc_id"}, {"name": "docnm_kwd"}, {"name": "product"}],
                "rows": [["doc-1", "products.xlsx", "desk"], ["doc-2", "products.xlsx", "monitor"]],
            },
        ]
    )
    chat_model = _StubChatModel(
        [
            "SELECT product FROM ragflow_tenant",
            "SELECT doc_id, docnm_kwd, product FROM ragflow_tenant",
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)

    result = asyncio.run(
        dialog_service.use_sql(
            question="show me column of product",
            field_map={"product": "product"},
            tenant_id="tenant-id",
            chat_mdl=chat_model,
            quota=True,
            kb_ids=None,
        )
    )

    assert result is not None
    assert "|product|Source|" in result["answer"]
    answer_lines = [ln.strip() for ln in result["answer"].splitlines() if ln.strip().startswith("|")]
    header, separator = answer_lines[0], answer_lines[1]
    assert header.count("|") == separator.count("|")
    assert len(chat_model.calls) == 2
    assert len(retriever.sql_calls) == 2


@pytest.mark.p2
def test_use_sql_separator_matches_header_without_doc_name(monkeypatch, force_es_engine):
    retriever = _StubRetriever(
        [
            {
                "columns": [{"name": "doc_id"}, {"name": "product"}],
                "rows": [["doc-1", "desk"]],
            },
        ]
    )
    chat_model = _StubChatModel(["SELECT doc_id, product FROM ragflow_tenant"])
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)

    result = asyncio.run(
        dialog_service.use_sql(
            question="show product with doc id only",
            field_map={"product": "product"},
            tenant_id="tenant-id",
            chat_mdl=chat_model,
            quota=True,
            kb_ids=None,
        )
    )

    assert result is not None
    answer_lines = [ln.strip() for ln in result["answer"].splitlines() if ln.strip().startswith("|")]
    assert answer_lines[0] == "|product|"
    assert answer_lines[1] == "|------"
    assert "|------|------|" not in result["answer"]


@pytest.mark.p2
def test_use_sql_keeps_aggregate_flow_without_source_repair(monkeypatch, force_es_engine):
    retriever = _StubRetriever(
        [
            {
                "columns": [{"name": "count(star)"}],
                "rows": [[6]],
            },
        ]
    )
    chat_model = _StubChatModel(
        [
            "SELECT COUNT(*) FROM ragflow_tenant",
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)

    result = asyncio.run(
        dialog_service.use_sql(
            question="how many rows are there",
            field_map={"product": "product"},
            tenant_id="tenant-id",
            chat_mdl=chat_model,
            quota=True,
            kb_ids=None,
        )
    )

    assert result is not None
    assert "|COUNT(*)|" in result["answer"]
    assert "Source" not in result["answer"]
    assert len(chat_model.calls) == 1
    assert len(retriever.sql_calls) == 1


@pytest.mark.p2
def test_use_sql_source_repair_is_bounded_to_single_retry(monkeypatch, force_es_engine):
    retriever = _StubRetriever(
        [
            {
                "columns": [{"name": "product"}],
                "rows": [["desk"]],
            },
            {
                "columns": [{"name": "product"}],
                "rows": [["desk"]],
            },
        ]
    )
    chat_model = _StubChatModel(
        [
            "SELECT product FROM ragflow_tenant",
            "SELECT product FROM ragflow_tenant WHERE product IS NOT NULL",
        ]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)

    result = asyncio.run(
        dialog_service.use_sql(
            question="show me column of product",
            field_map={"product": "product"},
            tenant_id="tenant-id",
            chat_mdl=chat_model,
            quota=True,
            kb_ids=None,
        )
    )

    assert result is not None
    assert "|product|" in result["answer"]
    assert "Source" not in result["answer"]
    assert len(chat_model.calls) == 2
    assert len(retriever.sql_calls) == 2


@pytest.mark.p2
def test_async_chat_uses_all_docs_when_no_doc_ids_selected(monkeypatch):
    retriever = _StubAsyncRetriever(
        {
            "total": 1,
            "chunks": [
                {
                    "chunk_id": "chunk-1",
                    "content_ltks": "chunk text",
                    "content_with_weight": "Chunk text from dataset.",
                    "doc_id": "doc-1",
                    "docnm_kwd": "doc.txt",
                    "kb_id": "kb-1",
                    "important_kwd": [],
                    "positions": [],
                    "vector": [0.1, 0.2],
                }
            ],
            "doc_aggs": [],
        }
    )
    chat_model = _StubChatModel(["stub answer"])
    dialog = SimpleNamespace(
        kb_ids=["kb-1"],
        llm_id="chat-model",
        tenant_llm_id="",
        tenant_id="tenant-id",
        llm_setting={},
        similarity_threshold=0.1,
        vector_similarity_weight=0.2,
        top_n=8,
        top_k=32,
        meta_data_filter=None,
        prompt_config={
            "quote": False,
            "keyword": False,
            "tts": False,
            "empty_response": "",
            "system": "Use only this knowledge: {knowledge}",
            "parameters": [{"key": "knowledge", "optional": False}],
            "reasoning": False,
            "toc_enhance": False,
            "use_kg": False,
        },
    )

    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda _tid, _llm_id: ["chat"])
    monkeypatch.setattr(
        dialog_service,
        "resolve_model_config",
        lambda *_args, **_kwargs: {"llm_factory": "unit", "max_tokens": 4096, "model_type": "chat"},
    )
    monkeypatch.setattr(dialog_service.TenantLangfuseService, "filter_by_tenant", lambda **_kwargs: None)
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda _dialog, **_kwargs: ([SimpleNamespace(tenant_id="tenant-id")], object(), None, chat_model, None),
    )
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_field_map", lambda _kb_ids: {})
    monkeypatch.setattr(dialog_service, "label_question", lambda _question, _kbs: None)
    monkeypatch.setattr(
        dialog_service,
        "kb_prompt",
        lambda kbinfos, _max_tokens: ["Chunk text from dataset."] if kbinfos["chunks"] else [],
    )
    monkeypatch.setattr(dialog_service, "message_fit_in", lambda msg, _max_tokens: (0, msg))

    async def _collect():
        items = []
        async for item in dialog_service.async_chat(dialog, [{"role": "user", "content": "What does the dataset say?"}], stream=False):
            items.append(item)
        return items

    result = asyncio.run(_collect())

    assert len(retriever.calls) == 1
    assert retriever.calls[0]["kwargs"]["doc_ids"] is None
    assert "Chunk text from dataset." in chat_model.calls[0]["system_prompt"]
    assert result[0]["answer"] == "stub answer"


@pytest.mark.p2
def test_async_chat_sql_retrieval_uses_dataset_owner_tenant_not_dialog_tenant(monkeypatch):
    """Regression test for a team-shared dataset SQL-retrieval tenant mismatch.

    When a chat (owned by tenant "chat-creator-tenant") references a table
    dataset owned by a different tenant ("dataset-owner-tenant"), SQL
    retrieval must build the doc-store index/table name from the dataset
    owner's tenant, not from dialog.tenant_id -- otherwise it queries an
    index that doesn't exist (see GitHub issue #18514).
    """
    shared_kb = SimpleNamespace(
        id="kb-shared",
        tenant_id="dataset-owner-tenant",
        parser_config={"field_map": {"product": "Product Name"}},
    )
    chat_model = _StubChatModel(["unused"])
    dialog = SimpleNamespace(
        kb_ids=["kb-shared"],
        llm_id="chat-model",
        tenant_llm_id="",
        tenant_id="chat-creator-tenant",
        llm_setting={},
        similarity_threshold=0.1,
        vector_similarity_weight=0.2,
        top_n=8,
        top_k=32,
        meta_data_filter=None,
        prompt_config={
            "quote": True,
            "keyword": False,
            "tts": False,
            "empty_response": "",
            "system": "Use only this knowledge: {knowledge}",
            "parameters": [{"key": "knowledge", "optional": False}],
            "reasoning": False,
            "toc_enhance": False,
            "use_kg": False,
        },
    )

    captured_calls = []

    async def _fake_use_sql(_question, _field_map, tenant_id, _chat_mdl, _quote, kb_ids, doc_ids=None):
        captured_calls.append({"tenant_id": tenant_id, "kb_ids": kb_ids})
        return {"answer": "sql answer", "reference": {"chunks": [], "doc_aggs": []}, "prompt": ""}

    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda _tid, _llm_id: ["chat"])
    monkeypatch.setattr(
        dialog_service,
        "resolve_model_config",
        lambda *_args, **_kwargs: {"llm_factory": "unit", "max_tokens": 4096, "model_type": "chat"},
    )
    monkeypatch.setattr(dialog_service.TenantLangfuseService, "filter_by_tenant", lambda **_kwargs: None)
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda _dialog, **_kwargs: ([shared_kb], object(), None, chat_model, None),
    )
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_field_map", lambda _kb_ids: {"product": "Product Name"})
    monkeypatch.setattr(dialog_service, "use_sql", _fake_use_sql)

    async def _collect():
        items = []
        async for item in dialog_service.async_chat(dialog, [{"role": "user", "content": "How many products?"}], stream=False):
            items.append(item)
        return items

    result = asyncio.run(_collect())

    assert len(captured_calls) == 1
    assert captured_calls[0]["tenant_id"] == "dataset-owner-tenant"
    assert captured_calls[0]["kb_ids"] == ["kb-shared"]
    assert result[0]["answer"] == "sql answer"


@pytest.mark.p2
def test_async_chat_sql_retrieval_ignores_empty_field_map_datasets(monkeypatch):
    mapped_kb = SimpleNamespace(
        id="kb-mapped",
        tenant_id="mapped-tenant",
        parser_config={"field_map": {"product": "Product Name"}},
    )
    empty_map_kb = SimpleNamespace(
        id="kb-empty-map",
        tenant_id="other-tenant",
        parser_config={"field_map": {}},
    )
    chat_model = _StubChatModel(["unused"])
    dialog = SimpleNamespace(
        kb_ids=["kb-mapped", "kb-empty-map"],
        llm_id="chat-model",
        tenant_llm_id="",
        tenant_id="chat-creator-tenant",
        llm_setting={},
        similarity_threshold=0.1,
        vector_similarity_weight=0.2,
        top_n=8,
        top_k=32,
        meta_data_filter=None,
        prompt_config={
            "quote": True,
            "keyword": False,
            "tts": False,
            "empty_response": "",
            "system": "Use only this knowledge: {knowledge}",
            "parameters": [{"key": "knowledge", "optional": False}],
            "reasoning": False,
            "toc_enhance": False,
            "use_kg": False,
        },
    )

    captured_calls = []

    async def _fake_use_sql(_question, _field_map, tenant_id, _chat_mdl, _quote, kb_ids, doc_ids=None):
        captured_calls.append({"tenant_id": tenant_id, "kb_ids": kb_ids})
        return {"answer": "sql answer", "reference": {"chunks": [], "doc_aggs": []}, "prompt": ""}

    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda _tid, _llm_id: ["chat"])
    monkeypatch.setattr(
        dialog_service,
        "resolve_model_config",
        lambda *_args, **_kwargs: {"llm_factory": "unit", "max_tokens": 4096, "model_type": "chat"},
    )
    monkeypatch.setattr(dialog_service.TenantLangfuseService, "filter_by_tenant", lambda **_kwargs: None)
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda _dialog, **_kwargs: ([mapped_kb, empty_map_kb], object(), None, chat_model, None),
    )
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_field_map", lambda _kb_ids: {"product": "Product Name"})
    monkeypatch.setattr(dialog_service, "use_sql", _fake_use_sql)

    async def _collect():
        items = []
        async for item in dialog_service.async_chat(dialog, [{"role": "user", "content": "How many products?"}], stream=False):
            items.append(item)
        return items

    result = asyncio.run(_collect())

    assert captured_calls == [{"tenant_id": "mapped-tenant", "kb_ids": ["kb-mapped"]}]
    assert result[0]["answer"] == "sql answer"


@pytest.mark.p2
def test_async_chat_skips_sql_retrieval_for_mixed_tenant_field_map_datasets(monkeypatch):
    """Regression test: SQL retrieval must not silently pick one tenant's index
    when the field-map datasets referenced by a chat span multiple tenants.

    use_sql() queries a single tenant's doc-store index per call, and re-running
    it once per tenant inline is too slow (each call round-trips an LLM to
    generate SQL). So when the datasets span more than one tenant, SQL retrieval
    is skipped entirely and the flow falls back to vector search, instead of
    querying only one tenant's index and dropping the other tenants' data.
    """
    kb_a = SimpleNamespace(id="kb-a", tenant_id="tenant-a", parser_config={"field_map": {"product": "Product Name"}})
    kb_b = SimpleNamespace(id="kb-b", tenant_id="tenant-b", parser_config={"field_map": {"product": "Product Name"}})

    retriever = _StubAsyncRetriever(
        {
            "total": 1,
            "chunks": [
                {
                    "chunk_id": "chunk-1",
                    "content_ltks": "chunk text",
                    "content_with_weight": "Chunk text from dataset.",
                    "doc_id": "doc-1",
                    "docnm_kwd": "doc.txt",
                    "kb_id": "kb-a",
                    "important_kwd": [],
                    "positions": [],
                    "vector": [0.1, 0.2],
                }
            ],
            "doc_aggs": [],
        }
    )
    chat_model = _StubChatModel(["stub answer"])
    dialog = SimpleNamespace(
        kb_ids=["kb-a", "kb-b"],
        llm_id="chat-model",
        tenant_llm_id="",
        tenant_id="tenant-a",
        llm_setting={},
        similarity_threshold=0.1,
        vector_similarity_weight=0.2,
        top_n=8,
        top_k=32,
        meta_data_filter=None,
        prompt_config={
            "quote": False,
            "keyword": False,
            "tts": False,
            "empty_response": "",
            "system": "Use only this knowledge: {knowledge}",
            "parameters": [{"key": "knowledge", "optional": False}],
            "reasoning": False,
            "toc_enhance": False,
            "use_kg": False,
        },
    )

    def _unexpected_use_sql(*_args, **_kwargs):
        raise AssertionError("use_sql must not be called for mixed-tenant field-map datasets")

    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda _tid, _llm_id: ["chat"])
    monkeypatch.setattr(
        dialog_service,
        "resolve_model_config",
        lambda *_args, **_kwargs: {"llm_factory": "unit", "max_tokens": 4096, "model_type": "chat"},
    )
    monkeypatch.setattr(dialog_service.TenantLangfuseService, "filter_by_tenant", lambda **_kwargs: None)
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda _dialog, **_kwargs: ([kb_a, kb_b], object(), None, chat_model, None),
    )
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_field_map", lambda _kb_ids: {"product": "Product Name"})
    monkeypatch.setattr(dialog_service, "use_sql", _unexpected_use_sql)
    monkeypatch.setattr(dialog_service, "label_question", lambda _question, _kbs: None)
    monkeypatch.setattr(
        dialog_service,
        "kb_prompt",
        lambda kbinfos, _max_tokens: ["Chunk text from dataset."] if kbinfos["chunks"] else [],
    )
    monkeypatch.setattr(dialog_service, "message_fit_in", lambda msg, _max_tokens: (0, msg))

    async def _collect():
        items = []
        async for item in dialog_service.async_chat(dialog, [{"role": "user", "content": "How many products?"}], stream=False):
            items.append(item)
        return items

    result = asyncio.run(_collect())

    assert len(retriever.calls) == 1
    call_args = retriever.calls[0]["args"]
    assert set(call_args[2]) == {"tenant-a", "tenant-b"}
    assert call_args[3] == ["kb-a", "kb-b"]
    assert result[0]["answer"] == "stub answer"
