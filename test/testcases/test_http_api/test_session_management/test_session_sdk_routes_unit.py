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
