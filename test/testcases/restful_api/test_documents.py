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
from utils.file_utils import create_txt_file


@pytest.mark.p1
def test_documents_upload_and_list(rest_client, create_dataset, tmp_path):
    dataset_id = create_dataset("dataset_upload_list")
    fp = create_txt_file(tmp_path / "upload_and_list.txt")
    with fp.open("rb") as file_obj:
        res = rest_client.post(
            f"/datasets/{dataset_id}/documents",
            files=[("file", (fp.name, file_obj))],
        )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert payload["data"][0]["dataset_id"] == dataset_id, payload

    list_res = rest_client.get(f"/datasets/{dataset_id}/documents")
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert list_payload["data"]["total"] >= 1, list_payload
    assert any(doc["name"] == fp.name for doc in list_payload["data"]["docs"]), list_payload


@pytest.mark.p2
def test_documents_upload_missing_file(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_upload_missing")
    res = rest_client.post(f"/datasets/{dataset_id}/documents")
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload
    assert payload["message"] == "No file part!", payload


@pytest.mark.p2
def test_documents_update_patch_and_delete(rest_client, create_document):
    dataset_id, document_id = create_document("update_target.txt")

    patch_res = rest_client.patch(
        f"/datasets/{dataset_id}/documents/{document_id}",
        json={"name": "updated_target.txt"},
    )
    assert patch_res.status_code == 200
    patch_payload = patch_res.json()
    assert patch_payload["code"] == 0, patch_payload
    assert patch_payload["data"]["name"] == "updated_target.txt", patch_payload

    delete_res = rest_client.delete(
        f"/datasets/{dataset_id}/documents",
        json={"ids": [document_id]},
    )
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 0, delete_payload
    assert delete_payload["data"]["deleted"] == 1, delete_payload

    list_res = rest_client.get(f"/datasets/{dataset_id}/documents")
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert all(doc["id"] != document_id for doc in list_payload["data"]["docs"]), list_payload


@pytest.mark.p2
def test_documents_parse_and_stop(rest_client, create_document):
    dataset_id, document_id = create_document("parse_target.txt")

    parse_res = rest_client.post(
        f"/datasets/{dataset_id}/documents/parse",
        json={"document_ids": [document_id]},
    )
    assert parse_res.status_code == 200
    parse_payload = parse_res.json()
    assert parse_payload["code"] == 0, parse_payload

    stop_res = rest_client.post(
        f"/datasets/{dataset_id}/documents/stop",
        json={"document_ids": [document_id]},
    )
    assert stop_res.status_code == 200
    stop_payload = stop_res.json()
    # Depending on timing this can be immediate stop success or "already completed".
    assert stop_payload["code"] in (0, 102), stop_payload
    if stop_payload["code"] == 102:
        assert "already completed" in stop_payload["message"], stop_payload


@pytest.mark.p2
def test_documents_metadata_update_path(rest_client, create_document):
    dataset_id, document_id = create_document("metadata_target.txt")

    res = rest_client.patch(
        f"/datasets/{dataset_id}/documents/metadatas",
        json={
            "selector": {"document_ids": [document_id]},
            "updates": [{"key": "author", "value": "qa"}],
            "deletes": [],
        },
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert payload["data"]["matched_docs"] == 1, payload
    assert payload["data"]["updated"] >= 1, payload
