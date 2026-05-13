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
import uuid

import pytest


@pytest.fixture
def memory_cleanup(rest_client):
    created_ids: list[str] = []

    def _cleanup():
        cleanup_errors = []
        for memory_id in created_ids:
            delete_res = rest_client.delete(f"/memories/{memory_id}")
            if delete_res.status_code != 200:
                cleanup_errors.append((memory_id, delete_res.status_code, delete_res.text))
                continue
            delete_payload = delete_res.json()
            if delete_payload["code"] not in (0, 404):
                cleanup_errors.append((memory_id, delete_res.status_code, delete_payload))
        assert not cleanup_errors, f"Memory cleanup failed: {cleanup_errors}"

    yield created_ids
    _cleanup()


@pytest.fixture
def create_memory_resource(rest_client, memory_cleanup):
    def _create(name_prefix: str = "restful_memory") -> str:
        payload = {
            "name": f"{name_prefix}_{uuid.uuid4().hex[:8]}",
            "memory_type": ["raw"],
            "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
            "llm_id": "glm-4-flash@ZHIPU-AI",
        }
        res = rest_client.post("/memories", json=payload)
        assert res.status_code == 200
        res_payload = res.json()
        assert res_payload["code"] == 0, res_payload
        memory_id = res_payload["data"]["id"]
        memory_cleanup.append(memory_id)
        return memory_id

    yield _create


def _add_message(rest_client, memory_id: str, user_input: str, agent_response: str) -> None:
    add_res = rest_client.post(
        "/messages",
        json={
            "memory_id": [memory_id],
            "agent_id": uuid.uuid4().hex,
            "session_id": uuid.uuid4().hex,
            "user_id": uuid.uuid4().hex,
            "user_input": user_input,
            "agent_response": agent_response,
        },
    )
    assert add_res.status_code == 200
    add_payload = add_res.json()
    assert add_payload["code"] == 0, add_payload


def _wait_for_memory_messages(rest_client, memory_id: str, timeout: float = 10, interval: float = 0.2) -> list[dict]:
    deadline = time.time() + timeout
    last_payload = None
    while time.time() < deadline:
        res = rest_client.get(f"/memories/{memory_id}")
        if res.status_code == 200:
            payload = res.json()
            last_payload = payload
            if payload.get("code") == 0:
                message_list = payload.get("data", {}).get("messages", {}).get("message_list", [])
                if message_list:
                    return message_list
        time.sleep(interval)
    pytest.fail(f"Timed out waiting for memory messages: {last_payload}")


@pytest.mark.p1
def test_memory_crud_cycle(rest_client, create_memory_resource):
    memory_id = create_memory_resource("restful_memory_crud")

    list_res = rest_client.get("/memories")
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert any(item["id"] == memory_id for item in list_payload["data"]["memory_list"]), list_payload

    config_res = rest_client.get(f"/memories/{memory_id}/config")
    assert config_res.status_code == 200
    config_payload = config_res.json()
    assert config_payload["code"] == 0, config_payload
    assert config_payload["data"]["id"] == memory_id, config_payload

    update_res = rest_client.put(
        f"/memories/{memory_id}",
        json={"name": f"updated_{uuid.uuid4().hex[:6]}", "permissions": "me"},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload

    delete_res = rest_client.delete(f"/memories/{memory_id}")
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 0, delete_payload


@pytest.mark.p2
def test_memory_create_missing_required_fields(rest_client):
    res = rest_client.post("/memories", json={"name": "missing_models", "memory_type": ["raw"]})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload


@pytest.mark.p1
def test_messages_add_list_recent_content_update_forget(rest_client, create_memory_resource):
    memory_id = create_memory_resource("restful_message_memory")
    _add_message(
        rest_client,
        memory_id,
        user_input="what is coriander?",
        agent_response="coriander can refer to leaves or seeds",
    )

    message_list = _wait_for_memory_messages(rest_client, memory_id)

    message_id = message_list[0]["message_id"]

    recent_res = rest_client.get("/messages", params={"memory_id": memory_id, "limit": 10})
    assert recent_res.status_code == 200
    recent_payload = recent_res.json()
    assert recent_payload["code"] == 0, recent_payload
    assert any(item["message_id"] == message_id for item in recent_payload["data"]), recent_payload

    content_res = rest_client.get(f"/messages/{memory_id}:{message_id}/content")
    assert content_res.status_code == 200
    content_payload = content_res.json()
    assert content_payload["code"] == 0, content_payload
    assert content_payload["data"]["content"], content_payload

    update_res = rest_client.put(f"/messages/{memory_id}:{message_id}", json={"status": False})
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload

    forget_res = rest_client.delete(f"/messages/{memory_id}:{message_id}")
    assert forget_res.status_code == 200
    forget_payload = forget_res.json()
    assert forget_payload["code"] == 0, forget_payload


@pytest.mark.p2
def test_message_status_validation_requires_boolean(rest_client, create_memory_resource):
    memory_id = create_memory_resource("restful_message_status_validation")
    _add_message(rest_client, memory_id, user_input="hello", agent_response="hello")

    message_id = _wait_for_memory_messages(rest_client, memory_id)[0]["message_id"]

    invalid_update = rest_client.put(f"/messages/{memory_id}:{message_id}", json={"status": "false"})
    assert invalid_update.status_code == 200
    invalid_payload = invalid_update.json()
    assert invalid_payload["code"] == 101, invalid_payload
    assert "Status must be a boolean." in invalid_payload["message"], invalid_payload


@pytest.mark.p2
def test_messages_recent_requires_memory_ids(rest_client):
    res = rest_client.get("/messages")
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload
    assert "memory_ids is required" in payload["message"], payload


@pytest.mark.p2
def test_message_search_route_contract(rest_client, create_memory_resource):
    memory_id = create_memory_resource("restful_message_search")
    _add_message(
        rest_client,
        memory_id,
        user_input="what is pineapple?",
        agent_response="pineapple is a tropical fruit",
    )

    _wait_for_memory_messages(rest_client, memory_id)

    res = rest_client.get("/messages/search", params={"memory_id": memory_id, "query": "pineapple", "top_n": 3})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert isinstance(payload["data"], list), payload
