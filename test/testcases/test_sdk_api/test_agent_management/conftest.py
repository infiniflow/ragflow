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

import copy
import os

import pytest
import requests
from ragflow_sdk import RAGFlow


@pytest.fixture(autouse=True)
def network_boundary(request, monkeypatch):
    original = requests.sessions.Session.request

    def guarded(session, method, url, **kwargs):
        if request.node.get_closest_marker("integration") is None:
            pytest.fail(f"Mocked SDK test attempted HTTP: {method} {url}")
        kwargs.setdefault("timeout", 30)
        return original(session, method, url, **kwargs)

    monkeypatch.setattr(requests.sessions.Session, "request", guarded)


@pytest.fixture
def dsl():
    # A model-free draft with no dangling component references.
    return {
        "components": {"begin": {"obj": {"component_name": "Begin", "params": {}}, "downstream": [], "upstream": []}},
        "graph": {"nodes": [{"id": "begin", "type": "beginNode", "data": {"label": "Begin", "name": "begin"}, "position": {"x": 0, "y": 0}}], "edges": []},
    }


@pytest.fixture
def client():
    return RAGFlow("unit-token", "http://sdk.invalid")


@pytest.fixture
def wire(monkeypatch):
    calls = []
    reply = {"code": 0, "data": True, "message": "success"}

    def request(_session, method, url, **kwargs):
        calls.append((method, url, kwargs))
        response = requests.Response()
        response.status_code = 200
        response.json = lambda: copy.deepcopy(reply)
        return response

    monkeypatch.setattr(requests.sessions.Session, "request", request)
    return calls, reply


@pytest.fixture
def live_client(monkeypatch):
    required = ("RAGFLOW_SDK_BASE_URL", "RAGFLOW_SDK_API_KEY")
    if os.environ.get("RAGFLOW_SDK_ISOLATED") != "1" or not all(os.environ.get(key) for key in required):
        pytest.fail("Environment blocked: requires RAGFLOW_SDK_ISOLATED=1, RAGFLOW_SDK_BASE_URL and RAGFLOW_SDK_API_KEY for a disposable Go backend and dedicated empty tenant")
    client = RAGFlow(os.environ["RAGFLOW_SDK_API_KEY"], os.environ["RAGFLOW_SDK_BASE_URL"])
    assert client.list_agents(page_size=1) == [], "Dedicated tenant must start without agents"
    created = set()
    sessions = set()
    post = client.post

    def track(path, *args, **kwargs):
        response = post(path, *args, **kwargs)
        body = response.json()
        if path == "/agents" and body.get("code") == 0:
            created.add(body["data"]["id"])
        if path.startswith("/agents/") and path.endswith("/sessions") and body.get("code") == 0:
            sessions.add((path, body["data"]["id"]))
        return response

    monkeypatch.setattr(client, "post", track)
    try:
        yield client
    finally:
        errors = []
        # Single-item HTTP deletion works; batch SDK deletion currently ignores IDs.
        for path, session_id in sessions:
            try:
                assert client.delete(f"{path}/{session_id}", {}).json()["code"] == 0
                assert client.get(f"{path}/{session_id}").json()["code"] == 404
            except Exception as error:
                errors.append(f"Session {session_id}: {error}")
        for agent_id in created:
            try:
                remaining = client.get(f"/agents/{agent_id}").json()
                if remaining["code"] == 0:
                    assert client.get(f"/agents/{agent_id}/sessions").json()["data"] == []
                    client.delete_agent(agent_id)
                assert client.get(f"/agents/{agent_id}").json()["code"] == 103
            except Exception as error:
                errors.append(f"Agent {agent_id}: {error}")
        assert not errors, "Cleanup failed: " + "; ".join(errors)
