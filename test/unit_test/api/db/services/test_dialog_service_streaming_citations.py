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
from copy import deepcopy
from types import SimpleNamespace

import pytest

pytestmark = [
    pytest.mark.filterwarnings("ignore:pkg_resources is deprecated as an API.*:UserWarning"),
]


def _install_cv2_stub_if_unavailable():
    original_cv2 = sys.modules.get("cv2")
    try:
        import cv2  # noqa: F401
        return original_cv2, False
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

    def _missing(*_args, **_kwargs):
        raise RuntimeError("cv2 runtime call is unavailable in this test environment")

    def _module_getattr(name):
        if name.isupper():
            return 0
        return _missing

    stub.__getattr__ = _module_getattr
    sys.modules["cv2"] = stub
    return original_cv2, True


_ORIGINAL_CV2, _INSTALLED_CV2_STUB = _install_cv2_stub_if_unavailable()

from api.db.services import dialog_service


@pytest.fixture(scope="module", autouse=True)
def _restore_cv2_module_state():
    yield
    if _INSTALLED_CV2_STUB:
        if _ORIGINAL_CV2 is None:
            sys.modules.pop("cv2", None)
        else:
            sys.modules["cv2"] = _ORIGINAL_CV2

class _StubStreamingChatModel:
    def __init__(self, *, streamed_outputs, final_answer):
        self.streamed_outputs = streamed_outputs
        self.final_answer = final_answer

    async def async_chat(self, _system_prompt, _messages, _llm_setting, **_kwargs):
        return self.final_answer

    async def async_chat_streamly_delta(self, _system_prompt, _messages, _llm_setting, **_kwargs):
        for chunk in self.streamed_outputs:
            yield chunk


class _StubCitationRetriever:
    def __init__(self, retrieval_result, decorated_answer, cited_indexes):
        self.retrieval_result = retrieval_result
        self.decorated_answer = decorated_answer
        self.cited_indexes = cited_indexes
        self.insert_citations_calls = []
        self.retrieval_calls = []

    async def retrieval(self, *args, **kwargs):
        self.retrieval_calls.append({"args": args, "kwargs": kwargs})
        return deepcopy(self.retrieval_result)

    def retrieval_by_children(self, chunks, _tenant_ids):
        return chunks

    def insert_citations(self, answer, chunk_texts, chunk_vectors, embd_mdl, **kwargs):
        self.insert_citations_calls.append(
            {
                "answer": answer,
                "chunk_texts": chunk_texts,
                "chunk_vectors": chunk_vectors,
                "embd_mdl": embd_mdl,
                "kwargs": kwargs,
            }
        )
        return self.decorated_answer, list(self.cited_indexes)


def _build_retrieval_result():
    return {
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
        "doc_aggs": [{"doc_id": "doc-1", "doc_name": "doc.txt", "count": 1}],
    }


def _build_dialog(*, quote):
    return SimpleNamespace(
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
            "quote": quote,
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


async def _collect_async_chat(dialog, **kwargs):
    items = []
    async for item in dialog_service.async_chat(
        dialog,
        [{"role": "user", "content": "What does the dataset say?"}],
        **kwargs,
    ):
        items.append(item)
    return items


async def _collect_async_ask(**kwargs):
    items = []
    async for item in dialog_service.async_ask(
        question="What does the dataset say?",
        kb_ids=["kb-1"],
        tenant_id="tenant-id",
        chat_llm_name="chat-model",
        **kwargs,
    ):
        items.append(item)
    return items


@pytest.fixture
def stream_test_setup(monkeypatch):
    retrieval_result = _build_retrieval_result()
    kbs = [SimpleNamespace(tenant_id="tenant-id", embd_id="embd-model", parser_id=None)]

    monkeypatch.setattr(dialog_service.TenantLLMService, "llm_id2llm_type", lambda _llm_id: "chat")
    monkeypatch.setattr(
        dialog_service.TenantLLMService,
        "get_model_config",
        lambda *_args, **_kwargs: {"llm_factory": "unit", "max_tokens": 4096},
    )
    monkeypatch.setattr(dialog_service.TenantLangfuseService, "filter_by_tenant", lambda **_kwargs: None)
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_field_map", lambda _kb_ids: {})
    monkeypatch.setattr(dialog_service.KnowledgebaseService, "get_by_ids", lambda _kb_ids: kbs)
    monkeypatch.setattr(dialog_service, "label_question", lambda _question, _kbs: None)
    monkeypatch.setattr(
        dialog_service,
        "kb_prompt",
        lambda kbinfos, _max_tokens: ["Chunk text from dataset."] if kbinfos["chunks"] else [],
    )
    monkeypatch.setattr(dialog_service, "message_fit_in", lambda msg, _max_tokens: (0, msg))
    monkeypatch.setattr(dialog_service, "get_model_config_by_type_and_name", lambda *_args, **_kwargs: {})
    monkeypatch.setattr(dialog_service, "LLMBundle", lambda *_args, **_kwargs: object())

    return {"retrieval_result": retrieval_result, "kbs": kbs}


@pytest.mark.p2
def test_async_chat_streaming_final_event_keeps_decorated_answer(monkeypatch, stream_test_setup):
    retriever = _StubCitationRetriever(
        stream_test_setup["retrieval_result"],
        "This is the final decorated answer with citation [ID:0]",
        {0},
    )
    chat_model = _StubStreamingChatModel(
        streamed_outputs=[
            "This is the raw streamed answer",
            "This is the raw streamed answer without markers",
        ],
        final_answer="unused in streaming",
    )

    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda _dialog: (stream_test_setup["kbs"], object(), None, chat_model, None),
    )

    result = asyncio.run(_collect_async_chat(_build_dialog(quote=True), stream=True))

    assert len(result) >= 2
    assert result[0]["final"] is False
    assert result[0]["reference"] == {}
    assert "[ID:0]" not in result[0]["answer"]

    final = result[-1]
    assert final["final"] is True
    assert final["answer"] == "This is the final decorated answer with citation [ID:0]"
    assert final["reference"]["doc_aggs"] == [{"doc_id": "doc-1", "doc_name": "doc.txt", "count": 1}]
    assert "vector" not in final["reference"]["chunks"][0]
    assert retriever.insert_citations_calls


