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

from concurrent.futures import ThreadPoolExecutor
import pytest
import requests
from test.testcases.configs import HOST_ADDRESS, INVALID_API_TOKEN, VERSION
from test.testcases.restful_api.helpers.client import RestClient
from test.testcases.utils import wait_for


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


@wait_for(20, 1, "Retrieval indexing timeout in RESTful batch 10 tests")
def _retrieval_has_question(rest_client, dataset_id, question):
    res = rest_client.post("/retrieval", json={"question": question, "dataset_ids": [dataset_id]})
    if res.status_code != 200:
        return False
    payload = res.json()
    if payload["code"] != 0:
        return False
    return len(payload["data"]["chunks"]) > 0


@wait_for(20, 1, "Retrieval indexing timeout waiting for chunk presence in RESTful batch 10 tests")
def _retrieval_has_chunks(rest_client, dataset_id, question, chunk_ids):
    res = rest_client.post("/retrieval", json={"question": question, "dataset_ids": [dataset_id]})
    if res.status_code != 200:
        return False
    payload = res.json()
    if payload["code"] != 0:
        return False
    retrieved_ids = {chunk["id"] for chunk in payload["data"]["chunks"]}
    expected_ids = set(chunk_ids)
    return expected_ids.issubset(retrieved_ids)


@wait_for(20, 1, "Retrieval indexing timeout waiting for chunk deletion in RESTful batch 10 tests")
def _retrieval_lacks_chunks(rest_client, dataset_id, question, chunk_ids):
    res = rest_client.post("/retrieval", json={"question": question, "dataset_ids": [dataset_id]})
    if res.status_code != 200:
        return False
    payload = res.json()
    if payload["code"] != 0:
        return False
    retrieved_ids = {chunk["id"] for chunk in payload["data"]["chunks"]}
    expected_ids = set(chunk_ids)
    return expected_ids.isdisjoint(retrieved_ids)


