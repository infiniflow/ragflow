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

import json

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


def _sse_events(response_text: str) -> list[str]:
    return [line[5:] for line in response_text.splitlines() if line.startswith("data:")]


@pytest.fixture
def create_agent_resource(rest_client):
    created_agent_ids: list[str] = []

    def _create(title: str = "restful_agent") -> str:
        res = rest_client.post("/agents", json={"title": title, "dsl": MINIMAL_DSL})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 0, payload
        agent_id = payload["data"]["id"]
        created_agent_ids.append(agent_id)
        return agent_id

    yield _create

    cleanup_errors = []
    for agent_id in created_agent_ids:
        res = rest_client.delete(f"/agents/{agent_id}")
        if res.status_code != 200:
            cleanup_errors.append((agent_id, res.status_code, res.text))
            continue
        payload = res.json()
        if payload["code"] not in (0, 103):
            cleanup_errors.append((agent_id, res.status_code, payload))
    assert not cleanup_errors, f"Agent cleanup failed: {cleanup_errors}"


@pytest.mark.p2
def test_agents_crud_validation_contract(rest_client, create_agent_resource):
    list_empty = rest_client.get("/agents", params={"title": "missing_restful_agent"})
    assert list_empty.status_code == 200
    list_empty_payload = list_empty.json()
    assert list_empty_payload["code"] == 0, list_empty_payload
    assert "canvas" in list_empty_payload["data"], list_empty_payload
    assert "total" in list_empty_payload["data"], list_empty_payload

    missing_dsl = rest_client.post("/agents", json={"title": "missing_dsl_agent"})
    assert missing_dsl.status_code == 200
    missing_dsl_payload = missing_dsl.json()
    assert missing_dsl_payload["code"] == 101, missing_dsl_payload
    assert "No DSL data in request" in missing_dsl_payload["message"], missing_dsl_payload

    missing_title = rest_client.post("/agents", json={"dsl": MINIMAL_DSL})
    assert missing_title.status_code == 200
    missing_title_payload = missing_title.json()
    assert missing_title_payload["code"] == 101, missing_title_payload
    assert "No title in request" in missing_title_payload["message"], missing_title_payload

    agent_id = create_agent_resource("restful_agent_crud")

    duplicate = rest_client.post("/agents", json={"title": "restful_agent_crud", "dsl": MINIMAL_DSL})
    assert duplicate.status_code == 200
    duplicate_payload = duplicate.json()
    assert duplicate_payload["code"] == 102, duplicate_payload
    assert "already exists" in duplicate_payload["message"], duplicate_payload

    get_res = rest_client.get(f"/agents/{agent_id}")
    assert get_res.status_code == 200
    get_payload = get_res.json()
    assert get_payload["code"] == 0, get_payload
    assert get_payload["data"]["id"] == agent_id, get_payload

    update_res = rest_client.put(f"/agents/{agent_id}", json={"title": "restful_agent_crud_updated", "dsl": MINIMAL_DSL})
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload

    list_after_update = rest_client.get("/agents", params={"title": "restful_agent_crud_updated"})
    assert list_after_update.status_code == 200
    list_after_update_payload = list_after_update.json()
    assert list_after_update_payload["code"] == 0, list_after_update_payload
    assert list_after_update_payload["data"]["total"] >= 1, list_after_update_payload

    delete_res = rest_client.delete(f"/agents/{agent_id}")
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 0, delete_payload
    assert delete_payload["data"] is True, delete_payload


@pytest.mark.p2
def test_agent_sessions_crud(rest_client, create_agent_resource):
    agent_id = create_agent_resource("restful_agent_sessions")

    create_session = rest_client.post(f"/agents/{agent_id}/sessions", json={"name": "agent_session_1"})
    assert create_session.status_code == 200
    create_session_payload = create_session.json()
    assert create_session_payload["code"] == 0, create_session_payload
    session_id = create_session_payload["data"]["id"]

    list_sessions = rest_client.get(f"/agents/{agent_id}/sessions")
    assert list_sessions.status_code == 200
    list_sessions_payload = list_sessions.json()
    assert list_sessions_payload["code"] == 0, list_sessions_payload
    assert isinstance(list_sessions_payload["data"], list), list_sessions_payload
    assert any(item["id"] == session_id for item in list_sessions_payload["data"]), list_sessions_payload

    get_session = rest_client.get(f"/agents/{agent_id}/sessions/{session_id}")
    assert get_session.status_code == 200
    get_session_payload = get_session.json()
    assert get_session_payload["code"] == 0, get_session_payload
    assert get_session_payload["data"]["id"] == session_id, get_session_payload

    delete_session = rest_client.delete(f"/agents/{agent_id}/sessions/{session_id}")
    assert delete_session.status_code == 200
    delete_session_payload = delete_session.json()
    assert delete_session_payload["code"] == 0, delete_session_payload


@pytest.mark.p2
def test_agent_chat_completion_validation(rest_client):
    missing_agent_id = rest_client.post(
        "/agents/chat/completions",
        json={"query": "hello", "stream": False},
    )
    assert missing_agent_id.status_code == 200
    missing_agent_id_payload = missing_agent_id.json()
    assert missing_agent_id_payload["code"] == 101, missing_agent_id_payload
    assert "`agent_id` is required." in missing_agent_id_payload["message"], missing_agent_id_payload


