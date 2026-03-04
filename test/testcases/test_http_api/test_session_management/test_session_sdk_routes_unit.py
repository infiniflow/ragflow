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
import importlib.util
import inspect
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _DummyManager:
    def route(self, *_args, **_kwargs):
        def decorator(func):
            return func

        return decorator


class _AwaitableValue:
    def __init__(self, value):
        self._value = value

    def __await__(self):
        async def _co():
            return self._value

        return _co().__await__()


class _Args(dict):
    def get(self, key, default=None, type=None):
        value = super().get(key, default)
        if value is None or type is None:
            return value
        try:
            return type(value)
        except (TypeError, ValueError):
            return default


class _StubHeaders:
    def __init__(self):
        self._items = []

    def add_header(self, key, value):
        self._items.append((key, value))

    def get(self, key, default=None):
        for existing_key, value in reversed(self._items):
            if existing_key == key:
                return value
        return default


class _StubResponse:
    def __init__(self, body, mimetype=None, content_type=None):
        self.body = body
        self.mimetype = mimetype
        self.content_type = content_type
        self.headers = _StubHeaders()


class _DummyUploadFile:
    def __init__(self, filename):
        self.filename = filename
        self.saved_path = None

    async def save(self, path):
        self.saved_path = path


def _run(coro):
    return asyncio.run(coro)


async def _collect_stream(body):
    items = []
    if hasattr(body, "__aiter__"):
        async for item in body:
            if isinstance(item, bytes):
                item = item.decode("utf-8")
            items.append(item)
    else:
        for item in body:
            if isinstance(item, bytes):
                item = item.decode("utf-8")
            items.append(item)
    return items


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


def _load_session_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]
    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    deepdoc_pkg = ModuleType("deepdoc")
    deepdoc_parser_pkg = ModuleType("deepdoc.parser")
    deepdoc_parser_pkg.__path__ = []

    class _StubPdfParser:
        pass

    class _StubExcelParser:
        pass

    class _StubDocxParser:
        pass

    deepdoc_parser_pkg.PdfParser = _StubPdfParser
    deepdoc_parser_pkg.ExcelParser = _StubExcelParser
    deepdoc_parser_pkg.DocxParser = _StubDocxParser
    deepdoc_pkg.parser = deepdoc_parser_pkg
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser", deepdoc_parser_pkg)

    deepdoc_excel_module = ModuleType("deepdoc.parser.excel_parser")
    deepdoc_excel_module.RAGFlowExcelParser = _StubExcelParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.excel_parser", deepdoc_excel_module)

    deepdoc_mineru_module = ModuleType("deepdoc.parser.mineru_parser")

    class _StubMinerUParser:
        pass

    deepdoc_mineru_module.MinerUParser = _StubMinerUParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.mineru_parser", deepdoc_mineru_module)

    deepdoc_paddle_module = ModuleType("deepdoc.parser.paddleocr_parser")

    class _StubPaddleOCRParser:
        pass

    deepdoc_paddle_module.PaddleOCRParser = _StubPaddleOCRParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser.paddleocr_parser", deepdoc_paddle_module)

    deepdoc_parser_utils = ModuleType("deepdoc.parser.utils")
    deepdoc_parser_utils.get_text = lambda *_args, **_kwargs: ""
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", deepdoc_parser_utils)
    monkeypatch.setitem(sys.modules, "xgboost", ModuleType("xgboost"))

    agent_pkg = ModuleType("agent")
    agent_pkg.__path__ = []
    agent_canvas_mod = ModuleType("agent.canvas")

    class _StubCanvas:
        def __init__(self, *_args, **_kwargs):
            self._dsl = "{}"

        def reset(self):
            return None

        def get_prologue(self):
            return "stub prologue"

        def get_component_input_form(self, _name):
            return {}

        def get_mode(self):
            return "chat"

        def __str__(self):
            return self._dsl

    agent_canvas_mod.Canvas = _StubCanvas
    agent_pkg.canvas = agent_canvas_mod
    monkeypatch.setitem(sys.modules, "agent", agent_pkg)
    monkeypatch.setitem(sys.modules, "agent.canvas", agent_canvas_mod)

    module_path = repo_root / "api" / "apps" / "sdk" / "session.py"
    spec = importlib.util.spec_from_file_location("test_session_sdk_routes_unit_module", module_path)
    module = importlib.util.module_from_spec(spec)
    module.manager = _DummyManager()
    monkeypatch.setitem(sys.modules, "test_session_sdk_routes_unit_module", module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p2
def test_create_and_update_guard_matrix(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"name": "session"}))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    res = _run(inspect.unwrap(module.create)("tenant-1", "chat-1"))
    assert res["message"] == "You do not own the assistant."

    dia = SimpleNamespace(prompt_config={"prologue": "hello"})
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [dia])
    monkeypatch.setattr(module.ConversationService, "save", lambda **_kwargs: None)
    monkeypatch.setattr(module.ConversationService, "get_by_id", lambda _id: (False, None))
    res = _run(inspect.unwrap(module.create)("tenant-1", "chat-1"))
    assert "Fail to create a session" in res["message"]

    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args()))
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (False, None))
    res = _run(inspect.unwrap(module.create_agent_session)("tenant-1", "agent-1"))
    assert res["message"] == "Agent not found."

    canvas = SimpleNamespace(dsl="{}", id="agent-1")
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, canvas))
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [])
    res = _run(inspect.unwrap(module.create_agent_session)("tenant-1", "agent-1"))
    assert res["message"] == "You cannot access the agent."

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({}))
    monkeypatch.setattr(module.ConversationService, "query", lambda **_kwargs: [])
    res = _run(inspect.unwrap(module.update)("tenant-1", "chat-1", "session-1"))
    assert res["message"] == "Session does not exist"

    monkeypatch.setattr(module.ConversationService, "query", lambda **_kwargs: [SimpleNamespace(id="session-1")])
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [])
    res = _run(inspect.unwrap(module.update)("tenant-1", "chat-1", "session-1"))
    assert res["message"] == "You do not own the session"

    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"message": []}))
    res = _run(inspect.unwrap(module.update)("tenant-1", "chat-1", "session-1"))
    assert "`message` can not be change" in res["message"]

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"reference": []}))
    res = _run(inspect.unwrap(module.update)("tenant-1", "chat-1", "session-1"))
    assert "`reference` can not be change" in res["message"]

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"name": ""}))
    res = _run(inspect.unwrap(module.update)("tenant-1", "chat-1", "session-1"))
    assert "`name` can not be empty" in res["message"]

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"name": "renamed"}))
    monkeypatch.setattr(module.ConversationService, "update_by_id", lambda *_args, **_kwargs: False)
    res = _run(inspect.unwrap(module.update)("tenant-1", "chat-1", "session-1"))
    assert res["message"] == "Session updates error"


