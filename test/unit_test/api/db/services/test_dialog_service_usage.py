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


def test_usage_dict_shape():
    usage = dialog_service._usage_dict(3, 4, dialog_service.timer())
    assert usage["total_tokens"] == 7
    assert isinstance(usage["duration_ms"], float)
    assert SimpleNamespace(**usage).duration_ms >= 0
