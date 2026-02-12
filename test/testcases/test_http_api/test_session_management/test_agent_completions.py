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
import pytest
import sys
from pathlib import Path
import types
import json

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from session_stub import load_session_module
from common import (
    agent_completions,
    create_agent,
    create_agent_session,
    delete_agent,
    delete_agent_sessions,
    list_agents,
)

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


class TestAgentCompletions:
    @pytest.mark.p2
    def test_agent_completion_stream_false(self, HttpApiAuth, agent_id):
        res = create_agent_session(HttpApiAuth, agent_id, payload={})
        assert res["code"] == 0, res
        session_id = res["data"]["id"]

        res = agent_completions(
            HttpApiAuth,
            agent_id,
            {"question": "hello", "stream": False, "session_id": session_id},
        )
        assert res["code"] == 0, res
        if isinstance(res["data"], dict):
            assert isinstance(res["data"].get("data"), dict), res
            content = res["data"]["data"].get("content", "")
            assert content, res
            assert "hello" in content, res
            assert res["data"].get("session_id") == session_id, res
        else:
            assert isinstance(res["data"], str), res
            assert res["data"].startswith("**ERROR**"), res


@pytest.mark.asyncio
async def test_agent_completions_stream_trace_and_done(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"stream": True, "return_trace": True}

    async def _agent_completion(**_kwargs):
        yield "data:{\"event\": \"bad\"}\n\n"
        yield "data:{\"event\": \"node_finished\", \"data\": {\"component_id\": \"c1\"}}\n\n"
        yield "data:{\"event\": \"message\", \"data\": {\"content\": \"hi\"}}\n\n"
        yield "data:{\"event\": \"message_end\", \"data\": {}}\n\n"

    mod.get_request_json = _get_request_json
    mod.agent_completion = _agent_completion

    resp = await mod.agent_completions("tenant", "agent")
    lines = []
    async for line in resp.response:
        lines.append(line)

    assert any("trace" in line for line in lines)
    assert any(line.strip() == "data:[DONE]" or "data:[DONE]" in line for line in lines)
    assert "text/event-stream" in resp.headers.get("Content-Type", "")


@pytest.mark.asyncio
async def test_agent_completions_non_stream_reference_trace(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"stream": False, "return_trace": True}

    async def _agent_completion(**_kwargs):
        yield "data:{\"event\": \"message\", \"data\": {\"content\": \"a\", \"reference\": {\"r1\": 1}}}\n\n"
        yield "data:{\"event\": \"node_finished\", \"data\": {\"component_id\": \"c1\"}}\n\n"
        yield "data:{\"event\": \"message\", \"data\": {\"content\": \"b\", \"reference\": {\"r2\": 2}}}\n\n"

    mod.get_request_json = _get_request_json
    mod.agent_completion = _agent_completion

    resp = await mod.agent_completions("tenant", "agent")
    data = resp["data"]["data"]
    assert data["content"] == "ab"
    assert data["reference"] == {"r1": 1, "r2": 2}
    assert "trace" in data


@pytest.mark.asyncio
async def test_agent_completions_non_stream_bad_json_error(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"stream": False}

    async def _agent_completion(**_kwargs):
        yield "data:{bad json}\n\n"

    mod.get_request_json = _get_request_json
    mod.agent_completion = _agent_completion

    resp = await mod.agent_completions("tenant", "agent")
    assert resp["data"].startswith("**ERROR**")