@pytest.mark.p2
def test_chat_completion_metadata_and_stream_paths(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module, "Response", _StubResponse)
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(kb_ids=["kb-1"])])
    monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kb_ids: [{"id": "doc-1"}])
    monkeypatch.setattr(module, "convert_conditions", lambda cond: cond.get("conditions", []))
    monkeypatch.setattr(module, "meta_filter", lambda *_args, **_kwargs: [])

    captured_requests = []

    async def fake_rag_completion(_tenant_id, _chat_id, **req):
        captured_requests.append(req)
        yield {"answer": "ok"}

    monkeypatch.setattr(module, "rag_completion", fake_rag_completion)

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(None))
    resp = _run(inspect.unwrap(module.chat_completion)("tenant-1", "chat-1"))
    assert isinstance(resp, _StubResponse)
    assert resp.headers.get("Content-Type") == "text/event-stream; charset=utf-8"
    _run(_collect_stream(resp.body))
    assert captured_requests[-1].get("question") == ""

    req_with_conditions = {
        "question": "hello",
        "session_id": "session-1",
        "metadata_condition": {"logic": "and", "conditions": [{"name": "author", "value": "bob"}]},
        "stream": True,
    }
    monkeypatch.setattr(module.ConversationService, "query", lambda **_kwargs: [SimpleNamespace(id="session-1")])
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(req_with_conditions))
    resp = _run(inspect.unwrap(module.chat_completion)("tenant-1", "chat-1"))
    _run(_collect_stream(resp.body))
    assert captured_requests[-1].get("doc_ids") == "-999"

    req_without_conditions = {
        "question": "hello",
        "session_id": "session-1",
        "metadata_condition": {"logic": "and", "conditions": []},
        "stream": True,
        "doc_ids": "legacy",
    }
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(req_without_conditions))
    resp = _run(inspect.unwrap(module.chat_completion)("tenant-1", "chat-1"))
    _run(_collect_stream(resp.body))
    assert "doc_ids" not in captured_requests[-1]


@pytest.mark.p2
def test_openai_chat_validation_matrix_unit(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module, "num_tokens_from_string", lambda _text: 1)
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(kb_ids=["kb-1"])])

    cases = [
        (
            {
                "model": "model",
                "messages": [{"role": "user", "content": "hello"}],
                "extra_body": "bad",
            },
            "extra_body must be an object.",
        ),
        (
            {
                "model": "model",
                "messages": [{"role": "user", "content": "hello"}],
                "extra_body": {"reference_metadata": "bad"},
            },
            "reference_metadata must be an object.",
        ),
        (
            {
                "model": "model",
                "messages": [{"role": "user", "content": "hello"}],
                "extra_body": {"reference_metadata": {"fields": "bad"}},
            },
            "reference_metadata.fields must be an array.",
        ),
        ({"model": "model", "messages": []}, "You have to provide messages."),
        (
            {"model": "model", "messages": [{"role": "assistant", "content": "hello"}]},
            "The last content of this conversation is not from user.",
        ),
        (
            {
                "model": "model",
                "messages": [{"role": "user", "content": "hello"}],
                "extra_body": {"metadata_condition": "bad"},
            },
            "metadata_condition must be an object.",
        ),
    ]

    for payload, expected in cases:
        monkeypatch.setattr(module, "get_request_json", lambda p=payload: _AwaitableValue(p))
        res = _run(inspect.unwrap(module.chat_completion_openai_like)("tenant-1", "chat-1"))
        assert expected in res["message"]


@pytest.mark.p2
def test_openai_stream_generator_branches_unit(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module, "Response", _StubResponse)
    monkeypatch.setattr(module, "num_tokens_from_string", lambda text: len(text or ""))
    monkeypatch.setattr(module, "convert_conditions", lambda cond: cond.get("conditions", []))
    monkeypatch.setattr(module, "meta_filter", lambda *_args, **_kwargs: [])
    monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kb_ids: [{"id": "doc-1"}])
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(kb_ids=["kb-1"])])
    monkeypatch.setattr(module, "_build_reference_chunks", lambda *_args, **_kwargs: [{"id": "ref-1"}])

    async def fake_async_chat(_dia, _msg, _stream, **_kwargs):
        yield {"start_to_think": True}
        yield {"answer": "R"}
        yield {"end_to_think": True}
        yield {"answer": ""}
        yield {"answer": "C"}
        yield {"final": True, "answer": "DONE", "reference": {"chunks": []}}
        raise RuntimeError("boom")

    monkeypatch.setattr(module, "async_chat", fake_async_chat)

    payload = {
        "model": "model",
        "stream": True,
        "messages": [
            {"role": "system", "content": "sys"},
            {"role": "assistant", "content": "preface"},
            {"role": "user", "content": "hello"},
        ],
        "extra_body": {
            "reference": True,
            "reference_metadata": {"include": True, "fields": ["author"]},
            "metadata_condition": {"logic": "and", "conditions": [{"name": "author", "value": "bob"}]},
        },
    }
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue(payload))

    resp = _run(inspect.unwrap(module.chat_completion_openai_like)("tenant-1", "chat-1"))
    assert isinstance(resp, _StubResponse)
    assert resp.headers.get("Content-Type") == "text/event-stream; charset=utf-8"

    chunks = _run(_collect_stream(resp.body))
    assert any("reasoning_content" in chunk for chunk in chunks)
    assert any("**ERROR**: boom" in chunk for chunk in chunks)
    assert any('"usage"' in chunk for chunk in chunks)
    assert any('"reference"' in chunk for chunk in chunks)
    assert chunks[-1].strip() == "data:[DONE]"


@pytest.mark.p2
def test_openai_nonstream_branch_unit(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module, "jsonify", lambda payload: payload)
    monkeypatch.setattr(module, "num_tokens_from_string", lambda text: len(text or ""))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(kb_ids=[])])

    async def fake_async_chat(_dia, _msg, _stream, **_kwargs):
        yield {"answer": "world", "reference": {}}

    monkeypatch.setattr(module, "async_chat", fake_async_chat)
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue(
            {
                "model": "model",
                "messages": [{"role": "user", "content": "hello"}],
                "stream": False,
            }
        ),
    )

    res = _run(inspect.unwrap(module.chat_completion_openai_like)("tenant-1", "chat-1"))
    assert res["choices"][0]["message"]["content"] == "world"


