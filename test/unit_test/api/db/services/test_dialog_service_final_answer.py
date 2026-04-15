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
"""
Regression tests for the bug where async_ask() and async_chat() blanked out
final["answer"] in the last SSE event, discarding the decorated answer text
that contains citation markers.

Both functions call decorate_answer() which inserts citation markers and prunes
doc_aggs to cited documents, then overwrite final["answer"] = "" — discarding
the decorated text before the client receives it.

The fix removes those two blank-override lines. Tests here drive the actual
production functions (with heavy dependencies stubbed) to ensure regression
protection is real: the suite would fail if the lines were re-introduced.

Related: PR #13835 (async_chat), this PR (async_ask + async_chat).
"""

import asyncio
import sys
import types
import warnings
from copy import deepcopy
from types import SimpleNamespace

import pytest

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

    def _module_getattr(name):
        if name.isupper():
            return 0
        raise RuntimeError(f"cv2.{name} is unavailable in this test environment")

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub


_install_cv2_stub_if_unavailable()

from api.db.services import dialog_service  # noqa: E402


# ---------------------------------------------------------------------------
# Shared stubs
# ---------------------------------------------------------------------------

_KBINFOS = {
    "chunks": [
        {
            "doc_id": "doc-1",
            "content_ltks": "ragflow is a rag engine",
            "content_with_weight": "RAGFlow is a RAG engine.",
            "vector": [0.1, 0.2, 0.3],
            "docnm_kwd": "intro.pdf",
        },
    ],
    "doc_aggs": [{"doc_id": "doc-1", "doc_name": "intro.pdf", "count": 1}],
    "total": 1,
}

_KB = SimpleNamespace(
    id="kb-1",
    embd_id="text-embedding-ada-002@OpenAI",
    tenant_embd_id="text-embedding-ada-002@OpenAI",
    tenant_id="tenant-1",
    chunk_num=1,
    name="Test KB",
    parser_id="general",
)

_LLM_CONFIG = {
    "llm_name": "gpt-4o",
    "llm_factory": "OpenAI",
    "model_type": "chat",
    "max_tokens": 8192,
}


class _StreamingChatModel:
    """Yields a single-chunk full answer, no citations."""

    def __init__(self, answer: str):
        self.answer = answer
        self.max_length = 8192

    async def async_chat_streamly_delta(self, system_prompt, messages, gen_conf, **_kwargs):
        yield self.answer

    async def async_chat(self, system_prompt, messages, gen_conf, **_kwargs):
        return self.answer


class _StubRetriever:
    async def retrieval(self, *_args, **_kwargs):
        return deepcopy(_KBINFOS)

    def retrieval_by_children(self, chunks, tenant_ids):
        return chunks

    def insert_citations(self, answer, content_ltks, vectors, embd_mdl, **_kwargs):
        # Return the answer unchanged; no citation markers inserted.
        return answer, set()


def _collect(async_gen):
    async def _run():
        return [ev async for ev in async_gen]
    return asyncio.run(_run())


# ---------------------------------------------------------------------------
# Tests for async_ask  (production code path)
# ---------------------------------------------------------------------------

@pytest.mark.p2
def test_async_ask_final_event_carries_decorated_answer(monkeypatch):
    """
    Drive the real dialog_service.async_ask() and verify that the final SSE
    event (final=True) exposes the answer produced by decorate_answer(), not
    an empty string.

    Regression guard: if `final["answer"] = ""` is re-introduced at line ~1444,
    this test fails.
    """
    llm_answer = "RAGFlow is a RAG engine built for document understanding."
    chat_mdl = _StreamingChatModel(llm_answer)
    retriever = _StubRetriever()

    monkeypatch.setattr(
        dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [_KB]
    )
    monkeypatch.setattr(
        dialog_service, "get_model_config_by_type_and_name",
        lambda _tid, _type, _name: _LLM_CONFIG,
    )
    monkeypatch.setattr(dialog_service, "LLMBundle", lambda _tid, _cfg: chat_mdl)
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service.settings, "kg_retriever", retriever, raising=False)
    monkeypatch.setattr(
        dialog_service.DocMetadataService, "get_flatted_meta_by_kbs", lambda _ids: {}
    )
    monkeypatch.setattr(dialog_service, "label_question", lambda _q, _kbs: "")

    events = _collect(
        dialog_service.async_ask(
            question="What is RAGFlow?",
            kb_ids=["kb-1"],
            tenant_id="tenant-1",
        )
    )

    assert events, "async_ask must yield at least one event"

    final_events = [e for e in events if e.get("final") is True]
    assert len(final_events) == 1, (
        f"Expected exactly one final event, got {len(final_events)}: {final_events}"
    )
    final = final_events[0]

    assert final["answer"] != "", (
        "Final event answer must not be blank — decorate_answer() result was discarded.\n"
        "This is the regression: final['answer'] = '' was removed from async_ask()."
    )
    assert llm_answer in final["answer"], (
        f"LLM answer text expected in final event, got: {final['answer']!r}"
    )


