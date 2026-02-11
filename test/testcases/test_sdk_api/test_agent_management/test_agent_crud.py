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
import uuid

import pytest


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


@pytest.mark.p2
class TestAgentCrud:
    def test_agent_crud_create_list_delete(self, client):
        title = f"sdk_agent_{uuid.uuid4().hex}"
        client.create_agent(title=title, dsl=MINIMAL_DSL)

        agents = client.list_agents(title=title)
        assert agents, agents
        assert any(agent.title == title for agent in agents), agents

        agent_id = None
        for agent in agents:
            if agent.title == title:
                agent_id = agent.id
                break

        try:
            assert agent_id, agents
        finally:
            if agent_id:
                try:
                    client.delete_agent(agent_id)
                except Exception:
                    pass
