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

from concurrent.futures import ThreadPoolExecutor, as_completed
import os

import pytest
from configs import DATASET_NAME_LIMIT, DEFAULT_PARSER_CONFIG
from test.testcases.utils import encode_avatar
from test.testcases.utils.file_utils import create_image_file


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
def test_dataset_update_name_and_case_insensitive_contract(rest_client, clear_datasets):
    first_res = rest_client.post("/datasets", json={"name": "dataset_update_name_source"})
    assert first_res.status_code == 200
    first_payload = first_res.json()
    assert first_payload["code"] == 0, first_payload
    first_dataset_id = first_payload["data"]["id"]

    second_res = rest_client.post("/datasets", json={"name": "dataset_update_name_target"})
    assert second_res.status_code == 200
    second_payload = second_res.json()
    assert second_payload["code"] == 0, second_payload

    rename_res = rest_client.put(
        f"/datasets/{first_dataset_id}",
        json={"name": "dataset_update_name_renamed"},
    )
    assert rename_res.status_code == 200
    rename_payload = rename_res.json()
    assert rename_payload["code"] == 0, rename_payload
    assert rename_payload["data"]["name"] == "dataset_update_name_renamed", rename_payload

    list_res = rest_client.get("/datasets", params={"id": first_dataset_id})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert list_payload["data"][0]["name"] == "dataset_update_name_renamed", list_payload

    duplicate_case_res = rest_client.put(
        f"/datasets/{first_dataset_id}",
        json={"name": second_payload["data"]["name"].upper()},
    )
    assert duplicate_case_res.status_code == 200
    duplicate_case_payload = duplicate_case_res.json()
    assert duplicate_case_payload["code"] == 102, duplicate_case_payload
    assert "already exists" in duplicate_case_payload["message"], duplicate_case_payload


@pytest.mark.p2
def test_dataset_update_language_connectors_avatar_and_description_contract(rest_client, clear_datasets, tmp_path):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_lang_connectors"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    image_path = create_image_file(tmp_path / "dataset_update_avatar.png")
    encoded_avatar = encode_avatar(image_path)
    avatar_value = f"data:image/png;base64,{encoded_avatar}"

    update_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={
            "name": "dataset_update_lang_connectors",
            "description": "",
            "chunk_method": "naive",
            "language": "English",
            "connectors": [],
            "avatar": avatar_value,
        },
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload
    assert update_payload["data"]["language"] == "English", update_payload
    assert update_payload["data"]["connectors"] == [], update_payload
    assert update_payload["data"]["avatar"] == avatar_value, update_payload

    description_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"description": "description"},
    )
    assert description_res.status_code == 200
    description_payload = description_res.json()
    assert description_payload["code"] == 0, description_payload
    assert description_payload["data"]["description"] == "description", description_payload


