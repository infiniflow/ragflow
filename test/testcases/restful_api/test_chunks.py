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
import os
import pytest
from test.testcases.configs import INVALID_API_TOKEN, INVALID_ID_32
from test.testcases.restful_api.helpers.client import RestClient
from test.testcases.utils import wait_for


def _assert_created_chunk_id(payload):
    chunk_id = payload["data"]["chunk"].get("id")
    assert chunk_id, payload
    assert isinstance(chunk_id, str), payload
    assert chunk_id.strip(), payload
    return chunk_id


@wait_for(10, 1, "Chunk indexing timeout in RESTful batch 09 tests")
def _chunk_count(rest_client, base_path, expected_total):
    res = rest_client.get(base_path)
    if res.status_code != 200:
        return False
    payload = res.json()
    if payload["code"] != 0:
        return False
    return payload["data"]["total"] == expected_total and len(payload["data"]["chunks"]) == min(expected_total, 30)


def _reset_chunk_batch(rest_client, base_path, count=4):
    cleanup_res = rest_client.delete(base_path, json={"chunk_ids": None, "delete_all": True})
    assert cleanup_res.status_code == 200, cleanup_res.text
    cleanup_payload = cleanup_res.json()
    assert cleanup_payload["code"] == 0, cleanup_payload

    baseline_res = rest_client.post(base_path, json={"content": "ragflow test upload"})
    assert baseline_res.status_code == 200, baseline_res.text
    baseline_payload = baseline_res.json()
    assert baseline_payload["code"] == 0, baseline_payload
    baseline_id = _assert_created_chunk_id(baseline_payload)

    chunk_ids = []
    for index in range(count):
        res = rest_client.post(base_path, json={"content": f"chunk test {index}"})
        assert res.status_code == 200, (index, res.text)
        payload = res.json()
        assert payload["code"] == 0, (index, payload)
        chunk_ids.append(_assert_created_chunk_id(payload))

    _chunk_count(rest_client, base_path, count + 1)
    return baseline_id, chunk_ids


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


@pytest.mark.p1
def test_chunk_add_requires_auth(create_document):
    dataset_id, document_id = create_document("chunk_add_auth.txt")
    path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.post(path, json={"content": "chunk test"})
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)


@pytest.mark.p1
def test_chunk_delete_requires_auth(create_document):
    dataset_id, document_id = create_document("chunk_delete_auth.txt")
    path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.delete(path, json={"chunk_ids": []})
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)


