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
import requests
from ragflow_sdk.modules.agent import Agent
from ragflow_sdk.modules.session import Session

pytestmark = pytest.mark.p2


def assert_call(wire, method, path, payload=None, params=None):
    calls, _ = wire
    actual_method, url, kwargs = calls[-1]
    assert actual_method == method
    assert url == f"http://sdk.invalid/api/v1{path}"
    assert kwargs["headers"] == {"Authorization": "Bearer unit-token"}
    assert kwargs.get("json") == payload
    assert kwargs.get("params") == params


@pytest.mark.parametrize("options", [{}, {"page": 2, "page_size": 5, "orderby": "create_time", "desc": False}])
def test_list_agents_pagination_and_deserialization(client, wire, dsl, options):
    wire[1]["data"] = {"canvas": [{"id": "a1", "title": "One", "dsl": dsl}, {"id": "a2", "title": "Two"}], "total": 8}
    agents = client.list_agents(**options)
    assert_call(wire, "get", "/agents", params={"page": 1, "page_size": 30, "orderby": "update_time", "desc": True} | options)
    assert all(isinstance(agent, Agent) and agent.rag is client for agent in agents)
    assert [(agent.id, agent.title) for agent in agents] == [("a1", "One"), ("a2", "Two")]
    assert agents[0].dsl.to_json() == dsl


def test_list_agents_empty(client, wire):
    wire[1]["data"] = {"canvas": [], "total": 0}
    assert client.list_agents() == []


def test_get_agent_detail(client, wire, dsl):
    wire[1]["data"] = {"id": "a1", "title": "One", "description": "details", "canvas_type": "Agent", "dsl": dsl, "tenant_id": "t1", "last_publish_time": 123}
    agent = client.get_agent("a1")
    assert_call(wire, "get", "/agents/a1")
    assert isinstance(agent, Agent) and agent.rag is client
    assert (agent.id, agent.title, agent.description, agent.canvas_type) == ("a1", "One", "details", "Agent")
    assert agent.dsl.to_json() == dsl
    assert (agent.tenant_id, agent.last_publish_time) == ("t1", 123)


@pytest.mark.parametrize("options", [{}, {"description": None, "canvas_type": None}, {"description": "desc"}, {"canvas_type": "Marketing"}, {"description": "", "canvas_type": ""}])
def test_create_agent_payload_and_return(client, wire, dsl, options):
    wire[1]["data"] = {"id": "a1", "title": "Title", "dsl": dsl}
    assert client.create_agent("Title", dsl, **options) is None
    assert_call(wire, "post", "/agents", {"title": "Title", "dsl": dsl} | {key: value for key, value in options.items() if value is not None})


@pytest.mark.parametrize("kwargs", [{}, {"title": "only-title"}, {"dsl": {}}])
def test_create_agent_required_arguments(client, wire, kwargs):
    with pytest.raises(TypeError):
        client.create_agent(**kwargs)
    assert wire[0] == []


@pytest.mark.parametrize(
    "patch",
    [
        {},
        {"title": None, "description": None, "dsl": None, "canvas_type": None},
        {"title": "New"},
        {"description": "New description"},
        {"description": ""},
        {"dsl": {}},
        {"canvas_type": "Agent"},
        {"title": "New", "description": "desc", "canvas_type": "Marketing"},
    ],
)
def test_update_agent_partial_payload(client, wire, patch):
    wire[1]["data"] = {"update_time": 123}
    assert client.update_agent("a1", **patch) is None
    assert_call(wire, "put", "/agents/a1", {key: value for key, value in patch.items() if value is not None})


def test_update_agent_dsl_payload(client, wire, dsl):
    assert client.update_agent("a1", dsl=dsl) is None
    assert_call(wire, "put", "/agents/a1", {"dsl": dsl})


def test_delete_agent_success(client, wire):
    assert client.delete_agent("a1") is None
    assert_call(wire, "delete", "/agents/a1", {})


@pytest.mark.parametrize(
    "method,kwargs,verb,path,code,message",
    [
        ("list_agents", {}, "get", "/agents", 100, "Unauthenticated"),
        ("get_agent", {"agent_id": "missing"}, "get", "/agents/missing", 103, "Make sure you have permission to access the agent."),
        ("create_agent", {"title": "", "dsl": {}}, "post", "/agents", 101, "no title in request"),
        ("update_agent", {"agent_id": "a1", "title": "duplicate"}, "put", "/agents/a1", 102, "duplicate already exists."),
        ("delete_agent", {"agent_id": "missing"}, "delete", "/agents/missing", 103, "Only the owner of the agent is authorized for this operation."),
        ("delete_agent", {"agent_id": "a1"}, "delete", "/agents/a1", 100, "Unauthenticated"),
    ],
)
def test_agent_api_errors_propagate(client, wire, method, kwargs, verb, path, code, message):
    # This proves SDK propagation, not server-side validation of mocked inputs.
    wire[1].update(code=code, message=message, data=None)
    with pytest.raises(Exception) as error:
        getattr(client, method)(**kwargs)
    assert str(error.value) == message
    assert wire[0][-1][:2] == (verb, f"http://sdk.invalid/api/v1{path}")


