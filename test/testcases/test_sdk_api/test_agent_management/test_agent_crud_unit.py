#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
from ragflow_sdk import RAGFlow
from ragflow_sdk.modules.agent import Agent


class _DummyResponse:
    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


@pytest.mark.p2
def test_list_agents_success_and_error(monkeypatch):
    client = RAGFlow("token", "http://localhost:9380")
    captured = {}

    def _ok_get(path, params=None, json=None):
        captured["path"] = path
        captured["params"] = params
        captured["json"] = json
        return _DummyResponse({"code": 0, "data": [{"id": "agent-1", "title": "Agent One"}]})

    monkeypatch.setattr(client, "get", _ok_get)
    agents = client.list_agents(title="Agent One")
    assert captured["path"] == "/agents"
    assert captured["params"]["title"] == "Agent One"
    assert isinstance(agents[0], Agent), str(agents)
    assert agents[0].id == "agent-1", str(agents[0])
    assert agents[0].title == "Agent One", str(agents[0])

    monkeypatch.setattr(client, "get", lambda *_args, **_kwargs: _DummyResponse({"code": 1, "message": "list boom"}))
    with pytest.raises(Exception) as exception_info:
        client.list_agents()
    assert "list boom" in str(exception_info.value), str(exception_info.value)


@pytest.mark.p2
def test_create_agent_payload_and_error(monkeypatch):
    client = RAGFlow("token", "http://localhost:9380")
    calls = []

    def _ok_post(path, json=None, stream=False, files=None):
        calls.append((path, json, stream, files))
        return _DummyResponse({"code": 0, "message": "ok"})

    monkeypatch.setattr(client, "post", _ok_post)
    client.create_agent("agent-title", {"graph": {}}, description=None)
    assert calls[-1][0] == "/agents"
    assert calls[-1][1] == {"title": "agent-title", "dsl": {"graph": {}}}

    client.create_agent("agent-title", {"graph": {}}, description="desc")
    assert calls[-1][1] == {"title": "agent-title", "dsl": {"graph": {}}, "description": "desc"}

    monkeypatch.setattr(client, "post", lambda *_args, **_kwargs: _DummyResponse({"code": 1, "message": "create boom"}))
    with pytest.raises(Exception) as exception_info:
        client.create_agent("agent-title", {"graph": {}})
    assert "create boom" in str(exception_info.value), str(exception_info.value)


@pytest.mark.p2
def test_update_agent_payload_matrix_and_error(monkeypatch):
    client = RAGFlow("token", "http://localhost:9380")
    calls = []

    def _ok_put(path, json):
        calls.append((path, json))
        return _DummyResponse({"code": 0, "message": "ok"})

    monkeypatch.setattr(client, "put", _ok_put)
    cases = [
        ({"title": "new-title"}, {"title": "new-title"}),
        ({"description": "new-description"}, {"description": "new-description"}),
        ({"dsl": {"nodes": []}}, {"dsl": {"nodes": []}}),
        (
            {"title": "new-title", "description": "new-description", "dsl": {"nodes": []}},
            {"title": "new-title", "description": "new-description", "dsl": {"nodes": []}},
        ),
    ]
    for kwargs, expected_payload in cases:
        client.update_agent("agent-1", **kwargs)
        assert calls[-1][0] == "/agents/agent-1"
        assert calls[-1][1] == expected_payload

    monkeypatch.setattr(client, "put", lambda *_args, **_kwargs: _DummyResponse({"code": 1, "message": "update boom"}))
    with pytest.raises(Exception) as exception_info:
        client.update_agent("agent-1", title="bad")
    assert "update boom" in str(exception_info.value), str(exception_info.value)


@pytest.mark.p2
def test_delete_agent_success_and_error(monkeypatch):
    client = RAGFlow("token", "http://localhost:9380")
    calls = []

    def _ok_delete(path, json):
        calls.append((path, json))
        return _DummyResponse({"code": 0, "message": "ok"})

    monkeypatch.setattr(client, "delete", _ok_delete)
    client.delete_agent("agent-1")
    assert calls[-1] == ("/agents/agent-1", {})

    monkeypatch.setattr(client, "delete", lambda *_args, **_kwargs: _DummyResponse({"code": 1, "message": "delete boom"}))
    with pytest.raises(Exception) as exception_info:
        client.delete_agent("agent-1")
    assert "delete boom" in str(exception_info.value), str(exception_info.value)
