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
Regression tests for rag_agent() routing image attachments away from the
agentic reasoning loop.

The agentic loop composes its final answer from retrieval results only (the
terminal `rag` tool streams the inner graph's cited answer), so an image
attached to the question could never influence it: the user got the
knowledge-base "not found" response instead of a description of the image.
When the dialog's chat model is vision-capable and the last user message
carries image attachments, rag_agent() must answer through async_chat(),
which embeds the images into the model messages.
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
from test_dialog_service_rag_agent_messages import (  # noqa: E402
    _KB,
    _RecordingChatModel,
    _StubRAGTools,
)


_IMAGE_FILE_DICT = {"id": "img-1", "name": "photo.jpeg", "mime_type": "image/jpeg", "created_by": "tenant-1"}


class _RecordingAsyncChat:
    """Stands in for async_chat and records that the plain path was taken."""

    def __init__(self):
        self.calls = []

    async def __call__(self, dialog, messages, stream=True, **kwargs):
        self.calls.append((dialog, messages, stream, kwargs))
        yield {"answer": "described the image", "reference": {}, "final": True}


def _dialog(llm_id="qwen3-vl-plus@Tongyi-Qianwen"):
    return SimpleNamespace(
        id="dialog-1",
        tenant_id="tenant-1",
        kb_ids=["kb-1"],
        llm_id=llm_id,
        llm_setting={"temperature": 0.1},
        prompt_config={},
        meta_data_filter=None,
    )


def _collect(gen):
    async def _run():
        return [item async for item in gen]

    return asyncio.run(_run())


def _patch_agentic_stubs(monkeypatch):
    chat_mdl = _RecordingChatModel()
    monkeypatch.setattr(dialog_service, "get_models", lambda _dialog, **_kw: ([_KB], None, None, chat_mdl, None))
    monkeypatch.setattr(dialog_service, "RAGTools", _StubRAGTools)
    return chat_mdl


@pytest.mark.p2
def test_rag_agent_routes_vision_image_questions_to_async_chat(monkeypatch):
    """Reasoning on + image attachment + vision-capable model must bypass the
    agentic loop (whose answer is retrieval-only) and use async_chat."""
    recording_chat = _RecordingAsyncChat()
    monkeypatch.setattr(dialog_service, "async_chat", recording_chat)

    def _fail_get_models(*_args, **_kwargs):
        raise AssertionError("agentic path must not run for vision image questions")

    monkeypatch.setattr(dialog_service, "get_models", _fail_get_models)
    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda _t, _m: ["chat", "vision"])

    messages = [{"role": "user", "content": "描述图片内容", "id": "m-1", "files": [dict(_IMAGE_FILE_DICT)]}]
    dialog = _dialog()

    results = _collect(dialog_service.rag_agent(dialog, messages, True, reasoning=1))

    assert len(recording_chat.calls) == 1
    called_dialog, called_messages, _stream, kwargs = recording_chat.calls[0]
    assert called_dialog is dialog
    assert called_messages is messages
    assert kwargs.get("reasoning") == 1
    assert results[-1]["answer"] == "described the image"


@pytest.mark.p2
def test_rag_agent_keeps_agentic_loop_for_text_only_model_with_images(monkeypatch):
    """Images attached to a text-only model cannot be answered either way, so
    the agentic loop must keep running."""
    recording_chat = _RecordingAsyncChat()
    monkeypatch.setattr(dialog_service, "async_chat", recording_chat)
    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda _t, _m: ["chat"])
    _patch_agentic_stubs(monkeypatch)
    monkeypatch.setattr(
        dialog_service,
        "get_files_content",
        lambda _last_message, _model_type: ("", ["data:image/jpeg;base64,QUJD"], []),
    )

    messages = [{"role": "user", "content": "描述图片内容", "id": "m-1", "files": [dict(_IMAGE_FILE_DICT)]}]

    results = _collect(dialog_service.rag_agent(_dialog("glm-4-flash@Zhipu"), messages, False, reasoning=1))

    assert recording_chat.calls == [], "agentic path must not delegate to async_chat for text-only models"
    assert results, "agentic path must produce an answer"


@pytest.mark.p2
def test_rag_agent_keeps_agentic_loop_without_images(monkeypatch):
    """Pure text questions with reasoning keep the agentic research loop."""
    recording_chat = _RecordingAsyncChat()
    monkeypatch.setattr(dialog_service, "async_chat", recording_chat)
    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda _t, _m: ["chat", "vision"])
    _patch_agentic_stubs(monkeypatch)

    messages = [{"role": "user", "content": "What is RAGFlow?", "id": "m-1"}]

    results = _collect(dialog_service.rag_agent(_dialog(), messages, False, reasoning=1))

    assert recording_chat.calls == []
    assert results


@pytest.mark.p2
def test_message_has_image_attachments_covers_client_shapes():
    assert dialog_service.message_has_image_attachments({"files": [_IMAGE_FILE_DICT]})
    assert dialog_service.message_has_image_attachments({"files": ["data:image/png;base64,AAA"]})
    assert dialog_service.message_has_image_attachments({"files": {"mime_type": "image/png"}})
    assert not dialog_service.message_has_image_attachments({"files": [{"mime_type": "application/pdf"}]})
    assert not dialog_service.message_has_image_attachments({"files": ["https://example.com/a.png"]})
    assert not dialog_service.message_has_image_attachments({})
    assert not dialog_service.message_has_image_attachments({"files": "not-a-list"})


@pytest.mark.p2
def test_dialog_model_vision_capable_resolves_enrolled_types(monkeypatch):
    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda _t, _m: ["chat", "vision"])
    assert dialog_service.dialog_model_vision_capable(_dialog()) is True

    monkeypatch.setattr(dialog_service, "resolve_model_type", lambda _t, _m: ["chat"])
    assert dialog_service.dialog_model_vision_capable(_dialog()) is False

    def _raise(_t, _m):
        raise LookupError("no such model")

    monkeypatch.setattr(dialog_service, "resolve_model_type", _raise)
    assert dialog_service.dialog_model_vision_capable(_dialog()) is False

    assert dialog_service.dialog_model_vision_capable(SimpleNamespace(tenant_id="t1", llm_id="")) is False

