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
from configs import DATASET_NAME_LIMIT


@pytest.mark.p1
class TestDatasetsAuthorization:
    def test_create_requires_auth(self, rest_client_noauth):
        res = rest_client_noauth.post("/datasets", json={"name": "auth_test"})
        assert res.status_code == 401
        payload = res.json()
        assert payload["code"] == 401, payload


@pytest.mark.p1
def test_dataset_crud_cycle(rest_client, clear_datasets):
    create_res = rest_client.post("/datasets", json={"name": "restful_dataset_crud"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    get_res = rest_client.get(f"/datasets/{dataset_id}")
    assert get_res.status_code == 200
    get_payload = get_res.json()
    assert get_payload["code"] == 0, get_payload
    assert get_payload["data"]["id"] == dataset_id, get_payload

    update_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"name": "restful_dataset_crud_updated"},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload
    assert update_payload["data"]["name"] == "restful_dataset_crud_updated", update_payload

    list_res = rest_client.get("/datasets", params={"id": dataset_id})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert len(list_payload["data"]) == 1, list_payload
    assert list_payload["data"][0]["id"] == dataset_id, list_payload
    assert list_payload.get("total_datasets", 0) >= 1, list_payload

    delete_res = rest_client.delete("/datasets", json={"ids": [dataset_id]})
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 0, delete_payload

    list_after_delete = rest_client.get("/datasets")
    assert list_after_delete.status_code == 200
    list_after_delete_payload = list_after_delete.json()
    assert list_after_delete_payload["code"] == 0, list_after_delete_payload
    assert all(dataset["id"] != dataset_id for dataset in list_after_delete_payload["data"]), list_after_delete_payload


@pytest.mark.p2
@pytest.mark.parametrize(
    "name, expected_fragment",
    [
        ("", "String should have at least 1 character"),
        (" ", "String should have at least 1 character"),
        ("a" * (DATASET_NAME_LIMIT + 1), f"String should have at most {DATASET_NAME_LIMIT} characters"),
    ],
    ids=["empty", "spaces", "too_long"],
)
def test_dataset_create_name_validation(rest_client, clear_datasets, name, expected_fragment):
    res = rest_client.post("/datasets", json={"name": name})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload
    assert expected_fragment in payload["message"], payload


@pytest.mark.p2
def test_dataset_list_ordering_and_pagination(rest_client, clear_datasets):
    for i in range(3):
        res = rest_client.post("/datasets", json={"name": f"dataset_page_{i}"})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 0, payload

    list_res = rest_client.get(
        "/datasets",
        params={"page": 1, "page_size": 2, "orderby": "create_time", "desc": "true"},
    )
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert len(list_payload["data"]) == 2, list_payload
    assert list_payload.get("total_datasets", 0) >= 3, list_payload


@pytest.mark.p2
def test_dataset_search_endpoint(rest_client, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    res = rest_client.post(
        f"/datasets/{dataset_id}/search",
        json={"question": "test TXT file", "page": 1, "size": 10},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert "chunks" in payload["data"], payload


@pytest.mark.p2
def test_dataset_search_requires_question(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_search_missing_question")
    res = rest_client.post(f"/datasets/{dataset_id}/search", json={})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload
    assert "question" in payload["message"], payload


@pytest.mark.p2
def test_dataset_tags_and_aggregation(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_tags")
    second_dataset_id = create_dataset("dataset_tags_second")

    list_tags_res = rest_client.get(f"/datasets/{dataset_id}/tags")
    assert list_tags_res.status_code == 200
    list_tags_payload = list_tags_res.json()
    # Known env/runtime behavior: this route can return 102 when retriever tag
    # backend is unavailable for an empty dataset. Keep route-contract coverage.
    assert list_tags_payload["code"] in (0, 102), list_tags_payload

    aggregate_res = rest_client.get(
        "/datasets/tags/aggregation",
        params={"dataset_ids": f"{dataset_id},{second_dataset_id}"},
    )
    assert aggregate_res.status_code == 200
    aggregate_payload = aggregate_res.json()
    assert aggregate_payload["code"] in (0, 102), aggregate_payload

    empty_aggregate_res = rest_client.get("/datasets/tags/aggregation")
    assert empty_aggregate_res.status_code == 200
    empty_aggregate_payload = empty_aggregate_res.json()
    assert empty_aggregate_payload["code"] != 0, empty_aggregate_payload


@pytest.mark.p2
def test_dataset_tags_delete_and_rename_validation(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_tag_mutation")

    delete_missing_tags = rest_client.delete(f"/datasets/{dataset_id}/tags", json={})
    assert delete_missing_tags.status_code == 200
    delete_missing_tags_payload = delete_missing_tags.json()
    assert delete_missing_tags_payload["code"] != 0, delete_missing_tags_payload

    delete_invalid_tags_type = rest_client.delete(f"/datasets/{dataset_id}/tags", json={"tags": "wrong"})
    assert delete_invalid_tags_type.status_code == 200
    delete_invalid_tags_type_payload = delete_invalid_tags_type.json()
    assert delete_invalid_tags_type_payload["code"] != 0, delete_invalid_tags_type_payload

    rename_empty = rest_client.put(
        f"/datasets/{dataset_id}/tags",
        json={"from_tag": "", "to_tag": ""},
    )
    assert rename_empty.status_code == 200
    rename_empty_payload = rename_empty.json()
    assert rename_empty_payload["code"] != 0, rename_empty_payload

    rename_invalid_dataset = rest_client.put(
        "/datasets/invalid_id/tags",
        json={"from_tag": "old", "to_tag": "new"},
    )
    assert rename_invalid_dataset.status_code == 200
    rename_invalid_dataset_payload = rename_invalid_dataset.json()
    assert rename_invalid_dataset_payload["code"] != 0, rename_invalid_dataset_payload


@pytest.mark.p2
def test_dataset_flattened_metadata(rest_client, create_dataset):
    first_dataset_id = create_dataset("flattened_meta_1")
    second_dataset_id = create_dataset("flattened_meta_2")

    flattened_res = rest_client.get(
        "/datasets/metadata/flattened",
        params={"dataset_ids": f"{first_dataset_id},{second_dataset_id}"},
    )
    assert flattened_res.status_code == 200
    flattened_payload = flattened_res.json()
    assert flattened_payload["code"] == 0, flattened_payload

    empty_ids_res = rest_client.get("/datasets/metadata/flattened")
    assert empty_ids_res.status_code == 200
    empty_ids_payload = empty_ids_res.json()
    assert empty_ids_payload["code"] != 0, empty_ids_payload

    invalid_dataset_res = rest_client.get(
        "/datasets/metadata/flattened",
        params={"dataset_ids": "invalid_id"},
    )
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] != 0, invalid_dataset_payload


@pytest.mark.p2
def test_dataset_ingestion_summary_and_logs(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_ingestions")

    summary_res = rest_client.get(f"/datasets/{dataset_id}/ingestions/summary")
    assert summary_res.status_code == 200
    summary_payload = summary_res.json()
    assert summary_payload["code"] == 0, summary_payload
    assert "doc_num" in summary_payload["data"], summary_payload
    assert "chunk_num" in summary_payload["data"], summary_payload
    assert "token_num" in summary_payload["data"], summary_payload
    assert "status" in summary_payload["data"], summary_payload

    logs_res = rest_client.get(
        f"/datasets/{dataset_id}/ingestions",
        params={"page": 1, "page_size": 10},
    )
    assert logs_res.status_code == 200
    logs_payload = logs_res.json()
    assert logs_payload["code"] == 0, logs_payload
    assert "total" in logs_payload["data"], logs_payload
    assert "logs" in logs_payload["data"], logs_payload

    not_found_log_res = rest_client.get(f"/datasets/{dataset_id}/ingestions/nonexistent_log")
    assert not_found_log_res.status_code == 200
    not_found_log_payload = not_found_log_res.json()
    assert not_found_log_payload["code"] != 0, not_found_log_payload


@pytest.mark.p2
def test_dataset_ingestion_invalid_dataset(rest_client):
    summary_res = rest_client.get("/datasets/invalid_id/ingestions/summary")
    assert summary_res.status_code == 200
    summary_payload = summary_res.json()
    assert summary_payload["code"] != 0, summary_payload

    logs_res = rest_client.get("/datasets/invalid_id/ingestions")
    assert logs_res.status_code == 200
    logs_payload = logs_res.json()
    assert logs_payload["code"] != 0, logs_payload

    log_res = rest_client.get("/datasets/invalid_id/ingestions/some_log_id")
    assert log_res.status_code == 200
    log_payload = log_res.json()
    assert log_payload["code"] != 0, log_payload


@pytest.mark.p2
def test_dataset_index_endpoints(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_index_endpoints")

    run_invalid_type = rest_client.post(
        f"/datasets/{dataset_id}/index",
        params={"type": "invalid_type"},
    )
    assert run_invalid_type.status_code == 200
    run_invalid_type_payload = run_invalid_type.json()
    assert run_invalid_type_payload["code"] != 0, run_invalid_type_payload

    run_no_docs = rest_client.post(
        f"/datasets/{dataset_id}/index",
        params={"type": "graph"},
    )
    assert run_no_docs.status_code == 200
    run_no_docs_payload = run_no_docs.json()
    assert run_no_docs_payload["code"] == 102, run_no_docs_payload

    trace_no_task = rest_client.get(
        f"/datasets/{dataset_id}/index",
        params={"type": "graph"},
    )
    assert trace_no_task.status_code == 200
    trace_no_task_payload = trace_no_task.json()
    assert trace_no_task_payload["code"] == 0, trace_no_task_payload
    assert trace_no_task_payload["data"] == {}, trace_no_task_payload

    delete_graph = rest_client.delete(f"/datasets/{dataset_id}/graph")
    assert delete_graph.status_code == 200
    delete_graph_payload = delete_graph.json()
    assert delete_graph_payload["code"] == 0, delete_graph_payload

    delete_invalid_type = rest_client.delete(f"/datasets/{dataset_id}/invalid_type")
    assert delete_invalid_type.status_code == 200
    delete_invalid_type_payload = delete_invalid_type.json()
    assert delete_invalid_type_payload["code"] != 0, delete_invalid_type_payload


@pytest.mark.p2
@pytest.mark.parametrize("index_type", ["graph", "raptor", "mindmap"])
def test_dataset_index_run_with_document_creates_task(rest_client, create_document, index_type):
    dataset_id, _ = create_document("dataset_index_graph_source.txt")
    run_graph = rest_client.post(
        f"/datasets/{dataset_id}/index",
        params={"type": index_type},
    )
    assert run_graph.status_code == 200
    run_graph_payload = run_graph.json()
    assert run_graph_payload["code"] == 0, run_graph_payload
    assert run_graph_payload["data"].get("task_id"), run_graph_payload


@pytest.mark.p2
def test_dataset_embedding_endpoints(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_embedding_endpoints")

    run_no_docs_res = rest_client.post(f"/datasets/{dataset_id}/embedding")
    assert run_no_docs_res.status_code == 200
    run_no_docs_payload = run_no_docs_res.json()
    assert run_no_docs_payload["code"] == 102, run_no_docs_payload

    missing_embd_id_res = rest_client.post(f"/datasets/{dataset_id}/embedding/check", json={})
    assert missing_embd_id_res.status_code == 200
    missing_embd_id_payload = missing_embd_id_res.json()
    assert missing_embd_id_payload["code"] != 0, missing_embd_id_payload

    invalid_dataset_res = rest_client.post("/datasets/invalid_id/embedding")
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] != 0, invalid_dataset_payload
