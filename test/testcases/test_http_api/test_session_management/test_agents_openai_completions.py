import json
import sys
from pathlib import Path
import types

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from session_stub import load_session_module


@pytest.mark.asyncio
async def test_agents_openai_validation_errors(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"model": "model", "messages": []}

    mod.get_request_json = _get_request_json
    mod.UserCanvasService.query = classmethod(lambda cls, **_kwargs: [])

    resp = await mod.agents_completion_openai_compatibility("tenant", "agent")
    assert resp["code"] != 0
    assert "at least one message" in resp["message"]

    async def _get_request_json_unauth():
        return {"model": "model", "messages": [{"role": "user", "content": "hi"}]}

    mod.get_request_json = _get_request_json_unauth
    resp = await mod.agents_completion_openai_compatibility("tenant", "agent")
    assert resp["code"] != 0
    assert "You don't own the agent" in resp["message"]


@pytest.mark.asyncio
async def test_agents_openai_filtered_messages_empty(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"model": "model", "messages": [{"role": "system", "content": "sys"}]}

    mod.get_request_json = _get_request_json
    mod.UserCanvasService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="agent")])

    resp = await mod.agents_completion_openai_compatibility("tenant", "agent")
    assert resp["content"] == "No valid messages found (user or assistant)."


@pytest.mark.asyncio
async def test_agents_openai_stream_headers(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"model": "model", "messages": [{"role": "user", "content": "hi"}], "stream": True}

    async def _completion_openai(*_args, **_kwargs):
        yield "data: {}\n\n"

    mod.get_request_json = _get_request_json
    mod.UserCanvasService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="agent")])
    mod.completion_openai = _completion_openai

    resp = await mod.agents_completion_openai_compatibility("tenant", "agent")
    assert "text/event-stream" in resp.headers.get("Content-Type", "")
    assert resp.headers.get("Cache-control") == "no-cache"


@pytest.mark.asyncio
async def test_agents_openai_non_stream_first_response(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"model": "model", "messages": [{"role": "user", "content": "hi"}], "stream": False}

    async def _completion_openai(*_args, **_kwargs):
        yield {"id": "resp-1"}

    mod.get_request_json = _get_request_json
    mod.UserCanvasService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="agent")])
    mod.completion_openai = _completion_openai

    resp = await mod.agents_completion_openai_compatibility("tenant", "agent")
    assert resp["id"] == "resp-1"


@pytest.mark.asyncio
async def test_agents_openai_non_stream_empty_generator(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"model": "model", "messages": [{"role": "user", "content": "hi"}], "stream": False}

    async def _completion_openai(*_args, **_kwargs):
        if False:
            yield None

    mod.get_request_json = _get_request_json
    mod.UserCanvasService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="agent")])
    mod.completion_openai = _completion_openai

    resp = await mod.agents_completion_openai_compatibility("tenant", "agent")
    assert resp is None
