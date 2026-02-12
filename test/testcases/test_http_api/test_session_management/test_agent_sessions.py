#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import requests
import pytest
import types
from pathlib import Path
import sys
from common import (
    create_agent,
    create_agent_session,
    delete_agent,
    delete_agent_sessions,
    list_agent_sessions,
    list_agents,
)
from configs import HOST_ADDRESS, VERSION

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from session_stub import load_session_module

AGENT_TITLE = "test_agent_http"
MINIMAL_DSL = {
    "components": {
        "begin": {
            "obj": {"component_name": "Begin", "params": {}},
            "downstream": ["message"],
            "upstream": [],
        },
        "message": {
            "obj": {"component_name": "Message", "params": {"content": ["{sys.query}"]}},
            "downstream": [],
            "upstream": ["begin"],
        },
    },
    "history": [],
    "retrieval": [],
    "path": [],
    "globals": {
        "sys.query": "",
        "sys.user_id": "",
        "sys.conversation_turns": 0,
        "sys.files": [],
    },
    "variables": {},
}

@pytest.fixture(scope="function")
def agent_id(HttpApiAuth, request):
    res = list_agents(HttpApiAuth, {"page_size": 1000})
    assert res["code"] == 0, res
    for agent in res.get("data", []):
        if agent.get("title") == AGENT_TITLE:
            delete_agent(HttpApiAuth, agent["id"])

    res = create_agent(HttpApiAuth, {"title": AGENT_TITLE, "dsl": MINIMAL_DSL})
    assert res["code"] == 0, res
    res = list_agents(HttpApiAuth, {"title": AGENT_TITLE})
    assert res["code"] == 0, res
    assert res.get("data"), res
    agent_id = res["data"][0]["id"]

    def cleanup():
        delete_agent_sessions(HttpApiAuth, agent_id)
        delete_agent(HttpApiAuth, agent_id)

    request.addfinalizer(cleanup)
    return agent_id


class TestAgentSessions:
    @pytest.mark.p2
    def test_create_list_delete_agent_sessions(self, HttpApiAuth, agent_id):
        res = create_agent_session(HttpApiAuth, agent_id, payload={})
        assert res["code"] == 0, res
        session_id = res["data"]["id"]
        assert res["data"]["agent_id"] == agent_id, res

        res = list_agent_sessions(HttpApiAuth, agent_id, params={"id": session_id})
        assert res["code"] == 0, res
        assert len(res["data"]) == 1, res
        assert res["data"][0]["id"] == session_id, res

        res = delete_agent_sessions(HttpApiAuth, agent_id, {"ids": [session_id]})
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_delete_agent_sessions_no_sessions(self, HttpApiAuth, agent_id):
        res = delete_agent_sessions(HttpApiAuth, agent_id)
        assert res.get("code") != 0, res
        assert "has no sessions" in res.get("message", ""), res

    @pytest.mark.p2
    def test_create_agent_missing_fields(self, HttpApiAuth):
        res = create_agent(HttpApiAuth, {"title": "missing_dsl"})
        assert res["code"] != 0, res
        assert "dsl" in res.get("message", "").lower(), res

        res = create_agent(HttpApiAuth, {"dsl": MINIMAL_DSL})
        assert res["code"] != 0, res
        assert "title" in res.get("message", "").lower(), res

    @pytest.mark.p2
    def test_create_agent_duplicate_title(self, HttpApiAuth, agent_id):
        res = create_agent(HttpApiAuth, {"title": AGENT_TITLE, "dsl": MINIMAL_DSL})
        assert res["code"] != 0, res
        assert "exist" in res.get("message", "").lower(), res

    @pytest.mark.p2
    def test_list_agents_not_found(self, HttpApiAuth):
        res = list_agents(HttpApiAuth, {"id": "invalid_agent_id"})
        assert res["code"] != 0, res
        assert "exist" in res.get("message", "").lower(), res

    @pytest.mark.p2
    def test_delete_agent_invalid_id_or_not_found(self, HttpApiAuth):
        res = delete_agent(HttpApiAuth, "invalid_agent_id")
        assert res["code"] != 0, res
        assert "authorized" in res.get("message", "").lower(), res

    @pytest.mark.p2
    def test_webhook_invalid_agent_id_early_exit(self):
        url = f"{HOST_ADDRESS}/api/{VERSION}/webhook/invalid_agent_id"
        res = requests.post(url=url)
        body = res.json()
        assert body.get("code") != 0, body
        assert "not found" in body.get("message", "").lower(), body


