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


def _assert_created_chunk_id(payload):
    chunk_id = payload["data"]["chunk"].get("id")
    assert chunk_id, payload
    assert isinstance(chunk_id, str), payload
    assert chunk_id.strip(), payload
    return chunk_id


@pytest.mark.p1
def test_chunks_add_list_get_update_delete_cycle(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_cycle.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"

    add_res = rest_client.post(
        base_path,
        json={"content": "batch2 chunk content", "important_keywords": ["batch2"], "questions": ["what is batch2?"]},
    )
    assert add_res.status_code == 200
    add_payload = add_res.json()
    assert add_payload["code"] == 0, add_payload
    chunk_id = _assert_created_chunk_id(add_payload)

    list_res = rest_client.get(base_path, params={"id": chunk_id})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert list_payload["data"]["total"] == 1, list_payload
    assert list_payload["data"]["chunks"][0]["id"] == chunk_id, list_payload

    get_res = rest_client.get(f"{base_path}/{chunk_id}")
    assert get_res.status_code == 200
    get_payload = get_res.json()
    assert get_payload["code"] == 0, get_payload
    assert get_payload["data"]["id"] == chunk_id, get_payload

    update_res = rest_client.patch(
        f"{base_path}/{chunk_id}",
        json={"content": "batch2 chunk content updated"},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload

    get_updated_res = rest_client.get(f"{base_path}/{chunk_id}")
    assert get_updated_res.status_code == 200
    get_updated_payload = get_updated_res.json()
    assert get_updated_payload["code"] == 0, get_updated_payload
    assert get_updated_payload["data"]["content_with_weight"] == "batch2 chunk content updated", get_updated_payload

    delete_candidate_res = rest_client.post(base_path, json={"content": "batch2 chunk content to delete"})
    assert delete_candidate_res.status_code == 200
    delete_candidate_payload = delete_candidate_res.json()
    assert delete_candidate_payload["code"] == 0, delete_candidate_payload
    delete_candidate_id = _assert_created_chunk_id(delete_candidate_payload)

    delete_res = rest_client.delete(base_path, json={"chunk_ids": [delete_candidate_id]})
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 0, delete_payload

    deleted_list_res = rest_client.get(base_path, params={"id": delete_candidate_id})
    assert deleted_list_res.status_code == 200
    deleted_list_payload = deleted_list_res.json()
    assert deleted_list_payload["code"] != 0, deleted_list_payload

    deleted_get_res = rest_client.get(f"{base_path}/{delete_candidate_id}")
    assert deleted_get_res.status_code == 200
    deleted_get_payload = deleted_get_res.json()
    assert deleted_get_payload["code"] != 0, deleted_get_payload


@pytest.mark.p2
def test_chunks_add_requires_content(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_requires_content.txt")
    res = rest_client.post(
        f"/datasets/{dataset_id}/documents/{document_id}/chunks",
        json={"content": " "},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert payload["message"] == "`content` is required", payload


@pytest.mark.p2
def test_chunks_list_empty_document(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_list_empty.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    list_res = rest_client.get(base_path)
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert "chunks" in list_payload["data"], list_payload
    assert "doc" in list_payload["data"], list_payload


@pytest.mark.p2
def test_chunks_delete_partial_invalid(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_delete_partial.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    delete_res = rest_client.delete(base_path, json={"chunk_ids": ["invalid_chunk_id"]})
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 102, delete_payload
    assert "expect 1" in delete_payload["message"], delete_payload