@pytest.mark.p1
def test_chunk_list_requires_auth(create_document):
    dataset_id, document_id = create_document("chunk_list_auth.txt")
    path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.get(path)
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)


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
def test_chunk_add_keyword_question_and_tag_contract(rest_client, create_document):
    add_cases = [
        (
            "important keywords",
            [
                ({"content": "chunk test", "important_keywords": ["a", "b", "c"]}, 0, ""),
                ({"content": "chunk test", "important_keywords": [""]}, 0, ""),
                ({"content": "chunk test", "important_keywords": [1]}, 100, "TypeError('sequence item 0: expected str instance, int found')"),
                ({"content": "chunk test", "important_keywords": ["a", "a"]}, 0, ""),
                ({"content": "chunk test", "important_keywords": "abc"}, 102, "`important_keywords` is required to be a list"),
                ({"content": "chunk test", "important_keywords": 123}, 102, "`important_keywords` is required to be a list"),
            ],
        ),
        (
            "questions",
            [
                ({"content": "chunk test", "questions": ["a", "b", "c"]}, 0, ""),
                ({"content": "chunk test", "questions": [""]}, 0, ""),
                ({"content": "chunk test", "questions": [1]}, 100, "TypeError('sequence item 0: expected str instance, int found')"),
                ({"content": "chunk test", "questions": ["a", "a"]}, 0, ""),
                ({"content": "chunk test", "questions": "abc"}, 102, "`questions` is required to be a list"),
                ({"content": "chunk test", "questions": 123}, 102, "`questions` is required to be a list"),
            ],
        ),
        (
            "tag_kwd",
            [
                ({"content": "chunk test", "tag_kwd": ["tag1", "tag2"]}, 0, ""),
                ({"content": "chunk test", "tag_kwd": [""]}, 0, ""),
                ({"content": "chunk test", "tag_kwd": [1]}, 102, "`tag_kwd` must be a list of strings"),
                ({"content": "chunk test", "tag_kwd": ["tag", "tag"]}, 0, ""),
                ({"content": "chunk test", "tag_kwd": "abc"}, 102, "`tag_kwd` is required to be a list"),
                ({"content": "chunk test", "tag_kwd": 123}, 102, "`tag_kwd` is required to be a list"),
            ],
        ),
    ]

    for group_index, (group_name, cases) in enumerate(add_cases):
        dataset_id, document_id = create_document(f"chunk_add_contracts_{group_index}.txt")
        base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
        for scenario_index, (payload, expected_code, expected_message) in enumerate(cases):
            scenario_name = f"{group_name}-{scenario_index}"
            before_payload = rest_client.get(base_path).json()
            assert before_payload["code"] == 0, (scenario_name, before_payload)
            before_total = before_payload["data"]["doc"]["chunk_count"]

            res = rest_client.post(base_path, json=payload)
            assert res.status_code == 200, (scenario_name, res.text)
            body = res.json()
            assert body["code"] == expected_code, (scenario_name, body)
            if expected_code == 0:
                chunk = body["data"]["chunk"]
                assert chunk["dataset_id"] == dataset_id, (scenario_name, body)
                assert chunk["document_id"] == document_id, (scenario_name, body)
                assert chunk["content"] == payload["content"], (scenario_name, body)
                if "important_keywords" in payload:
                    assert chunk["important_keywords"] == payload["important_keywords"], (scenario_name, body)
                if "questions" in payload:
                    assert chunk["questions"] == [str(q).strip() for q in payload["questions"] if str(q).strip()], (scenario_name, body)
                if "tag_kwd" in payload:
                    assert chunk["tag_kwd"] == payload["tag_kwd"], (scenario_name, body)
                after_payload = rest_client.get(base_path).json()
                assert after_payload["code"] == 0, (scenario_name, after_payload)
                assert after_payload["data"]["doc"]["chunk_count"] == before_total + 1, (scenario_name, after_payload)
            else:
                assert body["message"] == expected_message, (scenario_name, body)


@pytest.mark.p2
def test_chunk_add_invalid_dataset_and_document_contract(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_invalid_targets.txt")

    invalid_dataset_res = rest_client.post(
        f"/datasets/{INVALID_ID_32}/documents/{document_id}/chunks",
        json={"content": "chunk test"},
    )
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] == 102, invalid_dataset_payload
    assert invalid_dataset_payload["message"] == f"You don't own the dataset {INVALID_ID_32}.", invalid_dataset_payload

    invalid_document_res = rest_client.post(
        f"/datasets/{dataset_id}/documents/{INVALID_ID_32}/chunks",
        json={"content": "chunk test"},
    )
    assert invalid_document_res.status_code == 200
    invalid_document_payload = invalid_document_res.json()
    assert invalid_document_payload["code"] == 102, invalid_document_payload
    assert invalid_document_payload["message"] == f"You don't own the document {INVALID_ID_32}.", invalid_document_payload