@pytest.mark.p2
def test_async_chat_non_streaming_path_is_unchanged(monkeypatch, stream_test_setup):
    retriever = _StubCitationRetriever(
        stream_test_setup["retrieval_result"],
        "This is the decorated non-stream answer [ID:0]",
        {0},
    )
    chat_model = _StubStreamingChatModel(
        streamed_outputs=[],
        final_answer="This is the raw non-stream answer",
    )

    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda _dialog: (stream_test_setup["kbs"], object(), None, chat_model, None),
    )

    result = asyncio.run(_collect_async_chat(_build_dialog(quote=True), stream=False))

    assert len(result) == 1
    assert result[0]["answer"] == "This is the decorated non-stream answer [ID:0]"
    assert result[0]["reference"]["doc_aggs"] == [{"doc_id": "doc-1", "doc_name": "doc.txt", "count": 1}]
    assert retriever.insert_citations_calls


@pytest.mark.p2
def test_async_chat_streaming_without_quote_skips_citation_insertion(monkeypatch, stream_test_setup):
    retriever = _StubCitationRetriever(
        stream_test_setup["retrieval_result"],
        "This decoration should not be used [ID:0]",
        {0},
    )
    chat_model = _StubStreamingChatModel(
        streamed_outputs=[
            "This is the raw streamed answer",
            "This is the raw streamed answer without citations",
        ],
        final_answer="unused in streaming",
    )

    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)
    monkeypatch.setattr(
        dialog_service,
        "get_models",
        lambda _dialog: (stream_test_setup["kbs"], object(), None, chat_model, None),
    )

    result = asyncio.run(_collect_async_chat(_build_dialog(quote=False), stream=True, quote=False))

    final = result[-1]
    assert final["final"] is True
    assert final["answer"] == "This is the raw streamed answer without citations"
    assert final["reference"] == []
    assert retriever.insert_citations_calls == []


@pytest.mark.p2
def test_async_ask_streaming_final_event_keeps_decorated_answer(monkeypatch, stream_test_setup):
    retriever = _StubCitationRetriever(
        stream_test_setup["retrieval_result"],
        "Summary answer with citation [ID:0]",
        {0},
    )
    chat_model = _StubStreamingChatModel(
        streamed_outputs=[
            "Summary answer",
            "Summary answer without markers",
        ],
        final_answer="unused in streaming",
    )

    monkeypatch.setattr(dialog_service.settings, "retriever", retriever, raising=False)

    def _bundle(_tenant_id, model_config):
        if model_config.get("kind") == "chat":
            return chat_model
        return object()

    def _model_config(_tenant_id, llm_type, _name):
        return {"kind": "chat" if llm_type == dialog_service.LLMType.CHAT else "embedding"}

    monkeypatch.setattr(dialog_service, "LLMBundle", _bundle)
    monkeypatch.setattr(dialog_service, "get_model_config_by_type_and_name", _model_config)

    result = asyncio.run(_collect_async_ask())

    assert len(result) >= 2
    assert result[-1]["final"] is True
    assert result[-1]["answer"] == "Summary answer with citation [ID:0]"
    assert result[-1]["reference"]["doc_aggs"] == [{"doc_id": "doc-1", "doc_name": "doc.txt", "count": 1}]
