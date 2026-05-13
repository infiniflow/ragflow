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


@pytest.mark.p1
class TestChatsAuthorization:
    def test_create_requires_auth(self, rest_client_noauth):
        res = rest_client_noauth.post("/chats", json={"name": "chat_auth", "dataset_ids": []})
        assert res.status_code == 401


@pytest.mark.p1
def test_chat_crud_cycle(rest_client, clear_chats):
    create_res = rest_client.post(
        "/chats",
        json={"name": "restful_chat_crud", "dataset_ids": []},
    )
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    chat_id = create_payload["data"]["id"]

    list_res = rest_client.get("/chats", params={"id": chat_id})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    chats = list_payload["data"]["chats"]
    assert len(chats) == 1, list_payload
    assert chats[0]["id"] == chat_id, list_payload

    get_res = rest_client.get(f"/chats/{chat_id}")
    assert get_res.status_code == 200
    get_payload = get_res.json()
    assert get_payload["code"] == 0, get_payload
    assert get_payload["data"]["id"] == chat_id, get_payload

    update_res = rest_client.put(
        f"/chats/{chat_id}",
        json={"name": "restful_chat_crud_updated", "dataset_ids": []},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload
    assert update_payload["data"]["name"] == "restful_chat_crud_updated", update_payload

    patch_res = rest_client.patch(f"/chats/{chat_id}", json={"name": "restful_chat_crud_patched"})
    assert patch_res.status_code == 200
    patch_payload = patch_res.json()
    assert patch_payload["code"] == 0, patch_payload
    assert patch_payload["data"]["name"] == "restful_chat_crud_patched", patch_payload

    delete_res = rest_client.delete("/chats", json={"ids": [chat_id]})
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 0, delete_payload
    assert delete_payload["data"]["success_count"] == 1, delete_payload

    list_after_delete = rest_client.get("/chats", params={"id": chat_id})
    assert list_after_delete.status_code == 200
    list_after_delete_payload = list_after_delete.json()
    assert list_after_delete_payload["code"] == 0, list_after_delete_payload
    assert list_after_delete_payload["data"]["chats"] == [], list_after_delete_payload


@pytest.mark.p2
@pytest.mark.parametrize(
    "name, expected_fragment",
    [
        ("", "`name` is required."),
        (" ", "`name` is required."),
    ],
)
def test_chat_create_name_validation(rest_client, clear_chats, name, expected_fragment):
    res = rest_client.post("/chats", json={"name": name, "dataset_ids": []})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert expected_fragment in payload["message"], payload


@pytest.mark.p2
def test_chat_duplicate_name_validation(rest_client, clear_chats):
    first = rest_client.post("/chats", json={"name": "duplicate_chat_name", "dataset_ids": []})
    assert first.status_code == 200
    first_payload = first.json()
    assert first_payload["code"] == 0, first_payload

    second = rest_client.post("/chats", json={"name": "duplicate_chat_name", "dataset_ids": []})
    assert second.status_code == 200
    second_payload = second.json()
    assert second_payload["code"] == 102, second_payload
    assert "Duplicated chat name" in second_payload["message"], second_payload


@pytest.mark.p2
def test_chat_list_pagination(rest_client, clear_chats):
    for i in range(3):
        res = rest_client.post("/chats", json={"name": f"chat_page_{i}", "dataset_ids": []})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 0, payload

    page_res = rest_client.get("/chats", params={"page": 1, "page_size": 2, "orderby": "create_time", "desc": "true"})
    assert page_res.status_code == 200
    page_payload = page_res.json()
    assert page_payload["code"] == 0, page_payload
    assert len(page_payload["data"]["chats"]) == 2, page_payload
    assert page_payload["data"]["total"] >= 3, page_payload
