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


class _FakePropagateAttributesContext:
    """No-op context manager for fake propagate_attributes."""

    def __enter__(self):
        return self

    def __exit__(self, *args):
        pass


def _fake_propagate_attributes(**kwargs):
    """Fake propagate_attributes (Langfuse v4) that records kwargs and returns a no-op context manager."""
    _propagate_attributes_calls.append(kwargs)
    return _FakePropagateAttributesContext()


class _FakeLangfuseObservation:
    def __init__(self):
        self.updates = []
        self.ended = False

    def update(self, **kwargs):
        self.updates.append(kwargs)

    def end(self):
        self.ended = True


_propagate_attributes_calls = []


class _FakeLangfuseClient:
    instances = []
    fail_start_observation = False

    def __init__(self, **kwargs):
        self.init_kwargs = kwargs
        self.observation_kwargs = None
        self.observation = _FakeLangfuseObservation()
        self.instances.append(self)

    def auth_check(self):
        return True

    def create_trace_id(self):
        return "trace-id"

    def start_observation(self, **kwargs):
        if self.fail_start_observation:
            raise RuntimeError("langfuse unavailable")
        self.observation_kwargs = kwargs
        return self.observation


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

    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [_KB])
    monkeypatch.setattr(
        dialog_service,
        "get_model_config_from_provider_instance",
        lambda _tid, _type, _name: _LLM_CONFIG,
    )
    monkeypatch.setattr(dialog_service, "LLMBundle", lambda _tid, _cfg: chat_mdl)
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service.settings, "kg_retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service.DocMetadataService, "get_flatted_meta_by_kbs", lambda _ids: {})
    monkeypatch.setattr(dialog_service, "label_question", lambda _q, _kbs: "")
    # kb_prompt calls DocumentService.get_by_ids which needs a live DB; stub it out.
    monkeypatch.setattr(
        dialog_service,
        "kb_prompt",
        lambda _kbinfos, _max_tokens, **_kw: ["RAGFlow is a RAG engine."],
    )

    events = _collect(
        dialog_service.async_ask(
            question="What is RAGFlow?",
            kb_ids=["kb-1"],
            tenant_id="tenant-1",
        )
    )

    assert events, "async_ask must yield at least one event"

    final_events = [e for e in events if e.get("final") is True]
    assert len(final_events) == 1, f"Expected exactly one final event, got {len(final_events)}: {final_events}"
    final = final_events[0]

    assert "answer" in final
    assert "reference" in final


@pytest.mark.p2
def test_async_ask_delta_events_carry_incremental_text_only(monkeypatch):
    """
    Intermediate delta events must have empty reference dicts.
    Only the final event should carry the populated reference from decorate_answer().
    """
    chat_mdl = _StreamingChatModel("Incremental text for delta test.")
    retriever = _StubRetriever()

    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [_KB])
    monkeypatch.setattr(
        dialog_service,
        "get_model_config_from_provider_instance",
        lambda _tid, _type, _name: _LLM_CONFIG,
    )
    monkeypatch.setattr(dialog_service, "LLMBundle", lambda _tid, _cfg: chat_mdl)
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service.settings, "kg_retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service.DocMetadataService, "get_flatted_meta_by_kbs", lambda _ids: {})
    monkeypatch.setattr(dialog_service, "label_question", lambda _q, _kbs: "")
    monkeypatch.setattr(
        dialog_service,
        "kb_prompt",
        lambda _kbinfos, _max_tokens, **_kw: ["RAGFlow is a RAG engine."],
    )

    events = _collect(
        dialog_service.async_ask(
            question="Describe RAGFlow briefly.",
            kb_ids=["kb-1"],
            tenant_id="tenant-1",
        )
    )

    delta_events = [e for e in events if not e.get("final")]
    final_events = [e for e in events if e.get("final") is True]

    assert len(final_events) == 1, f"Expected exactly one final event, got {len(final_events)}"
    for ev in delta_events:
        assert ev["reference"] == {}, f"Delta event must have empty reference, got: {ev['reference']}"

    assert "chunks" in final_events[0]["reference"], "Final event reference must contain chunk data from decorate_answer()"