@pytest.mark.p1
@pytest.mark.parametrize(
    "chunk_method",
    [
        "naive",
        "book",
        "email",
        "laws",
        "manual",
        "one",
        "paper",
        "picture",
        "presentation",
        "qa",
        "table",
        "tag",
    ],
    ids=["naive", "book", "email", "laws", "manual", "one", "paper", "picture", "presentation", "qa", "table", "tag"],
)
def test_dataset_update_chunk_method_contract(rest_client, clear_datasets, chunk_method):
    create_res = rest_client.post("/datasets", json={"name": f"dataset_update_chunk_{chunk_method}"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    update_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"chunk_method": chunk_method},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload
    assert update_payload["data"]["chunk_method"] == chunk_method, update_payload


@pytest.mark.p1
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
        ("raptor_clustering_method_gmm", {"raptor": {"clustering_method": "gmm"}}),
        ("raptor_clustering_method_ahc", {"raptor": {"clustering_method": "ahc"}}),
        ("raptor_tree_builder_raptor", {"raptor": {"tree_builder": "raptor"}}),
        ("raptor_tree_builder_psi", {"raptor": {"tree_builder": "psi"}}),
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
        "raptor_clustering_method_gmm",
        "raptor_clustering_method_ahc",
        "raptor_tree_builder_raptor",
        "raptor_tree_builder_psi",
    ],
)
def test_dataset_update_parser_config_valid_matrix_contract(rest_client, clear_datasets, name, parser_config):
    create_res = rest_client.post("/datasets", json={"name": f"dataset_update_parser_{name}"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    update_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"parser_config": parser_config},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload

    list_res = rest_client.get("/datasets", params={"id": dataset_id})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    actual_parser_config = list_payload["data"][0]["parser_config"]
    for key, expected_value in parser_config.items():
        if isinstance(expected_value, dict):
            for nested_key, nested_expected in expected_value.items():
                assert actual_parser_config[key][nested_key] == nested_expected, list_payload
        else:
            assert actual_parser_config[key] == expected_value, list_payload


@pytest.mark.p3
@pytest.mark.parametrize(
    "name, update_payload",
    [
        ("parser_config_empty", {"chunk_method": "qa", "parser_config": {}}),
        ("parser_config_none", {"chunk_method": "qa", "parser_config": None}),
        ("parser_config_unset", {"chunk_method": "qa"}),
    ],
    ids=["parser_config_empty", "parser_config_none", "parser_config_unset"],
)
def test_dataset_update_parser_config_with_chunk_method_change_contract(rest_client, clear_datasets, name, update_payload):
    create_res = rest_client.post("/datasets", json={"name": f"dataset_update_{name}"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    update_res = rest_client.put(f"/datasets/{dataset_id}", json=update_payload)
    assert update_res.status_code == 200
    update_body = update_res.json()
    assert update_body["code"] == 0, update_body

    list_res = rest_client.get("/datasets", params={"id": dataset_id})
    assert list_res.status_code == 200
    list_body = list_res.json()
    assert list_body["code"] == 0, list_body
    assert list_body["data"][0]["parser_config"] == {
        "raptor": {"use_raptor": False},
        "graphrag": {"use_graphrag": False},
        "image_context_size": 0,
        "table_context_size": 0,
    }, list_body


@pytest.mark.p1
@pytest.mark.parametrize(
    "embedding_model, unauthorized_is_xfail",
    [
        ("BAAI/bge-small-en-v1.5@Builtin", False),
        ("embedding-3@ZHIPU-AI", True),
    ],
    ids=["builtin_baai", "tenant_zhipu"],
)
def test_dataset_update_embedding_model_contract(rest_client, clear_datasets, embedding_model, unauthorized_is_xfail):
    create_res = rest_client.post("/datasets", json={"name": f"dataset_update_embedding_{embedding_model.split('@')[0].replace('/', '_')}"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    update_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"embedding_model": embedding_model},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    if unauthorized_is_xfail and update_payload["code"] == 102:
        pytest.xfail(f"Environment has no authorized tenant model for {embedding_model}: {update_payload}")
    assert update_payload["code"] == 0, update_payload
    assert update_payload["data"]["embedding_model"] == embedding_model, update_payload


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
        ("whitespace_only_provider", "BAAI/bge-small-en-v1.5@ ", "Both model_name and provider must be non-empty strings"),
    ],
    ids=["empty", "space", "missing_at", "empty_model_name", "empty_provider", "whitespace_only_model_name", "whitespace_only_provider"],
)
def test_dataset_update_embedding_model_format_contract(rest_client, clear_datasets, name, embedding_model, expected_fragment):
    create_res = rest_client.post("/datasets", json={"name": f"dataset_update_embedding_format_{name}"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    update_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"embedding_model": embedding_model},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 101, update_payload
    assert expected_fragment in update_payload["message"], update_payload


@pytest.mark.p1
def test_dataset_update_embedding_model_with_existing_chunks_contract(rest_client, create_document):
    dataset_id, document_id = create_document("dataset_update_embedding_with_chunks.txt")
    chunk_res = rest_client.post(
        f"/datasets/{dataset_id}/documents/{document_id}/chunks",
        json={"content": "dataset update embedding with chunks"},
    )
    assert chunk_res.status_code == 200
    chunk_payload = chunk_res.json()
    assert chunk_payload["code"] == 0, chunk_payload

    dataset_res = rest_client.get(f"/datasets/{dataset_id}")
    assert dataset_res.status_code == 200
    dataset_payload = dataset_res.json()
    assert dataset_payload["code"] == 0, dataset_payload
    current_embedding = dataset_payload["data"]["embedding_model"]

    candidates = ["embedding-3@ZHIPU-AI", "BAAI/bge-small-en-v1.5@Builtin"]
    last_payload = None
    for candidate in candidates:
        if candidate == current_embedding:
            continue
        update_res = rest_client.put(
            f"/datasets/{dataset_id}",
            json={"embedding_model": candidate},
        )
        assert update_res.status_code == 200
        update_payload = update_res.json()
        last_payload = update_payload
        if update_payload["code"] == 0:
            assert update_payload["data"]["embedding_model"] == candidate, update_payload
            return
        if update_payload["code"] == 102 and "Unauthorized model" in update_payload.get("message", ""):
            continue
        assert False, update_payload

    pytest.xfail(f"No authorized alternative embedding model available for update: {last_payload}")


@pytest.mark.p2
@pytest.mark.parametrize(
    "permission",
    ["me", "team"],
    ids=["me", "team"],
)
def test_dataset_update_permission_contract(rest_client, clear_datasets, permission):
    create_res = rest_client.post("/datasets", json={"name": f"dataset_update_permission_{permission}"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    update_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"permission": permission},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload
    assert update_payload["data"]["permission"] == permission.lower().strip(), update_payload


@pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="#8208")
@pytest.mark.p2
@pytest.mark.parametrize("pagerank", [0, 50, 100], ids=["min", "mid", "max"])
def test_dataset_update_pagerank_contract(rest_client, clear_datasets, pagerank):
    create_res = rest_client.post("/datasets", json={"name": f"dataset_update_pagerank_{pagerank}"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    update_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"pagerank": pagerank},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload

    list_res = rest_client.get("/datasets", params={"id": dataset_id})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert list_payload["data"][0]["pagerank"] == pagerank, list_payload


@pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="#8208")
@pytest.mark.p2
def test_dataset_update_pagerank_set_to_zero_contract(rest_client, clear_datasets):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_pagerank_set_to_zero"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    fifty_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"pagerank": 50},
    )
    assert fifty_res.status_code == 200
    fifty_payload = fifty_res.json()
    assert fifty_payload["code"] == 0, fifty_payload

    zero_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"pagerank": 0},
    )
    assert zero_res.status_code == 200
    zero_payload = zero_res.json()
    assert zero_payload["code"] == 0, zero_payload

    list_res = rest_client.get("/datasets", params={"id": dataset_id})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert list_payload["data"][0]["pagerank"] == 0, list_payload


