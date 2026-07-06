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
Regression tests for #15286: streaming responses from
`/api/v1/openai/<chat_id>/chat/completions` duplicated the whole answer.

RAGFlow streams the body as incremental `delta.content` chunks, then emits a
terminating `final` event carrying the *complete* (decorated) answer. The
handler used to re-emit that full answer as one more `delta.content` chunk,
so the client received the entire message twice. The fix keeps the complete
answer out of the content stream and exposes it only via the trailing chunk's
`final_content` / `reference` fields.

These tests drive `_stream_chat_completion_sse` directly with a fake event
stream, so no live server or LLM is required.
"""

import asyncio
import importlib.util
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace


class _PassthroughManager:
    def route(self, *_args, **_kwargs):
        return lambda func: func


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


def _load_openai_api(monkeypatch):
    """Load api/apps/restful_apis/openai_api.py with the heavy deps stubbed."""
    repo_root = Path(__file__).resolve().parents[5]
    apps_mod = ModuleType("api.apps")
    apps_mod.__path__ = [str(repo_root / "api" / "apps")]
    apps_mod.current_user = SimpleNamespace(id="tenant-1")
    apps_mod.login_required = lambda func: func
    monkeypatch.setitem(sys.modules, "api.apps", apps_mod)

    _stub(monkeypatch, "quart", Response=object, jsonify=lambda *a, **k: None)
    # Pre-register nested modules so importlib finds them directly in
    # sys.modules without trying to traverse the stubbed parent package.
    _stub(
        monkeypatch,
        "api.apps.restful_apis._generation_params",
        extract_generation_config=lambda req: {},
        merge_generation_config=lambda *_a, **_k: None,
    )
    _stub(monkeypatch, "api.db.services.dialog_service", DialogService=SimpleNamespace(), async_chat=lambda *_a, **_k: None)
    _stub(monkeypatch, "api.db.services.doc_metadata_service", DocMetadataService=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_model_config_from_provider_instance=lambda *_a, **_k: {},
        get_api_key=lambda *_a, **_k: "key",
    )
    _stub(
        monkeypatch,
        "api.utils.api_utils",
        get_error_data_result=lambda *a, **k: {"code": 102},
        get_request_json=lambda: {},
        validate_request=lambda *_a, **_k: lambda func: func,
    )
    _stub(monkeypatch, "common.constants", RetCode=SimpleNamespace(ARGUMENT_ERROR=102), StatusEnum=SimpleNamespace(VALID=SimpleNamespace(value="1")))
    _stub(monkeypatch, "common.metadata_utils", convert_conditions=lambda *_a, **_k: None, meta_filter=lambda *_a, **_k: [])
    # Deterministic token counter so usage math is predictable.
    _stub(monkeypatch, "common.token_utils", num_tokens_from_string=lambda s: len(s or ""))
    # chunks_format just echoes the reference payload for the reference test.
    _stub(monkeypatch, "rag.prompts.generator", chunks_format=lambda reference: list(reference) if isinstance(reference, list) else [])
    _stub(monkeypatch, "api.utils.reference_metadata_utils", enrich_chunks_with_document_metadata=lambda *_a, **_k: None)

    module_path = repo_root / "api" / "apps" / "restful_apis" / "openai_api.py"
    spec = importlib.util.spec_from_file_location("test_openai_stream_openai_api", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _PassthroughManager()
    monkeypatch.setitem(sys.modules, "test_openai_stream_openai_api", module)
    spec.loader.exec_module(module)
    return module


async def _aiter(events):
    for event in events:
        yield event


def _collect_sse(module, events, **kwargs):
    """Run the SSE generator over `events` and return parsed JSON chunks
    (the trailing `[DONE]` sentinel excluded)."""

    async def run():
        out = []
        async for raw in module._stream_chat_completion_sse(_aiter(events), **kwargs):
            assert raw.startswith("data:")
            payload = raw[len("data:") :].strip()
            if payload == "[DONE]":
                out.append("[DONE]")
            else:
                out.append(json.loads(payload))
        return out

    return asyncio.run(run())


def _content_pieces(chunks):
    return [c["choices"][0]["delta"].get("content") for c in chunks if c != "[DONE]"]


_BASE_KWARGS = dict(completion_id="chatcmpl-x", requested_model="model", prompt="hi")


# --------------------------------------------------------------------------- #
# The actual bug.
# --------------------------------------------------------------------------- #
def test_body_is_streamed_exactly_once(monkeypatch):
    module = _load_openai_api(monkeypatch)
    events = [
        {"answer": "Hello "},
        {"answer": "world"},
        {"final": True, "answer": "Hello world", "reference": {}},
    ]
    chunks = _collect_sse(module, events, need_reference=False, **_BASE_KWARGS)

    streamed = "".join(p for p in _content_pieces(chunks) if isinstance(p, str))
    assert streamed == "Hello world"  # not "Hello worldHello world"


def test_final_answer_not_reemitted_as_content_chunk(monkeypatch):
    module = _load_openai_api(monkeypatch)
    events = [
        {"answer": "Hello "},
        {"answer": "world"},
        {"final": True, "answer": "Hello world", "reference": {}},
    ]
    chunks = _collect_sse(module, events, need_reference=False, **_BASE_KWARGS)

    # No single content chunk should carry the whole answer.
    assert all(p != "Hello world" for p in _content_pieces(chunks) if isinstance(p, str))
    # Two delta events -> two content chunks before the terminating chunk.
    assert [p for p in _content_pieces(chunks) if isinstance(p, str)] == ["Hello ", "world"]


def test_terminating_chunk_has_stop_and_null_content(monkeypatch):
    module = _load_openai_api(monkeypatch)
    events = [{"answer": "Hi"}, {"final": True, "answer": "Hi", "reference": {}}]
    chunks = _collect_sse(module, events, need_reference=False, **_BASE_KWARGS)

    assert chunks[-1] == "[DONE]"
    final_chunk = chunks[-2]
    assert final_chunk["choices"][0]["finish_reason"] == "stop"
    assert final_chunk["choices"][0]["delta"]["content"] is None
    assert final_chunk["usage"]["completion_tokens"] == len("Hi")


# --------------------------------------------------------------------------- #
# The complete answer must still be reachable, just not in the content stream.
# --------------------------------------------------------------------------- #
def test_final_content_and_reference_on_trailing_chunk(monkeypatch):
    module = _load_openai_api(monkeypatch)
    events = [
        {"answer": "Hello "},
        {"answer": "world"},
        {"final": True, "answer": "Hello world", "reference": [{"id": "c1"}]},
    ]
    chunks = _collect_sse(module, events, need_reference=True, **_BASE_KWARGS)

    streamed = "".join(p for p in _content_pieces(chunks) if isinstance(p, str))
    assert streamed == "Hello world"

    final_delta = chunks[-2]["choices"][0]["delta"]
    assert final_delta["final_content"] == "Hello world"
    assert final_delta["reference"] == [{"id": "c1"}]


# --------------------------------------------------------------------------- #
# Reasoning ("think") deltas stream separately and are also not duplicated.
# --------------------------------------------------------------------------- #
def test_reasoning_content_streamed_separately(monkeypatch):
    module = _load_openai_api(monkeypatch)
    events = [
        {"start_to_think": True, "answer": ""},
        {"answer": "thinking"},
        {"end_to_think": True, "answer": ""},
        {"answer": "answer"},
        {"final": True, "answer": "answer", "reference": {}},
    ]
    chunks = _collect_sse(module, events, need_reference=False, **_BASE_KWARGS)

    reasoning = "".join(c["choices"][0]["delta"].get("reasoning_content") for c in chunks if c != "[DONE]" and isinstance(c["choices"][0]["delta"].get("reasoning_content"), str))
    content = "".join(p for p in _content_pieces(chunks) if isinstance(p, str))
    assert reasoning == "thinking"
    assert content == "answer"
