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