@pytest.mark.p2
def test_agent_chat_completion_nonstream(rest_client, create_agent_resource):
    agent_id = create_agent_resource("restful_agent_nonstream")
    create_session = rest_client.post(f"/agents/{agent_id}/sessions", json={"name": "agent_completion_session"})
    assert create_session.status_code == 200
    create_session_payload = create_session.json()
    assert create_session_payload["code"] == 0, create_session_payload
    session_id = create_session_payload["data"]["id"]

    res = rest_client.post(
        "/agents/chat/completions",
        json={"agent_id": agent_id, "query": "hello", "stream": False, "session_id": session_id},
        timeout=60,
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert isinstance(payload["data"], dict), payload
    assert isinstance(payload["data"].get("data"), dict), payload
    assert "content" in payload["data"]["data"], payload


@pytest.mark.p2
def test_agent_chat_completion_stream_structure_and_done(rest_client, create_agent_resource):
    agent_id = create_agent_resource("restful_agent_stream")
    create_session = rest_client.post(f"/agents/{agent_id}/sessions", json={"name": "agent_stream_session"})
    assert create_session.status_code == 200
    create_session_payload = create_session.json()
    assert create_session_payload["code"] == 0, create_session_payload
    session_id = create_session_payload["data"]["id"]

    res = rest_client.post(
        "/agents/chat/completions",
        json={
            "agent_id": agent_id,
            "query": "hello",
            "stream": True,
            "session_id": session_id,
            "return_trace": True,
        },
        timeout=60,
    )
    assert res.status_code == 200
    content_type = res.headers.get("Content-Type", "")
    assert "text/event-stream" in content_type, content_type

    events = _sse_events(res.text)
    assert events, res.text
    assert events[-1].strip() == "[DONE]", events[-1]

    json_events = [json.loads(evt) for evt in events if evt.strip() != "[DONE]"]
    assert json_events, events
    assert any(isinstance(evt, dict) for evt in json_events), json_events


@pytest.mark.p2
def test_agent_openai_compatible_mode(rest_client, create_agent_resource):
    agent_id = create_agent_resource("restful_agent_openai_compat")

    missing_messages = rest_client.post(
        "/agents/chat/completions",
        json={"agent_id": agent_id, "openai-compatible": True, "model": "model", "messages": []},
    )
    assert missing_messages.status_code == 200
    missing_messages_payload = missing_messages.json()
    assert missing_messages_payload["code"] == 102, missing_messages_payload
    assert "at least one message" in missing_messages_payload["message"], missing_messages_payload

    nonstream = rest_client.post(
        "/agents/chat/completions",
        json={
            "agent_id": agent_id,
            "openai-compatible": True,
            "model": "model",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
        },
        timeout=60,
    )
    assert nonstream.status_code == 200
    nonstream_payload = nonstream.json()
    assert isinstance(nonstream_payload, dict), nonstream_payload
    assert "choices" in nonstream_payload, nonstream_payload

    stream = rest_client.post(
        "/agents/chat/completions",
        json={
            "agent_id": agent_id,
            "openai-compatible": True,
            "model": "model",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": True,
        },
        timeout=60,
    )
    assert stream.status_code == 200
    stream_content_type = stream.headers.get("Content-Type", "")
    assert "text/event-stream" in stream_content_type, stream_content_type


@pytest.mark.p2
def test_agent_support_routes_auth_and_contracts(rest_client, rest_client_noauth, create_agent_resource):
    prompts_unauth = rest_client_noauth.get("/agents/prompts")
    assert prompts_unauth.status_code == 401
    assert prompts_unauth.json()["code"] == 401

    prompts = rest_client.get("/agents/prompts")
    assert prompts.status_code == 200
    prompts_payload = prompts.json()
    assert prompts_payload["code"] == 0, prompts_payload
    assert "task_analysis" in prompts_payload["data"], prompts_payload
    assert "citation_guidelines" in prompts_payload["data"], prompts_payload

    templates = rest_client.get("/agents/templates")
    assert templates.status_code == 200
    templates_payload = templates.json()
    assert templates_payload["code"] == 0, templates_payload
    assert isinstance(templates_payload["data"], list), templates_payload

    agent_id = create_agent_resource("restful_agent_support")
    versions = rest_client.get(f"/agents/{agent_id}/versions")
    assert versions.status_code == 200
    versions_payload = versions.json()
    assert versions_payload["code"] == 0, versions_payload
    assert isinstance(versions_payload["data"], list), versions_payload

    logs = rest_client.get(f"/agents/{agent_id}/logs/missing_message")
    assert logs.status_code == 200
    logs_payload = logs.json()
    assert logs_payload["code"] == 0, logs_payload
    assert isinstance(logs_payload["data"], dict), logs_payload


@pytest.mark.p2
def test_agent_webhook_logs_empty_poll_contract(rest_client, create_agent_resource):
    agent_id = create_agent_resource("restful_agent_webhook_logs")
    res = rest_client.get(f"/agents/{agent_id}/webhook/logs", params={"since_ts": 0})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert payload["data"]["events"] == [], payload
    assert payload["data"]["finished"] is False, payload
    assert "next_since_ts" in payload["data"], payload


@pytest.mark.p2
def test_agent_db_connection_validates_required_fields(rest_client):
    res = rest_client.post("/agents/test_db_connection", json={"db_type": "mysql"})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload
    assert "required argument are missing" in payload["message"], payload


@pytest.mark.p2
def test_agent_rerun_requires_required_fields(rest_client):
    res = rest_client.post("/agents/rerun", json={"id": "flow-1"})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload
    assert "required argument are missing" in payload["message"], payload