@pytest.mark.p2
def test_agents_openai_compatibility_unit(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module, "Response", _StubResponse)
    monkeypatch.setattr(module, "jsonify", lambda payload: payload)
    monkeypatch.setattr(module, "num_tokens_from_string", lambda text: len(text or ""))

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"model": "model", "messages": []}))
    res = _run(inspect.unwrap(module.agents_completion_openai_compatibility)("tenant-1", "agent-1"))
    assert "at least one message" in res["message"]

    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"model": "model", "messages": [{"role": "user", "content": "hello"}]}),
    )
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [])
    res = _run(inspect.unwrap(module.agents_completion_openai_compatibility)("tenant-1", "agent-1"))
    assert "don't own the agent" in res["message"]

    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [SimpleNamespace(id="agent-1")])
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"model": "model", "messages": [{"role": "system", "content": "system only"}]}),
    )
    res = _run(inspect.unwrap(module.agents_completion_openai_compatibility)("tenant-1", "agent-1"))
    assert "No valid messages found" in json.dumps(res)

    captured_calls = []

    async def _completion_openai_stream(*args, **kwargs):
        captured_calls.append((args, kwargs))
        yield "data:stream"

    monkeypatch.setattr(module, "completion_openai", _completion_openai_stream)
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue(
            {
                "model": "model",
                "messages": [
                    {"role": "assistant", "content": "preface"},
                    {"role": "user", "content": "latest question"},
                ],
                "stream": True,
                "metadata": {"id": "meta-session"},
            }
        ),
    )
    resp = _run(inspect.unwrap(module.agents_completion_openai_compatibility)("tenant-1", "agent-1"))
    assert isinstance(resp, _StubResponse)
    assert resp.headers.get("Content-Type") == "text/event-stream; charset=utf-8"
    _run(_collect_stream(resp.body))
    assert captured_calls[-1][0][2] == "latest question"

    async def _completion_openai_nonstream(*args, **kwargs):
        captured_calls.append((args, kwargs))
        yield {"id": "non-stream"}

    monkeypatch.setattr(module, "completion_openai", _completion_openai_nonstream)
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue(
            {
                "model": "model",
                "messages": [
                    {"role": "user", "content": "first"},
                    {"role": "assistant", "content": "middle"},
                    {"role": "user", "content": "final user"},
                ],
                "stream": False,
                "session_id": "session-1",
                "temperature": 0.5,
            }
        ),
    )
    res = _run(inspect.unwrap(module.agents_completion_openai_compatibility)("tenant-1", "agent-1"))
    assert res["id"] == "non-stream"
    assert captured_calls[-1][0][2] == "final user"
    assert captured_calls[-1][1]["stream"] is False
    assert captured_calls[-1][1]["session_id"] == "session-1"


@pytest.mark.p2
def test_agent_completions_stream_and_nonstream_unit(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module, "Response", _StubResponse)

    async def _agent_stream(*_args, **_kwargs):
        yield "data:not-json"
        yield "data:" + json.dumps({"event": "node_finished", "data": {"component_id": "c1"}})
        yield "data:" + json.dumps({"event": "other", "data": {}})
        yield "data:" + json.dumps({"event": "message", "data": {"content": "hello"}})

    monkeypatch.setattr(module, "agent_completion", _agent_stream)
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"stream": True, "return_trace": True}))

    resp = _run(inspect.unwrap(module.agent_completions)("tenant-1", "agent-1"))
    chunks = _run(_collect_stream(resp.body))
    assert resp.headers.get("Content-Type") == "text/event-stream; charset=utf-8"
    assert any('"trace"' in chunk for chunk in chunks)
    assert any("hello" in chunk for chunk in chunks)
    assert chunks[-1].strip() == "data:[DONE]"

    async def _agent_nonstream(*_args, **_kwargs):
        yield "data:" + json.dumps({"event": "message", "data": {"content": "A", "reference": {"doc": "r"}}})
        yield "data:" + json.dumps({"event": "node_finished", "data": {"component_id": "c2"}})

    monkeypatch.setattr(module, "agent_completion", _agent_nonstream)
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"stream": False, "return_trace": True}))
    res = _run(inspect.unwrap(module.agent_completions)("tenant-1", "agent-1"))
    assert res["data"]["data"]["content"] == "A"
    assert res["data"]["data"]["reference"] == {"doc": "r"}
    assert res["data"]["data"]["trace"][0]["component_id"] == "c2"

    async def _agent_nonstream_broken(*_args, **_kwargs):
        yield "data:{"

    monkeypatch.setattr(module, "agent_completion", _agent_nonstream_broken)
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"stream": False, "return_trace": False}))
    res = _run(inspect.unwrap(module.agent_completions)("tenant-1", "agent-1"))
    assert res["data"].startswith("**ERROR**")


@pytest.mark.p2
def test_list_session_projection_unit(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args({})))
    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])

    convs = [
        {
            "id": "session-1",
            "dialog_id": "chat-1",
            "message": [{"role": "assistant", "content": "hello", "prompt": "internal"}],
            "reference": [
                {
                    "chunks": [
                        {
                            "chunk_id": "chunk-1",
                            "content_with_weight": "weighted",
                            "doc_id": "doc-1",
                            "docnm_kwd": "doc-name",
                            "kb_id": "kb-1",
                            "image_id": "img-1",
                            "positions": [1, 2],
                        }
                    ]
                }
            ],
        }
    ]
    monkeypatch.setattr(module.ConversationService, "get_list", lambda *_args, **_kwargs: convs)

    res = _run(inspect.unwrap(module.list_session)("tenant-1", "chat-1"))
    assert res["data"][0]["chat_id"] == "chat-1"
    assert "reference" not in res["data"][0]
    assert "prompt" not in res["data"][0]["messages"][0]
    assert res["data"][0]["messages"][0]["reference"][0]["positions"] == [1, 2]


@pytest.mark.p2
def test_list_agent_session_projection_unit(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module, "request", SimpleNamespace(args=_Args({})))
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [SimpleNamespace(id="agent-1")])

    conv_non_list_reference = {
        "id": "session-1",
        "dialog_id": "agent-1",
        "message": [{"role": "assistant", "content": "hello", "prompt": "internal"}],
        "reference": {"unexpected": "shape"},
    }
    monkeypatch.setattr(module.API4ConversationService, "get_list", lambda *_args, **_kwargs: (1, [conv_non_list_reference]))
    res = _run(inspect.unwrap(module.list_agent_session)("tenant-1", "agent-1"))
    assert res["data"][0]["agent_id"] == "agent-1"
    assert "prompt" not in res["data"][0]["messages"][0]

    conv_with_chunks = {
        "id": "session-2",
        "dialog_id": "agent-1",
        "message": [
            {"role": "user", "content": "question"},
            {"role": "assistant", "content": "answer", "prompt": "internal"},
        ],
        "reference": [
            {
                "chunks": [
                    "not-a-dict",
                    {
                        "chunk_id": "chunk-2",
                        "content_with_weight": "weighted",
                        "doc_id": "doc-2",
                        "docnm_kwd": "doc-name-2",
                        "kb_id": "kb-2",
                        "image_id": "img-2",
                        "positions": [9],
                    },
                ]
            }
        ],
    }
    monkeypatch.setattr(module.API4ConversationService, "get_list", lambda *_args, **_kwargs: (1, [conv_with_chunks]))
    res = _run(inspect.unwrap(module.list_agent_session)("tenant-1", "agent-1"))
    projected_chunk = res["data"][0]["messages"][1]["reference"][0]
    assert projected_chunk["image_id"] == "img-2"
    assert projected_chunk["positions"] == [9]


