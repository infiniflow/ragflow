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
from concurrent.futures import ThreadPoolExecutor, as_completed
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
def test_dataset_create_name_and_case_insensitive_contract(rest_client, clear_datasets):
    name = "CaseInsensitive"

    first_res = rest_client.post("/datasets", json={"name": name.upper()})
    assert first_res.status_code == 200
    first_payload = first_res.json()
    assert first_payload["code"] == 0, first_payload
    assert first_payload["data"]["name"] == name.upper(), first_payload

    second_res = rest_client.post("/datasets", json={"name": name.lower()})
    assert second_res.status_code == 200
    second_payload = second_res.json()
    assert second_payload["code"] == 0, second_payload
    assert second_payload["data"]["name"] == f"{name.lower()}(1)", second_payload


@pytest.mark.p2
def test_dataset_create_avatar_and_description_contract(rest_client, clear_datasets):
    avatar = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/w8AAgMBgN6J1tQAAAAASUVORK5CYII="
    payload = {
        "name": "dataset_avatar_description",
        "avatar": avatar,
        "description": "description",
    }
    res = rest_client.post("/datasets", json=payload)
    assert res.status_code == 200
    body = res.json()
    assert body["code"] == 0, body
    assert body["data"]["avatar"] == avatar, body
    assert body["data"]["description"] == "description", body


@pytest.mark.p2
@pytest.mark.parametrize(
    "name, chunk_method",
    [
        ("naive", "naive"),
        ("book", "book"),
        ("email", "email"),
        ("laws", "laws"),
        ("manual", "manual"),
        ("one", "one"),
        ("paper", "paper"),
        ("picture", "picture"),
        ("presentation", "presentation"),
        ("qa", "qa"),
        ("table", "table"),
        ("tag", "tag"),
    ],
    ids=["naive", "book", "email", "laws", "manual", "one", "paper", "picture", "presentation", "qa", "table", "tag"],
)
def test_dataset_create_chunk_method_contract(rest_client, clear_datasets, name, chunk_method):
    res = rest_client.post("/datasets", json={"name": name, "chunk_method": chunk_method})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert payload["data"]["chunk_method"] == chunk_method, payload


@pytest.mark.p2
@pytest.mark.parametrize(
    "name, permission",
    [
        ("me", "me"),
        ("team", "team"),
    ],
    ids=["me", "team"],
)
def test_dataset_create_permission_contract(rest_client, clear_datasets, name, permission):
    res = rest_client.post("/datasets", json={"name": name, "permission": permission})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert payload["data"]["permission"] == permission, payload


@pytest.mark.p2
@pytest.mark.parametrize(
    "name, embedding_model",
    [
        ("builtin_baai", "BAAI/bge-small-en-v1.5@Builtin"),
        ("tenant_zhipu", "embedding-3@ZHIPU-AI"),
    ],
    ids=["builtin_baai", "tenant_zhipu"],
)
def test_dataset_create_embedding_model_contract(rest_client, clear_datasets, name, embedding_model):
    res = rest_client.post("/datasets", json={"name": name, "embedding_model": embedding_model})
    assert res.status_code == 200
    payload = res.json()
    if embedding_model == "embedding-3@ZHIPU-AI" and payload["code"] == 102:
        pytest.xfail(f"Environment has no authorized tenant model for {embedding_model}: {payload}")
    assert payload["code"] == 0, payload
    assert payload["data"]["embedding_model"] == embedding_model, payload


@pytest.mark.p2
@pytest.mark.parametrize(
    "name, embedding_model, expected_fragment",
    [
        ("empty", "", "Embedding model identifier must follow <model_name>@<provider> format"),
        ("space", " ", "Embedding model identifier must follow <model_name>@<provider> format"),
        ("missing_at", "BAAI/bge-small-en-v1.5Builtin", "Embedding model identifier must follow <model_name>@<provider> format"),
        ("missing_model_name", "@Builtin", "Both model_name and provider must be non-empty strings"),
        ("missing_provider", "BAAI/bge-small-en-v1.5@", "Both model_name and provider must be non-empty strings"),
        ("whitespace_only_model_name", " @Builtin", "Both model_name and provider must be non-empty strings"),
        ("whitespace_only_provider", "BAAI/bge-small-env1.5@ ", "Both model_name and provider must be non-empty strings"),
    ],
    ids=["empty", "space", "missing_at", "empty_model_name", "empty_provider", "whitespace_only_model_name", "whitespace_only_provider"],
)
def test_dataset_create_embedding_model_format_contract(rest_client, clear_datasets, name, embedding_model, expected_fragment):
    res = rest_client.post("/datasets", json={"name": name, "embedding_model": embedding_model})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload
    assert expected_fragment in payload["message"], payload


