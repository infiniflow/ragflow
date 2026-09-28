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
Every final answer produced by the chat services must carry a ``usage`` block
(``prompt_tokens``, ``completion_tokens``, ``total_tokens``, ``duration_ms``)
so that ``async_iframe_completion`` can persist tokens/duration on the
``api_4_conversation`` row instead of always writing zeros.
"""

from types import SimpleNamespace

import pytest

from test_dialog_service_final_answer import _KB, _LLM_CONFIG, _StreamingChatModel, _StubRetriever, _collect, _make_dialog
from test_dialog_service_rag_agent_messages import _DIALOG as _AGENT_DIALOG
from test_dialog_service_rag_agent_messages import _KB as _AGENT_KB
from test_dialog_service_rag_agent_messages import _RecordingChatModel

from api.db.services import dialog_service

_USAGE_KEYS = {"prompt_tokens", "completion_tokens", "total_tokens", "duration_ms"}


def _assert_usage(usage, *, expect_completion_tokens=True):
    assert set(usage.keys()) == _USAGE_KEYS
    assert usage["total_tokens"] == usage["prompt_tokens"] + usage["completion_tokens"]
    assert usage["duration_ms"] >= 0
    if expect_completion_tokens:
        assert usage["completion_tokens"] > 0
        assert usage["prompt_tokens"] > 0


def _stub_solo_dependencies(monkeypatch, chat_mdl):
    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda _tid, _llm_id: ["chat"])
    monkeypatch.setattr(dialog_service, "resolve_model_config", lambda _tid, _type, _llm_id: _LLM_CONFIG)
    monkeypatch.setattr(dialog_service, "LLMBundle", lambda *_args, **_kwargs: chat_mdl)


def _make_solo_dialog():
    dialog = _make_dialog(None)
    dialog.kb_ids = []
    dialog.prompt_config["system"] = "You are helpful."
    return dialog


@pytest.mark.p2
@pytest.mark.parametrize("stream", [True, False])
def test_async_chat_solo_final_event_carries_usage(monkeypatch, stream):
    chat_mdl = _StreamingChatModel("RAGFlow is a RAG engine with deep document understanding.")
    _stub_solo_dependencies(monkeypatch, chat_mdl)

    events = _collect(dialog_service.async_chat_solo(_make_solo_dialog(), [{"role": "user", "content": "What is RAGFlow?"}], stream=stream))

    final_events = [e for e in events if e.get("final", True)]
    assert len(final_events) == 1, final_events
    _assert_usage(final_events[0]["usage"])
    # Intermediate deltas never carry usage.
    assert all("usage" not in e for e in events if e.get("final") is False)


@pytest.mark.p2
def test_async_chat_empty_response_carries_usage(monkeypatch):
    """No knowledge retrieved + configured empty_response: still a final event with usage."""
    chat_mdl = _StreamingChatModel("unused")

    class _EmptyRetriever(_StubRetriever):
        async def retrieval(self, *_args, **_kwargs):
            return {"chunks": [], "doc_aggs": [], "total": 0}

    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda _tid, _llm_id: ["chat"])
    monkeypatch.setattr(dialog_service, "resolve_model_config", lambda _tid, _type, _llm_id: _LLM_CONFIG)
    monkeypatch.setattr(dialog_service.TenantLangfuseService, "filter_by_tenant", lambda tenant_id: None)
    monkeypatch.setattr(dialog_service, "get_models", lambda _dialog, **_kwargs: ([_KB], chat_mdl, None, chat_mdl, None))
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_field_map", lambda _kb_ids: {})
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [_KB])
    monkeypatch.setattr(dialog_service.settings, "retriever", _EmptyRetriever(), raising=False)
    monkeypatch.setattr(dialog_service, "label_question", lambda _q, _kbs: "")
    monkeypatch.setattr(dialog_service, "kb_prompt", lambda _kbinfos, _max_tokens, **_kw: [])

    dialog = _make_dialog(chat_mdl)
    dialog.prompt_config["empty_response"] = "Sorry, nothing found."

    events = _collect(dialog_service.async_chat(dialog, [{"role": "user", "content": "What is RAGFlow?"}], stream=True, quote=True))

    final_events = [e for e in events if e.get("final") is True]
    assert len(final_events) == 1, final_events
    assert "Sorry, nothing found." in final_events[0]["answer"]
    _assert_usage(final_events[0]["usage"], expect_completion_tokens=False)
    assert final_events[0]["usage"]["total_tokens"] == 0


@pytest.mark.p2
@pytest.mark.parametrize("stream", [True, False])
def test_async_chat_final_event_carries_usage(monkeypatch, stream):
    chat_mdl = _StreamingChatModel("RAGFlow handles document parsing with deep understanding.")
    retriever = _StubRetriever()

    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda _tid, _llm_id: ["chat"])
    monkeypatch.setattr(dialog_service, "resolve_model_config", lambda _tid, _type, _llm_id: _LLM_CONFIG)
    monkeypatch.setattr(dialog_service.TenantLangfuseService, "filter_by_tenant", lambda tenant_id: None)
    monkeypatch.setattr(dialog_service, "get_models", lambda _dialog, **_kwargs: ([_KB], chat_mdl, None, chat_mdl, None))
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_field_map", lambda _kb_ids: {})
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_by_ids", lambda _ids: [_KB])
    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(dialog_service, "label_question", lambda _q, _kbs: "")
    monkeypatch.setattr(dialog_service, "kb_prompt", lambda _kbinfos, _max_tokens, **_kw: ["RAGFlow is a RAG engine."])

    events = _collect(dialog_service.async_chat(_make_dialog(chat_mdl), [{"role": "user", "content": "What is RAGFlow?"}], stream=stream, quote=True))

    final_events = [e for e in events if e.get("final", True)]
    assert len(final_events) == 1, final_events
    _assert_usage(final_events[0]["usage"])


def test_prompt_tokens_counts_text_blocks_in_structured_content():
    plain = [{"role": "user", "content": "What is RAGFlow?"}]
    structured = [
        {
            "role": "user",
            "content": [
                {"type": "text", "text": "What is RAGFlow?"},
                {"type": "image_url", "image_url": {"url": "data:image/png;base64,AAAA"}},
            ],
        }
    ]
    assert dialog_service._prompt_tokens("sys", plain) > dialog_service._prompt_tokens("sys", [])
    assert dialog_service._prompt_tokens("sys", structured) == dialog_service._prompt_tokens("sys", plain)


@pytest.mark.p2
def test_async_chat_solo_usage_counts_multimodal_prompt(monkeypatch):
    chat_mdl = _StreamingChatModel("A picture of the RAGFlow logo.")
    _stub_solo_dependencies(monkeypatch, chat_mdl)
    # Turn the last user message into content blocks, as the multimodal path does.
    monkeypatch.setattr(
        dialog_service,
        "convert_last_user_msg_to_multimodal",
        lambda msg, _uris, _factory: msg.__setitem__(
            -1, {"role": "user", "content": [{"type": "text", "text": msg[-1]["content"]}, {"type": "image_url", "image_url": {"url": "data:image/png;base64,AAAA"}}]}
        ),
    )
    monkeypatch.setattr(dialog_service, "get_files_content", lambda _m, _t: ("", ["data:image/png;base64,AAAA"], []))

    events = _collect(dialog_service.async_chat_solo(_make_solo_dialog(), [{"role": "user", "content": "What is on this picture?"}], stream=False))

    usage = events[-1]["usage"]
    assert usage["prompt_tokens"] == dialog_service._prompt_tokens("You are helpful.", [{"role": "user", "content": "What is on this picture?"}])
    assert usage["prompt_tokens"] > 0


# ---------------------------------------------------------------------------
# rag_agent (agentic path): outer model + inner graph, each counted once
# ---------------------------------------------------------------------------

_AGENT_ANSWER = "RAGFlow is a RAG engine."
_AGENT_QUESTION = [{"role": "user", "content": "What is RAGFlow?"}]


def _make_stub_rag_tools(inner_prompt: dict | None, inner_completion: dict | None):
    class _StubRAGTools:
        def __init__(self, *_args, **_kwargs):
            self.kbinfos = {"chunks": [], "doc_aggs": []}
            self.tools = []
            if inner_prompt is not None:
                self.llm_stats = SimpleNamespace(prompt_tokens=dict(inner_prompt), completion_tokens=dict(inner_completion))

        def sys_prompt(self):
            return "You are a helpful assistant."

    return _StubRAGTools


def _drive_rag_agent_usage(monkeypatch, chat_mdl, rag_tools_cls):
    monkeypatch.setattr(dialog_service, "get_models", lambda _dialog, **_kw: ([_AGENT_KB], None, None, chat_mdl, None))
    monkeypatch.setattr(dialog_service, "RAGTools", rag_tools_cls)
    monkeypatch.setattr(dialog_service, "tts", lambda _mdl, _text: None)
    events = _collect(dialog_service.rag_agent(_AGENT_DIALOG, list(_AGENT_QUESTION), False, reasoning="2"))
    assert len(events) == 1, events
    return events[0]["usage"]


@pytest.mark.p2
def test_rag_agent_usage_does_not_count_inner_answer_twice(monkeypatch):
    """Inner graph recorded usage, outer model did not: the answer text is NOT re-estimated."""
    chat_mdl = _RecordingChatModel()
    rag_tools_cls = _make_stub_rag_tools({"planner": 100, "agent": 250}, {"planner": 20, "agent": 80})

    usage = _drive_rag_agent_usage(monkeypatch, chat_mdl, rag_tools_cls)

    expected_outer_prompt = dialog_service._prompt_tokens("You are a helpful assistant.", _AGENT_QUESTION)
    assert usage["prompt_tokens"] == expected_outer_prompt + 350
    assert usage["completion_tokens"] == 100
    assert usage["total_tokens"] == usage["prompt_tokens"] + usage["completion_tokens"]


@pytest.mark.p2
def test_rag_agent_usage_adds_outer_provider_usage_to_inner_counters(monkeypatch):
    """Both sides reported usage: exact sum, no tokenizer estimate involved."""
    chat_mdl = _RecordingChatModel()
    chat_mdl.mdl = SimpleNamespace(last_usage={"prompt_tokens": 40, "completion_tokens": 15, "total_tokens": 55})
    rag_tools_cls = _make_stub_rag_tools({"agent": 300}, {"agent": 60})

    usage = _drive_rag_agent_usage(monkeypatch, chat_mdl, rag_tools_cls)

    assert usage == {"prompt_tokens": 340, "completion_tokens": 75, "total_tokens": 415, "duration_ms": usage["duration_ms"]}
    assert chat_mdl.mdl.terminal_tools == {"rag"}


@pytest.mark.p2
def test_rag_agent_usage_falls_back_to_estimates_when_nothing_recorded(monkeypatch):
    """No provider usage anywhere (or no llm_stats at all): estimate prompt and answer from text."""
    chat_mdl = _RecordingChatModel()
    rag_tools_cls = _make_stub_rag_tools(None, None)

    usage = _drive_rag_agent_usage(monkeypatch, chat_mdl, rag_tools_cls)

    assert usage["prompt_tokens"] == dialog_service._prompt_tokens("You are a helpful assistant.", _AGENT_QUESTION)
    assert usage["completion_tokens"] == dialog_service.num_tokens_from_string(_AGENT_ANSWER)
    assert usage["completion_tokens"] > 0


def test_usage_dict_shape():
    usage = dialog_service._usage_dict(3, 4, dialog_service.timer())
    assert usage["total_tokens"] == 7
    assert isinstance(usage["duration_ms"], float)
    assert SimpleNamespace(**usage).duration_ms >= 0
