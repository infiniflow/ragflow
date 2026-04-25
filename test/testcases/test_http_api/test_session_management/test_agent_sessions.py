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
import requests
from common import (
    create_agent,
    delete_agent,
    delete_all_agent_sessions,
    list_agents,
)
from configs import HOST_ADDRESS, VERSION

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


def _agent_items(res):
    data = res.get("data", [])
    if isinstance(data, dict):
        return data.get("canvas", [])
    return data

@pytest.fixture(scope="function")
def agent_id(HttpApiAuth, request):
    res = list_agents(HttpApiAuth, {"page_size": 1000})
    assert res["code"] == 0, res
    for agent in _agent_items(res):
        if agent.get("title") == AGENT_TITLE:
            delete_agent(HttpApiAuth, agent["id"])

    res = create_agent(HttpApiAuth, {"title": AGENT_TITLE, "dsl": MINIMAL_DSL})
    assert res["code"] == 0, res
    res = list_agents(HttpApiAuth, {"title": AGENT_TITLE})
    assert res["code"] == 0, res
    agents = _agent_items(res)
    assert agents, res
    agent_id = agents[0]["id"]

    def cleanup():
        delete_all_agent_sessions(HttpApiAuth, agent_id)
        delete_agent(HttpApiAuth, agent_id)

    request.addfinalizer(cleanup)
    return agent_id


class TestAgentSessions:

    @pytest.mark.p2
    def test_agent_crud_validation_contract(self, HttpApiAuth, agent_id):
        res = list_agents(HttpApiAuth, {"id": "missing-agent-id", "title": "missing-agent-title"})
        assert res["code"] == 0, res
        assert isinstance(res.get("data"), dict), res
        assert "canvas" in res["data"], res
        assert "total" in res["data"], res

        res = list_agents(HttpApiAuth, {"title": AGENT_TITLE, "desc": "true", "page_size": 1})
        assert res["code"] == 0, res

        res = create_agent(HttpApiAuth, {"title": "missing-dsl-agent"})
        assert res["code"] == 101, res
        assert "No DSL data in request" in res["message"], res

        res = create_agent(HttpApiAuth, {"dsl": MINIMAL_DSL})
        assert res["code"] == 101, res
        assert "No title in request" in res["message"], res

        res = create_agent(HttpApiAuth, {"title": AGENT_TITLE, "dsl": MINIMAL_DSL})
        assert res["code"] == 102, res
        assert "already exists" in res["message"], res

        update_url = f"{HOST_ADDRESS}/api/{VERSION}/agents/invalid-agent-id"
        res = requests.put(update_url, auth=HttpApiAuth, json={"title": "updated", "dsl": MINIMAL_DSL}).json()
        assert res["code"] == 103, res
        assert "Only owner of canvas authorized" in res["message"], res

        res = delete_agent(HttpApiAuth, "invalid-agent-id")
        assert res["code"] == 103, res
        assert "Only owner of canvas authorized" in res["message"], res