@pytest.mark.p2
def test_delete_routes_partial_duplicate_unit(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module.DialogService, "query", lambda **_kwargs: [SimpleNamespace(id="chat-1")])
    monkeypatch.setattr(module.ConversationService, "delete_by_id", lambda *_args, **_kwargs: True)

    def _conversation_query(**kwargs):
        if "id" not in kwargs:
            return [SimpleNamespace(id="seed")]
        if kwargs["id"] == "ok":
            return [SimpleNamespace(id="ok")]
        return []

    monkeypatch.setattr(module.ConversationService, "query", _conversation_query)

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"ids": ["ok", "bad"]}))
    monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
    res = _run(inspect.unwrap(module.delete)("tenant-1", "chat-1"))
    assert res["code"] == 0
    assert res["data"]["success_count"] == 1
    assert res["data"]["errors"] == ["The chat doesn't own the session bad"]

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"ids": ["bad"]}))
    monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))
    res = _run(inspect.unwrap(module.delete)("tenant-1", "chat-1"))
    assert res["message"] == "The chat doesn't own the session bad"

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"ids": ["ok", "ok"]}))
    monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (["ok"], ["Duplicate session ids: ok"]))
    res = _run(inspect.unwrap(module.delete)("tenant-1", "chat-1"))
    assert res["code"] == 0
    assert res["data"]["success_count"] == 1
    assert res["data"]["errors"] == ["Duplicate session ids: ok"]

    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [SimpleNamespace(id="agent-1")])
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"ids": ["session-1"]}))
    monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))

    def _agent_query(**kwargs):
        if "id" not in kwargs:
            return [SimpleNamespace(id="session-1")]
        if kwargs["id"] == "session-1":
            return [SimpleNamespace(id="session-1")]
        return []

    monkeypatch.setattr(module.API4ConversationService, "query", _agent_query)
    monkeypatch.setattr(module.API4ConversationService, "delete_by_id", lambda *_args, **_kwargs: True)
    res = _run(inspect.unwrap(module.delete_agent_session)("tenant-1", "agent-1"))
    assert res["code"] == 0


@pytest.mark.p2
def test_delete_agent_session_error_matrix_unit(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [SimpleNamespace(id="agent-1")])
    monkeypatch.setattr(module.API4ConversationService, "delete_by_id", lambda *_args, **_kwargs: True)

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"ids": ["ok", "missing"]}))
    monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (ids, []))

    def _query_partial(**kwargs):
        if "id" not in kwargs:
            return [SimpleNamespace(id="ok"), SimpleNamespace(id="missing")]
        if kwargs["id"] == "ok":
            return [SimpleNamespace(id="ok")]
        return []

    monkeypatch.setattr(module.API4ConversationService, "query", _query_partial)
    res = _run(inspect.unwrap(module.delete_agent_session)("tenant-1", "agent-1"))
    assert res["data"]["success_count"] == 1
    assert res["data"]["errors"] == ["The agent doesn't own the session missing"]

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"ids": ["missing"]}))

    def _query_all_failed(**kwargs):
        if "id" not in kwargs:
            return [SimpleNamespace(id="missing")]
        return []

    monkeypatch.setattr(module.API4ConversationService, "query", _query_all_failed)
    res = _run(inspect.unwrap(module.delete_agent_session)("tenant-1", "agent-1"))
    assert res["message"] == "The agent doesn't own the session missing"

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"ids": ["ok", "ok"]}))
    monkeypatch.setattr(module, "check_duplicate_ids", lambda ids, _kind: (["ok"], ["Duplicate session ids: ok"]))

    def _query_duplicate(**kwargs):
        if "id" not in kwargs:
            return [SimpleNamespace(id="ok")]
        if kwargs["id"] == "ok":
            return [SimpleNamespace(id="ok")]
        return []

    monkeypatch.setattr(module.API4ConversationService, "query", _query_duplicate)
    res = _run(inspect.unwrap(module.delete_agent_session)("tenant-1", "agent-1"))
    assert res["data"]["success_count"] == 1
    assert res["data"]["errors"] == ["Duplicate session ids: ok"]


@pytest.mark.p2
def test_sessions_ask_route_validation_and_stream_unit(monkeypatch):
    module = _load_session_module(monkeypatch)
    monkeypatch.setattr(module, "Response", _StubResponse)

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"dataset_ids": ["kb-1"]}))
    res = _run(inspect.unwrap(module.ask_about)("tenant-1"))
    assert res["message"] == "`question` is required."

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"question": "q"}))
    res = _run(inspect.unwrap(module.ask_about)("tenant-1"))
    assert res["message"] == "`dataset_ids` is required."

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"question": "q", "dataset_ids": "kb-1"}))
    res = _run(inspect.unwrap(module.ask_about)("tenant-1"))
    assert res["message"] == "`dataset_ids` should be a list."

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: False)
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"question": "q", "dataset_ids": ["kb-1"]}))
    res = _run(inspect.unwrap(module.ask_about)("tenant-1"))
    assert res["message"] == "You don't own the dataset kb-1."

    monkeypatch.setattr(module.KnowledgebaseService, "accessible", lambda *_args, **_kwargs: True)
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [SimpleNamespace(chunk_num=0)])
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"question": "q", "dataset_ids": ["kb-1"]}))
    res = _run(inspect.unwrap(module.ask_about)("tenant-1"))
    assert res["message"] == "The dataset kb-1 doesn't own parsed file"

    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [SimpleNamespace(chunk_num=1)])
    captured = {}

    async def _streaming_async_ask(question, kb_ids, uid):
        captured["question"] = question
        captured["kb_ids"] = kb_ids
        captured["uid"] = uid
        yield {"answer": "first"}
        raise RuntimeError("ask stream boom")

    monkeypatch.setattr(module, "async_ask", _streaming_async_ask)
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"question": "q", "dataset_ids": ["kb-1"]}))
    resp = _run(inspect.unwrap(module.ask_about)("tenant-1"))
    assert isinstance(resp, _StubResponse)
    assert resp.headers.get("Content-Type") == "text/event-stream; charset=utf-8"
    chunks = _run(_collect_stream(resp.body))
    assert any('"answer": "first"' in chunk for chunk in chunks)
    assert any('"code": 500' in chunk and "**ERROR**: ask stream boom" in chunk for chunk in chunks)
    assert '"data": true' in chunks[-1].lower()
    assert captured == {"question": "q", "kb_ids": ["kb-1"], "uid": "tenant-1"}