@pytest.mark.p2
def test_retrieval_requires_auth_contract(ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    for scenario_name, token, expected_code, expected_message in (
        ("missing token", None, 0, "`Authorization` can't be empty"),
        ("invalid token", INVALID_API_TOKEN, 109, "Authentication error: API key is invalid!"),
    ):
        client = RestClient(token=token)
        res = client.post("/retrieval", json={"question": "chunk", "dataset_ids": [dataset_id]})
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == expected_code, (scenario_name, payload)
        assert payload["message"] == expected_message, (scenario_name, payload)


@pytest.mark.p2
def test_retrieval_page_and_page_size_contract(rest_client, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    cases = [
        ("page none", {"question": "chunk", "dataset_ids": [dataset_id], "page": None, "page_size": 2}, 100, "TypeError"),
        ("page zero", {"question": "chunk", "dataset_ids": [dataset_id], "page": 0, "page_size": 2}, 0, ""),
        ("page two", {"question": "chunk", "dataset_ids": [dataset_id], "page": 2, "page_size": 2}, 0, ""),
        ("page three", {"question": "chunk", "dataset_ids": [dataset_id], "page": 3, "page_size": 2}, 0, ""),
        ("page str", {"question": "chunk", "dataset_ids": [dataset_id], "page": "3", "page_size": 2}, 0, ""),
        ("page negative", {"question": "chunk", "dataset_ids": [dataset_id], "page": -1, "page_size": 2}, 0, ""),
        ("page alpha", {"question": "chunk", "dataset_ids": [dataset_id], "page": "a", "page_size": 2}, 100, "invalid literal for int()"),
        ("page_size none", {"question": "chunk", "dataset_ids": [dataset_id], "page_size": None}, 100, "TypeError"),
        ("page_size one", {"question": "chunk", "dataset_ids": [dataset_id], "page_size": 1}, 0, ""),
        ("page_size five", {"question": "chunk", "dataset_ids": [dataset_id], "page_size": 5}, 0, ""),
        ("page_size str", {"question": "chunk", "dataset_ids": [dataset_id], "page_size": "1"}, 0, ""),
        ("page_size alpha", {"question": "chunk", "dataset_ids": [dataset_id], "page_size": "a"}, 100, "invalid literal for int()"),
    ]
    for scenario_name, payload, expected_code, expected_message in cases:
        res = rest_client.post("/retrieval", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_code != 0:
            assert expected_message in body["message"], (scenario_name, body)


@pytest.mark.p2
def test_retrieval_highlight_keyword_and_invalid_params_contract(rest_client, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()

    highlight_cases = [
        ("highlight true", True, True),
        ("highlight true str", "True", True),
        ("highlight false", False, False),
        ("highlight false str", "False", False),
        ("highlight none", None, False),
    ]
    for scenario_name, highlight_value, expect_highlight in highlight_cases:
        res = rest_client.post(
            "/retrieval",
            json={"question": "chunk", "dataset_ids": [dataset_id], "highlight": highlight_value},
        )
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == 0, (scenario_name, body)
        for chunk in body["data"]["chunks"]:
            if expect_highlight:
                assert "highlight" in chunk, (scenario_name, body)
            else:
                assert "highlight" not in chunk, (scenario_name, body)

    invalid_highlight = rest_client.post(
        "/retrieval",
        json={"question": "chunk", "dataset_ids": [dataset_id], "highlight": "not_bool"},
    )
    assert invalid_highlight.status_code == 200
    invalid_highlight_payload = invalid_highlight.json()
    assert invalid_highlight_payload["code"] == 102, invalid_highlight_payload
    assert invalid_highlight_payload["message"] == "`highlight` should be a boolean", invalid_highlight_payload

    for scenario_name, keyword_value in (
        ("keyword true", True),
        ("keyword true str", "True"),
        ("keyword false", False),
        ("keyword false str", "False"),
        ("keyword none", None),
    ):
        keyword_res = rest_client.post(
            "/retrieval",
            json={"question": "chunk test", "dataset_ids": [dataset_id], "keyword": keyword_value},
        )
        assert keyword_res.status_code == 200, (scenario_name, keyword_res.text)
        keyword_payload = keyword_res.json()
        assert keyword_payload["code"] == 0, (scenario_name, keyword_payload)
        assert isinstance(keyword_payload["data"]["chunks"], list), (scenario_name, keyword_payload)

    invalid_params_res = rest_client.post(
        "/retrieval",
        json={"question": "chunk", "dataset_ids": [dataset_id], "a": "b"},
    )
    assert invalid_params_res.status_code == 200
    invalid_params_payload = invalid_params_res.json()
    assert invalid_params_payload["code"] == 0, invalid_params_payload


@pytest.mark.p2
def test_retrieval_vector_similarity_and_top_k_contract(rest_client, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    cases = [
        ("vector 0", {"vector_similarity_weight": 0}, 0, ""),
        ("vector 0.5", {"vector_similarity_weight": 0.5}, 0, ""),
        ("vector 10", {"vector_similarity_weight": 10}, 0, ""),
        ("vector alpha", {"vector_similarity_weight": "a"}, 100, "could not convert string to float"),
        ("top_k 10", {"top_k": 10}, 0, ""),
        ("top_k 1", {"top_k": 1}, 0, ""),
        ("top_k -1", {"top_k": -1}, 102, "`top_k` must be greater than 0"),
        ("top_k alpha", {"top_k": "a"}, 100, "invalid literal for int()"),
    ]
    for scenario_name, updates, expected_code, expected_message in cases:
        payload = {"question": "chunk", "dataset_ids": [dataset_id]}
        payload.update(updates)
        res = rest_client.post("/retrieval", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_code != 0:
            assert expected_message in body["message"], (scenario_name, body)


@pytest.mark.p2
def test_retrieval_rerank_unknown_contract(rest_client, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    res = rest_client.post(
        "/retrieval",
        json={"question": "chunk", "dataset_ids": [dataset_id], "rerank_id": "unknown"},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] != 0, payload
    assert payload["message"], payload


@pytest.mark.p2
def test_retrieval_concurrent_contract(rest_client, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    payload = {"question": "chunk", "dataset_ids": [dataset_id]}
    with ThreadPoolExecutor(max_workers=5) as executor:
        results = list(executor.map(lambda _: rest_client.post("/retrieval", json=payload).json(), range(20)))
    assert len(results) == 20, results
    assert all(result["code"] == 0 for result in results), results


@pytest.mark.p2
def test_deleted_chunk_not_in_retrieval_contract(rest_client, create_document):
    dataset_id, document_id = create_document("retrieval_deleted_chunk.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    content = "UNIQUE_TEST_CONTENT_12520_REST"

    add_res = rest_client.post(base_path, json={"content": content})
    assert add_res.status_code == 200
    add_payload = add_res.json()
    assert add_payload["code"] == 0, add_payload
    chunk_id = add_payload["data"]["chunk"]["id"]

    _retrieval_has_chunks(rest_client, dataset_id, content, [chunk_id])

    delete_res = rest_client.delete(base_path, json={"chunk_ids": [chunk_id]})
    assert delete_res.status_code == 200
    assert delete_res.json()["code"] == 0
    _retrieval_lacks_chunks(rest_client, dataset_id, content, [chunk_id])


@pytest.mark.p2
def test_deleted_chunks_batch_not_in_retrieval_contract(rest_client, create_document):
    dataset_id, document_id = create_document("retrieval_deleted_chunks_batch.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    chunk_ids = []
    for index in range(3):
        content = f"BATCH_DELETE_TEST_CHUNK_{index}_REST_12520"
        add_res = rest_client.post(base_path, json={"content": content})
        assert add_res.status_code == 200
        add_payload = add_res.json()
        assert add_payload["code"] == 0, add_payload
        chunk_ids.append(add_payload["data"]["chunk"]["id"])
    _retrieval_has_chunks(rest_client, dataset_id, "BATCH_DELETE_TEST_CHUNK", chunk_ids)

    delete_res = rest_client.delete(base_path, json={"chunk_ids": chunk_ids})
    assert delete_res.status_code == 200
    assert delete_res.json()["code"] == 0
    _retrieval_lacks_chunks(rest_client, dataset_id, "BATCH_DELETE_TEST_CHUNK", chunk_ids)


@pytest.mark.p2
def test_related_questions_contract(auth, rest_client, rest_client_noauth):
    tokens_res = requests.get(
        f"{HOST_ADDRESS}/api/{VERSION}/system/tokens",
        headers={"Authorization": auth},
        timeout=30,
    )
    assert tokens_res.status_code == 200, tokens_res.text
    tokens_payload = tokens_res.json()
    assert tokens_payload["code"] == 0, tokens_payload
    assert tokens_payload["data"], tokens_payload
    beta_token = tokens_payload["data"][0]["beta"]

    success_client = RestClient(token=beta_token)
    success_res = success_client.post("/searchbots/related_questions", json={"question": "ragflow", "industry": "search"})
    assert success_res.status_code == 200
    success_payload = success_res.json()
    assert success_payload["code"] == 0, success_payload
    assert isinstance(success_payload["data"], list), success_payload

    missing_res = rest_client.post("/searchbots/related_questions", json={"industry": "search"})
    assert missing_res.status_code == 200
    missing_payload = missing_res.json()
    assert missing_payload["code"] == 101, missing_payload
    assert "question" in missing_payload["message"], missing_payload

    invalid_auth_res = rest_client_noauth.post(
        "/searchbots/related_questions",
        json={"question": "ragflow", "industry": "search"},
        headers={"Authorization": "invalid"},
    )
    assert invalid_auth_res.status_code == 200
    invalid_auth_payload = invalid_auth_res.json()
    assert invalid_auth_payload["code"] == 102, invalid_auth_payload
    assert "Authorization is not valid!" in invalid_auth_payload["message"], invalid_auth_payload
