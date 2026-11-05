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

import time
from uuid import uuid4

import pytest
from ragflow_sdk import RAGFlow

pytestmark = [pytest.mark.p2, pytest.mark.integration]


def create_agent(client, dsl, **options):
    title = f"sdk-agent-{uuid4().hex}"
    assert client.create_agent(title, dsl, **options) is None
    matches = [agent for agent in client.list_agents(page_size=100) if agent.title == title]
    assert len(matches) == 1
    return client.get_agent(matches[0].id)


def test_agent_crud_persistence(live_client, dsl):
    agent = create_agent(live_client, dsl, description="description", canvas_type="Agent")
    assert agent.description == "description"
    assert agent.canvas_type == "Agent"
    assert agent.permission == "me"
    assert agent.canvas_category == "agent_canvas"
    assert agent.dsl.to_json() == dsl
    assert live_client.delete_agent(agent.id) is None
    assert agent.id not in {row.id for row in live_client.list_agents()}
    with pytest.raises(Exception):
        live_client.get_agent(agent.id)
    with pytest.raises(Exception):
        live_client.delete_agent(agent.id)


@pytest.mark.parametrize("patch", [{}, {"title": "  Renamed  "}, {"description": "changed"}, {"description": ""}, {"canvas_type": "Marketing"}, {"dsl": {}}])
def test_agent_partial_updates_persist(live_client, dsl, patch):
    agent = create_agent(live_client, dsl, description="original", canvas_type="Agent")
    assert live_client.update_agent(agent.id, **patch) is None
    fresh = live_client.get_agent(agent.id)
    assert fresh.title == patch.get("title", agent.title).strip()
    assert fresh.description == patch.get("description", "original")
    assert fresh.canvas_type == patch.get("canvas_type", "Agent")
    if "dsl" not in patch:
        assert fresh.dsl.to_json() == dsl
    else:
        # Go omits an empty DSL map from the detail response (json omitempty).
        assert fresh.dsl is None
        assert "dsl" not in live_client.get(f"/agents/{agent.id}").json()["data"]


def test_agent_dsl_update_persists(live_client, dsl):
    agent = create_agent(live_client, dsl)
    dsl["graph"]["nodes"][0]["position"]["x"] = 90
    live_client.update_agent(agent.id, dsl=dsl)
    assert live_client.get_agent(agent.id).dsl.to_json() == dsl


@pytest.mark.parametrize(
    "title,dsl_value,options",
    [
        ("", {}, {}),
        ("  ", {}, {}),
        (None, {}, {}),
        (42, {}, {}),
        ("Title", None, {}),
        ("Title", [], {}),
        ("Title", "invalid", {}),
        ("Title", {}, {"description": []}),
        ("Title", {}, {"canvas_type": []}),
    ],
)
def test_agent_create_invalid_values(live_client, title, dsl_value, options):
    # Raw HTTP reaches the Go decoder even when optional SDK type checking is installed.
    response = live_client.post("/agents", {"title": title, "dsl": dsl_value} | options).json()
    assert response["code"] == 101, response
    assert live_client.list_agents() == []


@pytest.mark.parametrize("value", [[], "invalid", 42])
def test_agent_update_rejects_nonobject_dsl(live_client, dsl, value):
    agent = create_agent(live_client, dsl)
    response = live_client.put(f"/agents/{agent.id}", {"dsl": value}).json()
    assert response["code"] != 0, response
    assert "dsl must be an object" in response["message"]
    assert live_client.get_agent(agent.id).dsl.to_json() == dsl


def test_agent_duplicate_title_contract(live_client, dsl):
    agent = create_agent(live_client, dsl)
    live_client.create_agent(agent.title, dsl)
    rows = live_client.list_agents()
    assert {row.title for row in rows} == {agent.title, agent.title + "(1)"}
    second = next(row for row in rows if row.id != agent.id)
    with pytest.raises(Exception, match="already exists"):
        live_client.update_agent(second.id, title=agent.title)
    assert live_client.get_agent(second.id).title == second.title


def test_agent_listing_pages(live_client, dsl):
    agents = [create_agent(live_client, dsl) for _ in range(3)]
    pages = [live_client.list_agents(page=page, page_size=1) for page in (1, 2, 3)]
    assert all(len(page) == 1 for page in pages)
    assert {page[0].id for page in pages} == {agent.id for agent in agents}
    assert live_client.list_agents(page=4, page_size=1) == []
    rows = live_client.list_agents(orderby="update_time", desc=False)
    assert [row.update_time for row in rows] == sorted(row.update_time for row in rows)


def test_agent_creation_order_after_update(live_client, dsl):
    first = create_agent(live_client, dsl)
    time.sleep(0.02)
    second = create_agent(live_client, dsl)
    assert first.create_time < second.create_time
    time.sleep(0.02)
    live_client.update_agent(first.id, description="updated oldest")
    assert live_client.get_agent(first.id).update_time > second.update_time
    assert [agent.id for agent in live_client.list_agents(orderby="create_time", desc=False)] == [first.id, second.id]


@pytest.mark.parametrize("method,options", [("get_agent", {}), ("update_agent", {"description": "no"}), ("delete_agent", {})])
def test_agent_missing_id(live_client, method, options):
    with pytest.raises(Exception):
        getattr(live_client, method)(uuid4().hex, **options)


def test_agent_invalid_credentials_cannot_delete(live_client, dsl):
    agent = create_agent(live_client, dsl)
    denied = RAGFlow("invalid-audit-token", live_client.api_url.removesuffix("/api/v1"))
    with pytest.raises(Exception):
        denied.delete_agent(agent.id)
    assert live_client.get_agent(agent.id).id == agent.id


def test_agent_session_creation_and_listing(live_client, dsl):
    agent = create_agent(live_client, dsl)
    first = agent.create_session(name=" first ", source="sdk")
    second = agent.create_session()
    assert (first.name, first.source) == ("first", "sdk")
    assert second.name == "session"
    sessions = agent.list_sessions()
    assert {session.id for session in sessions} == {first.id, second.id}
    assert all(session.agent_id == agent.id and session.message == [] for session in sessions)
    filtered = agent.list_sessions(page_size=1, id=first.id, orderby="create_time", desc=False)
    assert [session.id for session in filtered] == [first.id]
    assert agent.list_sessions(page=3, page_size=1) == []


@pytest.mark.parametrize("delete_all", [False, True])
def test_agent_session_deletion_persists(live_client, dsl, delete_all):
    agent = create_agent(live_client, dsl)
    first = agent.create_session(name="delete")
    second = agent.create_session(name="survivor")
    if delete_all:
        agent.delete_sessions(delete_all=True)
    else:
        agent.delete_sessions(ids=[first.id])
    expected = set() if delete_all else {second.id}
    assert {session.id for session in agent.list_sessions()} == expected


def test_created_agent_session_binding(live_client, dsl):
    agent = create_agent(live_client, dsl)
    session = agent.create_session()
    assert session.agent_id == agent.id
