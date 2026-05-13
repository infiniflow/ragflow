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

import uuid

import pytest


def _memory_payload(name: str) -> dict:
    return {
        "name": name,
        "memory_type": ["raw"],
        "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
        "llm_id": "glm-4-flash@ZHIPU-AI",
    }


def _create_memory(rest_client, name: str) -> dict:
    res = rest_client.post("/memories", json=_memory_payload(name))
    assert res.status_code == 200
    payload = res.json()
    if payload["code"] == 0:
        return payload["data"]

    pytest.fail(f"Failed to create memory: {payload}")


@pytest.fixture
def memory_resource(rest_client):
    memory = _create_memory(rest_client, f"restful_memory_{uuid.uuid4().hex[:8]}")
    memory_id = memory["id"]
    try:
        yield memory
    finally:
        delete_res = rest_client.delete(f"/memories/{memory_id}")
        assert delete_res.status_code == 200, delete_res.text
        delete_payload = delete_res.json()
        assert delete_payload["code"] in (0, 404), delete_payload


@pytest.mark.p2
def test_memory_and_message_routes_require_auth(rest_client_noauth):
    memory_res = rest_client_noauth.get("/memories")
    assert memory_res.status_code == 401
    memory_payload = memory_res.json()
    assert memory_payload["code"] == 401, memory_payload

    msg_list_res = rest_client_noauth.get("/messages")
    assert msg_list_res.status_code == 401
    msg_list_payload = msg_list_res.json()
    assert msg_list_payload["code"] == 401, msg_list_payload

    msg_search_res = rest_client_noauth.get("/messages/search")
    assert msg_search_res.status_code == 401
    msg_search_payload = msg_search_res.json()
    assert msg_search_payload["code"] == 401, msg_search_payload


@pytest.mark.p2
def test_memory_crud_and_config(rest_client):
    memory = _create_memory(rest_client, f"restful_memory_crud_{uuid.uuid4().hex[:8]}")
    memory_id = memory["id"]
    try:
        config_res = rest_client.get(f"/memories/{memory_id}/config")
        assert config_res.status_code == 200
        config_payload = config_res.json()
        assert config_payload["code"] == 0, config_payload
        assert config_payload["data"]["id"] == memory_id, config_payload

        list_res = rest_client.get("/memories", params={"keywords": memory["name"]})
        assert list_res.status_code == 200
        list_payload = list_res.json()
        assert list_payload["code"] == 0, list_payload
        assert any(item["id"] == memory_id for item in list_payload["data"]["memory_list"]), list_payload

        update_res = rest_client.put(f"/memories/{memory_id}", json={"name": "restful_memory_updated"})
        assert update_res.status_code == 200
        update_payload = update_res.json()
        assert update_payload["code"] == 0, update_payload
    finally:
        delete_res = rest_client.delete(f"/memories/{memory_id}")
        assert delete_res.status_code == 200, delete_res.text
        delete_payload = delete_res.json()
        assert delete_payload["code"] in (0, 404), delete_payload


@pytest.mark.p2
def test_memory_update_invalid_name(rest_client, memory_resource):
    memory_id = memory_resource["id"]
    res = rest_client.put(f"/memories/{memory_id}", json={"name": " "})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload
    assert "cannot be empty" in payload["message"], payload


@pytest.mark.p2
def test_messages_list_and_search_validation_contracts(rest_client, memory_resource):
    memory_id = memory_resource["id"]

    list_res = rest_client.get("/messages", params={"memory_id": memory_id, "limit": 10})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert isinstance(list_payload["data"], list), list_payload

    missing_memory_res = rest_client.get("/messages")
    assert missing_memory_res.status_code == 200
    missing_memory_payload = missing_memory_res.json()
    assert missing_memory_payload["code"] == 101, missing_memory_payload
    assert "memory_ids is required" in missing_memory_payload["message"], missing_memory_payload

    search_res = rest_client.get("/messages/search", params={"memory_id": memory_id, "query": "coriander"})
    assert search_res.status_code == 200
    search_payload = search_res.json()
    assert search_payload["code"] == 0, search_payload
    assert isinstance(search_payload["data"], list), search_payload

    search_no_memory = rest_client.get("/messages/search", params={"query": "coriander"})
    assert search_no_memory.status_code == 200
    search_no_memory_payload = search_no_memory.json()
    assert search_no_memory_payload["code"] == 0, search_no_memory_payload
    assert isinstance(search_no_memory_payload["data"], list), search_no_memory_payload


@pytest.mark.p2
def test_message_update_forget_and_content_error_contracts(rest_client, memory_resource):
    memory_id = memory_resource["id"]

    invalid_status_res = rest_client.put(
        f"/messages/{memory_id}:1",
        json={"status": "false"},
    )
    assert invalid_status_res.status_code == 200
    invalid_status_payload = invalid_status_res.json()
    assert invalid_status_payload["code"] == 101, invalid_status_payload
    assert "Status must be a boolean" in invalid_status_payload["message"], invalid_status_payload

    missing_content_res = rest_client.get(f"/messages/{memory_id}:999999/content")
    assert missing_content_res.status_code == 200
    missing_content_payload = missing_content_res.json()
    assert missing_content_payload["code"] == 404, missing_content_payload

    invalid_memory_forget = rest_client.delete("/messages/missing_memory_id:1")
    assert invalid_memory_forget.status_code == 200
    invalid_memory_forget_payload = invalid_memory_forget.json()
    assert invalid_memory_forget_payload["code"] == 404, invalid_memory_forget_payload

    invalid_memory_update = rest_client.put("/messages/missing_memory_id:1", json={"status": False})
    assert invalid_memory_update.status_code == 200
    invalid_memory_update_payload = invalid_memory_update.json()
    assert invalid_memory_update_payload["code"] == 404, invalid_memory_update_payload