@pytest.mark.p2
def test_sessions_related_questions_prompt_build_unit(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({}))
    res = _run(inspect.unwrap(module.related_questions)("tenant-1"))
    assert res["message"] == "`question` is required."

    captured = {}

    class _FakeLLMBundle:
        def __init__(self, *args, **kwargs):
            captured["bundle_args"] = args
            captured["bundle_kwargs"] = kwargs

        async def async_chat(self, prompt, messages, options):
            captured["prompt"] = prompt
            captured["messages"] = messages
            captured["options"] = options
            return "1. First related\n2. Second related\nplain text"

    monkeypatch.setattr(module, "LLMBundle", _FakeLLMBundle)
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"question": "solar energy", "industry": "renewables"}),
    )
    res = _run(inspect.unwrap(module.related_questions)("tenant-1"))
    assert res["data"] == ["First related", "Second related"]
    assert "Keep the term length between 2-4 words" in captured["prompt"]
    assert "related terms can also help search engines" in captured["prompt"]
    assert "Ensure all search terms are relevant to the industry: renewables." in captured["prompt"]
    assert "Keywords: solar energy" in captured["messages"][0]["content"]
    assert captured["options"] == {"temperature": 0.9}


@pytest.mark.p2
def test_chatbot_routes_auth_stream_nonstream_unit(monkeypatch):
    module = _load_session_module(monkeypatch)
    monkeypatch.setattr(module, "Response", _StubResponse)

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer"}))
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({}))
    res = _run(inspect.unwrap(module.chatbot_completions)("dialog-1"))
    assert res["message"] == "Authorization is not valid!"

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer bad"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])
    res = _run(inspect.unwrap(module.chatbot_completions)("dialog-1"))
    assert "API key is invalid" in res["message"]

    stream_calls = []

    async def _iframe_stream(dialog_id, **req):
        stream_calls.append((dialog_id, dict(req)))
        yield "data:stream-chunk"

    monkeypatch.setattr(module, "iframe_completion", _iframe_stream)
    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer ok"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"stream": True}))
    resp = _run(inspect.unwrap(module.chatbot_completions)("dialog-1"))
    assert isinstance(resp, _StubResponse)
    assert resp.headers.get("Content-Type") == "text/event-stream; charset=utf-8"
    _run(_collect_stream(resp.body))
    assert stream_calls[-1][0] == "dialog-1"
    assert stream_calls[-1][1]["quote"] is False

    async def _iframe_nonstream(_dialog_id, **_req):
        yield {"answer": "non-stream"}

    monkeypatch.setattr(module, "iframe_completion", _iframe_nonstream)
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"stream": False, "quote": True}))
    res = _run(inspect.unwrap(module.chatbot_completions)("dialog-1"))
    assert res["data"]["answer"] == "non-stream"

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer"}))
    res = _run(inspect.unwrap(module.chatbots_inputs)("dialog-1"))
    assert res["message"] == "Authorization is not valid!"

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer invalid"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])
    res = _run(inspect.unwrap(module.chatbots_inputs)("dialog-1"))
    assert "API key is invalid" in res["message"]

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer ok"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module.DialogService, "get_by_id", lambda _dialog_id: (False, None))
    res = _run(inspect.unwrap(module.chatbots_inputs)("dialog-404"))
    assert res["message"] == "Can't find dialog by ID: dialog-404"


@pytest.mark.p2
def test_agentbot_routes_auth_stream_nonstream_unit(monkeypatch):
    module = _load_session_module(monkeypatch)
    monkeypatch.setattr(module, "Response", _StubResponse)

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer"}))
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({}))
    res = _run(inspect.unwrap(module.agent_bot_completions)("agent-1"))
    assert res["message"] == "Authorization is not valid!"

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer bad"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])
    res = _run(inspect.unwrap(module.agent_bot_completions)("agent-1"))
    assert "API key is invalid" in res["message"]

    async def _agent_stream(*_args, **_kwargs):
        yield "data:agent-stream"

    monkeypatch.setattr(module, "agent_completion", _agent_stream)
    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer ok"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"stream": True}))
    resp = _run(inspect.unwrap(module.agent_bot_completions)("agent-1"))
    assert isinstance(resp, _StubResponse)
    assert resp.headers.get("Content-Type") == "text/event-stream; charset=utf-8"
    _run(_collect_stream(resp.body))

    async def _agent_nonstream(*_args, **_kwargs):
        yield {"answer": "agent-non-stream"}

    monkeypatch.setattr(module, "agent_completion", _agent_nonstream)
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"stream": False}))
    res = _run(inspect.unwrap(module.agent_bot_completions)("agent-1"))
    assert res["data"]["answer"] == "agent-non-stream"

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer"}))
    res = _run(inspect.unwrap(module.begin_inputs)("agent-1"))
    assert res["message"] == "Authorization is not valid!"

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer bad"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])
    res = _run(inspect.unwrap(module.begin_inputs)("agent-1"))
    assert "API key is invalid" in res["message"]

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer ok"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _agent_id: (False, None))
    res = _run(inspect.unwrap(module.begin_inputs)("agent-404"))
    assert res["message"] == "Can't find agent by ID: agent-404"


@pytest.mark.p2
def test_searchbots_ask_embedded_auth_and_stream_unit(monkeypatch):
    module = _load_session_module(monkeypatch)
    monkeypatch.setattr(module, "Response", _StubResponse)

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer"}))
    res = _run(inspect.unwrap(module.ask_about_embedded)())
    assert res["message"] == "Authorization is not valid!"

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer bad"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])
    res = _run(inspect.unwrap(module.ask_about_embedded)())
    assert "API key is invalid" in res["message"]

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer ok"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"question": "embedded q", "kb_ids": ["kb-1"], "search_id": "search-1"}),
    )
    monkeypatch.setattr(module.SearchService, "get_detail", lambda _search_id: {"search_config": {"mode": "test"}})
    captured = {}

    async def _embedded_async_ask(question, kb_ids, uid, search_config=None):
        captured["question"] = question
        captured["kb_ids"] = kb_ids
        captured["uid"] = uid
        captured["search_config"] = search_config
        yield {"answer": "embedded-answer"}
        raise RuntimeError("embedded stream boom")

    monkeypatch.setattr(module, "async_ask", _embedded_async_ask)
    resp = _run(inspect.unwrap(module.ask_about_embedded)())
    assert isinstance(resp, _StubResponse)
    assert resp.headers.get("Content-Type") == "text/event-stream; charset=utf-8"
    chunks = _run(_collect_stream(resp.body))
    assert any('"answer": "embedded-answer"' in chunk for chunk in chunks)
    assert any('"code": 500' in chunk and "**ERROR**: embedded stream boom" in chunk for chunk in chunks)
    assert '"data": true' in chunks[-1].lower()
    assert captured["search_config"] == {"mode": "test"}