def test_agent_optional_fields(client):
    agent = Agent(client, {"id": "a1", "title": "One"})
    assert (agent.id, agent.title) == ("a1", "One")
    assert all(getattr(agent, field) is None for field in ("avatar", "canvas_type", "description", "dsl"))


def test_explicit_dsl_round_trip(client, dsl):
    result = Agent.Dsl(client, dsl).to_json()
    assert result["components"] == dsl["components"]
    assert result["graph"] == dsl["graph"]


def test_default_dsl_references_existing_components(client):
    dsl = Agent.Dsl(client, {}).to_json()
    for component in dsl["components"].values():
        assert set(component["downstream"]) <= dsl["components"].keys()
        assert set(component["upstream"]) <= dsl["components"].keys()
    assert {node["id"] for node in dsl["graph"]["nodes"]} == dsl["components"].keys()


@pytest.mark.parametrize("options", [{}, {"name": "Session", "source": "sdk"}])
def test_agent_create_session_request(client, wire, options):
    wire[1]["data"] = {"id": "s1", "agent_id": "a1", "name": "session", "message": []}
    session = Agent(client, {"id": "a1"}).create_session(**options)
    assert_call(wire, "post", "/agents/a1/sessions", options)
    assert isinstance(session, Session) and session.rag is client
    assert (session.id, session.name, session.agent_id) == ("s1", "session", "a1")


@pytest.mark.parametrize("options", [{}, {"page": 2, "page_size": 5, "orderby": "update_time", "desc": False, "id": "s1"}])
def test_agent_list_sessions(client, wire, options):
    wire[1].update(data=[{"id": "s1", "agent_id": "a1", "name": "one", "message": []}], total=1)
    sessions = Agent(client, {"id": "a1"}).list_sessions(**options)
    assert_call(wire, "get", "/agents/a1/sessions", params={"page": 1, "page_size": 30, "orderby": "create_time", "desc": True, "id": None} | options)
    assert len(sessions) == 1 and isinstance(sessions[0], Session)
    assert (sessions[0].id, sessions[0].agent_id, sessions[0].name) == ("s1", "a1", "one")


def test_agent_list_sessions_empty(client, wire):
    wire[1].update(data=[], total=0)
    assert Agent(client, {"id": "a1"}).list_sessions() == []


def test_listed_agent_session_history(client, wire):
    history = [{"role": "user", "content": "stored question"}]
    wire[1].update(data=[{"id": "s1", "agent_id": "a1", "message": history}], total=1)
    assert Agent(client, {"id": "a1"}).list_sessions()[0].message == history


@pytest.mark.parametrize("options,payload", [({"ids": ["s1", "s2"]}, {"ids": ["s1", "s2"]}), ({"delete_all": True}, {"ids": None, "delete_all": True})])
def test_agent_delete_sessions_payload(client, wire, options, payload):
    assert Agent(client, {"id": "a1"}).delete_sessions(**options) is None
    assert_call(wire, "delete", "/agents/a1/sessions", payload)


@pytest.mark.parametrize("ids", [None, []])
def test_agent_delete_sessions_noop(client, wire, ids):
    assert Agent(client, {"id": "a1"}).delete_sessions(ids=ids) is None
    assert_call(wire, "delete", "/agents/a1/sessions", {"ids": ids})


@pytest.mark.parametrize("method,options,verb", [("create_session", {"name": "bad"}, "post"), ("list_sessions", {}, "get"), ("delete_sessions", {"ids": ["missing"]}, "delete")])
def test_agent_session_errors(client, wire, method, options, verb):
    wire[1].update(code=103, message="agent not found or no permission", data=None)
    with pytest.raises(Exception, match="^agent not found or no permission$"):
        getattr(Agent(client, {"id": "missing"}), method)(**options)
    assert wire[0][-1][:2] == (verb, "http://sdk.invalid/api/v1/agents/missing/sessions")


@pytest.mark.parametrize(
    "method,options",
    [("list_agents", {}), ("get_agent", {"agent_id": "a1"}), ("create_agent", {"title": "Title", "dsl": {}}), ("update_agent", {"agent_id": "a1"}), ("delete_agent", {"agent_id": "a1"})],
)
def test_agent_transport_errors_propagate(client, monkeypatch, method, options):
    def unavailable(*args, **kwargs):
        raise requests.ConnectionError("offline")

    monkeypatch.setattr(requests.sessions.Session, "request", unavailable)
    with pytest.raises(requests.ConnectionError, match="^offline$"):
        getattr(client, method)(**options)


def test_agent_create_session_custom_dsl(client, wire, dsl):
    wire[1]["data"] = {"id": "s1", "dialog_id": "a1", "dsl": dsl}
    session = Agent(client, {"id": "a1"}).create_session(dsl=dsl)
    assert_call(wire, "post", "/agents/a1/sessions", {"dsl": dsl})
    assert session.dsl.to_json() == dsl