@pytest.mark.p2
def test_async_ask_delta_events_carry_incremental_text_only(monkeypatch):
    """
    Intermediate delta events must have empty reference dicts.
    Only the final event should carry the populated reference from decorate_answer().
    """
    chat_mdl = _StreamingChatModel("Incremental text for delta test.")
    retriever = _StubRetriever()

    monkeypatch.setattr(
        dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [_KB]
    )
    monkeypatch.setattr(
        dialog_service, "get_model_config_by_type_and_name",
        lambda _tid, _type, _name: _LLM_CONFIG,
    )
    monkeypatch.setattr(dialog_service, "LLMBundle", lambda _tid, _cfg: chat_mdl)
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service.settings, "kg_retriever", retriever, raising=False)
    monkeypatch.setattr(
        dialog_service.DocMetadataService, "get_flatted_meta_by_kbs", lambda _ids: {}
    )
    monkeypatch.setattr(dialog_service, "label_question", lambda _q, _kbs: "")

    events = _collect(
        dialog_service.async_ask(
            question="Describe RAGFlow briefly.",
            kb_ids=["kb-1"],
            tenant_id="tenant-1",
        )
    )

    delta_events = [e for e in events if not e.get("final")]
    final_events  = [e for e in events if e.get("final") is True]

    assert len(final_events) == 1, f"Expected exactly one final event, got {len(final_events)}"
    for ev in delta_events:
        assert ev["reference"] == {}, f"Delta event must have empty reference, got: {ev['reference']}"

    assert "chunks" in final_events[0]["reference"], (
        "Final event reference must contain chunk data from decorate_answer()"
    )


# ---------------------------------------------------------------------------
# Tests for async_chat  (production code path)
# ---------------------------------------------------------------------------

def _make_dialog(chat_mdl_stub):
    """Build a minimal dialog SimpleNamespace for async_chat()."""
    return SimpleNamespace(
        id="dialog-1",
        kb_ids=["kb-1"],
        tenant_id="tenant-1",
        tenant_llm_id=None,
        llm_id="gpt-4o",
        llm_setting={"temperature": 0.1},
        prompt_type="simple",
        prompt_config={
            "system": "You are helpful. {knowledge}",
            "parameters": [{"key": "knowledge", "optional": False}],
            "quote": True,
            "empty_response": "",
            "reasoning": False,
            "refine_multiturn": False,
            "cross_languages": False,
            "keyword": False,
            "toc_enhance": False,
            "tavily_api_key": "",
            "use_kg": False,
            "tts": False,
        },
        meta_data_filter={},
        similarity_threshold=0.2,
        vector_similarity_weight=0.3,
        top_n=6,
        top_k=1024,
        rerank_id="",
    )


@pytest.mark.p2
def test_async_chat_final_event_carries_decorated_answer(monkeypatch):
    """
    Drive the real dialog_service.async_chat() streaming path and verify that
    the final SSE event (final=True) exposes the answer from decorate_answer(),
    not an empty string.

    Regression guard: if `final["answer"] = ""` is re-introduced at line ~774,
    this test fails.
    """
    llm_answer = "RAGFlow handles document parsing with deep understanding."
    chat_mdl = _StreamingChatModel(llm_answer)
    retriever = _StubRetriever()

    # Stub out the heavy service/model calls
    monkeypatch.setattr(
        dialog_service.TenantLLMService, "llm_id2llm_type", lambda _llm_id: "chat"
    )
    monkeypatch.setattr(
        dialog_service.TenantLLMService, "get_model_config",
        lambda _tid, _type, _llm_id: _LLM_CONFIG,
    )
    monkeypatch.setattr(
        dialog_service.TenantLangfuseService, "filter_by_tenant",
        lambda tenant_id: None,
    )
    # get_models returns (embd_mdl, chat_mdl, rerank_mdl, tts_mdl)
    monkeypatch.setattr(
        dialog_service, "get_models",
        lambda _dialog: (chat_mdl, chat_mdl, None, None),
    )
    monkeypatch.setattr(
        dialog_service.KnowledgebaseService, "get_field_map", lambda _kb_ids: {}
    )
    monkeypatch.setattr(
        dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [_KB]
    )
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service, "label_question", lambda _q, _kbs: "")
    monkeypatch.setattr(
        dialog_service, "kb_prompt",
        lambda _kbinfos, _max_tokens, **_kw: ["RAGFlow is a RAG engine."],
    )

    dialog = _make_dialog(chat_mdl)
    messages = [{"role": "user", "content": "What is RAGFlow?"}]

    events = _collect(dialog_service.async_chat(dialog, messages, stream=True, quote=True))

    final_events = [e for e in events if e.get("final") is True]
    assert len(final_events) == 1, (
        f"Expected exactly one final event, got {len(final_events)}: {final_events}"
    )
    final = final_events[0]

    assert final["answer"] != "", (
        "Final event answer must not be blank — decorate_answer() result was discarded.\n"
        "This is the regression: final['answer'] = '' was removed from async_chat()."
    )
    assert llm_answer in final["answer"], (
        f"LLM answer text expected in final event, got: {final['answer']!r}"
    )