@pytest.mark.p2
def test_searchbots_retrieval_test_embedded_matrix_unit(monkeypatch):
    module = _load_session_module(monkeypatch)
    handler = inspect.unwrap(module.retrieval_test_embedded)

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer"}))
    res = _run(handler())
    assert res["message"] == "Authorization is not valid!"

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer invalid"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])
    res = _run(handler())
    assert "API key is invalid" in res["message"]

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer ok"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"kb_id": [], "question": "q"}))
    res = _run(handler())
    assert res["message"] == "Please specify dataset firstly."

    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="")])
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"kb_id": "kb-1", "question": "q"}))
    res = _run(handler())
    assert res["message"] == "permission denined."

    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"kb_id": ["kb-no-access"], "question": "q"}))
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-a")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [])
    res = _run(handler())
    assert "Only owner of dataset authorized for this operation." in res["message"]

    llm_calls = []

    def _fake_llm_bundle(tenant_id, llm_type, *args, **kwargs):
        llm_calls.append((tenant_id, llm_type, args, kwargs))
        return SimpleNamespace(tenant_id=tenant_id, llm_type=llm_type, args=args, kwargs=kwargs)

    monkeypatch.setattr(module, "LLMBundle", _fake_llm_bundle)
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"kb_id": "kb-1", "question": "q", "meta_data_filter": {"method": "auto"}}),
    )
    monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kb_ids: [{"id": "doc-1"}])

    async def _apply_filter(_meta_filter, _metas, _question, _chat_mdl, _local_doc_ids):
        return ["doc-filtered"]

    monkeypatch.setattr(module, "apply_meta_data_filter", _apply_filter)
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-a")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [SimpleNamespace(id="kb-1")])
    monkeypatch.setattr(module.KnowledgebaseService, "get_by_id", lambda _kb_id: (False, None))
    res = _run(handler())
    assert res["message"] == "Knowledgebase not found!"
    assert any(call[1] == module.LLMType.CHAT for call in llm_calls)

    llm_calls.clear()
    retrieval_capture = {}

    async def _fake_retrieval(
        question,
        embd_mdl,
        tenant_ids,
        kb_ids,
        page,
        size,
        similarity_threshold,
        vector_similarity_weight,
        top,
        local_doc_ids,
        rerank_mdl=None,
        highlight=None,
        rank_feature=None,
    ):
        retrieval_capture.update(
            {
                "question": question,
                "embd_mdl": embd_mdl,
                "tenant_ids": tenant_ids,
                "kb_ids": kb_ids,
                "page": page,
                "size": size,
                "similarity_threshold": similarity_threshold,
                "vector_similarity_weight": vector_similarity_weight,
                "top": top,
                "local_doc_ids": local_doc_ids,
                "rerank_mdl": rerank_mdl,
                "highlight": highlight,
                "rank_feature": rank_feature,
            }
        )
        return {"chunks": [{"id": "chunk-1", "vector": [0.1]}]}

    async def _translate(_tenant_id, _chat_id, question, _langs):
        return question + "-translated"

    monkeypatch.setattr(module, "cross_languages", _translate)
    monkeypatch.setattr(module, "label_question", lambda _question, _kbs: ["label-1"])
    monkeypatch.setattr(module.settings, "retriever", SimpleNamespace(retrieval=_fake_retrieval))
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue(
            {
                "kb_id": "kb-1",
                "question": "translated-q",
                "doc_ids": ["doc-seed"],
                "cross_languages": ["es"],
                "search_id": "search-1",
            }
        ),
    )
    monkeypatch.setattr(
        module.SearchService,
        "get_detail",
        lambda _search_id: {
            "search_config": {
                "meta_data_filter": {"method": "auto"},
                "chat_id": "chat-for-filter",
                "similarity_threshold": 0.42,
                "vector_similarity_weight": 0.8,
                "top_k": 7,
                "rerank_id": "reranker-model",
            }
        },
    )
    monkeypatch.setattr(module.DocMetadataService, "get_flatted_meta_by_kbs", lambda _kb_ids: [{"id": "doc-2"}])
    monkeypatch.setattr(module, "apply_meta_data_filter", _apply_filter)
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-a")])
    monkeypatch.setattr(module.KnowledgebaseService, "query", lambda **_kwargs: [SimpleNamespace(id="kb-1")])
    monkeypatch.setattr(
        module.KnowledgebaseService,
        "get_by_id",
        lambda _kb_id: (True, SimpleNamespace(tenant_id="tenant-kb", embd_id="embd-model")),
    )
    res = _run(handler())
    assert res["code"] == 0
    assert res["data"]["labels"] == ["label-1"]
    assert "vector" not in res["data"]["chunks"][0]
    assert retrieval_capture["kb_ids"] == ["kb-1"]
    assert retrieval_capture["tenant_ids"] == ["tenant-a"]
    assert retrieval_capture["question"] == "translated-q-translated"
    assert retrieval_capture["similarity_threshold"] == 0.42
    assert retrieval_capture["vector_similarity_weight"] == 0.8
    assert retrieval_capture["top"] == 7
    assert retrieval_capture["local_doc_ids"] == ["doc-filtered"]
    assert retrieval_capture["rank_feature"] == ["label-1"]
    assert retrieval_capture["rerank_mdl"] is not None
    assert any(call[1] == module.LLMType.EMBEDDING.value and call[3].get("llm_name") == "embd-model" for call in llm_calls)

    llm_calls.clear()

    async def _fake_keyword_extraction(_chat_mdl, question):
        return f"-{question}-keywords"

    async def _fake_kg_retrieval(question, tenant_ids, kb_ids, _embd_mdl, _chat_mdl):
        return {
            "id": "kg-chunk",
            "question": question,
            "tenant_ids": tenant_ids,
            "kb_ids": kb_ids,
            "content_with_weight": 1,
            "vector": [0.5],
        }

    monkeypatch.setattr(module, "keyword_extraction", _fake_keyword_extraction)
    monkeypatch.setattr(module.settings, "kg_retriever", SimpleNamespace(retrieval=_fake_kg_retrieval))
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue(
            {
                "kb_id": "kb-1",
                "question": "keyword-q",
                "rerank_id": "manual-reranker",
                "keyword": True,
                "use_kg": True,
            }
        ),
    )
    monkeypatch.setattr(
        module.KnowledgebaseService,
        "get_by_id",
        lambda _kb_id: (True, SimpleNamespace(tenant_id="tenant-kb", embd_id="embd-model")),
    )
    res = _run(handler())
    assert res["code"] == 0
    assert res["data"]["chunks"][0]["id"] == "kg-chunk"
    assert all("vector" not in chunk for chunk in res["data"]["chunks"])
    assert any(call[1] == module.LLMType.RERANK.value for call in llm_calls)

    async def _raise_not_found(*_args, **_kwargs):
        raise RuntimeError("x not_found y")

    monkeypatch.setattr(module.settings, "retriever", SimpleNamespace(retrieval=_raise_not_found))
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"kb_id": "kb-1", "question": "q"}),
    )
    res = _run(handler())
    assert res["message"] == "No chunk found! Check the chunk status please!"


