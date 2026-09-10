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
Regression tests for rag_agent() forwarding message bookkeeping keys to the LLM.

Stored and client-supplied messages carry keys the chat schema does not define
(`id`, `created_at`, and `conversationId` from the web client). rag_agent() used
to deepcopy the message list wholesale, so providers that validate message
properties strictly, such as Groq, rejected the request.
"""

import asyncio
import sys
import types
import warnings
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


_DIALOG = SimpleNamespace(
    id="dialog-1",
    tenant_id="tenant-1",
    kb_ids=["kb-1"],
    llm_id="gpt-4o@OpenAI",
    llm_setting={"temperature": 0.1},
    prompt_config={"reasoning": 1},
    meta_data_filter=None,
    similarity_threshold=0.2,
    vector_similarity_weight=0.3,
    top_n=6,
    rerank_candidates_count=64,
    top_k=1024,
)

_KB = SimpleNamespace(id="kb-1", tenant_id="tenant-1")


class _RecordingChatModel:
    """Records the message list rag_agent() hands to the provider."""

    def __init__(self):
        self.is_tools = True
        self.model_config = {"model_type": "chat", "llm_factory": "OpenAI"}
        self.mdl = None
        self.sent_messages = None

    def bind_tools(self, toolcall_session, tools):
        pass

    async def async_chat(self, system_prompt, messages, gen_conf, **_kwargs):
        self.sent_messages = messages
        return "RAGFlow is a RAG engine."


class _StubRAGTools:
    def __init__(self, *_args, **_kwargs):
        self.kbinfos = {"chunks": [], "doc_aggs": []}
        self.tools = []

    def sys_prompt(self):
        return "You are a helpful assistant."


def _drive_rag_agent(monkeypatch, messages):
    chat_mdl = _RecordingChatModel()
    monkeypatch.setattr(dialog_service, "get_models", lambda _dialog, **_kw: ([_KB], None, None, chat_mdl, None))
    monkeypatch.setattr(dialog_service, "RAGTools", _StubRAGTools)
    monkeypatch.setattr(dialog_service, "tts", lambda _mdl, _text: None)

    async def _run():
        return [ev async for ev in dialog_service.rag_agent(_DIALOG, messages, False, reasoning="2")]

    events = asyncio.run(_run())
    assert events, "rag_agent must yield an answer event"
    return chat_mdl


@pytest.mark.p2
def test_rag_agent_falls_back_to_retrieval_when_model_has_no_tools(monkeypatch):
    """Reasoning must not bypass KB retrieval for models without tool support."""

    class _NoToolsChatModel(_RecordingChatModel):
        def __init__(self):
            super().__init__()
            self.is_tools = False

    chat_mdl = _NoToolsChatModel()
    fallback_calls = []

    async def _fallback(_dialog, _messages, stream=True, **_kwargs):
        fallback_calls.append((stream, _kwargs))
        yield {"answer": "retrieved answer", "reference": {"chunks": [{"doc_id": "doc-1"}]}}

    monkeypatch.setattr(dialog_service, "get_models", lambda _dialog, **_kw: ([_KB], None, None, chat_mdl, None))
    monkeypatch.setattr(dialog_service, "async_chat", _fallback)

    async def _run():
        return [
            event
            async for event in dialog_service.rag_agent(
                _DIALOG,
                [{"role": "user", "content": "What is RAGFlow?"}],
                False,
                reasoning="2",
                doc_ids=["doc-1"],
            )
        ]

    events = asyncio.run(_run())

    assert fallback_calls == [(False, {"reasoning": "2", "doc_ids": "doc-1"})]
    assert events[0]["answer"] == "retrieved answer"


@pytest.mark.p2
def test_rag_agent_drops_bookkeeping_keys_before_the_provider_call(monkeypatch):
    """Groq rejects any message property the chat schema does not define."""
    messages = [
        {"role": "assistant", "content": "Hi! How can I help?", "id": "m-1", "created_at": 1755900000.0},
        {"role": "user", "content": "What is RAGFlow?", "id": "m-2", "conversationId": "conv-1", "doc_ids": ["doc-1"]},
    ]

    chat_mdl = _drive_rag_agent(monkeypatch, messages)

    assert chat_mdl.sent_messages == [
        {"role": "assistant", "content": "Hi! How can I help?"},
        {"role": "user", "content": "What is RAGFlow?"},
    ]


@pytest.mark.p2
def test_rag_agent_keeps_tool_call_protocol_fields(monkeypatch):
    """`tool_calls` and `tool_call_id` belong to the chat schema and pair the two messages together."""
    tool_calls = [{"id": "call_1", "type": "function", "function": {"name": "weather", "arguments": "{}"}}]
    messages = [
        {"role": "user", "content": "Get the weather.", "id": "m-1"},
        {"role": "assistant", "content": None, "tool_calls": tool_calls, "id": "m-2"},
        {"role": "tool", "tool_call_id": "call_1", "content": '{"temp_c": 20}', "id": "m-3"},
        {"role": "user", "content": "Should I take a coat?", "id": "m-4"},
    ]

    chat_mdl = _drive_rag_agent(monkeypatch, messages)

    assert chat_mdl.sent_messages[1] == {"role": "assistant", "content": None, "tool_calls": tool_calls}
    assert chat_mdl.sent_messages[2] == {"role": "tool", "tool_call_id": "call_1", "content": '{"temp_c": 20}'}


@pytest.mark.p2
def test_rag_agent_leaves_the_caller_messages_untouched(monkeypatch):
    """chat_api persists the same dicts after the answer, so the keys must survive the call."""
    messages = [{"role": "user", "content": "What is RAGFlow?", "id": "m-1", "conversationId": "conv-1"}]

    _drive_rag_agent(monkeypatch, messages)

    assert messages[0]["id"] == "m-1"
    assert messages[0]["conversationId"] == "conv-1"


@pytest.mark.p2
def test_rag_agent_preserves_multimodal_content_parts(monkeypatch):
    """`/chat/completions` accepts content-part arrays, so content must be passed through as-is."""
    content = [{"type": "text", "text": "Describe this image"}, {"type": "image_url", "image_url": {"url": "data:image/png;base64,AAAA"}}]
    messages = [{"role": "user", "content": content, "id": "m-1"}]

    chat_mdl = _drive_rag_agent(monkeypatch, messages)

    assert chat_mdl.sent_messages[0]["content"] == content


@pytest.mark.p2
def test_render_reasoning_system_prompt_substitutes_date_and_knowledge():
    """The reasoning path should honor the dialog system prompt like async_chat does."""
    dialog = SimpleNamespace(kb_ids=["kb-1"])
    prompt_config = {"system": "Role: pirate. Date: {date}. Context: '{knowledge}'."}
    kwargs = {}

    rendered = dialog_service._render_reasoning_system_prompt(dialog, prompt_config, kwargs)

    assert rendered.startswith("Role: pirate. Date: 2")
    assert "Context: ''." in rendered


@pytest.mark.p2
def test_render_reasoning_system_prompt_replaces_optional_missing_parameters():
    """Optional parameters missing from kwargs are blanked out, not left as placeholders."""
    dialog = SimpleNamespace(kb_ids=[])
    prompt_config = {
        "system": "Lang: {language}.",
        "parameters": [{"key": "language", "optional": True}],
    }
    kwargs = {"date": "2026-08-27"}

    rendered = dialog_service._render_reasoning_system_prompt(dialog, prompt_config, kwargs)

    # Missing optional parameters are replaced with a space, matching async_chat.
    assert rendered == "Lang:  ."