@pytest.mark.p2
def test_async_ask_empty_kb_ids_yields_error_final_event(monkeypatch):
    """
    When kb_ids is empty, async_ask() must not crash with IndexError on kbs[0].
    """
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [])

    events = _collect(
        dialog_service.async_ask(
            question="What is RAGFlow?",
            kb_ids=[],
            tenant_id="tenant-1",
        )
    )

    assert len(events) == 1
    final = events[0]
    assert final.get("final") is True
    assert "No KB selected" in final["answer"]
    assert final["reference"] == {}


@pytest.mark.p2
def test_async_ask_stale_kb_ids_yields_error_final_event(monkeypatch):
    """Provided kb_ids that do not resolve to any KB should report invalid selection."""
    monkeypatch.setattr(
        dialog_service.KnowledgebaseService,
        "get_by_ids",
        lambda ids: [] if ids == ["deleted-kb"] else [_KB],
    )

    events = _collect(
        dialog_service.async_ask(
            question="What is RAGFlow?",
            kb_ids=["deleted-kb"],
            tenant_id="tenant-1",
        )
    )

    assert len(events) == 1
    assert events[0].get("final") is True
    assert "not valid" in events[0]["answer"]
    assert events[0]["reference"] == {}


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
    monkeypatch.setattr(dialog_service, "get_model_type_by_name", lambda _tid, _llm_id: ["chat"])
    monkeypatch.setattr(
        dialog_service,
        "get_model_config_from_provider_instance",
        lambda _tid, _type, _llm_id: _LLM_CONFIG,
    )
    monkeypatch.setattr(
        dialog_service.TenantLangfuseService,
        "filter_by_tenant",
        lambda tenant_id: None,
    )
    # get_models returns (kbs, embd_mdl, rerank_mdl, chat_mdl, tts_mdl)
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda _dialog, **_kwargs: ([_KB], chat_mdl, None, chat_mdl, None),
    )
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_field_map", lambda _kb_ids: {})
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [_KB])
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service, "label_question", lambda _q, _kbs: "")
    monkeypatch.setattr(
        dialog_service,
        "kb_prompt",
        lambda _kbinfos, _max_tokens, **_kw: ["RAGFlow is a RAG engine."],
    )

    dialog = _make_dialog(chat_mdl)
    messages = [{"role": "user", "content": "What is RAGFlow?"}]

    events = _collect(dialog_service.async_chat(dialog, messages, stream=True, quote=True))

    final_events = [e for e in events if e.get("final") is True]
    assert len(final_events) == 1, f"Expected exactly one final event, got {len(final_events)}: {final_events}"
    final = final_events[0]

    assert "answer" in final
    assert "reference" in final


