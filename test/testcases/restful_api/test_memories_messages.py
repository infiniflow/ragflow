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
def clear_memories(rest_client):
    def _cleanup():
        list_res = rest_client.get("/memories")
        if list_res.status_code != 200:
            return
        list_payload = list_res.json()
        if list_payload.get("code") != 0:
            return
        memory_list = list_payload.get("data", {}).get("memory_list", [])
        for memory in memory_list:
            memory_id = memory.get("id")
            if not memory_id:
                continue
            delete_res = rest_client.delete(f"/memories/{memory_id}")
            if delete_res.status_code != 200:
                continue
            delete_payload = delete_res.json()
            assert delete_payload["code"] in (0, 404), delete_payload

    yield
    _cleanup()


@pytest.fixture
def create_memory_resource(rest_client, clear_memories):
    created_ids: list[str] = []

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
        if res_payload["code"] != 0:
            msg = str(res_payload.get("message", ""))
            if "Tenant Model" in msg or "not found" in msg:
                pytest.skip(f"Memory prerequisites unavailable: {msg}")
        assert res_payload["code"] == 0, res_payload
        memory_id = res_payload["data"]["id"]
        created_ids.append(memory_id)
        return memory_id

    yield _create

    for memory_id in created_ids:
        delete_res = rest_client.delete(f"/memories/{memory_id}")
        if delete_res.status_code != 200:
            continue
        delete_payload = delete_res.json()
        assert delete_payload["code"] in (0, 404), delete_payload


def _add_message_or_skip(rest_client, memory_id: str, user_input: str, agent_response: str) -> None:
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
    if add_payload["code"] != 0:
        msg = str(add_payload.get("message", ""))
        if "encode\"" in msg or "encode'" in msg or "NoneType" in msg:
            pytest.skip(f"Message embedding runtime unavailable in this env: {msg}")
    assert add_payload["code"] == 0, add_payload


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
    _add_message_or_skip(
        rest_client,
        memory_id,
        user_input="what is coriander?",
        agent_response="coriander can refer to leaves or seeds",
    )

    time.sleep(1)

    memory_messages_res = rest_client.get(f"/memories/{memory_id}")
    assert memory_messages_res.status_code == 200
    memory_messages_payload = memory_messages_res.json()
    assert memory_messages_payload["code"] == 0, memory_messages_payload
    message_list = memory_messages_payload["data"]["messages"]["message_list"]
    assert message_list, memory_messages_payload

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
    _add_message_or_skip(rest_client, memory_id, user_input="hello", agent_response="hello")

    time.sleep(1)

    list_res = rest_client.get(f"/memories/{memory_id}")
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    message_id = list_payload["data"]["messages"]["message_list"][0]["message_id"]

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
    _add_message_or_skip(
        rest_client,
        memory_id,
        user_input="what is pineapple?",
        agent_response="pineapple is a tropical fruit",
    )

    time.sleep(1)

    res = rest_client.get("/messages/search", params={"memory_id": memory_id, "query": "pineapple", "top_n": 3})
    assert res.status_code == 200
    payload = res.json()
    if payload["code"] != 0:
        msg = str(payload.get("message", ""))
        if "encode_queries" in msg:
            pytest.skip(f"Message search embedding runtime unavailable in this env: {msg}")
    assert payload["code"] == 0, payload
    assert isinstance(payload["data"], list), payload
