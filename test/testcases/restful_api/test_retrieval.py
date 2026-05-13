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
def test_dataset_search_rest_endpoint(rest_client, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    res = rest_client.post(
        f"/datasets/{dataset_id}/search",
        json={"question": "test TXT file", "top_k": 5},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert "chunks" in payload["data"], payload


@pytest.mark.p2
def test_multi_dataset_search_rest_endpoint(rest_client, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    res = rest_client.post(
        "/datasets/search",
        json={"dataset_ids": [dataset_id], "question": "test TXT file", "top_k": 5},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert "chunks" in payload["data"], payload


@pytest.mark.p2
def test_multi_dataset_search_with_metadata_filter(rest_client, ensure_parsed_document):
    dataset_id, document_id = ensure_parsed_document()
    meta_res = rest_client.patch(
        f"/datasets/{dataset_id}/documents/metadatas",
        json={
            "selector": {"document_ids": [document_id]},
            "updates": [{"key": "author", "value": "qa_batch2"}],
            "deletes": [],
        },
    )
    assert meta_res.status_code == 200
    meta_payload = meta_res.json()
    assert meta_payload["code"] == 0, meta_payload

    res = rest_client.post(
        "/datasets/search",
        json={
            "dataset_ids": [dataset_id],
            "question": "test TXT file",
            "meta_data_filter": {
                "method": "manual",
                "logic": "and",
                "manual": [{"key": "author", "op": "=", "value": "qa_batch2"}],
            },
        },
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert "chunks" in payload["data"], payload


@pytest.mark.p2
def test_retrieval_compatibility_endpoint(rest_client, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    # /api/v1/retrieval is SDK compatibility endpoint from api/apps/sdk/doc.py.
    res = rest_client.post(
        "/retrieval",
        json={"dataset_ids": [dataset_id], "question": "test TXT file", "top_k": 5},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert "chunks" in payload["data"], payload


@pytest.mark.p2
def test_retrieval_compatibility_requires_dataset_ids(rest_client):
    res = rest_client.post("/retrieval", json={"question": "test"})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert payload["message"] == "`dataset_ids` is required.", payload


@pytest.mark.p2
def test_retrieval_compatibility_requires_auth(rest_client_noauth):
    res = rest_client_noauth.post("/retrieval", json={"question": "test", "dataset_ids": ["x"]})
    assert res.status_code == 401
    payload = res.json()
    # token_required preserves legacy payload code/message while returning HTTP 401.
    assert payload["code"] == 0, payload
    assert payload["message"] == "`Authorization` can't be empty", payload