@pytest.mark.asyncio
async def test_create_agent_session_agent_not_found(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod.UserCanvasService.get_by_id = classmethod(lambda cls, _id: (False, None))
    resp = await mod.create_agent_session("tenant", "agent")
    assert resp["code"] != 0
    assert resp["message"] == "Agent not found."


@pytest.mark.asyncio
async def test_create_agent_session_access_denied(monkeypatch):
    mod = load_session_module(monkeypatch)

    cvs = types.SimpleNamespace(id="agent", dsl="{}")
    mod.UserCanvasService.get_by_id = classmethod(lambda cls, _id: (True, cvs))
    mod.UserCanvasService.query = classmethod(lambda cls, **_kwargs: [])

    resp = await mod.create_agent_session("tenant", "agent")
    assert resp["code"] != 0
    assert resp["message"] == "You cannot access the agent."


@pytest.mark.asyncio
async def test_list_agent_sessions_invalid_owner(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod.UserCanvasService.query = classmethod(lambda cls, **_kwargs: [])
    resp = await mod.list_agent_session("tenant", "agent")
    assert resp["code"] != 0
    assert "You don't own the agent" in resp["message"]


@pytest.mark.asyncio
async def test_list_agent_sessions_desc_false_and_empty(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod.UserCanvasService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="agent")])

    captured = {}

    def _get_list(agent_id, tenant_id, page_number, items_per_page, orderby, desc, _id, user_id, include_dsl):
        captured["desc"] = desc
        return 0, []

    mod.API4ConversationService.get_list = classmethod(lambda cls, *args, **kwargs: _get_list(*args, **kwargs))
    mod._stub_request.args = {"desc": "false"}
    resp = await mod.list_agent_session("tenant", "agent")
    assert resp["code"] == 0
    assert resp["data"] == []
    assert captured["desc"] is False


@pytest.mark.asyncio
async def test_list_agent_sessions_reference_mapping_guards(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod.UserCanvasService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="agent")])

    convs = [
        {
            "id": "c1",
            "dialog_id": "agent",
            "message": [{"role": "user", "content": "hi"}, {"role": "assistant", "content": "ok", "prompt": "p"}],
            "reference": "not-list",
        },
        {
            "id": "c2",
            "dialog_id": "agent",
            "message": [{"role": "user", "content": "hi"}, {"role": "assistant", "content": "ok"}],
            "reference": [{"chunks": ["bad", {"chunk_id": "ch1", "content": "c"}]}],
        },
    ]

    mod.API4ConversationService.get_list = classmethod(
        lambda cls, *args, **kwargs: (2, [dict(item) for item in convs])
    )

    resp = await mod.list_agent_session("tenant", "agent")
    assert resp["code"] == 0
    data = resp["data"]
    assert data[0]["messages"][1].get("prompt") is None
    assert data[0]["messages"][1]["reference"] == []
    assert data[1]["messages"][1]["reference"][0]["id"] == "ch1"


@pytest.mark.asyncio
async def test_delete_agent_sessions_not_owner(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod.UserCanvasService.query = classmethod(lambda cls, **_kwargs: [])
    resp = await mod.delete_agent_session("tenant", "agent")
    assert resp["code"] != 0
    assert "You don't own the agent" in resp["message"]


@pytest.mark.asyncio
async def test_delete_agent_sessions_partial_errors_and_duplicates(monkeypatch):
    mod = load_session_module(monkeypatch)
    mod.UserCanvasService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="agent")])
    mod._stub_request._json = {"ids": ["s1", "s2"]}

    async def _get_request_json():
        return {"ids": ["s1", "s2"]}

    mod.get_request_json = _get_request_json

    mod.API4ConversationService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="s1"), types.SimpleNamespace(id="s2")])

    def _query_by_id(**kwargs):
        if kwargs.get("id") == "s1":
            return []
        return [types.SimpleNamespace(id=kwargs.get("id"))]

    mod.API4ConversationService.query = classmethod(lambda cls, **kwargs: _query_by_id(**kwargs) if "id" in kwargs else [types.SimpleNamespace(id="s1"), types.SimpleNamespace(id="s2")])

    mod.check_duplicate_ids = lambda ids, _kind="session": (ids, [])
    resp = await mod.delete_agent_session("tenant", "agent")
    assert resp["code"] == 0
    assert "Partially deleted" in resp.get("message", "")

    mod.API4ConversationService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="s1")])
    mod.check_duplicate_ids = lambda ids, _kind="session": (["s1"], ["duplicate s1"])
    mod.API4ConversationService.query = classmethod(lambda cls, **kwargs: [types.SimpleNamespace(id="s1")] if "id" in kwargs else [types.SimpleNamespace(id="s1")])
    resp = await mod.delete_agent_session("tenant", "agent")
    assert resp["code"] == 0
    assert "Partially deleted" in resp.get("message", "")

    mod.API4ConversationService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="s1")])
    mod.check_duplicate_ids = lambda ids, _kind="session": (["s1"], ["duplicate s1"])
    mod.API4ConversationService.query = classmethod(lambda cls, **kwargs: [] if "id" in kwargs else [types.SimpleNamespace(id="s1")])
    resp = await mod.delete_agent_session("tenant", "agent")
    assert resp["code"] != 0
    assert "duplicate" in resp.get("message", "")