@pytest.mark.p2
def test_searchbots_related_questions_embedded_matrix_unit(monkeypatch):
    module = _load_session_module(monkeypatch)
    handler = inspect.unwrap(module.related_questions_embedded)

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer"}))
    res = _run(handler())
    assert res["message"] == "Authorization is not valid!"

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer bad"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])
    res = _run(handler())
    assert "API key is invalid" in res["message"]

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer ok"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="")])
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"question": "q"}))
    res = _run(handler())
    assert res["message"] == "permission denined."

    captured = {}

    class _FakeChatBundle:
        async def async_chat(self, prompt, messages, options):
            captured["prompt"] = prompt
            captured["messages"] = messages
            captured["options"] = options
            return "1. Alpha\n2. Beta\nignored"

    def _fake_bundle(*args, **_kwargs):
        captured["bundle_args"] = args
        return _FakeChatBundle()

    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"question": "solar", "search_id": "search-1"}),
    )
    monkeypatch.setattr(
        module.SearchService,
        "get_detail",
        lambda _search_id: {"search_config": {"chat_id": "chat-x", "llm_setting": {"temperature": 0.2}}},
    )
    monkeypatch.setattr(module, "LLMBundle", _fake_bundle)
    res = _run(handler())
    assert res["code"] == 0
    assert res["data"] == ["Alpha", "Beta"]
    assert captured["bundle_args"] == ("tenant-1", module.LLMType.CHAT, "chat-x")
    assert captured["options"] == {"temperature": 0.2}
    assert "Keywords: solar" in captured["messages"][0]["content"]


@pytest.mark.p2
def test_searchbots_detail_share_embedded_matrix_unit(monkeypatch):
    module = _load_session_module(monkeypatch)
    handler = inspect.unwrap(module.detail_share_embedded)

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer"}, args={"search_id": "s-1"}))
    res = _run(handler())
    assert res["message"] == "Authorization is not valid!"

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer bad"}, args={"search_id": "s-1"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])
    res = _run(handler())
    assert "API key is invalid" in res["message"]

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer ok"}, args={"search_id": "s-1"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="")])
    res = _run(handler())
    assert res["message"] == "permission denined."

    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module.UserTenantService, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-a")])
    monkeypatch.setattr(module.SearchService, "query", lambda **_kwargs: [])
    res = _run(handler())
    assert res["code"] == module.RetCode.OPERATING_ERROR
    assert "Has no permission for this operation." in res["message"]

    monkeypatch.setattr(module.SearchService, "query", lambda **_kwargs: [SimpleNamespace(id="s-1")])
    monkeypatch.setattr(module.SearchService, "get_detail", lambda _sid: None)
    res = _run(handler())
    assert res["message"] == "Can't find this Search App!"

    monkeypatch.setattr(module.SearchService, "get_detail", lambda _sid: {"id": "s-1", "name": "search-app"})
    res = _run(handler())
    assert res["code"] == 0
    assert res["data"]["id"] == "s-1"


@pytest.mark.p2
def test_searchbots_mindmap_embedded_matrix_unit(monkeypatch):
    module = _load_session_module(monkeypatch)
    handler = inspect.unwrap(module.mindmap)

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer"}))
    res = _run(handler())
    assert res["message"] == "Authorization is not valid!"

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer bad"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [])
    res = _run(handler())
    assert "API key is invalid" in res["message"]

    monkeypatch.setattr(module, "request", SimpleNamespace(headers={"Authorization": "Bearer ok"}))
    monkeypatch.setattr(module.APIToken, "query", lambda **_kwargs: [SimpleNamespace(tenant_id="tenant-1")])
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"question": "q", "kb_ids": ["kb-1"]}))

    captured = {}

    async def _gen_ok(question, kb_ids, tenant_id, search_config):
        captured["params"] = (question, kb_ids, tenant_id, search_config)
        return {"nodes": [question]}

    monkeypatch.setattr(module, "gen_mindmap", _gen_ok)
    res = _run(handler())
    assert res["code"] == 0
    assert res["data"] == {"nodes": ["q"]}
    assert captured["params"] == ("q", ["kb-1"], "tenant-1", {})

    monkeypatch.setattr(
        module,
        "get_request_json",
        lambda: _AwaitableValue({"question": "q2", "kb_ids": ["kb-1"], "search_id": "search-1"}),
    )
    monkeypatch.setattr(module.SearchService, "get_detail", lambda _sid: {"search_config": {"mode": "graph"}})
    res = _run(handler())
    assert res["code"] == 0
    assert captured["params"] == ("q2", ["kb-1"], "tenant-1", {"mode": "graph"})

    async def _gen_error(*_args, **_kwargs):
        return {"error": "mindmap boom"}

    monkeypatch.setattr(module, "gen_mindmap", _gen_error)
    res = _run(handler())
    assert "mindmap boom" in res["message"]


