import sys
from pathlib import Path
import types

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from session_stub import load_session_module


@pytest.mark.asyncio
async def test_chatbot_completions_auth_and_defaults(monkeypatch):
    mod = load_session_module(monkeypatch)

    mod._stub_request.headers = {"Authorization": "invalid"}
    async def _get_request_json():
        return {"stream": False}

    mod.get_request_json = _get_request_json
    resp = await mod.chatbot_completions("dialog")
    assert resp["code"] != 0
    assert "Authorization is not valid" in resp["message"]

    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    captured = {}

    async def _iframe_completion(_dialog_id, **req):
        captured.update(req)
        yield {"answer": "ok"}

    mod.iframe_completion = _iframe_completion
    resp = await mod.chatbot_completions("dialog")
    assert resp["code"] == 0
    assert captured["quote"] is False


@pytest.mark.asyncio
async def test_chatbot_completions_stream_headers(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {"stream": True}

    async def _iframe_completion(*_args, **_kwargs):
        yield "data: {}\n\n"

    mod.get_request_json = _get_request_json
    mod.iframe_completion = _iframe_completion
    resp = await mod.chatbot_completions("dialog")
    assert "text/event-stream" in resp.headers.get("Content-Type", "")
    assert resp.headers.get("Cache-control") == "no-cache"


@pytest.mark.asyncio
async def test_chatbot_completions_non_stream_response(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {"stream": False}

    async def _iframe_completion(*_args, **_kwargs):
        yield {"answer": "ok"}

    mod.get_request_json = _get_request_json
    mod.iframe_completion = _iframe_completion
    resp = await mod.chatbot_completions("dialog")
    assert resp["code"] == 0
    assert resp["data"]["answer"] == "ok"


@pytest.mark.asyncio
async def test_chatbot_completions_empty_generator(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {"stream": False}

    async def _iframe_completion(*_args, **_kwargs):
        if False:
            yield None

    mod.get_request_json = _get_request_json
    mod.iframe_completion = _iframe_completion
    resp = await mod.chatbot_completions("dialog")
    assert resp is None


@pytest.mark.asyncio
async def test_chatbots_inputs_auth_and_dialog_missing(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "invalid"}
    resp = await mod.chatbots_inputs("dialog")
    assert resp["code"] != 0
    assert "Authorization is not valid" in resp["message"]

    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    mod.DialogService.get_by_id = classmethod(lambda cls, _id: (False, None))
    resp = await mod.chatbots_inputs("dialog")
    assert resp["code"] != 0
    assert "Can't find dialog" in resp["message"]


@pytest.mark.asyncio
async def test_chatbots_inputs_returns_metadata(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    dialog = types.SimpleNamespace(name="n", icon="i", prompt_config={"prologue": "p", "tavily_api_key": "k"})
    mod.DialogService.get_by_id = classmethod(lambda cls, _id: (True, dialog))

    resp = await mod.chatbots_inputs("dialog")
    assert resp["code"] == 0
    assert resp["data"]["title"] == "n"
    assert resp["data"]["has_tavily_key"] is True


@pytest.mark.asyncio
async def test_agentbot_completions_auth_and_defaults(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "invalid"}

    async def _get_request_json():
        return {"stream": False}

    mod.get_request_json = _get_request_json
    resp = await mod.agent_bot_completions("agent")
    assert resp["code"] != 0
    assert "Authorization is not valid" in resp["message"]

    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    captured = {}

    async def _agent_completion(_tenant_id, _agent_id, **req):
        captured.update(req)
        yield {"answer": "ok"}

    mod.agent_completion = _agent_completion
    resp = await mod.agent_bot_completions("agent")
    assert resp["code"] == 0


@pytest.mark.asyncio
async def test_agentbot_completions_stream_headers(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {"stream": True}

    async def _agent_completion(*_args, **_kwargs):
        yield "data: {}\n\n"

    mod.get_request_json = _get_request_json
    mod.agent_completion = _agent_completion
    resp = await mod.agent_bot_completions("agent")
    assert "text/event-stream" in resp.headers.get("Content-Type", "")


@pytest.mark.asyncio
async def test_agentbot_completions_non_stream_response(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {"stream": False}

    async def _agent_completion(*_args, **_kwargs):
        yield {"answer": "ok"}

    mod.get_request_json = _get_request_json
    mod.agent_completion = _agent_completion
    resp = await mod.agent_bot_completions("agent")
    assert resp["code"] == 0
    assert resp["data"]["answer"] == "ok"


@pytest.mark.asyncio
async def test_agentbot_completions_empty_generator(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])

    async def _get_request_json():
        return {"stream": False}

    async def _agent_completion(*_args, **_kwargs):
        if False:
            yield None

    mod.get_request_json = _get_request_json
    mod.agent_completion = _agent_completion
    resp = await mod.agent_bot_completions("agent")
    assert resp is None


@pytest.mark.asyncio
async def test_agentbot_inputs_auth_and_agent_missing(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "invalid"}
    resp = await mod.begin_inputs("agent")
    assert resp["code"] != 0
    assert "Authorization is not valid" in resp["message"]

    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    mod.UserCanvasService.get_by_id = classmethod(lambda cls, _id: (False, None))
    resp = await mod.begin_inputs("agent")
    assert resp["code"] != 0
    assert "Can't find agent" in resp["message"]


@pytest.mark.asyncio
async def test_agentbot_inputs_returns_canvas_payload(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod._stub_request.headers = {"Authorization": "Bearer token"}
    mod.APIToken.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(tenant_id="t")])
    cvs = types.SimpleNamespace(id="agent", title="t", avatar="a", dsl={})
    mod.UserCanvasService.get_by_id = classmethod(lambda cls, _id: (True, cvs))
    resp = await mod.begin_inputs("agent")
    assert resp["code"] == 0
    assert "inputs" in resp["data"]
    assert resp["data"]["prologue"] == "prologue"