@pytest.mark.p2
def test_dataset_create_parser_config_missing_raptor_and_graphrag(rest_client, clear_datasets):
    payload = {
        "name": "test_parser_config_missing_fields",
        "parser_config": {"chunk_token_num": 1024},
    }
    res = rest_client.post("/datasets", json=payload)
    assert res.status_code == 200
    body = res.json()
    assert body["code"] == 0, body
    parser_config = body["data"]["parser_config"]
    assert "raptor" in parser_config, body
    assert "graphrag" in parser_config, body
    assert parser_config["raptor"]["use_raptor"] is False, body
    assert parser_config["graphrag"]["use_graphrag"] is False, body
    assert parser_config["chunk_token_num"] == 1024, body


@pytest.mark.p3
def test_dataset_create_1k_contract(rest_client, clear_datasets):
    for i in range(1_000):
        res = rest_client.post("/datasets", json={"name": f"dataset_{i}"})
        assert res.status_code == 200, (i, res.text)
        payload = res.json()
        assert payload["code"] == 0, (i, payload)


@pytest.mark.p3
def test_dataset_create_concurrent_contract(rest_client, clear_datasets):
    count = 100
    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(rest_client.post, "/datasets", json={"name": f"dataset_{i}"}) for i in range(count)]
        responses = list(as_completed(futures))
    assert len(responses) == count, responses
    for index, future in enumerate(futures):
        res = future.result()
        assert res.status_code == 200, (index, res.text)
        payload = res.json()
        assert payload["code"] == 0, (index, payload)


@pytest.mark.p2
@pytest.mark.parametrize(
    "name, parser_config",
    [
        ("auto_keywords_min", {"auto_keywords": 0}),
        ("auto_keywords_mid", {"auto_keywords": 16}),
        ("auto_keywords_max", {"auto_keywords": 32}),
        ("auto_questions_min", {"auto_questions": 0}),
        ("auto_questions_mid", {"auto_questions": 5}),
        ("auto_questions_max", {"auto_questions": 10}),
        ("chunk_token_num_min", {"chunk_token_num": 1}),
        ("chunk_token_num_mid", {"chunk_token_num": 1024}),
        ("chunk_token_num_max", {"chunk_token_num": 2048}),
        ("delimiter", {"delimiter": "\n"}),
        ("delimiter_space", {"delimiter": " "}),
        ("html4excel_true", {"html4excel": True}),
        ("html4excel_false", {"html4excel": False}),
        ("layout_recognize_DeepDOC", {"layout_recognize": "DeepDOC"}),
        ("layout_recognize_navie", {"layout_recognize": "Plain Text"}),
        ("tag_kb_ids", {"tag_kb_ids": ["1", "2"]}),
        ("topn_tags_min", {"topn_tags": 1}),
        ("topn_tags_mid", {"topn_tags": 5}),
        ("topn_tags_max", {"topn_tags": 10}),
        ("filename_embd_weight_min", {"filename_embd_weight": 0.1}),
        ("filename_embd_weight_mid", {"filename_embd_weight": 0.5}),
        ("filename_embd_weight_max", {"filename_embd_weight": 1.0}),
        ("task_page_size_min", {"task_page_size": 1}),
        ("task_page_size_None", {"task_page_size": None}),
        ("pages", {"pages": [[1, 100]]}),
        ("pages_none", {"pages": None}),
        ("graphrag_true", {"graphrag": {"use_graphrag": True}}),
        ("graphrag_false", {"graphrag": {"use_graphrag": False}}),
        ("graphrag_entity_types", {"graphrag": {"entity_types": ["age", "sex", "height", "weight"]}}),
        ("graphrag_method_general", {"graphrag": {"method": "general"}}),
        ("graphrag_method_light", {"graphrag": {"method": "light"}}),
        ("graphrag_community_true", {"graphrag": {"community": True}}),
        ("graphrag_community_false", {"graphrag": {"community": False}}),
        ("graphrag_resolution_true", {"graphrag": {"resolution": True}}),
        ("graphrag_resolution_false", {"graphrag": {"resolution": False}}),
        ("raptor_true", {"raptor": {"use_raptor": True}}),
        ("raptor_false", {"raptor": {"use_raptor": False}}),
        ("raptor_prompt", {"raptor": {"prompt": "Who are you?"}}),
        ("raptor_max_token_min", {"raptor": {"max_token": 1}}),
        ("raptor_max_token_mid", {"raptor": {"max_token": 1024}}),
        ("raptor_max_token_max", {"raptor": {"max_token": 2048}}),
        ("raptor_threshold_min", {"raptor": {"threshold": 0.0}}),
        ("raptor_threshold_mid", {"raptor": {"threshold": 0.5}}),
        ("raptor_threshold_max", {"raptor": {"threshold": 1.0}}),
        ("raptor_max_cluster_min", {"raptor": {"max_cluster": 1}}),
        ("raptor_max_cluster_mid", {"raptor": {"max_cluster": 512}}),
        ("raptor_max_cluster_max", {"raptor": {"max_cluster": 1024}}),
        ("raptor_random_seed_min", {"raptor": {"random_seed": 0}}),
        ("parent_child_true", {"parent_child": {"use_parent_child": True}}),
        ("parent_child_false", {"parent_child": {"use_parent_child": False}}),
        ("parent_child_delimiter", {"parent_child": {"children_delimiter": "\n\n"}}),
        ("parent_child_delimiter_custom", {"parent_child": {"use_parent_child": True, "children_delimiter": "。"}}),
    ],
    ids=[
        "auto_keywords_min",
        "auto_keywords_mid",
        "auto_keywords_max",
        "auto_questions_min",
        "auto_questions_mid",
        "auto_questions_max",
        "chunk_token_num_min",
        "chunk_token_num_mid",
        "chunk_token_num_max",
        "delimiter",
        "delimiter_space",
        "html4excel_true",
        "html4excel_false",
        "layout_recognize_DeepDOC",
        "layout_recognize_navie",
        "tag_kb_ids",
        "topn_tags_min",
        "topn_tags_mid",
        "topn_tags_max",
        "filename_embd_weight_min",
        "filename_embd_weight_mid",
        "filename_embd_weight_max",
        "task_page_size_min",
        "task_page_size_None",
        "pages",
        "pages_none",
        "graphrag_true",
        "graphrag_false",
        "graphrag_entity_types",
        "graphrag_method_general",
        "graphrag_method_light",
        "graphrag_community_true",
        "graphrag_community_false",
        "graphrag_resolution_true",
        "graphrag_resolution_false",
        "raptor_true",
        "raptor_false",
        "raptor_prompt",
        "raptor_max_token_min",
        "raptor_max_token_mid",
        "raptor_max_token_max",
        "raptor_threshold_min",
        "raptor_threshold_mid",
        "raptor_threshold_max",
        "raptor_max_cluster_min",
        "raptor_max_cluster_mid",
        "raptor_max_cluster_max",
        "raptor_random_seed_min",
        "parent_child_true",
        "parent_child_false",
        "parent_child_delimiter",
        "parent_child_delimiter_custom",
    ],
)
def test_dataset_create_parser_config_valid_matrix_contract(rest_client, clear_datasets, name, parser_config):
    payload = {"name": name, "parser_config": parser_config}
    res = rest_client.post("/datasets", json=payload)
    assert res.status_code == 200
    body = res.json()
    assert body["code"] == 0, body
    actual_parser_config = body["data"]["parser_config"]
    for key, expected_value in parser_config.items():
        if isinstance(expected_value, dict):
            for nested_key, nested_expected in expected_value.items():
                assert actual_parser_config[key][nested_key] == nested_expected, body
        else:
            assert actual_parser_config[key] == expected_value, body