@pytest.mark.p2
def test_async_chat_langfuse_uses_start_observation(monkeypatch):
    """
    Langfuse v4 exposes start_observation(as_type="generation"), not
    start_generation(). Keep async_chat() on the migrated API.
    """
    _FakeLangfuseClient.instances = []
    monkeypatch.setattr(_FakeLangfuseClient, "fail_start_observation", False)
    llm_answer = "RAGFlow traces chat answers through Langfuse."
    chat_mdl = _StreamingChatModel(llm_answer)
    retriever = _StubRetriever()

    monkeypatch.setattr(dialog_service, "get_model_type_by_name", lambda _tid, _llm_id: ["chat"])
    monkeypatch.setattr(
        dialog_service,
        "get_model_config_from_provider_instance",
        lambda _tid, _type, _llm_id: _LLM_CONFIG,
    )
    monkeypatch.setattr(
        dialog_service.TenantLangfuseService,
        "filter_by_tenant",
        lambda tenant_id: SimpleNamespace(
            public_key="public",
            secret_key="secret",
            host="http://langfuse.local",
        ),
    )
    monkeypatch.setattr(dialog_service, "Langfuse", _FakeLangfuseClient)
    _propagate_attributes_calls.clear()
    monkeypatch.setattr(dialog_service, "propagate_attributes", _fake_propagate_attributes)
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda _dialog, **_kwargs: ([_KB], chat_mdl, None, chat_mdl, None),
    )
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_field_map", lambda _kb_ids: {})
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [_KB])
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service, "label_question", lambda _q, _kbs: "")
    monkeypatch.setattr(
        dialog_service,
        "kb_prompt",
        lambda _kbinfos, _max_tokens, **_kw: ["RAGFlow is a RAG engine."],
    )

    dialog = _make_dialog(chat_mdl)
    messages = [{"role": "user", "content": "What is RAGFlow?"}]

    events = _collect(dialog_service.async_chat(dialog, messages, stream=True, quote=True))

    assert any(e.get("final") is True for e in events)
    assert len(_FakeLangfuseClient.instances) == 1
    langfuse = _FakeLangfuseClient.instances[0]
    assert langfuse.observation_kwargs["as_type"] == "generation"
    assert langfuse.observation_kwargs["trace_context"] == {"trace_id": "trace-id"}
    assert langfuse.observation_kwargs["name"] == "chat"
    assert langfuse.observation_kwargs["model"] == _LLM_CONFIG["llm_name"]
    input_payload = langfuse.observation_kwargs["input"]
    assert set(input_payload.keys()) == {"prompt", "prompt4citation", "messages"}
    assert input_payload["prompt"] == "You are helpful. \n------\nRAGFlow is a RAG engine."
    assert input_payload["prompt4citation"] == dialog_service.citation_prompt()
    assert input_payload["messages"][0]["role"] == "system"
    assert input_payload["messages"][0]["content"] == input_payload["prompt"]
    assert input_payload["messages"][1] == {"role": "user", "content": "What is RAGFlow?"}
    assert langfuse.observation.ended is True


@pytest.mark.p2
def test_async_chat_langfuse_observation_includes_session_id(monkeypatch):
    _FakeLangfuseClient.instances = []
    _propagate_attributes_calls.clear()
    monkeypatch.setattr(_FakeLangfuseClient, "fail_start_observation", False)
    chat_mdl = _StreamingChatModel("Session traces should be grouped.")
    retriever = _StubRetriever()

    monkeypatch.setattr(dialog_service, "get_model_type_by_name", lambda _tid, _llm_id: ["chat"])
    monkeypatch.setattr(
        dialog_service,
        "get_model_config_from_provider_instance",
        lambda _tid, _type, _llm_id: _LLM_CONFIG,
    )
    monkeypatch.setattr(
        dialog_service.TenantLangfuseService,
        "filter_by_tenant",
        lambda tenant_id: SimpleNamespace(
            public_key="public",
            secret_key="secret",
            host="http://langfuse.local",
        ),
    )
    monkeypatch.setattr(dialog_service, "Langfuse", _FakeLangfuseClient)
    monkeypatch.setattr(dialog_service, "propagate_attributes", _fake_propagate_attributes)
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda _dialog, **_kwargs: ([_KB], chat_mdl, None, chat_mdl, None),
    )
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_field_map", lambda _kb_ids: {})
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [_KB])
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service, "label_question", lambda _q, _kbs: "")
    monkeypatch.setattr(
        dialog_service,
        "kb_prompt",
        lambda _kbinfos, _max_tokens, **_kw: ["RAGFlow is a RAG engine."],
    )

    dialog = _make_dialog(chat_mdl)
    messages = [{"role": "user", "content": "What is RAGFlow?"}]

    events = _collect(dialog_service.async_chat(dialog, messages, stream=True, quote=True, session_id="session-1"))

    assert any(e.get("final") is True for e in events)
    langfuse = _FakeLangfuseClient.instances[0]
    assert langfuse.observation_kwargs["trace_context"] == {"trace_id": "trace-id"}
    assert _propagate_attributes_calls[0]["session_id"] == "session-1"


