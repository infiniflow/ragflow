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

<<<<<<< HEAD
=======
    def insert_citations(self, answer, *_args, **_kwargs):
        return answer, [0]


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
    assert len(chat_model.calls) == 2
    assert len(retriever.sql_calls) == 2


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
    monkeypatch.setattr(dialog_service.TenantLLMService, "llm_id2llm_type", lambda _llm_id: "chat")
    monkeypatch.setattr(
        dialog_service.TenantLLMService,
        "get_model_config",
        lambda *_args, **_kwargs: {"llm_factory": "unit", "max_tokens": 4096},
    )
    monkeypatch.setattr(dialog_service.TenantLangfuseService, "filter_by_tenant", lambda **_kwargs: None)
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda _dialog: ([SimpleNamespace(tenant_id="tenant-id")], object(), None, chat_model, None),
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
def test_async_ask_uses_all_docs_when_search_config_has_no_doc_ids(monkeypatch):
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
            "doc_aggs": [{"doc_id": "doc-1"}],
        }
    )

    class _StubStreamChatModel:
        max_length = 4096

        def async_chat_streamly_delta(self, *_args, **_kwargs):
            return object()

    def _fake_bundle(_tenant_id, model_config, *args, **kwargs):
        if model_config.get("model_type") == dialog_service.LLMType.CHAT:
            return _StubStreamChatModel()
        return SimpleNamespace()

    async def _fake_stream(_stream_iter):
        yield ("text", "partial", SimpleNamespace(full_text="final answer", endswith_think=False, buffer=""))

    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(
        dialog_service.KnowledgebaseService,
        "get_by_ids",
        lambda _kb_ids: [SimpleNamespace(tenant_id="tenant-1", embd_id="embd-1", parser_id="naive")],
    )
    monkeypatch.setattr(
        dialog_service,
        "get_model_config_by_type_and_name",
        lambda _tenant_id, model_type, _name: {"model_type": model_type},
    )
    monkeypatch.setattr(dialog_service, "LLMBundle", _fake_bundle)
    monkeypatch.setattr(dialog_service, "kb_prompt", lambda _kbinfos, _max_tokens: ["Chunk text from dataset."])
    monkeypatch.setattr(dialog_service, "label_question", lambda _question, _kbs: ["label-1"])
    monkeypatch.setattr(
        dialog_service,
        "PROMPT_JINJA_ENV",
        SimpleNamespace(from_string=lambda _template: SimpleNamespace(render=lambda **_kwargs: "system prompt")),
    )
    monkeypatch.setattr(dialog_service, "_stream_with_think_delta", _fake_stream)
    monkeypatch.setattr(dialog_service, "chunks_format", lambda refs: refs["chunks"])

    async def _collect():
        items = []
        async for item in dialog_service.async_ask("What does the dataset say?", ["kb-1"], "tenant-1", chat_llm_name="chat-model", search_config={}):
            items.append(item)
        return items

    result = asyncio.run(_collect())

    assert len(retriever.calls) == 1
    assert retriever.calls[0]["kwargs"]["doc_ids"] is None
    assert result[-1]["final"] is True


@pytest.mark.p2
def test_gen_mindmap_uses_all_docs_when_search_config_has_no_doc_ids(monkeypatch):
    retriever = _StubAsyncRetriever(
        {
            "total": 1,
            "chunks": [
                {
                    "chunk_id": "chunk-1",
                    "content_with_weight": "Chunk text from dataset.",
                }
            ],
        }
    )

    class _StubMindMapExtractor:
        def __init__(self, _chat_mdl):
            pass

        async def __call__(self, chunks):
            return SimpleNamespace(output={"mind_map": chunks})

    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(
        dialog_service.KnowledgebaseService,
        "get_by_ids",
        lambda _kb_ids: [SimpleNamespace(tenant_id="tenant-1", tenant_embd_id=None, embd_id="embd-1")],
    )
    monkeypatch.setattr(dialog_service, "get_model_config_by_type_and_name", lambda *_args, **_kwargs: {"model_type": "chat"})
    monkeypatch.setattr(dialog_service, "get_tenant_default_model_by_type", lambda *_args, **_kwargs: {"model_type": "chat"})
    monkeypatch.setattr(dialog_service, "LLMBundle", lambda *_args, **_kwargs: SimpleNamespace())
    monkeypatch.setattr(dialog_service, "label_question", lambda _question, _kbs: ["label-1"])
    monkeypatch.setattr(dialog_service, "MindMapExtractor", _StubMindMapExtractor)

    result = asyncio.run(dialog_service.gen_mindmap("What does the dataset say?", ["kb-1"], "tenant-1", search_config={}))

    assert len(retriever.calls) == 1
    assert retriever.calls[0]["kwargs"]["doc_ids"] is None
    assert result == {"mind_map": ["Chunk text from dataset."]}