@pytest.mark.p1
@pytest.mark.parametrize(
    "name, parser_config, expected_raptor, expected_graphrag",
    [
        ("test_parser_config_only_raptor", {"chunk_token_num": 1024, "raptor": {"use_raptor": True}}, True, False),
        ("test_parser_config_only_graphrag", {"chunk_token_num": 1024, "graphrag": {"use_graphrag": True}}, False, True),
        (
            "test_parser_config_both_fields",
            {"chunk_token_num": 1024, "raptor": {"use_raptor": True}, "graphrag": {"use_graphrag": True}},
            True,
            True,
        ),
    ],
    ids=["only_raptor", "only_graphrag", "both_fields"],
)
def test_dataset_create_parser_config_bugfix_contract(
    rest_client, clear_datasets, name, parser_config, expected_raptor, expected_graphrag
):
    res = rest_client.post("/datasets", json={"name": name, "parser_config": parser_config})
    assert res.status_code == 200
    body = res.json()
    assert body["code"] == 0, body
    actual_parser_config = body["data"]["parser_config"]
    assert "raptor" in actual_parser_config, body
    assert "graphrag" in actual_parser_config, body
    assert actual_parser_config["raptor"]["use_raptor"] is expected_raptor, body
    assert actual_parser_config["graphrag"]["use_graphrag"] is expected_graphrag, body


@pytest.mark.p2
@pytest.mark.parametrize(
    "chunk_method",
    ["qa", "manual", "paper", "book", "laws", "presentation"],
    ids=["qa", "manual", "paper", "book", "laws", "presentation"],
)
def test_dataset_create_parser_config_different_chunk_methods_contract(rest_client, clear_datasets, chunk_method):
    payload = {
        "name": f"test_parser_config_{chunk_method}",
        "chunk_method": chunk_method,
        "parser_config": {"chunk_token_num": 512},
    }
    res = rest_client.post("/datasets", json=payload)
    assert res.status_code == 200
    body = res.json()
    assert body["code"] == 0, body
    parser_config = body["data"]["parser_config"]
    assert parser_config["chunk_token_num"] == 512, body
    assert "raptor" in parser_config, body
    assert "graphrag" in parser_config, body
    assert parser_config["raptor"]["use_raptor"] is False, body
    assert parser_config["graphrag"]["use_graphrag"] is False, body


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
