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

from ragflow_sdk.modules.agent import Agent
from ragflow_sdk.modules.session import Session


class _StubResponse:
    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


@pytest.mark.p2
class TestAgentSessions:
    def test_agent_session_methods_success_and_error(self, monkeypatch):
        agent = Agent(object(), {"id": "agent_id"})

        def _post(_path, _json=None, stream=False, files=None, **_kwargs):
            return _StubResponse({"code": 0, "data": {"agent_id": "agent_id", "id": "sess_id"}})

        def _get(_path, _params=None, **_kwargs):
            return _StubResponse({"code": 0, "data": [{"agent_id": "agent_id", "id": "sess_id"}]})

        def _rm(_path, _json=None, **_kwargs):
            return _StubResponse({"code": 1, "message": "boom"})

        monkeypatch.setattr(agent, "post", _post)
        monkeypatch.setattr(agent, "get", _get)
        monkeypatch.setattr(agent, "rm", _rm)

        session = agent.create_session()
        assert isinstance(session, Session)

        sessions = agent.list_sessions()
        assert sessions
        assert all(isinstance(item, Session) for item in sessions)

        with pytest.raises(Exception) as excinfo:
            agent.delete_sessions(ids=["sess_id"])
        assert "boom" in str(excinfo.value).lower()
