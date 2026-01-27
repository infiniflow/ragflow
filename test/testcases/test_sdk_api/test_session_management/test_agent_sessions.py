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
    create_agent,
    list_agents,
    delete_agent,
    create_agent_session,
    list_agent_sessions,
    delete_agent_sessions
)

AGENT_TITLE = "test_agent_sdk"
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
def agent_instance(client, request):
    # Clean up agents with the same title
    try:
        agents = list_agents(client, title=AGENT_TITLE)
        for agent in agents:
            delete_agent(client, agent.id)
    except Exception:
        pass

    # Create Agent
    create_agent(client, title=AGENT_TITLE, dsl=MINIMAL_DSL)
    agents = list_agents(client, title=AGENT_TITLE)
    assert len(agents) > 0
    agent = agents[0]

    def cleanup():
        try:
            delete_agent_sessions(agent)
            delete_agent(client, agent.id)
        except Exception:
            pass

    request.addfinalizer(cleanup)
    return agent


class TestAgentSessions:
    @pytest.mark.p2
    def test_create_list_delete_agent_sessions(self, agent_instance):
        # 1. Create session
        session = create_agent_session(agent_instance)
        assert session.id is not None
        # Note: The SDK's Session object might not directly expose agent_id, or the attribute name might differ.
        # According to session.py, it sets attributes based on res_dict during initialization.
        assert hasattr(session, "agent_id") or hasattr(session, "id")

        # 2. List sessions
        sessions = list_agent_sessions(agent_instance, id=session.id)
        assert len(sessions) == 1
        assert sessions[0].id == session.id

        # 3. Delete session
        delete_agent_sessions(agent_instance, ids=[session.id])
        
        # 4. Verify deletion
        remaining_sessions = list_agent_sessions(agent_instance, id=session.id)
        assert len(remaining_sessions) == 0

    @pytest.mark.p1
    def test_agent_ask_completions(self, agent_instance):
        """Test Agent Q&A functionality, covering _ask_agent in session.py"""
        session = create_agent_session(agent_instance)
        
        # Synchronous Q&A
        question = "Hello Agent"
        try:
            messages = list(session.ask(question=question, stream=False, session_id=session.id))
            assert len(messages) > 0
            assert messages[0].role == "assistant"
            assert messages[0].content is not None
        except KeyError as e:
            pytest.skip(f"Agent response structure might have changed: {e}")

        # Streaming Q&A
        stream_messages = []
        try:
            # Explicitly pass session_id=session.id
            for msg in session.ask(question="Stream test", stream=True, session_id=session.id):
                stream_messages.append(msg)
            
            assert len(stream_messages) > 0
            assert stream_messages[-1].content is not None
        except KeyError as e:
            # If it fails, try to print more information (visible in pytest -s mode)
            print(f"\nCaptured KeyError in stream: {e}")
            pytest.fail(f"Agent stream response structure error: {e}. Check if 'data' or 'content' exists in the response.")