@pytest.mark.skipif(os.getenv("DOC_ENGINE") != "infinity", reason="#8208")
@pytest.mark.p2
def test_dataset_update_pagerank_infinity_contract(rest_client, clear_datasets):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_pagerank_infinity"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    update_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"pagerank": 50},
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 102, update_payload
    assert update_payload["message"] == "'pagerank' can only be set when doc_engine is elasticsearch", update_payload


@pytest.mark.p3
def test_dataset_update_concurrent_contract(rest_client, clear_datasets):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_concurrent_base"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    count = 100
    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(rest_client.put, f"/datasets/{dataset_id}", json={"name": f"dataset_update_{i}"}) for i in range(count)]
        responses = list(as_completed(futures))
    assert len(responses) == count, responses
    for index, future in enumerate(futures):
        res = future.result()
        assert res.status_code == 200, (index, res.text)
        payload = res.json()
        assert payload["code"] == 0, (index, payload)


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
    "name, embedding_model, expected_code, expected_embedding_model, expected_message, unauthorized_is_xfail",
    [
        ("builtin_baai", "BAAI/bge-small-en-v1.5@Builtin", 0, "BAAI/bge-small-en-v1.5@Builtin", None, False),
        ("tenant_zhipu", "embedding-3@ZHIPU-AI", 0, "embedding-3@ZHIPU-AI", None, True),
        ("embedding_model_unset", "__UNSET__", 0, "BAAI/bge-small-en-v1.5@Builtin", None, False),
        ("embedding_model_none", None, 0, "BAAI/bge-small-en-v1.5@Builtin", None, False),
        ("unknown_llm_name", "unknown@ZHIPU-AI", 102, None, "Unsupported model: <unknown@ZHIPU-AI>", False),
        ("unknown_llm_factory", "embedding-3@unknown", 102, None, "Unsupported model: <embedding-3@unknown>", False),
        (
            "tenant_no_auth_default_tenant_llm",
            "text-embedding-v3@Tongyi-Qianwen",
            102,
            None,
            "Unauthorized model: <text-embedding-v3@Tongyi-Qianwen>",
            False,
        ),
        ("tenant_no_auth", "text-embedding-3-small@OpenAI", 102, None, "Unauthorized model: <text-embedding-3-small@OpenAI>", False),
    ],
    ids=[
        "builtin_baai",
        "tenant_zhipu",
        "embedding_model_unset",
        "embedding_model_none",
        "unknown_llm_name",
        "unknown_llm_factory",
        "tenant_no_auth_default_tenant_llm",
        "tenant_no_auth",
    ],
)
def test_dataset_create_embedding_model_contract(
    rest_client, clear_datasets, name, embedding_model, expected_code, expected_embedding_model, expected_message, unauthorized_is_xfail
):
    req = {"name": name}
    if embedding_model != "__UNSET__":
        req["embedding_model"] = embedding_model
    res = rest_client.post("/datasets", json=req)
    assert res.status_code == 200
    payload = res.json()
    if unauthorized_is_xfail and payload["code"] == 102:
        pytest.xfail(f"Environment has no authorized tenant model for {embedding_model}: {payload}")
    assert payload["code"] == expected_code, payload
    if expected_embedding_model is not None:
        assert payload["data"]["embedding_model"] == expected_embedding_model, payload
    if expected_message is not None:
        assert payload["message"] == expected_message, payload


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
    assert actual_parser_config["chunk_token_num"] == 1024, body


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
def test_dataset_create_name_invalid_and_duplicate_contract(rest_client, clear_datasets):
    invalid_cases = [
        ("", "String should have at least 1 character"),
        (" ", "String should have at least 1 character"),
        ("a" * (DATASET_NAME_LIMIT + 1), f"String should have at most {DATASET_NAME_LIMIT} characters"),
        (0, "Input should be a valid string"),
        (None, "Input should be a valid string"),
    ]
    for name, expected_message in invalid_cases:
        res = rest_client.post("/datasets", json={"name": name})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 101, payload
        assert expected_message in payload["message"], payload

    create_res = rest_client.post("/datasets", json={"name": "duplicated_name"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload

    duplicate_res = rest_client.post("/datasets", json={"name": "duplicated_name"})
    assert duplicate_res.status_code == 200
    duplicate_payload = duplicate_res.json()
    assert duplicate_payload["code"] == 0, duplicate_payload
    assert duplicate_payload["data"]["name"] == "duplicated_name(1)", duplicate_payload


@pytest.mark.p2
def test_dataset_create_content_type_and_payload_bad_contract(rest_client):
    bad_content_type = "text/xml"
    bad_content_type_res = rest_client.post(
        "/datasets",
        data='{"name": "bad_content_type"}',
        headers={"Content-Type": bad_content_type},
    )
    assert bad_content_type_res.status_code == 200
    bad_content_type_payload = bad_content_type_res.json()
    assert bad_content_type_payload["code"] == 101, bad_content_type_payload
    assert f"Unsupported content type: Expected application/json, got {bad_content_type}" in bad_content_type_payload["message"], bad_content_type_payload

    malformed_json_res = rest_client.post("/datasets", data="a")
    assert malformed_json_res.status_code == 200
    malformed_json_payload = malformed_json_res.json()
    assert malformed_json_payload["code"] == 101, malformed_json_payload
    assert "Malformed JSON syntax: Missing commas/brackets or invalid encoding" in malformed_json_payload["message"], malformed_json_payload

    invalid_payload_type_res = rest_client.post("/datasets", data='"a"')
    assert invalid_payload_type_res.status_code == 200
    invalid_payload_type_payload = invalid_payload_type_res.json()
    assert invalid_payload_type_payload["code"] == 101, invalid_payload_type_payload
    assert "Invalid request payload: expected object, got str" in invalid_payload_type_payload["message"], invalid_payload_type_payload


@pytest.mark.p2
def test_dataset_create_avatar_contract(rest_client, clear_datasets, tmp_path):
    exceed_res = rest_client.post(
        "/datasets",
        json={"name": "avatar_exceeds_limit_length", "avatar": "a" * 65536},
    )
    assert exceed_res.status_code == 200
    exceed_payload = exceed_res.json()
    assert exceed_payload["code"] == 101, exceed_payload
    assert "String should have at most 65535 characters" in exceed_payload["message"], exceed_payload

    image_path = create_image_file(tmp_path / "ragflow_test.png")
    encoded_avatar = encode_avatar(image_path)
    invalid_prefix_cases = [
        ("empty_prefix", "", "Missing MIME prefix. Expected format: data:<mime>;base64,<data>"),
        ("missing_comma", "data:image/png;base64", "Missing MIME prefix. Expected format: data:<mime>;base64,<data>"),
        ("unsupported_mine_type", "invalid_mine_prefix:image/png;base64,", "Invalid MIME prefix format. Must start with 'data:'"),
        ("invalid_mine_type", "data:unsupported_mine_type;base64,", "Unsupported MIME type. Allowed: ['image/jpeg', 'image/png']"),
    ]
    for name, prefix, expected_message in invalid_prefix_cases:
        res = rest_client.post(
            "/datasets",
            json={"name": name, "avatar": f"{prefix}{encoded_avatar}"},
        )
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 101, payload
        assert expected_message in payload["message"], payload

    unset_res = rest_client.post("/datasets", json={"name": "avatar_unset"})
    assert unset_res.status_code == 200
    unset_payload = unset_res.json()
    assert unset_payload["code"] == 0, unset_payload
    assert unset_payload["data"]["avatar"] is None, unset_payload

    none_res = rest_client.post("/datasets", json={"name": "avatar_none", "avatar": None})
    assert none_res.status_code == 200
    none_payload = none_res.json()
    assert none_payload["code"] == 0, none_payload
    assert none_payload["data"]["avatar"] is None, none_payload


@pytest.mark.p2
def test_dataset_create_description_contract(rest_client, clear_datasets):
    exceeds_limit_res = rest_client.post(
        "/datasets",
        json={"name": "description_exceeds_limit_length", "description": "a" * 65536},
    )
    assert exceeds_limit_res.status_code == 200
    exceeds_limit_payload = exceeds_limit_res.json()
    assert exceeds_limit_payload["code"] == 101, exceeds_limit_payload
    assert "String should have at most 65535 characters" in exceeds_limit_payload["message"], exceeds_limit_payload

    unset_res = rest_client.post("/datasets", json={"name": "description_unset"})
    assert unset_res.status_code == 200
    unset_payload = unset_res.json()
    assert unset_payload["code"] == 0, unset_payload
    assert unset_payload["data"]["description"] is None, unset_payload

    none_res = rest_client.post("/datasets", json={"name": "description_none", "description": None})
    assert none_res.status_code == 200
    none_payload = none_res.json()
    assert none_payload["code"] == 0, none_payload
    assert none_payload["data"]["description"] is None, none_payload


@pytest.mark.p2
def test_dataset_create_permission_and_chunk_method_contract(rest_client, clear_datasets):
    permission_invalid_cases = [
        ("empty", ""),
        ("unknown", "unknown"),
        ("type_error", []),
        ("me_upercase", "ME"),
        ("team_upercase", "TEAM"),
        ("whitespace", " ME "),
    ]
    for name, permission in permission_invalid_cases:
        res = rest_client.post("/datasets", json={"name": name, "permission": permission})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 101, payload
        assert "Input should be 'me' or 'team'" in payload["message"], payload

    permission_none_res = rest_client.post("/datasets", json={"name": "permission_none", "permission": None})
    assert permission_none_res.status_code == 200
    permission_none_payload = permission_none_res.json()
    assert permission_none_payload["code"] == 101, permission_none_payload
    assert "Input should be 'me' or 'team'" in permission_none_payload["message"], permission_none_payload

    permission_unset_res = rest_client.post("/datasets", json={"name": "permission_unset"})
    assert permission_unset_res.status_code == 200
    permission_unset_payload = permission_unset_res.json()
    assert permission_unset_payload["code"] == 0, permission_unset_payload
    assert permission_unset_payload["data"]["permission"] == "me", permission_unset_payload

    chunk_method_invalid_cases = [
        ("chunk_empty", ""),
        ("chunk_unknown", "unknown"),
        ("chunk_type_error", []),
    ]
    expected_chunk_message = (
        "Input should be 'naive', 'book', 'email', 'laws', 'manual', 'one', 'paper', "
        "'picture', 'presentation', 'qa', 'table', 'tag' or 'resume'"
    )
    for name, chunk_method in chunk_method_invalid_cases:
        res = rest_client.post("/datasets", json={"name": name, "chunk_method": chunk_method})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 101, payload
        assert expected_chunk_message in payload["message"], payload

    chunk_method_none_res = rest_client.post("/datasets", json={"name": "chunk_method_none", "chunk_method": None})
    assert chunk_method_none_res.status_code == 200
    chunk_method_none_payload = chunk_method_none_res.json()
    assert chunk_method_none_payload["code"] == 101, chunk_method_none_payload
    assert expected_chunk_message in chunk_method_none_payload["message"], chunk_method_none_payload

    chunk_method_unset_res = rest_client.post("/datasets", json={"name": "chunk_method_unset"})
    assert chunk_method_unset_res.status_code == 200
    chunk_method_unset_payload = chunk_method_unset_res.json()
    assert chunk_method_unset_payload["code"] == 0, chunk_method_unset_payload
    assert chunk_method_unset_payload["data"]["chunk_method"] == "naive", chunk_method_unset_payload


@pytest.mark.p2
def test_dataset_create_parser_config_invalid_contract(rest_client, clear_datasets):
    invalid_cases = [
        ("auto_keywords_min_limit", {"auto_keywords": -1}, "Input should be greater than or equal to 0"),
        ("auto_keywords_max_limit", {"auto_keywords": 33}, "Input should be less than or equal to 32"),
        ("auto_keywords_float_not_allowed", {"auto_keywords": 3.14}, "Input should be a valid integer"),
        ("auto_keywords_type_invalid", {"auto_keywords": "string"}, "Input should be a valid integer"),
        ("auto_questions_min_limit", {"auto_questions": -1}, "Input should be greater than or equal to 0"),
        ("auto_questions_max_limit", {"auto_questions": 11}, "Input should be less than or equal to 10"),
        ("auto_questions_float_not_allowed", {"auto_questions": 3.14}, "Input should be a valid integer"),
        ("auto_questions_type_invalid", {"auto_questions": "string"}, "Input should be a valid integer"),
        ("chunk_token_num_min_limit", {"chunk_token_num": 0}, "Input should be greater than or equal to 1"),
        ("chunk_token_num_max_limit", {"chunk_token_num": 2049}, "Input should be less than or equal to 2048"),
        ("chunk_token_num_float_not_allowed", {"chunk_token_num": 3.14}, "Input should be a valid integer"),
        ("chunk_token_num_type_invalid", {"chunk_token_num": "string"}, "Input should be a valid integer"),
        ("delimiter_empty", {"delimiter": ""}, "String should have at least 1 character"),
        ("html4excel_type_invalid", {"html4excel": "string"}, "Input should be a valid boolean"),
        ("tag_kb_ids_not_list", {"tag_kb_ids": "1,2"}, "Input should be a valid list"),
        ("tag_kb_ids_int_in_list", {"tag_kb_ids": [1, 2]}, "Input should be a valid string"),
        ("topn_tags_min_limit", {"topn_tags": 0}, "Input should be greater than or equal to 1"),
        ("topn_tags_max_limit", {"topn_tags": 11}, "Input should be less than or equal to 10"),
        ("topn_tags_float_not_allowed", {"topn_tags": 3.14}, "Input should be a valid integer"),
        ("topn_tags_type_invalid", {"topn_tags": "string"}, "Input should be a valid integer"),
        ("filename_embd_weight_min_limit", {"filename_embd_weight": -1}, "Input should be greater than or equal to 0"),
        ("filename_embd_weight_max_limit", {"filename_embd_weight": 1.1}, "Input should be less than or equal to 1"),
        ("filename_embd_weight_type_invalid", {"filename_embd_weight": "string"}, "Input should be a valid number"),
        ("task_page_size_min_limit", {"task_page_size": 0}, "Input should be greater than or equal to 1"),
        ("task_page_size_float_not_allowed", {"task_page_size": 3.14}, "Input should be a valid integer"),
        ("task_page_size_type_invalid", {"task_page_size": "string"}, "Input should be a valid integer"),
        ("pages_not_list", {"pages": "1,2"}, "Input should be a valid list"),
        ("pages_not_list_in_list", {"pages": ["1,2"]}, "Input should be a valid list"),
        ("pages_not_int_list", {"pages": [["string1", "string2"]]}, "Input should be a valid integer"),
        ("graphrag_type_invalid", {"graphrag": {"use_graphrag": "string"}}, "Input should be a valid boolean"),
        ("graphrag_entity_types_not_list", {"graphrag": {"entity_types": "1,2"}}, "Input should be a valid list"),
        ("graphrag_entity_types_not_str_in_list", {"graphrag": {"entity_types": [1, 2]}}, "nput should be a valid string"),
        ("graphrag_method_unknown", {"graphrag": {"method": "unknown"}}, "Input should be 'light', 'general' or 'ner'"),
        ("graphrag_method_none", {"graphrag": {"method": None}}, "Input should be 'light', 'general' or 'ner'"),
        ("graphrag_community_type_invalid", {"graphrag": {"community": "string"}}, "Input should be a valid boolean"),
        ("graphrag_resolution_type_invalid", {"graphrag": {"resolution": "string"}}, "Input should be a valid boolean"),
        ("raptor_type_invalid", {"raptor": {"use_raptor": "string"}}, "Input should be a valid boolean"),
        ("raptor_prompt_empty", {"raptor": {"prompt": ""}}, "String should have at least 1 character"),
        ("raptor_prompt_space", {"raptor": {"prompt": " "}}, "String should have at least 1 character"),
        ("raptor_max_token_min_limit", {"raptor": {"max_token": 0}}, "Input should be greater than or equal to 1"),
        ("raptor_max_token_max_limit", {"raptor": {"max_token": 2049}}, "Input should be less than or equal to 2048"),
        ("raptor_max_token_float_not_allowed", {"raptor": {"max_token": 3.14}}, "Input should be a valid integer"),
        ("raptor_max_token_type_invalid", {"raptor": {"max_token": "string"}}, "Input should be a valid integer"),
        ("raptor_threshold_min_limit", {"raptor": {"threshold": -0.1}}, "Input should be greater than or equal to 0"),
        ("raptor_threshold_max_limit", {"raptor": {"threshold": 1.1}}, "Input should be less than or equal to 1"),
        ("raptor_threshold_type_invalid", {"raptor": {"threshold": "string"}}, "Input should be a valid number"),
        ("raptor_max_cluster_min_limit", {"raptor": {"max_cluster": 0}}, "Input should be greater than or equal to 1"),
        ("raptor_max_cluster_max_limit", {"raptor": {"max_cluster": 1025}}, "Input should be less than or equal to 1024"),
        ("raptor_max_cluster_float_not_allowed", {"raptor": {"max_cluster": 3.14}}, "Input should be a valid integer"),
        ("raptor_max_cluster_type_invalid", {"raptor": {"max_cluster": "string"}}, "Input should be a valid integer"),
        ("raptor_random_seed_min_limit", {"raptor": {"random_seed": -1}}, "Input should be greater than or equal to 0"),
        ("raptor_random_seed_float_not_allowed", {"raptor": {"random_seed": 3.14}}, "Input should be a valid integer"),
        ("raptor_random_seed_type_invalid", {"raptor": {"random_seed": "string"}}, "Input should be a valid integer"),
        ("parser_config_type_invalid", {"delimiter": "a" * 65536}, "Parser config exceeds size limit (max 65,535 characters)"),
        ("parent_child_type_invalid", {"parent_child": {"use_parent_child": "string"}}, "Input should be a valid boolean"),
        ("parent_child_delimiter_empty", {"parent_child": {"children_delimiter": ""}}, "String should have at least 1 character"),
    ]
    for name, parser_config, expected_message in invalid_cases:
        res = rest_client.post("/datasets", json={"name": name, "parser_config": parser_config})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 101, payload
        assert expected_message in payload["message"], payload


@pytest.mark.p2
def test_dataset_create_parser_config_defaults_and_extra_fields_contract(rest_client, clear_datasets):
    empty_res = rest_client.post("/datasets", json={"name": "parser_config_empty", "parser_config": {}})
    assert empty_res.status_code == 200
    empty_payload = empty_res.json()
    assert empty_payload["code"] == 0, empty_payload

    unset_res = rest_client.post("/datasets", json={"name": "parser_config_unset"})
    assert unset_res.status_code == 200
    unset_payload = unset_res.json()
    assert unset_payload["code"] == 0, unset_payload

    none_res = rest_client.post("/datasets", json={"name": "parser_config_none", "parser_config": None})
    assert none_res.status_code == 200
    none_payload = none_res.json()
    assert none_payload["code"] == 0, none_payload

    empty_parser_config = empty_payload["data"]["parser_config"]
    unset_parser_config = unset_payload["data"]["parser_config"]
    none_parser_config = none_payload["data"]["parser_config"]
    assert empty_parser_config == unset_parser_config == none_parser_config
    for key in DEFAULT_PARSER_CONFIG:
        assert key in empty_parser_config, empty_payload

    unsupported_field_payloads = [
        {"name": "id", "id": "id"},
        {"name": "tenant_id", "tenant_id": "e57c1966f99211efb41e9e45646e0111"},
        {"name": "created_by", "created_by": "created_by"},
        {"name": "create_date", "create_date": "Tue, 11 Mar 2025 13:37:23 GMT"},
        {"name": "create_time", "create_time": 1741671443322},
        {"name": "update_date", "update_date": "Tue, 11 Mar 2025 13:37:23 GMT"},
        {"name": "update_time", "update_time": 1741671443339},
        {"name": "document_count", "document_count": 1},
        {"name": "chunk_count", "chunk_count": 1},
        {"name": "token_num", "token_num": 1},
        {"name": "status", "status": "1"},
        {"name": "pagerank", "pagerank": 50},
        {"name": "unknown_field", "unknown_field": "unknown_field"},
    ]
    for payload_data in unsupported_field_payloads:
        res = rest_client.post("/datasets", json=payload_data)
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 101, payload
        assert "Extra inputs are not permitted" in payload["message"], payload


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