@pytest.mark.p2
def test_sequence2txt_embedded_validation_and_stream_matrix_unit(monkeypatch):
    module = _load_session_module(monkeypatch)
    handler = inspect.unwrap(module.sequence2txt)
    monkeypatch.setattr(module, "Response", _StubResponse)
    monkeypatch.setattr(module.tempfile, "mkstemp", lambda suffix: (11, f"/tmp/audio{suffix}"))
    monkeypatch.setattr(module.os, "close", lambda _fd: None)

    def _set_request(form, files):
        monkeypatch.setattr(
            module,
            "request",
            SimpleNamespace(form=_AwaitableValue(form), files=_AwaitableValue(files)),
        )

    _set_request({"stream": "false"}, {})
    res = _run(handler("tenant-1"))
    assert "Missing 'file' in multipart form-data" in res["message"]

    _set_request({"stream": "false"}, {"file": _DummyUploadFile("bad.txt")})
    res = _run(handler("tenant-1"))
    assert "Unsupported audio format: .txt" in res["message"]

    _set_request({"stream": "false"}, {"file": _DummyUploadFile("audio.wav")})
    monkeypatch.setattr(module.TenantService, "get_info_by", lambda _tid: [])
    res = _run(handler("tenant-1"))
    assert res["message"] == "Tenant not found!"

    _set_request({"stream": "false"}, {"file": _DummyUploadFile("audio.wav")})
    monkeypatch.setattr(module.TenantService, "get_info_by", lambda _tid: [{"tenant_id": "tenant-1", "asr_id": ""}])
    res = _run(handler("tenant-1"))
    assert res["message"] == "No default ASR model is set"

    class _SyncASR:
        def transcription(self, _path):
            return "transcribed text"

        def stream_transcription(self, _path):
            return []

    _set_request({"stream": "false"}, {"file": _DummyUploadFile("audio.wav")})
    monkeypatch.setattr(module.TenantService, "get_info_by", lambda _tid: [{"tenant_id": "tenant-1", "asr_id": "asr-x"}])
    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _SyncASR())
    monkeypatch.setattr(module.os, "remove", lambda _path: (_ for _ in ()).throw(RuntimeError("cleanup fail")))
    res = _run(handler("tenant-1"))
    assert res["code"] == 0
    assert res["data"]["text"] == "transcribed text"

    class _StreamASR:
        def transcription(self, _path):
            return ""

        def stream_transcription(self, _path):
            yield {"event": "partial", "text": "hello"}

    _set_request({"stream": "true"}, {"file": _DummyUploadFile("audio.wav")})
    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _StreamASR())
    monkeypatch.setattr(module.os, "remove", lambda _path: None)
    resp = _run(handler("tenant-1"))
    assert isinstance(resp, _StubResponse)
    assert resp.content_type == "text/event-stream"
    chunks = _run(_collect_stream(resp.body))
    assert any('"event": "partial"' in chunk for chunk in chunks)

    class _ErrorASR:
        def transcription(self, _path):
            return ""

        def stream_transcription(self, _path):
            raise RuntimeError("stream asr boom")

    _set_request({"stream": "true"}, {"file": _DummyUploadFile("audio.wav")})
    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _ErrorASR())
    monkeypatch.setattr(module.os, "remove", lambda _path: (_ for _ in ()).throw(RuntimeError("cleanup boom")))
    resp = _run(handler("tenant-1"))
    chunks = _run(_collect_stream(resp.body))
    assert any("stream asr boom" in chunk for chunk in chunks)


@pytest.mark.p2
def test_tts_embedded_stream_and_error_matrix_unit(monkeypatch):
    module = _load_session_module(monkeypatch)
    handler = inspect.unwrap(module.tts)
    monkeypatch.setattr(module, "Response", _StubResponse)
    monkeypatch.setattr(module, "get_request_json", lambda: _AwaitableValue({"text": "A。B"}))

    monkeypatch.setattr(module.TenantService, "get_info_by", lambda _tid: [])
    res = _run(handler("tenant-1"))
    assert res["message"] == "Tenant not found!"

    monkeypatch.setattr(module.TenantService, "get_info_by", lambda _tid: [{"tenant_id": "tenant-1", "tts_id": ""}])
    res = _run(handler("tenant-1"))
    assert res["message"] == "No default TTS model is set"

    class _TTSOk:
        def tts(self, txt):
            if not txt:
                return []
            yield f"chunk-{txt}".encode("utf-8")

    monkeypatch.setattr(module.TenantService, "get_info_by", lambda _tid: [{"tenant_id": "tenant-1", "tts_id": "tts-x"}])
    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _TTSOk())
    resp = _run(handler("tenant-1"))
    assert resp.mimetype == "audio/mpeg"
    assert resp.headers.get("Cache-Control") == "no-cache"
    assert resp.headers.get("Connection") == "keep-alive"
    assert resp.headers.get("X-Accel-Buffering") == "no"
    chunks = _run(_collect_stream(resp.body))
    assert any("chunk-A" in chunk for chunk in chunks)
    assert any("chunk-B" in chunk for chunk in chunks)

    class _TTSErr:
        def tts(self, _txt):
            raise RuntimeError("tts boom")

    monkeypatch.setattr(module, "LLMBundle", lambda *_args, **_kwargs: _TTSErr())
    resp = _run(handler("tenant-1"))
    chunks = _run(_collect_stream(resp.body))
    assert any('"code": 500' in chunk and "**ERROR**: tts boom" in chunk for chunk in chunks)


@pytest.mark.p2
def test_build_reference_chunks_metadata_matrix_unit(monkeypatch):
    module = _load_session_module(monkeypatch)

    monkeypatch.setattr(module, "chunks_format", lambda _reference: [{"dataset_id": "kb-1", "document_id": "doc-1"}])
    res = module._build_reference_chunks([], include_metadata=False)
    assert res == [{"dataset_id": "kb-1", "document_id": "doc-1"}]

    monkeypatch.setattr(module, "chunks_format", lambda _reference: [{"dataset_id": "kb-1"}, {"document_id": "doc-2"}])
    res = module._build_reference_chunks([], include_metadata=True)
    assert all("document_metadata" not in chunk for chunk in res)

    monkeypatch.setattr(module, "chunks_format", lambda _reference: [{"dataset_id": "kb-1", "document_id": "doc-1"}])
    monkeypatch.setattr(module.DocMetadataService, "get_metadata_for_documents", lambda _doc_ids, _kb_id: {"doc-1": {"author": "alice"}})
    res = module._build_reference_chunks([], include_metadata=True, metadata_fields=[1, None])
    assert "document_metadata" not in res[0]

    source_chunks = [
        {"dataset_id": "kb-1", "document_id": "doc-1"},
        {"dataset_id": "kb-2", "document_id": "doc-2"},
        {"dataset_id": "kb-1", "document_id": "doc-3"},
        {"dataset_id": "kb-1", "document_id": None},
    ]
    monkeypatch.setattr(module, "chunks_format", lambda _reference: [dict(chunk) for chunk in source_chunks])

    def _get_metadata(_doc_ids, kb_id):
        if kb_id == "kb-1":
            return {"doc-1": {"author": "alice", "year": 2024}}
        if kb_id == "kb-2":
            return {"doc-2": {"author": "bob", "tag": "rag"}}
        return {}

    monkeypatch.setattr(module.DocMetadataService, "get_metadata_for_documents", _get_metadata)
    res = module._build_reference_chunks([], include_metadata=True, metadata_fields=["author", "missing", 3])
    assert res[0]["document_metadata"] == {"author": "alice"}
    assert res[1]["document_metadata"] == {"author": "bob"}
    assert "document_metadata" not in res[2]
    assert "document_metadata" not in res[3]