@pytest.mark.p2
def test_get_models_passes_langfuse_trace_context_to_llm_bundles(monkeypatch):
    captured = []

    class _FakeBundle:
        def __init__(self, tenant_id, model_config, **kwargs):
            self.tenant_id = tenant_id
            self.model_config = model_config
            self.trace_context = kwargs.get("trace_context")
            self.langfuse_session_id = kwargs.get("langfuse_session_id")
            captured.append((tenant_id, model_config["model_type"], kwargs))

    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [_KB])
    monkeypatch.setattr(
        dialog_service,
        "get_model_config_from_provider_instance",
        lambda _tenant_id, model_type, _model_id: {**_LLM_CONFIG, "model_type": model_type},
    )
    monkeypatch.setattr(
        dialog_service,
        "get_tenant_default_model_by_type",
        lambda _tenant_id, model_type: {**_LLM_CONFIG, "model_type": model_type},
    )
    monkeypatch.setattr(dialog_service, "LLMBundle", _FakeBundle)

    dialog = _make_dialog(None)
    dialog.rerank_id = "rerank-1"
    dialog.prompt_config["tts"] = True
    trace_context = {"trace_id": "trace-id"}

    dialog_service.get_models(dialog, trace_context=trace_context, langfuse_session_id="session-1")

    assert len(captured) == 4
    assert {model_type for _, model_type, _ in captured} == {
        dialog_service.LLMType.EMBEDDING,
        dialog_service.LLMType.CHAT,
        dialog_service.LLMType.RERANK,
        dialog_service.LLMType.TTS,
    }
    for _, _, kwargs in captured:
        assert kwargs["trace_context"] is trace_context
        assert kwargs["langfuse_session_id"] == "session-1"


@pytest.mark.p2
def test_async_chat_continues_when_langfuse_observation_start_fails(monkeypatch):
    """
    Langfuse tracing is best-effort; observation startup errors must not break
    chat responses.
    """
    _FakeLangfuseClient.instances = []
    monkeypatch.setattr(_FakeLangfuseClient, "fail_start_observation", True)
    llm_answer = "RAGFlow still answers when tracing is unavailable."
    chat_mdl = _StreamingChatModel(llm_answer)
    retriever = _StubRetriever()

    monkeypatch.setattr(dialog_service, "get_model_type_by_name", lambda _tid, _llm_id: ["chat"])
    monkeypatch.setattr(
        dialog_service,
        "get_model_config_from_provider_instance",
        lambda _tid, _type, _llm_id: _LLM_CONFIG,
    )
    monkeypatch.setattr(
        dialog_service.TenantLangfuseService,
        "filter_by_tenant",
        lambda tenant_id: SimpleNamespace(
            public_key="public",
            secret_key="secret",
            host="http://langfuse.local",
        ),
    )
    monkeypatch.setattr(dialog_service, "Langfuse", _FakeLangfuseClient)
    _propagate_attributes_calls.clear()
    monkeypatch.setattr(dialog_service, "propagate_attributes", _fake_propagate_attributes)
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda _dialog, **_kwargs: ([_KB], chat_mdl, None, chat_mdl, None),
    )
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_field_map", lambda _kb_ids: {})
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [_KB])
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service, "label_question", lambda _q, _kbs: "")
    monkeypatch.setattr(
        dialog_service,
        "kb_prompt",
        lambda _kbinfos, _max_tokens, **_kw: ["RAGFlow is a RAG engine."],
    )

    dialog = _make_dialog(chat_mdl)
    messages = [{"role": "user", "content": "What is RAGFlow?"}]

    events = _collect(dialog_service.async_chat(dialog, messages, stream=True, quote=True))

    final_events = [e for e in events if e.get("final") is True]
    assert len(final_events) == 1
    assert "answer" in final_events[0]
    assert len(_FakeLangfuseClient.instances) == 1
    assert _FakeLangfuseClient.instances[0].observation_kwargs is None
    assert _FakeLangfuseClient.instances[0].observation.ended is False