@pytest.mark.p2
def test_chunk_add_repeated_and_deleted_document_contract(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_repeat_deleted.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"

    first_payload = rest_client.get(base_path).json()
    assert first_payload["code"] == 0, first_payload
    initial_count = first_payload["data"]["doc"]["chunk_count"]

    first_add_res = rest_client.post(base_path, json={"content": "chunk test"})
    second_add_res = rest_client.post(base_path, json={"content": "chunk test"})
    first_add_payload = first_add_res.json()
    second_add_payload = second_add_res.json()
    assert first_add_payload["code"] == 0, first_add_payload
    assert second_add_payload["code"] == 0, second_add_payload
    assert first_add_payload["data"]["chunk"]["id"] == second_add_payload["data"]["chunk"]["id"], (first_add_payload, second_add_payload)

    repeated_list_payload = rest_client.get(base_path).json()
    assert repeated_list_payload["code"] == 0, repeated_list_payload
    assert repeated_list_payload["data"]["doc"]["chunk_count"] == initial_count + 2, repeated_list_payload
    assert repeated_list_payload["data"]["total"] == 1, repeated_list_payload

    delete_document_res = rest_client.delete(f"/datasets/{dataset_id}/documents", json={"ids": [document_id]})
    assert delete_document_res.status_code == 200
    delete_document_payload = delete_document_res.json()
    assert delete_document_payload["code"] == 0, delete_document_payload

    add_after_delete_res = rest_client.post(base_path, json={"content": "chunk test"})
    assert add_after_delete_res.status_code == 200
    add_after_delete_payload = add_after_delete_res.json()
    assert add_after_delete_payload["code"] == 102, add_after_delete_payload
    assert add_after_delete_payload["message"] == f"You don't own the document {document_id}.", add_after_delete_payload


@pytest.mark.p2
@pytest.mark.parametrize("count", [20])
def test_chunk_concurrent_add_contract(rest_client, create_document, count):
    dataset_id, document_id = create_document("chunk_concurrent_add.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    baseline_payload = rest_client.get(base_path).json()
    assert baseline_payload["code"] == 0, baseline_payload
    initial_count = baseline_payload["data"]["doc"]["chunk_count"]

    with ThreadPoolExecutor(max_workers=5) as executor:
        results = list(
            executor.map(
                lambda index: rest_client.post(base_path, json={"content": f"chunk test {index}"}).json(),
                range(count),
            )
        )
    assert len(results) == count, results
    assert all(result["code"] == 0 for result in results), results

    final_payload = rest_client.get(base_path).json()
    assert final_payload["code"] == 0, final_payload
    assert final_payload["data"]["doc"]["chunk_count"] == initial_count + count, final_payload


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
def test_chunk_delete_basic_contract(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_delete_basic.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"

    cases = [
        ("none payload", None, 0, "", 5),
        ("invalid only", {"chunk_ids": ["invalid_id"]}, 102, "rm_chunk deleted chunks 0, expect 1", 5),
        ("delete first", lambda ids: {"chunk_ids": ids[:1]}, 0, "", 4),
        ("delete generated", lambda ids: {"chunk_ids": ids}, 0, "", 1),
        ("empty ids", {"chunk_ids": []}, 0, "", 5),
    ]

    for scenario_name, payload, expected_code, expected_message, remaining in cases:
        _reset_chunk_batch(rest_client, base_path)
        request_body = payload
        generated_ids = rest_client.get(base_path).json()["data"]["chunks"][1:]
        generated_ids = [chunk["id"] for chunk in generated_ids]
        if callable(payload):
            request_body = payload(generated_ids)
        res = rest_client.delete(base_path, json=request_body)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_message:
            assert body.get("message", "") == expected_message, (scenario_name, body)

        list_payload = rest_client.get(base_path).json()
        assert list_payload["code"] == 0, (scenario_name, list_payload)
        assert len(list_payload["data"]["chunks"]) == remaining, (scenario_name, list_payload)
        assert list_payload["data"]["total"] == remaining, (scenario_name, list_payload)


@pytest.mark.p2
def test_chunk_delete_partial_duplicate_repeat_and_invalid_target_contract(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_delete_detail.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"

    for scenario_name, payload_builder in (
        ("invalid first", lambda ids: {"chunk_ids": ["invalid_id"] + ids}),
        ("invalid middle", lambda ids: {"chunk_ids": ids[:1] + ["invalid_id"] + ids[1:]}),
        ("invalid last", lambda ids: {"chunk_ids": ids + ["invalid_id"]}),
    ):
        _, generated_ids = _reset_chunk_batch(rest_client, base_path)
        res = rest_client.delete(base_path, json=payload_builder(generated_ids))
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == 102, (scenario_name, body)
        assert body["message"] == "rm_chunk deleted chunks 4, expect 5", (scenario_name, body)

        list_payload = rest_client.get(base_path).json()
        assert list_payload["code"] == 0, (scenario_name, list_payload)
        assert list_payload["data"]["total"] == 1, (scenario_name, list_payload)

    _, generated_ids = _reset_chunk_batch(rest_client, base_path)
    duplicate_res = rest_client.delete(base_path, json={"chunk_ids": generated_ids * 2})
    assert duplicate_res.status_code == 200
    duplicate_payload = duplicate_res.json()
    assert duplicate_payload["code"] == 0, duplicate_payload
    assert duplicate_payload["data"]["success_count"] == 4, duplicate_payload
    assert len(duplicate_payload["data"]["errors"]) == 4, duplicate_payload
    assert all(error.startswith("Duplicate chunk ids: ") for error in duplicate_payload["data"]["errors"]), duplicate_payload
    duplicate_list_payload = rest_client.get(base_path).json()
    assert duplicate_list_payload["code"] == 0, duplicate_list_payload
    assert duplicate_list_payload["data"]["total"] == 1, duplicate_list_payload

    _, generated_ids = _reset_chunk_batch(rest_client, base_path)
    first_delete_res = rest_client.delete(base_path, json={"chunk_ids": generated_ids})
    assert first_delete_res.status_code == 200
    assert first_delete_res.json()["code"] == 0
    second_delete_res = rest_client.delete(base_path, json={"chunk_ids": generated_ids})
    assert second_delete_res.status_code == 200
    second_delete_payload = second_delete_res.json()
    assert second_delete_payload["code"] == 102, second_delete_payload
    assert second_delete_payload["message"] == "rm_chunk deleted chunks 0, expect 4", second_delete_payload

    invalid_dataset_res = rest_client.delete(
        f"/datasets/{INVALID_ID_32}/documents/{document_id}/chunks",
        json={"chunk_ids": ["chunk-id"]},
    )
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] == 102, invalid_dataset_payload
    assert invalid_dataset_payload["message"] == f"You don't own the dataset {INVALID_ID_32}.", invalid_dataset_payload

    invalid_document_res = rest_client.delete(
        f"/datasets/{dataset_id}/documents/{INVALID_ID_32}/chunks",
        json={"chunk_ids": ["chunk-id"]},
    )
    assert invalid_document_res.status_code == 200
    invalid_document_payload = invalid_document_res.json()
    assert invalid_document_payload["code"] == 102, invalid_document_payload
    assert invalid_document_payload["message"] == f"You don't own the document {INVALID_ID_32}.", invalid_document_payload


@pytest.mark.p2
def test_chunk_delete_web_legacy_basic_variants(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_delete_web_legacy_again.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    cases = [
        ("web invalid id", {"chunk_ids": ["invalid_id"]}, 102, 5),
        ("web delete first", lambda ids: {"chunk_ids": ids[:1]}, 0, 4),
        ("web delete generated", lambda ids: {"chunk_ids": ids}, 0, 1),
        ("web empty ids", {"chunk_ids": []}, 0, 5),
    ]
    for scenario_name, payload, expected_code, remaining in cases:
        _, generated_ids = _reset_chunk_batch(rest_client, base_path)
        request_body = payload(generated_ids) if callable(payload) else payload
        res = rest_client.delete(base_path, json=request_body)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        list_payload = rest_client.get(base_path).json()
        assert list_payload["code"] == 0, (scenario_name, list_payload)
        assert list_payload["data"]["total"] == remaining, (scenario_name, list_payload)


@pytest.mark.p2
def test_chunk_delete_concurrent_and_bulk_contract(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_delete_bulk_contract.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"

    rest_client.delete(base_path, json={"chunk_ids": None, "delete_all": True})
    for index in range(12):
        payload = rest_client.post(base_path, json={"content": f"chunk test {index}"}).json()
        assert payload["code"] == 0, payload
    ids_payload = rest_client.get(base_path).json()
    assert ids_payload["code"] == 0, ids_payload
    chunk_ids = [chunk["id"] for chunk in ids_payload["data"]["chunks"]]

    with ThreadPoolExecutor(max_workers=5) as executor:
        results = list(executor.map(lambda chunk_id: rest_client.delete(base_path, json={"chunk_ids": [chunk_id]}).json(), chunk_ids))
    assert len(results) == len(chunk_ids), results
    assert all(result["code"] == 0 for result in results), results

    final_payload = rest_client.get(base_path).json()
    assert final_payload["code"] == 0, final_payload
    assert final_payload["data"]["total"] == 0, final_payload

    rest_client.delete(base_path, json={"chunk_ids": None, "delete_all": True})
    for index in range(40):
        payload = rest_client.post(base_path, json={"content": f"bulk chunk {index}"}).json()
        assert payload["code"] == 0, payload
    bulk_ids_payload = rest_client.get(base_path, params={"page_size": 200}).json()
    assert bulk_ids_payload["code"] == 0, bulk_ids_payload
    bulk_ids = [chunk["id"] for chunk in bulk_ids_payload["data"]["chunks"]]
    bulk_res = rest_client.delete(base_path, json={"chunk_ids": bulk_ids})
    assert bulk_res.status_code == 200
    bulk_payload = bulk_res.json()
    assert bulk_payload["code"] == 0, bulk_payload


@pytest.mark.p2
def test_chunk_list_default_get_id_and_invalid_target_contract(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_list_core.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    baseline_id, generated_ids = _reset_chunk_batch(rest_client, base_path)

    default_res = rest_client.get(base_path)
    assert default_res.status_code == 200
    default_payload = default_res.json()
    assert default_payload["code"] == 0, default_payload
    assert default_payload["data"]["total"] == 5, default_payload
    assert len(default_payload["data"]["chunks"]) == 5, default_payload

    get_res = rest_client.get(f"{base_path}/{generated_ids[0]}")
    assert get_res.status_code == 200
    get_payload = get_res.json()
    assert get_payload["code"] == 0, get_payload
    assert get_payload["data"]["id"] == generated_ids[0], get_payload
    assert get_payload["data"]["doc_id"] == document_id, get_payload

    invalid_get_res = rest_client.get(f"{base_path}/unknown")
    assert invalid_get_res.status_code == 200
    invalid_get_payload = invalid_get_res.json()
    assert invalid_get_payload["code"] == 102, invalid_get_payload
    assert invalid_get_payload["message"] == "Chunk not found!", invalid_get_payload

    id_cases = [
        ("id none", {"id": None}, 0, 5, None),
        ("id empty", {"id": ""}, 0, 5, None),
        ("id valid", {"id": generated_ids[0]}, 0, 1, generated_ids[0]),
        ("id invalid", {"id": "unknown"}, 102, None, None),
    ]
    for scenario_name, params, expected_code, expected_total, expected_id in id_cases:
        res = rest_client.get(base_path, params=params)
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == expected_code, (scenario_name, payload)
        if expected_code == 0:
            assert payload["data"]["total"] == expected_total, (scenario_name, payload)
            if expected_id is not None:
                assert payload["data"]["chunks"][0]["id"] == expected_id, (scenario_name, payload)
        else:
            assert payload["message"] == f"Chunk not found: {dataset_id}/unknown", (scenario_name, payload)

    invalid_dataset_res = rest_client.get(f"/datasets/{INVALID_ID_32}/documents/{document_id}/chunks")
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] == 102, invalid_dataset_payload
    assert invalid_dataset_payload["message"] == f"You don't own the dataset {INVALID_ID_32}.", invalid_dataset_payload

    invalid_document_res = rest_client.get(f"/datasets/{dataset_id}/documents/{INVALID_ID_32}/chunks")
    assert invalid_document_res.status_code == 200
    invalid_document_payload = invalid_document_res.json()
    assert invalid_document_payload["code"] == 102, invalid_document_payload
    assert invalid_document_payload["message"] == f"You don't own the document {INVALID_ID_32}.", invalid_document_payload


@pytest.mark.p2
@pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="infinity")
def test_chunk_list_keyword_and_invalid_param_contract(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_list_keywords.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    _reset_chunk_batch(rest_client, base_path)

    cases = [
        ("keywords none", {"keywords": None}, 5),
        ("keywords empty", {"keywords": ""}, 5),
        ("keywords exact one", {"keywords": "1"}, 1),
        ("keywords chunk", {"keywords": "chunk"}, 4),
        ("keywords ragflow", {"keywords": "ragflow"}, 1),
        ("keywords unknown", {"keywords": "unknown"}, 0),
        ("invalid params ignored", {"a": "b"}, 5),
    ]

    for scenario_name, params, expected_total in cases:
        res = rest_client.get(base_path, params=params)
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 0, (scenario_name, payload)
        assert payload["data"]["total"] == expected_total, (scenario_name, payload)
        assert len(payload["data"]["chunks"]) == expected_total, (scenario_name, payload)


@pytest.mark.p2
@pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="infinity")
def test_chunk_list_page_and_page_size_contract(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_list_paging.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    _reset_chunk_batch(rest_client, base_path)

    cases = [
        ("page none", {"page": None, "page_size": 2}, 0, 2, ""),
        ("page zero", {"page": 0, "page_size": 2}, 100, None, "ValueError('Search does not support negative slicing.')"),
        ("page two", {"page": 2, "page_size": 2}, 0, 2, ""),
        ("page three", {"page": 3, "page_size": 2}, 0, 1, ""),
        ("page string", {"page": "3", "page_size": 2}, 0, 1, ""),
        ("page negative", {"page": -1, "page_size": 2}, 100, None, "ValueError('Search does not support negative slicing.')"),
        ("page alpha", {"page": "a", "page_size": 2}, 100, None, "ValueError(\"invalid literal for int() with base 10: 'a'\")"),
        ("page_size none", {"page_size": None}, 0, 5, ""),
        ("page_size zero", {"page_size": 0}, 0, 5, ""),
        ("page_size one", {"page_size": 1}, 0, 1, ""),
        ("page_size six", {"page_size": 6}, 0, 5, ""),
        ("page_size string", {"page_size": "1"}, 0, 1, ""),
        ("page_size negative", {"page_size": -1}, 0, 5, ""),
        ("page_size alpha", {"page_size": "a"}, 100, None, "ValueError(\"invalid literal for int() with base 10: 'a'\")"),
    ]

    for scenario_name, params, expected_code, expected_total, expected_message in cases:
        res = rest_client.get(base_path, params=params)
        assert res.status_code == 200, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == expected_code, (scenario_name, payload)
        if expected_code == 0:
            assert payload["data"]["total"] == 5, (scenario_name, payload)
            assert len(payload["data"]["chunks"]) == expected_total, (scenario_name, payload)
        else:
            assert expected_message in payload["message"], (scenario_name, payload)


@pytest.mark.p2
def test_chunk_list_concurrent_contract(rest_client, create_document):
    dataset_id, document_id = create_document("chunk_list_concurrent.txt")
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    _reset_chunk_batch(rest_client, base_path)

    with ThreadPoolExecutor(max_workers=5) as executor:
        results = list(executor.map(lambda _: rest_client.get(base_path).json(), range(20)))
    assert len(results) == 20, results
    assert all(result["code"] == 0 for result in results), results
    assert all(result["data"]["total"] == 5 for result in results), results


def _create_chunk_for_update(rest_client, create_document, file_name):
    dataset_id, document_id = create_document(file_name)
    base_path = f"/datasets/{dataset_id}/documents/{document_id}/chunks"
    add_res = rest_client.post(base_path, json={"content": "chunk update test"})
    assert add_res.status_code == 200, add_res.text
    add_payload = add_res.json()
    assert add_payload["code"] == 0, add_payload
    chunk_id = add_payload["data"]["chunk"]["id"]
    return dataset_id, document_id, chunk_id, base_path


@pytest.mark.p2
def test_chunk_update_requires_auth(rest_client, create_document):
    _, _, chunk_id, base_path = _create_chunk_for_update(rest_client, create_document, "chunk_update_auth.txt")
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.patch(f"{base_path}/{chunk_id}", json={"content": "updated"})
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)


@pytest.mark.p2
def test_chunk_update_content_and_available_contract(rest_client, create_document):
    content_cases = [
        ("content none", {"content": None}, 0, ""),
        ("content empty", {"content": ""}, 102, "`content` is required"),
        ("content text", {"content": "update chunk"}, 0, ""),
        ("content spaces", {"content": " "}, 102, "`content` is required"),
        ("content punctuation", {"content": "\n!?。；！？\"'"}, 0, ""),
    ]
    for scenario_name, payload, expected_code, expected_message in content_cases:
        _, _, chunk_id, base_path = _create_chunk_for_update(rest_client, create_document, f"{scenario_name}.txt")
        res = rest_client.patch(f"{base_path}/{chunk_id}", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_code != 0:
            assert body["message"] == expected_message, (scenario_name, body)

    available_cases = [
        ("available true", {"available": True}, 0, ""),
        ("available true str", {"available": "True"}, 100, "invalid literal for int()"),
        ("available one", {"available": 1}, 0, ""),
        ("available false", {"available": False}, 0, ""),
        ("available false str", {"available": "False"}, 100, "invalid literal for int()"),
        ("available zero", {"available": 0}, 0, ""),
    ]
    for scenario_name, payload, expected_code, expected_message in available_cases:
        _, _, chunk_id, base_path = _create_chunk_for_update(rest_client, create_document, f"{scenario_name}.txt")
        res = rest_client.patch(f"{base_path}/{chunk_id}", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_code != 0:
            assert expected_message in body["message"], (scenario_name, body)


@pytest.mark.p2
def test_chunk_update_keywords_questions_and_tag_contract(rest_client, create_document):
    _, _, chunk_id, base_path = _create_chunk_for_update(rest_client, create_document, "chunk_update_fields.txt")
    cases = [
        ("important keywords", {"important_keywords": ["a", "b", "c"]}, 0, ""),
        ("important keywords empty", {"important_keywords": [""]}, 0, ""),
        ("important keywords int", {"important_keywords": [1]}, 100, "TypeError"),
        ("important keywords dup", {"important_keywords": ["a", "a"]}, 0, ""),
        ("important keywords str", {"important_keywords": "abc"}, 102, "`important_keywords` should be a list"),
        ("important keywords number", {"important_keywords": 123}, 102, "`important_keywords` should be a list"),
        ("questions", {"questions": ["a", "b", "c"]}, 0, ""),
        ("questions empty", {"questions": [""]}, 0, ""),
        ("questions int", {"questions": [1]}, 100, "TypeError"),
        ("questions dup", {"questions": ["a", "a"]}, 0, ""),
        ("questions str", {"questions": "abc"}, 102, "`questions` should be a list"),
        ("questions number", {"questions": 123}, 102, "`questions` should be a list"),
        ("tag kwd", {"tag_kwd": ["tag1", "tag2"]}, 0, ""),
        ("tag kwd empty", {"tag_kwd": [""]}, 0, ""),
        ("tag kwd int in list", {"tag_kwd": [1]}, 102, "`tag_kwd` must be a list of strings"),
        ("tag kwd dup", {"tag_kwd": ["tag", "tag"]}, 0, ""),
        ("tag kwd str", {"tag_kwd": "tag"}, 102, "`tag_kwd` should be a list"),
        ("tag kwd number", {"tag_kwd": 123}, 102, "`tag_kwd` should be a list"),
    ]
    for scenario_name, payload, expected_code, expected_message in cases:
        res = rest_client.patch(f"{base_path}/{chunk_id}", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_code != 0:
            assert expected_message in body["message"], (scenario_name, body)


@pytest.mark.p2
def test_chunk_update_invalid_target_and_param_contract(rest_client, create_document):
    dataset_id, document_id, chunk_id, base_path = _create_chunk_for_update(rest_client, create_document, "chunk_update_invalid_targets.txt")

    invalid_dataset_res = rest_client.patch(
        f"/datasets/{INVALID_ID_32}/documents/{document_id}/chunks/{chunk_id}",
        json={"content": "updated"},
    )
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] == 102, invalid_dataset_payload
    assert invalid_dataset_payload["message"] in {
        f"You don't own the dataset {INVALID_ID_32}.",
        f"Can't find this chunk {chunk_id}",
    }, invalid_dataset_payload

    invalid_document_res = rest_client.patch(
        f"/datasets/{dataset_id}/documents/{INVALID_ID_32}/chunks/{chunk_id}",
        json={"content": "updated"},
    )
    assert invalid_document_res.status_code == 200
    invalid_document_payload = invalid_document_res.json()
    assert invalid_document_payload["code"] == 102, invalid_document_payload
    assert invalid_document_payload["message"] == f"You don't own the document {INVALID_ID_32}.", invalid_document_payload

    invalid_chunk_res = rest_client.patch(
        f"{base_path}/{INVALID_ID_32}",
        json={"content": "updated"},
    )
    assert invalid_chunk_res.status_code == 200
    invalid_chunk_payload = invalid_chunk_res.json()
    assert invalid_chunk_payload["code"] == 102, invalid_chunk_payload
    assert invalid_chunk_payload["message"] == f"Can't find this chunk {INVALID_ID_32}", invalid_chunk_payload

    for scenario_name, payload in (
        ("unknown key", {"unknown_key": "unknown_value"}),
        ("empty payload", {}),
    ):
        res = rest_client.patch(f"{base_path}/{chunk_id}", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == 0, (scenario_name, body)


@pytest.mark.p2
def test_chunk_update_repeated_concurrent_and_deleted_document_contract(rest_client, create_document):
    dataset_id, document_id, chunk_id, base_path = _create_chunk_for_update(
        rest_client, create_document, "chunk_update_repeated_concurrent_deleted.txt"
    )

    first_res = rest_client.patch(f"{base_path}/{chunk_id}", json={"content": "chunk test 1"})
    assert first_res.status_code == 200
    assert first_res.json()["code"] == 0

    second_res = rest_client.patch(f"{base_path}/{chunk_id}", json={"content": "chunk test 2"})
    assert second_res.status_code == 200
    assert second_res.json()["code"] == 0

    get_after_repeat = rest_client.get(f"{base_path}/{chunk_id}")
    assert get_after_repeat.status_code == 200
    get_after_repeat_payload = get_after_repeat.json()
    assert get_after_repeat_payload["code"] == 0, get_after_repeat_payload
    assert get_after_repeat_payload["data"]["content_with_weight"] == "chunk test 2", get_after_repeat_payload

    chunk_ids = [chunk_id]
    for index in range(3):
        add_res = rest_client.post(base_path, json={"content": f"concurrent update {index}"})
        assert add_res.status_code == 200, add_res.text
        add_payload = add_res.json()
        assert add_payload["code"] == 0, add_payload
        chunk_ids.append(add_payload["data"]["chunk"]["id"])

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = []
        for index in range(20):
            target_id = chunk_ids[index % len(chunk_ids)]
            futures.append(
                executor.submit(
                    lambda cid, i: rest_client.patch(
                        f"{base_path}/{cid}",
                        json={"content": f"update chunk test {i}"},
                    ).json(),
                    target_id,
                    index,
                )
            )
        results = [future.result() for future in futures]
    assert len(results) == 20, results
    assert all(item["code"] == 0 for item in results), results

    delete_document_res = rest_client.delete(f"/datasets/{dataset_id}/documents", json={"ids": [document_id]})
    assert delete_document_res.status_code == 200
    assert delete_document_res.json()["code"] == 0

    update_after_delete = rest_client.patch(f"{base_path}/{chunk_id}", json={"content": "after delete"})
    assert update_after_delete.status_code == 200
    update_after_delete_payload = update_after_delete.json()
    assert update_after_delete_payload["code"] == 102, update_after_delete_payload
    assert update_after_delete_payload["message"] in {
        f"You don't own the document {document_id}.",
        f"Can't find this chunk {chunk_id}",
    }, update_after_delete_payload
