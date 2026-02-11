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
from common import (
    create_agent,
    create_agent_session,
    delete_agent,
    delete_agent_sessions,
    list_agent_sessions,
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
