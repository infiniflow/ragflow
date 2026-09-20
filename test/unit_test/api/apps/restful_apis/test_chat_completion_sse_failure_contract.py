import importlib
import inspect
import json
import socket
import sys
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest

from common import settings

_original_init_settings = settings.init_settings
_original_get_secret_key = settings.get_secret_key
_original_socket_connect = socket.socket.connect
_original_create_connection = socket.create_connection
_isolated_module_namespaces = ("common.connection_utils", "deepdoc.parser", "rag.llm")


def _is_isolated_module(module_name):
    return any(module_name == namespace or module_name.startswith(f"{namespace}.") for namespace in _isolated_module_namespaces)


_isolated_modules = {key: sys.modules[key] for key in sys.modules if _is_isolated_module(key)}


def _blocked_import_socket_connect(*args, **kwargs):
    raise AssertionError("network access attempted while importing chat_api in an offline test")


settings.init_settings = lambda: None
settings.get_secret_key = lambda: "offline-test-secret"
socket.socket.connect = _blocked_import_socket_connect
socket.create_connection = _blocked_import_socket_connect
try:
    for key in _isolated_modules:
        del sys.modules[key]
    importlib.invalidate_caches()
    chat_api = importlib.import_module("api.apps.restful_apis.chat_api")
finally:
    for key in list(sys.modules):
        if _is_isolated_module(key):
            del sys.modules[key]
    sys.modules.update(_isolated_modules)
    settings.init_settings = _original_init_settings
    settings.get_secret_key = _original_get_secret_key
    socket.socket.connect = _original_socket_connect
    socket.create_connection = _original_create_connection


@pytest.fixture(autouse=True)
def block_network(monkeypatch):
    def _blocked(*args, **kwargs):
        raise AssertionError("network access attempted in an offline chat_api SSE test")

    monkeypatch.setattr(socket.socket, "connect", _blocked)
    monkeypatch.setattr(socket, "create_connection", _blocked)


async def _run_stream(monkeypatch, rag_agent_events):
    req = {
        "messages": [{"role": "user", "content": "hi"}],
        "llm_id": "test-model",
        "stream": True,
    }

    async def fake_rag_agent(dia, msg, stream, **kwargs):
        for event in rag_agent_events:
            if isinstance(event, BaseException):
                raise event
            yield event

    def fake_get_api_key(**kwargs):
        return True

    monkeypatch.setattr(chat_api, "rag_agent", fake_rag_agent)
    monkeypatch.setattr(chat_api, "current_user", SimpleNamespace(id="tenant-test"))
    monkeypatch.setattr(chat_api, "get_request_json", AsyncMock(return_value=dict(req)))
    monkeypatch.setattr(chat_api, "get_api_key", fake_get_api_key)

    handler = inspect.unwrap(chat_api.session_completion)
    resp = await handler()

    events = []
    async for chunk in resp.response:
        text = chunk.decode("utf-8") if isinstance(chunk, (bytes, bytearray)) else chunk
        for line in text.split("\n\n"):
            line = line.strip()
            if line.startswith("data:"):
                events.append(json.loads(line[len("data:") :]))
    return events


async def test_successful_stream_ends_with_exactly_one_success_terminal(monkeypatch):
    events = await _run_stream(
        monkeypatch,
        [
            {"answer": "Hello", "reference": {}, "audio_binary": None, "final": False},
            {"answer": "", "reference": {}, "audio_binary": None, "final": True},
        ],
    )
    assert all(event["code"] == 0 for event in events)
    terminals = [event for event in events if event.get("data") is True]
    assert len(terminals) == 1, f"expected exactly one success terminal, got: {events}"
    assert events[-1] == {"code": 0, "message": "", "data": True}


async def test_failed_stream_ends_with_error_and_no_success_terminal(monkeypatch):
    events = await _run_stream(
        monkeypatch,
        [
            {"answer": "partial", "reference": {}, "audio_binary": None, "final": False},
            RuntimeError("agentic RAG research failed: boom"),
        ],
    )
    error_events = [event for event in events if event["code"] == 500]
    assert error_events == [
        {
            "code": 500,
            "message": "agentic RAG research failed: boom",
            "data": {
                "answer": "**ERROR**: agentic RAG research failed: boom",
                "reference": [],
            },
        }
    ]
    assert not any(event.get("data") is True for event in events), f"a success terminal leaked into a failed stream: {events}"


async def test_failure_before_any_token_still_ends_stream_without_success_terminal(monkeypatch):
    events = await _run_stream(monkeypatch, [RuntimeError("boom before any token")])
    assert events == [
        {
            "code": 500,
            "message": "boom before any token",
            "data": {
                "answer": "**ERROR**: boom before any token",
                "reference": [],
            },
        }
    ]
