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

import os
import uuid
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from configs import DATASET_NAME_LIMIT

from test.testcases.configs import INVALID_API_TOKEN
from test.testcases.restful_api.helpers.assertions import assert_auth_error
from test.testcases.restful_api.helpers.client import RestClient
from test.testcases.utils import encode_avatar
from test.testcases.utils.file_utils import create_image_file, create_txt_file

ARGUMENT_ERROR_CODE = 102
PARSER_ID_FIELD = "parser_id"


def _parser_id_fields(chunk_method):
    """Build the parser_id field; the Go API requires parse_type alongside it."""
    return {PARSER_ID_FIELD: chunk_method, "parse_type": 1}


def _expected_chunk_method(chunk_method):
    """Return the API-visible parser ID."""
    if chunk_method == "naive":
        return "general"
    return chunk_method


def _assert_go_pipeline_parser_config(parser_config, chunker_prefix="GeneralChunker:"):
    assert parser_config
    assert "File" in parser_config, parser_config
    assert any(key.startswith("Parser:") for key in parser_config), parser_config
    assert any(key.startswith(chunker_prefix) for key in parser_config), parser_config
    assert all(isinstance(params, dict) for params in parser_config.values()), parser_config
    assert "chunk_token_num" not in parser_config, parser_config
    assert "raptor" not in parser_config, parser_config
    assert "graphrag" not in parser_config, parser_config


def _is_infinity_doc_engine(rest_client: RestClient) -> bool:
    env_engine = (os.getenv("DOC_ENGINE") or "").strip().lower()
    if env_engine:
        return env_engine == "infinity"
    try:
        res = rest_client.get("/system/status")
        if res.status_code != 200:
            return False
        payload = res.json()
        if payload.get("code") != 0:
            return False
        engine = str(payload.get("data", {}).get("doc_engine", {}).get("type", "")).strip().lower()
        return engine == "infinity"
    except Exception:
        return False


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
            **_parser_id_fields("naive"),
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
    ],
    ids=["naive", "book", "email", "laws", "manual", "one", "paper", "picture", "presentation", "qa", "table"],
)
def test_dataset_update_chunk_method_contract(rest_client, clear_datasets, chunk_method):
    create_res = rest_client.post("/datasets", json={"name": f"dataset_update_chunk_{chunk_method}"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    update_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json=_parser_id_fields(chunk_method),
    )
    assert update_res.status_code == 200
    update_payload = update_res.json()
    assert update_payload["code"] == 0, update_payload
    assert update_payload["data"][PARSER_ID_FIELD] == _expected_chunk_method(chunk_method), update_payload


@pytest.mark.p3
@pytest.mark.parametrize(
    "name, update_payload",
    [
        ("parser_config_empty", {**_parser_id_fields("qa"), "parser_config": {}}),
        ("parser_config_none", {**_parser_id_fields("qa"), "parser_config": None}),
        ("parser_config_unset", _parser_id_fields("qa")),
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
    actual_parser_config = list_body["data"][0]["parser_config"]
    assert isinstance(actual_parser_config, dict) and actual_parser_config, list_body
    assert "raptor" not in actual_parser_config, list_body
    assert "graphrag" not in actual_parser_config, list_body


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
        ("empty", "", "embedding model identifier must follow <model_name>@<provider> format"),
        ("space", " ", "embedding model identifier must follow <model_name>@<provider> format"),
        ("missing_at", "BAAI/bge-small-en-v1.5Builtin", "embedding model identifier must follow <model_name>@<provider> format"),
        ("missing_model_name", "@Builtin", "both model_name and provider must be non-empty strings"),
        ("missing_provider", "BAAI/bge-small-en-v1.5@", "both model_name and provider must be non-empty strings"),
        ("whitespace_only_model_name", " @Builtin", "both model_name and provider must be non-empty strings"),
        ("whitespace_only_provider", "BAAI/bge-small-en-v1.5@ ", "both model_name and provider must be non-empty strings"),
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
    assert update_payload["code"] == ARGUMENT_ERROR_CODE, update_payload
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

    candidates = ["embedding-3@CI@ZHIPU-AI", "BAAI/bge-small-en-v1.5@Local@Builtin"]
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


@pytest.mark.p2
@pytest.mark.parametrize("pagerank", [0, 50, 100], ids=["min", "mid", "max"])
def test_dataset_update_pagerank_contract(rest_client, clear_datasets, pagerank):
    if _is_infinity_doc_engine(rest_client):
        pytest.skip("#8208")
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


@pytest.mark.p2
def test_dataset_update_pagerank_set_to_zero_contract(rest_client, clear_datasets):
    if _is_infinity_doc_engine(rest_client):
        pytest.skip("#8208")
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


@pytest.mark.p2
def test_dataset_update_pagerank_infinity_contract(rest_client, clear_datasets):
    if not _is_infinity_doc_engine(rest_client):
        pytest.skip("#8208")
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


@pytest.mark.p1
def test_dataset_update_requires_auth_contract(rest_client, clear_datasets):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_auth_contract"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.put(f"/datasets/{dataset_id}", json={"name": "dataset_update_auth_invalid"})
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert_auth_error(payload, scenario_name)


@pytest.mark.p2
def test_dataset_update_content_type_and_payload_contract(rest_client, clear_datasets):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_payload_contract"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    bad_content_type = "text/xml"
    bad_content_type_res = rest_client.put(
        f"/datasets/{dataset_id}",
        data='{"name": "bad_content_type"}',
        headers={"Content-Type": bad_content_type},
    )
    assert bad_content_type_res.status_code == 200
    bad_content_type_payload = bad_content_type_res.json()
    assert bad_content_type_payload["code"] == 101, bad_content_type_payload
    assert f"Unsupported content type: Expected application/json, got {bad_content_type}" in bad_content_type_payload["message"], bad_content_type_payload

    malformed_json_res = rest_client.put(f"/datasets/{dataset_id}", data="a")
    assert malformed_json_res.status_code == 200
    malformed_json_payload = malformed_json_res.json()
    assert malformed_json_payload["code"] == 101, malformed_json_payload
    assert "Malformed JSON syntax: Missing commas/brackets or invalid encoding" in malformed_json_payload["message"], malformed_json_payload

    invalid_payload_type_res = rest_client.put(f"/datasets/{dataset_id}", data='"a"')
    assert invalid_payload_type_res.status_code == 200
    invalid_payload_type_payload = invalid_payload_type_res.json()
    assert invalid_payload_type_payload["code"] == 101, invalid_payload_type_payload
    assert "Invalid request payload: expected object, got str" in invalid_payload_type_payload["message"], invalid_payload_type_payload

    empty_payload_res = rest_client.put(f"/datasets/{dataset_id}", json={})
    assert empty_payload_res.status_code == 200
    empty_payload = empty_payload_res.json()
    assert empty_payload["code"] == 102, empty_payload
    assert empty_payload["message"] == "no properties were modified", empty_payload

    unset_payload_res = rest_client.put(f"/datasets/{dataset_id}")
    assert unset_payload_res.status_code == 200
    unset_payload = unset_payload_res.json()
    assert unset_payload["code"] == 101, unset_payload
    assert "Malformed JSON syntax: Missing commas/brackets or invalid encoding" in unset_payload["message"], unset_payload


@pytest.mark.p2
def test_dataset_update_identifier_validation_contract(rest_client):
    payload = {"name": "dataset_update_identifier_validation"}

    not_uuid_res = rest_client.put("/datasets/not_uuid", json=payload)
    assert not_uuid_res.status_code == 200
    not_uuid_payload = not_uuid_res.json()
    assert not_uuid_payload["code"] == 101, not_uuid_payload
    assert "Invalid UUID format" in not_uuid_payload["message"], not_uuid_payload

    not_uuid1_res = rest_client.put(f"/datasets/{uuid.uuid4().hex}", json=payload)
    assert not_uuid1_res.status_code == 200
    not_uuid1_payload = not_uuid1_res.json()
    assert not_uuid1_payload["code"] == 102, not_uuid1_payload
    assert "lacks permission for dataset" in not_uuid1_payload["message"], not_uuid1_payload

    wrong_uuid_res = rest_client.put("/datasets/d94a8dc02c9711f0930f7fbc369eab6d", json=payload)
    assert wrong_uuid_res.status_code == 200
    wrong_uuid_payload = wrong_uuid_res.json()
    assert wrong_uuid_payload["code"] == 102, wrong_uuid_payload
    assert "lacks permission for dataset" in wrong_uuid_payload["message"], wrong_uuid_payload


@pytest.mark.p2
def test_dataset_update_avatar_invalid_contract(rest_client, clear_datasets, tmp_path):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_avatar_invalid_contract"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    exceed_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"avatar": "a" * 65536},
    )
    assert exceed_res.status_code == 200
    exceed_payload = exceed_res.json()
    assert exceed_payload["code"] == ARGUMENT_ERROR_CODE, exceed_payload
    assert "String should have at most 65535 characters" in exceed_payload["message"], exceed_payload

    image_path = create_image_file(tmp_path / "dataset_update_avatar_invalid.png")
    encoded_avatar = encode_avatar(image_path)
    invalid_prefix_cases = [
        ("", "missing MIME prefix. Expected format: data:<mime>;base64,<data>"),
        ("data:image/png;base64", "missing MIME prefix. Expected format: data:<mime>;base64,<data>"),
        ("invalid_mine_prefix:image/png;base64,", "invalid MIME prefix format. Must start with 'data:'"),
        ("data:unsupported_mine_type;base64,", "Unsupported MIME type. Allowed: ['image/jpeg', 'image/png']"),
    ]
    for prefix, expected_message in invalid_prefix_cases:
        res = rest_client.put(
            f"/datasets/{dataset_id}",
            json={"avatar": f"{prefix}{encoded_avatar}"},
        )
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == ARGUMENT_ERROR_CODE, payload
        if expected_message.startswith("Unsupported MIME type"):
            expected_message = "unsupported MIME type. Allowed: [image/jpeg image/png]"
        assert expected_message in payload["message"], payload


@pytest.mark.p2
def test_dataset_update_description_validation_contract(rest_client, clear_datasets):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_description_contract"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    exceeds_limit_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"description": "a" * 65536},
    )
    assert exceeds_limit_res.status_code == 200
    exceeds_limit_payload = exceeds_limit_res.json()
    assert exceeds_limit_payload["code"] == ARGUMENT_ERROR_CODE, exceeds_limit_payload
    assert "String should have at most 65535 characters" in exceeds_limit_payload["message"], exceeds_limit_payload


@pytest.mark.p2
def test_dataset_update_name_invalid_and_duplicate_contract(rest_client, clear_datasets):
    first_res = rest_client.post("/datasets", json={"name": "dataset_update_name_invalid_first"})
    assert first_res.status_code == 200
    first_payload = first_res.json()
    assert first_payload["code"] == 0, first_payload
    first_dataset_id = first_payload["data"]["id"]

    second_res = rest_client.post("/datasets", json={"name": "dataset_update_name_invalid_second"})
    assert second_res.status_code == 200
    second_payload = second_res.json()
    assert second_payload["code"] == 0, second_payload

    invalid_cases = [
        ("", "String should have at least 1 character"),
        (" ", "String should have at least 1 character"),
        ("a" * (DATASET_NAME_LIMIT + 1), f"String should have at most {DATASET_NAME_LIMIT} characters"),
        (0, "Input should be a valid string"),
    ]
    for name, expected_message in invalid_cases:
        res = rest_client.put(f"/datasets/{first_dataset_id}", json={"name": name})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == ARGUMENT_ERROR_CODE, payload
        if not isinstance(name, str):
            assert "cannot unmarshal" in payload["message"] and ".name" in payload["message"], payload
        else:
            assert expected_message in payload["message"], payload

    duplicated_res = rest_client.put(
        f"/datasets/{first_dataset_id}",
        json={"name": second_payload["data"]["name"]},
    )
    assert duplicated_res.status_code == 200
    duplicated_payload = duplicated_res.json()
    assert duplicated_payload["code"] == 102, duplicated_payload
    assert f"Dataset name '{second_payload['data']['name']}' already exists" == duplicated_payload["message"], duplicated_payload


@pytest.mark.p2
def test_dataset_update_embedding_model_invalid_contract(rest_client, clear_datasets):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_embedding_invalid_contract"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    invalid_cases = [
        ("unknown@ZHIPU-AI", "Model unknown@ZHIPU-AI not found for model embedding"),
        ("embedding-3@unknown", "Provider unknown not found for model embedding-3@unknown."),
        ("text-embedding-v3@Tongyi-Qianwen", "Provider Tongyi-Qianwen not found for model text-embedding-v3@Tongyi-Qianwen."),
        ("text-embedding-3-small@OpenAI", "Provider OpenAI not found for model text-embedding-3-small@OpenAI."),
    ]
    for embedding_model, _ in invalid_cases:
        res = rest_client.put(
            f"/datasets/{dataset_id}",
            json={"embedding_model": embedding_model},
        )
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 102, payload
        assert "lookup failed: record not found" in payload["message"], payload


@pytest.mark.p2
def test_dataset_update_permission_invalid_contract(rest_client, clear_datasets):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_permission_invalid_contract"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    invalid_permissions = ["", "unknown", [], "ME", "TEAM", " ME "]
    for permission in invalid_permissions:
        res = rest_client.put(f"/datasets/{dataset_id}", json={"permission": permission})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == ARGUMENT_ERROR_CODE, payload
        if not isinstance(permission, str):
            assert "cannot unmarshal" in payload["message"] and ".permission" in payload["message"], payload
        else:
            assert "Input should be 'me' or 'team'" in payload["message"], payload


@pytest.mark.p2
def test_dataset_update_chunk_method_invalid_contract(rest_client, clear_datasets):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_chunk_method_invalid_contract"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    for chunk_method in ("", "unknown", []):
        res = rest_client.put(f"/datasets/{dataset_id}", json=_parser_id_fields(chunk_method))
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == ARGUMENT_ERROR_CODE, payload
        if not isinstance(chunk_method, str):
            assert "cannot unmarshal" in payload["message"] and f".{PARSER_ID_FIELD}" in payload["message"], payload
        elif chunk_method == "":
            assert payload["message"] == "parser_id is required when parse_type is BuiltIn", payload
        else:
            assert payload["message"].startswith("input should be 'general', 'qa'") and payload["message"].endswith("or 'email'"), payload


@pytest.mark.p2
def test_dataset_update_parser_config_defaults_contract(rest_client, clear_datasets):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_parser_defaults_contract"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    empty_res = rest_client.put(f"/datasets/{dataset_id}", json={"parser_config": {}})
    assert empty_res.status_code == 200
    empty_payload = empty_res.json()
    assert empty_payload["code"] == 0, empty_payload

    none_res = rest_client.put(f"/datasets/{dataset_id}", json={"parser_config": None})
    assert none_res.status_code == 200
    none_payload = none_res.json()
    assert none_payload["code"] == 0, none_payload

    list_res = rest_client.get("/datasets", params={"id": dataset_id})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    parser_config = list_payload["data"][0]["parser_config"]
    assert isinstance(parser_config, dict) and parser_config, list_payload
    assert "raptor" not in parser_config, list_payload
    assert "graphrag" not in parser_config, list_payload


@pytest.mark.p3
@pytest.mark.parametrize(
    "children_delimiter",
    ["|", ";", "\n"],
    ids=["delimiter_pipe", "delimiter_semicolon", "delimiter_newline"],
)
def test_dataset_update_parser_config_component_scoped_roundtrip(rest_client, clear_datasets, children_delimiter):
    # The intended contract is component-scoped: every parser_config key must be
    # keyed by a component id (e.g. "GeneralChunker:<id>"). This test verifies a
    # dataset-level setting reaches the runtime chunker node and round-trips.
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_component_scoped"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    # Discover the real chunker component id from the created config.
    list_res = rest_client.get("/datasets", params={"id": dataset_id})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    parser_config = list_payload["data"][0]["parser_config"]
    chunker_id = next(
        (k for k in parser_config if k.split(":", 1)[0].endswith("Chunker")),
        None,
    )
    assert chunker_id is not None, parser_config

    update_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={
            "parser_config": {
                chunker_id: {
                    "parent_child": {
                        "use_parent_child": True,
                        "children_delimiter": children_delimiter,
                    }
                }
            }
        },
    )
    assert update_res.status_code == 200
    assert update_res.json()["code"] == 0, update_res.json()

    list_res2 = rest_client.get("/datasets", params={"id": dataset_id})
    assert list_res2.status_code == 200
    final_config = list_res2.json()["data"][0]["parser_config"]
    chunker = final_config[chunker_id]
    assert chunker["parent_child"]["use_parent_child"] is True, final_config
    assert chunker["children_delimiters"] == [children_delimiter], final_config


@pytest.mark.p3
def test_dataset_update_parser_config_accepts_bare_file_node(rest_client, clear_datasets):
    # The DSL emits a single input node keyed by the bare id "File" (no
    # component colon). The Go backend tolerates this legacy shape (it is
    # dropped server-side) rather than rejecting it, so existing frontends that
    # send it must not receive a 101 error. See PR #20445.
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_bare_file"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    update_res = rest_client.put(
        f"/datasets/{dataset_id}",
        json={"parser_config": {"File": {"layout_recognize": "DeepDOC"}}},
    )
    assert update_res.status_code == 200
    assert update_res.json()["code"] == 0, update_res.json()


@pytest.mark.p2
def test_dataset_update_field_unset_and_unsupported_contract(rest_client, clear_datasets):
    create_res = rest_client.post("/datasets", json={"name": "dataset_update_field_unset_contract"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    dataset_id = create_payload["data"]["id"]

    list_res = rest_client.get("/datasets", params={"id": dataset_id})
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    original_data = list_payload["data"][0]

    name_update_res = rest_client.put(f"/datasets/{dataset_id}", json={"name": "dataset_update_field_unset_renamed"})
    assert name_update_res.status_code == 200
    name_update_payload = name_update_res.json()
    assert name_update_payload["code"] == 0, name_update_payload

    after_list_res = rest_client.get("/datasets", params={"id": dataset_id})
    assert after_list_res.status_code == 200
    after_list_payload = after_list_res.json()
    assert after_list_payload["code"] == 0, after_list_payload
    assert after_list_payload["data"][0]["avatar"] == original_data["avatar"], after_list_payload
    assert after_list_payload["data"][0]["description"] == original_data["description"], after_list_payload
    assert after_list_payload["data"][0]["embedding_model"] == original_data["embedding_model"], after_list_payload
    assert after_list_payload["data"][0]["permission"] == original_data["permission"], after_list_payload
    assert after_list_payload["data"][0][PARSER_ID_FIELD] == original_data[PARSER_ID_FIELD], after_list_payload
    assert after_list_payload["data"][0]["pagerank"] == original_data["pagerank"], after_list_payload
    assert after_list_payload["data"][0]["parser_config"] == original_data["parser_config"], after_list_payload

    unsupported_field_payloads = [
        {"id": "id"},
        {"tenant_id": "e57c1966f99211efb41e9e45646e0111"},
        {"created_by": "created_by"},
        {"create_date": "Tue, 11 Mar 2025 13:37:23 GMT"},
        {"create_time": 1741671443322},
        {"update_date": "Tue, 11 Mar 2025 13:37:23 GMT"},
        {"update_time": 1741671443339},
        {"document_count": 1},
        {"chunk_count": 1},
        {"token_num": 1},
        {"status": "1"},
        {"unknown_field": "unknown_field"},
    ]
    for payload_data in unsupported_field_payloads:
        res = rest_client.put(f"/datasets/{dataset_id}", json=payload_data)
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == 101, payload
        assert "Extra inputs are not permitted" in payload["message"], payload


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
    assert payload["code"] == ARGUMENT_ERROR_CODE, payload
    if not name:
        expected_fragment = "failed on the 'required' tag"
    elif not name.strip():
        expected_fragment = "dataset name can't be empty"
    else:
        expected_fragment = f"Dataset name length is {len(name)} which is large than {DATASET_NAME_LIMIT}"
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
    ],
    ids=["naive", "book", "email", "laws", "manual", "one", "paper", "picture", "presentation", "qa", "table"],
)
def test_dataset_create_chunk_method_contract(rest_client, clear_datasets, name, chunk_method):
    res = rest_client.post("/datasets", json={"name": name, **_parser_id_fields(chunk_method)})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert payload["data"][PARSER_ID_FIELD] == _expected_chunk_method(chunk_method), payload


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
        ("builtin_baai", "BAAI/bge-small-en-v1.5@Local@Builtin", 0, "BAAI/bge-small-en-v1.5@Local@Builtin", None, False),
        ("tenant_zhipu", "embedding-3@CI@ZHIPU-AI", 0, "embedding-3@CI@ZHIPU-AI", None, True),
        ("embedding_model_unset", "__UNSET__", 0, "BAAI/bge-small-en-v1.5@Local@Builtin", None, False),
        ("embedding_model_none", None, 0, "BAAI/bge-small-en-v1.5@Local@Builtin", None, False),
        ("unknown_llm_name", "unknown@ZHIPU-AI", 102, None, "Model unknown@ZHIPU-AI not found for model embedding", False),
        ("unknown_llm_factory", "embedding-3@unknown", 102, None, "Provider unknown not found for model embedding-3@unknown.", False),
        (
            "tenant_no_auth_default_tenant_llm",
            "text-embedding-v3@Tongyi-Qianwen",
            102,
            None,
            "Provider Tongyi-Qianwen not found for model text-embedding-v3@Tongyi-Qianwen.",
            False,
        ),
        ("tenant_no_auth", "text-embedding-3-small@OpenAI", 102, None, "Provider OpenAI not found for model text-embedding-3-small@OpenAI.", False),
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
def test_dataset_create_embedding_model_contract(rest_client, clear_datasets, name, embedding_model, expected_code, expected_embedding_model, expected_message, unauthorized_is_xfail):
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
        if name in {"embedding_model_unset", "embedding_model_none"}:
            assert payload["data"]["embedding_model"].startswith("BAAI/bge-small-en-v1.5"), payload
        else:
            assert payload["data"]["embedding_model"] == expected_embedding_model, payload
    if expected_message is not None:
        assert "lookup failed: record not found" in payload["message"], payload


@pytest.mark.p2
@pytest.mark.parametrize(
    "name, embedding_model, expected_fragment",
    [
        ("empty", "", "embedding model identifier must follow <model_name>@<provider> format"),
        ("space", " ", "embedding model identifier must follow <model_name>@<provider> format"),
        ("missing_at", "BAAI/bge-small-en-v1.5Builtin", "embedding model identifier must follow <model_name>@<provider> format"),
        ("missing_model_name", "@Builtin", "both model_name and provider must be non-empty strings"),
        ("missing_provider", "BAAI/bge-small-en-v1.5@", "both model_name and provider must be non-empty strings"),
        ("whitespace_only_model_name", " @Builtin", "both model_name and provider must be non-empty strings"),
        ("whitespace_only_provider", "BAAI/bge-small-env1.5@ ", "both model_name and provider must be non-empty strings"),
    ],
    ids=["empty", "space", "missing_at", "empty_model_name", "empty_provider", "whitespace_only_model_name", "whitespace_only_provider"],
)
def test_dataset_create_embedding_model_format_contract(rest_client, clear_datasets, name, embedding_model, expected_fragment):
    res = rest_client.post("/datasets", json={"name": name, "embedding_model": embedding_model})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == ARGUMENT_ERROR_CODE, payload
    assert expected_fragment in payload["message"], payload


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
        assert payload["code"] == ARGUMENT_ERROR_CODE, payload
        if not isinstance(permission, str):
            assert "cannot unmarshal" in payload["message"] and ".permission" in payload["message"], payload
        else:
            assert "Input should be 'me' or 'team'" in payload["message"], payload

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
    for name, chunk_method in chunk_method_invalid_cases:
        res = rest_client.post("/datasets", json={"name": name, **_parser_id_fields(chunk_method)})
        assert res.status_code == 200
        payload = res.json()
        assert payload["code"] == ARGUMENT_ERROR_CODE, payload
        if not isinstance(chunk_method, str):
            assert "cannot unmarshal" in payload["message"] and f".{PARSER_ID_FIELD}" in payload["message"], payload
        elif chunk_method == "":
            assert payload["message"] == "parser_id is required when parse_type is BuiltIn", payload
        else:
            assert payload["message"].startswith("input should be 'general', 'qa'") and payload["message"].endswith("or 'email'"), payload

    chunk_method_none_res = rest_client.post("/datasets", json={"name": "chunk_method_none", **_parser_id_fields(None)})
    assert chunk_method_none_res.status_code == 200
    chunk_method_none_payload = chunk_method_none_res.json()
    assert chunk_method_none_payload["code"] == ARGUMENT_ERROR_CODE, chunk_method_none_payload
    assert chunk_method_none_payload["message"] == "parser_id is required when parse_type is BuiltIn", chunk_method_none_payload

    chunk_method_unset_res = rest_client.post("/datasets", json={"name": "chunk_method_unset"})
    assert chunk_method_unset_res.status_code == 200
    chunk_method_unset_payload = chunk_method_unset_res.json()
    assert chunk_method_unset_payload["code"] == 0, chunk_method_unset_payload
    assert chunk_method_unset_payload["data"][PARSER_ID_FIELD] == _expected_chunk_method("naive"), chunk_method_unset_payload


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
    _assert_go_pipeline_parser_config(empty_parser_config)

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
def test_dataset_delete_contract_matrix(rest_client, clear_datasets):
    ids = []
    for i in range(3):
        create_res = rest_client.post("/datasets", json={"name": f"dataset_delete_matrix_{i}"})
        assert create_res.status_code == 200
        create_payload = create_res.json()
        assert create_payload["code"] == 0, create_payload
        ids.append(create_payload["data"]["id"])

    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        auth_res = client.delete("/datasets", json={"ids": [ids[0]]})
        assert auth_res.status_code == 401, (scenario_name, auth_res.text)
        auth_payload = auth_res.json()
        assert_auth_error(auth_payload, scenario_name)

    bad_content_type = "text/xml"
    bad_content_type_res = rest_client.delete(
        "/datasets",
        data='{"ids": []}',
        headers={"Content-Type": bad_content_type},
    )
    assert bad_content_type_res.status_code == 200
    bad_content_type_payload = bad_content_type_res.json()
    assert bad_content_type_payload["code"] == 101, bad_content_type_payload
    assert f"Unsupported content type: Expected application/json, got {bad_content_type}" in bad_content_type_payload["message"], bad_content_type_payload

    malformed_json_res = rest_client.delete("/datasets", data="a")
    assert malformed_json_res.status_code == 200
    malformed_json_payload = malformed_json_res.json()
    assert malformed_json_payload["code"] == 101, malformed_json_payload
    assert "Malformed JSON syntax: Missing commas/brackets or invalid encoding" in malformed_json_payload["message"], malformed_json_payload

    invalid_payload_type_res = rest_client.delete("/datasets", data='"a"')
    assert invalid_payload_type_res.status_code == 200
    invalid_payload_type_payload = invalid_payload_type_res.json()
    assert invalid_payload_type_payload["code"] == 101, invalid_payload_type_payload
    assert "Invalid request payload: expected object, got str" in invalid_payload_type_payload["message"], invalid_payload_type_payload

    unset_payload_res = rest_client.delete("/datasets")
    assert unset_payload_res.status_code == 200
    unset_payload = unset_payload_res.json()
    assert unset_payload["code"] == 101, unset_payload
    assert "Malformed JSON syntax: Missing commas/brackets or invalid encoding" in unset_payload["message"], unset_payload

    single_delete_res = rest_client.delete("/datasets", json={"ids": [ids[0]]})
    assert single_delete_res.status_code == 200
    single_delete_payload = single_delete_res.json()
    assert single_delete_payload["code"] == 0, single_delete_payload

    list_after_single = rest_client.get("/datasets")
    assert list_after_single.status_code == 200
    list_after_single_payload = list_after_single.json()
    assert list_after_single_payload["code"] == 0, list_after_single_payload
    assert len(list_after_single_payload["data"]) == 2, list_after_single_payload

    ids_empty_res = rest_client.delete("/datasets", json={"ids": []})
    assert ids_empty_res.status_code == 200
    ids_empty_payload = ids_empty_res.json()
    assert ids_empty_payload["code"] == 0, ids_empty_payload

    ids_none_res = rest_client.delete("/datasets", json={"ids": None})
    assert ids_none_res.status_code == 200
    ids_none_payload = ids_none_res.json()
    assert ids_none_payload["code"] == 0, ids_none_payload

    id_not_uuid_res = rest_client.delete("/datasets", json={"ids": ["not_uuid"]})
    assert id_not_uuid_res.status_code == 200
    id_not_uuid_payload = id_not_uuid_res.json()
    assert id_not_uuid_payload["code"] == 101, id_not_uuid_payload
    assert "Invalid UUID format" in id_not_uuid_payload["message"], id_not_uuid_payload

    id_not_uuid1_res = rest_client.delete("/datasets", json={"ids": [uuid.uuid4().hex]})
    assert id_not_uuid1_res.status_code == 200
    id_not_uuid1_payload = id_not_uuid1_res.json()
    assert id_not_uuid1_payload["code"] == 102, id_not_uuid1_payload
    assert "lacks permission for dataset" in id_not_uuid1_payload["message"], id_not_uuid1_payload

    id_wrong_uuid_res = rest_client.delete("/datasets", json={"ids": ["d94a8dc02c9711f0930f7fbc369eab6d"]})
    assert id_wrong_uuid_res.status_code == 200
    id_wrong_uuid_payload = id_wrong_uuid_res.json()
    assert id_wrong_uuid_payload["code"] == 102, id_wrong_uuid_payload
    assert "lacks permission for dataset" in id_wrong_uuid_payload["message"], id_wrong_uuid_payload

    list_res = rest_client.get("/datasets")
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    remaining_ids = [dataset["id"] for dataset in list_payload["data"]]

    for invalid_ids in (
        ["d94a8dc02c9711f0930f7fbc369eab6d"] + remaining_ids,
        remaining_ids[:1] + ["d94a8dc02c9711f0930f7fbc369eab6d"] + remaining_ids[1:],
        remaining_ids + ["d94a8dc02c9711f0930f7fbc369eab6d"],
    ):
        partial_invalid_res = rest_client.delete("/datasets", json={"ids": invalid_ids})
        assert partial_invalid_res.status_code == 200
        partial_invalid_payload = partial_invalid_res.json()
        assert partial_invalid_payload["code"] == 102, partial_invalid_payload
        assert "lacks permission for dataset" in partial_invalid_payload["message"], partial_invalid_payload

    duplicate_ids_res = rest_client.delete("/datasets", json={"ids": remaining_ids + remaining_ids})
    assert duplicate_ids_res.status_code == 200
    duplicate_ids_payload = duplicate_ids_res.json()
    assert duplicate_ids_payload["code"] == 101, duplicate_ids_payload
    assert "Duplicate ids:" in duplicate_ids_payload["message"], duplicate_ids_payload

    repeated_delete_payload = {"ids": remaining_ids}
    first_delete_res = rest_client.delete("/datasets", json=repeated_delete_payload)
    assert first_delete_res.status_code == 200
    first_delete_payload = first_delete_res.json()
    assert first_delete_payload["code"] == 0, first_delete_payload

    second_delete_res = rest_client.delete("/datasets", json=repeated_delete_payload)
    assert second_delete_res.status_code == 200
    second_delete_payload = second_delete_res.json()
    assert second_delete_payload["code"] == 102, second_delete_payload
    assert "lacks permission for dataset" in second_delete_payload["message"], second_delete_payload

    unsupported_field_res = rest_client.delete("/datasets", json={"unknown_field": "unknown_field"})
    assert unsupported_field_res.status_code == 200
    unsupported_field_payload = unsupported_field_res.json()
    assert unsupported_field_payload["code"] == 101, unsupported_field_payload
    assert "Extra inputs are not permitted" in unsupported_field_payload["message"], unsupported_field_payload


@pytest.mark.p3
def test_dataset_delete_bulk_and_concurrent_contract(rest_client, clear_datasets):
    bulk_ids = []
    for i in range(1000):
        create_res = rest_client.post("/datasets", json={"name": f"dataset_delete_bulk_{i}"})
        assert create_res.status_code == 200
        create_payload = create_res.json()
        assert create_payload["code"] == 0, create_payload
        bulk_ids.append(create_payload["data"]["id"])

    bulk_delete_res = rest_client.delete("/datasets", json={"ids": bulk_ids})
    assert bulk_delete_res.status_code == 200
    bulk_delete_payload = bulk_delete_res.json()
    assert bulk_delete_payload["code"] == 0, bulk_delete_payload

    list_after_bulk_delete = rest_client.get("/datasets")
    assert list_after_bulk_delete.status_code == 200
    list_after_bulk_delete_payload = list_after_bulk_delete.json()
    assert list_after_bulk_delete_payload["code"] == 0, list_after_bulk_delete_payload
    assert len(list_after_bulk_delete_payload["data"]) == 0, list_after_bulk_delete_payload

    concurrent_ids = []
    for i in range(100):
        create_res = rest_client.post("/datasets", json={"name": f"dataset_delete_concurrent_{i}"})
        assert create_res.status_code == 200
        create_payload = create_res.json()
        assert create_payload["code"] == 0, create_payload
        concurrent_ids.append(create_payload["data"]["id"])

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(rest_client.delete, "/datasets", json={"ids": [dataset_id]}) for dataset_id in concurrent_ids]

    responses = list(as_completed(futures))
    assert len(responses) == len(concurrent_ids), responses
    for future in futures:
        res = future.result()
        assert res.status_code == 200, res.text
        payload = res.json()
        assert payload["code"] == 0, payload


@pytest.mark.p1
def test_dataset_list_requires_auth_contract(rest_client, clear_datasets):
    rest_client.post("/datasets", json={"name": "dataset_list_auth_contract"})

    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.get("/datasets")
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert_auth_error(payload, scenario_name)


@pytest.mark.p2
def test_dataset_list_query_contract_matrix(rest_client, clear_datasets):
    dataset_ids = []
    for i in range(5):
        create_res = rest_client.post("/datasets", json={"name": f"dataset_{i}"})
        assert create_res.status_code == 200
        create_payload = create_res.json()
        assert create_payload["code"] == 0, create_payload
        dataset_ids.append(create_payload["data"]["id"])

    params_unset_res = rest_client.get("/datasets")
    assert params_unset_res.status_code == 200
    params_unset_payload = params_unset_res.json()
    assert params_unset_payload["code"] == 0, params_unset_payload
    assert len(params_unset_payload["data"]) == 5, params_unset_payload

    params_empty_res = rest_client.get("/datasets", params={})
    assert params_empty_res.status_code == 200
    params_empty_payload = params_empty_res.json()
    assert params_empty_payload["code"] == 0, params_empty_payload
    assert len(params_empty_payload["data"]) == 5, params_empty_payload

    for params, expected_size in (
        ({"page": 2, "page_size": 2}, 2),
        ({"page": 3, "page_size": 2}, 1),
        ({"page": 4, "page_size": 2}, 0),
        ({"page": "2", "page_size": 2}, 2),
        ({"page": 1, "page_size": 10}, 5),
    ):
        page_res = rest_client.get("/datasets", params=params)
        assert page_res.status_code == 200
        page_payload = page_res.json()
        assert page_payload["code"] == 0, page_payload
        assert len(page_payload["data"]) == expected_size, page_payload

    for params, expected_fragment in (
        ({"page": 0}, "Input should be greater than or equal to 1"),
        ({"page": "a"}, "Input should be a valid integer, unable to parse string as an integer"),
    ):
        page_invalid_res = rest_client.get("/datasets", params=params)
        assert page_invalid_res.status_code == 200
        page_invalid_payload = page_invalid_res.json()
        assert page_invalid_payload["code"] == 101, page_invalid_payload
        assert expected_fragment in page_invalid_payload["message"], page_invalid_payload

    page_none_res = rest_client.get("/datasets", params={"page": None})
    assert page_none_res.status_code == 200
    page_none_payload = page_none_res.json()
    assert page_none_payload["code"] == 0, page_none_payload
    assert len(page_none_payload["data"]) == 5, page_none_payload

    for params, expected_size in (
        ({"page_size": 1}, 1),
        ({"page_size": 3}, 3),
        ({"page_size": 5}, 5),
        ({"page_size": 6}, 5),
        ({"page_size": "1"}, 1),
    ):
        page_size_res = rest_client.get("/datasets", params=params)
        assert page_size_res.status_code == 200
        page_size_payload = page_size_res.json()
        assert page_size_payload["code"] == 0, page_size_payload
        assert len(page_size_payload["data"]) == expected_size, page_size_payload

    for params, expected_fragment in (
        ({"page_size": 0}, "Input should be greater than or equal to 1"),
        ({"page_size": "a"}, "Input should be a valid integer, unable to parse string as an integer"),
    ):
        page_size_invalid_res = rest_client.get("/datasets", params=params)
        assert page_size_invalid_res.status_code == 200
        page_size_invalid_payload = page_size_invalid_res.json()
        assert page_size_invalid_payload["code"] == 101, page_size_invalid_payload
        assert expected_fragment in page_size_invalid_payload["message"], page_size_invalid_payload

    page_size_none_res = rest_client.get("/datasets", params={"page_size": None})
    assert page_size_none_res.status_code == 200
    page_size_none_payload = page_size_none_res.json()
    assert page_size_none_payload["code"] == 0, page_size_none_payload
    assert len(page_size_none_payload["data"]) == 5, page_size_none_payload

    for params in ({"orderby": "create_time"}, {"orderby": "update_time"}):
        orderby_res = rest_client.get("/datasets", params=params)
        assert orderby_res.status_code == 200
        orderby_payload = orderby_res.json()
        assert orderby_payload["code"] == 0, orderby_payload

    for params in (
        {"orderby": ""},
        {"orderby": "unknown"},
        {"orderby": "CREATE_TIME"},
        {"orderby": "UPDATE_TIME"},
        {"orderby": " create_time "},
    ):
        orderby_invalid_res = rest_client.get("/datasets", params=params)
        assert orderby_invalid_res.status_code == 200
        orderby_invalid_payload = orderby_invalid_res.json()
        assert orderby_invalid_payload["code"] == 101, orderby_invalid_payload
        assert "Input should be 'create_time' or 'update_time'" in orderby_invalid_payload["message"], orderby_invalid_payload

    orderby_none_res = rest_client.get("/datasets", params={"orderby": None})
    assert orderby_none_res.status_code == 200
    orderby_none_payload = orderby_none_res.json()
    assert orderby_none_payload["code"] == 0, orderby_none_payload

    for params in (
        {"desc": True},
        {"desc": False},
        {"desc": "true"},
        {"desc": "false"},
        {"desc": 1},
        {"desc": 0},
        {"desc": "yes"},
        {"desc": "no"},
        {"desc": "y"},
        {"desc": "n"},
    ):
        desc_res = rest_client.get("/datasets", params=params)
        assert desc_res.status_code == 200
        desc_payload = desc_res.json()
        assert desc_payload["code"] == 0, desc_payload

    for params in ({"desc": 3.14}, {"desc": "unknown"}):
        desc_invalid_res = rest_client.get("/datasets", params=params)
        assert desc_invalid_res.status_code == 200
        desc_invalid_payload = desc_invalid_res.json()
        assert desc_invalid_payload["code"] == 101, desc_invalid_payload
        assert "Input should be a valid boolean, unable to interpret input" in desc_invalid_payload["message"], desc_invalid_payload

    desc_none_res = rest_client.get("/datasets", params={"desc": None})
    assert desc_none_res.status_code == 200
    desc_none_payload = desc_none_res.json()
    assert desc_none_payload["code"] == 0, desc_none_payload

    name_res = rest_client.get("/datasets", params={"name": "dataset_1"})
    assert name_res.status_code == 200
    name_payload = name_res.json()
    assert name_payload["code"] == 0, name_payload
    assert len(name_payload["data"]) == 1, name_payload
    assert name_payload["data"][0]["name"] == "dataset_1", name_payload

    name_wrong_res = rest_client.get("/datasets", params={"name": "wrong name"})
    assert name_wrong_res.status_code == 200
    name_wrong_payload = name_wrong_res.json()
    assert name_wrong_payload["code"] == 102, name_wrong_payload
    assert "lacks permission for dataset" in name_wrong_payload["message"], name_wrong_payload

    name_empty_res = rest_client.get("/datasets", params={"name": ""})
    assert name_empty_res.status_code == 200
    name_empty_payload = name_empty_res.json()
    assert name_empty_payload["code"] == 0, name_empty_payload
    assert len(name_empty_payload["data"]) == 5, name_empty_payload

    name_none_res = rest_client.get("/datasets", params={"name": None})
    assert name_none_res.status_code == 200
    name_none_payload = name_none_res.json()
    assert name_none_payload["code"] == 0, name_none_payload
    assert len(name_none_payload["data"]) == 5, name_none_payload

    id_res = rest_client.get("/datasets", params={"id": dataset_ids[0]})
    assert id_res.status_code == 200
    id_payload = id_res.json()
    assert id_payload["code"] == 0, id_payload
    assert len(id_payload["data"]) == 1, id_payload
    assert id_payload["data"][0]["id"] == dataset_ids[0], id_payload

    id_not_uuid_res = rest_client.get("/datasets", params={"id": "not_uuid"})
    assert id_not_uuid_res.status_code == 200
    id_not_uuid_payload = id_not_uuid_res.json()
    assert id_not_uuid_payload["code"] == 101, id_not_uuid_payload
    assert "Invalid UUID format" in id_not_uuid_payload["message"], id_not_uuid_payload

    id_not_uuid1_res = rest_client.get("/datasets", params={"id": uuid.uuid4().hex})
    assert id_not_uuid1_res.status_code == 200
    id_not_uuid1_payload = id_not_uuid1_res.json()
    assert id_not_uuid1_payload["code"] == 102, id_not_uuid1_payload
    assert "lacks permission for dataset" in id_not_uuid1_payload["message"], id_not_uuid1_payload

    id_wrong_uuid_res = rest_client.get("/datasets", params={"id": "d94a8dc02c9711f0930f7fbc369eab6d"})
    assert id_wrong_uuid_res.status_code == 200
    id_wrong_uuid_payload = id_wrong_uuid_res.json()
    assert id_wrong_uuid_payload["code"] == 102, id_wrong_uuid_payload
    assert "lacks permission for dataset" in id_wrong_uuid_payload["message"], id_wrong_uuid_payload

    id_empty_res = rest_client.get("/datasets", params={"id": ""})
    assert id_empty_res.status_code == 200
    id_empty_payload = id_empty_res.json()
    assert id_empty_payload["code"] == 101, id_empty_payload
    assert "Invalid UUID format" in id_empty_payload["message"], id_empty_payload

    id_none_res = rest_client.get("/datasets", params={"id": None})
    assert id_none_res.status_code == 200
    id_none_payload = id_none_res.json()
    assert id_none_payload["code"] == 0, id_none_payload
    assert len(id_none_payload["data"]) == 5, id_none_payload

    name_id_match_res = rest_client.get("/datasets", params={"id": dataset_ids[0], "name": "dataset_0"})
    assert name_id_match_res.status_code == 200
    name_id_match_payload = name_id_match_res.json()
    assert name_id_match_payload["code"] == 0, name_id_match_payload
    assert len(name_id_match_payload["data"]) == 1, name_id_match_payload

    name_id_mismatch_res = rest_client.get("/datasets", params={"id": dataset_ids[0], "name": "dataset_1"})
    assert name_id_mismatch_res.status_code == 200
    name_id_mismatch_payload = name_id_mismatch_res.json()
    assert name_id_mismatch_payload["code"] == 0, name_id_mismatch_payload
    assert len(name_id_mismatch_payload["data"]) == 0, name_id_mismatch_payload

    for dataset_id, name in ((dataset_ids[0], "wrong_name"), (uuid.uuid1().hex, "dataset_0")):
        name_id_wrong_res = rest_client.get("/datasets", params={"id": dataset_id, "name": name})
        assert name_id_wrong_res.status_code == 200
        name_id_wrong_payload = name_id_wrong_res.json()
        assert name_id_wrong_payload["code"] == 102, name_id_wrong_payload
        assert "lacks permission for dataset" in name_id_wrong_payload["message"], name_id_wrong_payload

    unsupported_field_res = rest_client.get("/datasets", params={"unknown_field": "unknown_field"})
    assert unsupported_field_res.status_code == 200
    unsupported_field_payload = unsupported_field_res.json()
    assert unsupported_field_payload["code"] == 101, unsupported_field_payload
    assert "Extra inputs are not permitted" in unsupported_field_payload["message"], unsupported_field_payload


@pytest.mark.p3
def test_dataset_list_concurrent_contract(rest_client, clear_datasets):
    for i in range(5):
        create_res = rest_client.post("/datasets", json={"name": f"dataset_list_concurrent_{i}"})
        assert create_res.status_code == 200
        create_payload = create_res.json()
        assert create_payload["code"] == 0, create_payload

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(rest_client.get, "/datasets") for _ in range(100)]
    responses = list(as_completed(futures))
    assert len(responses) == 100, responses
    for future in futures:
        res = future.result()
        assert res.status_code == 200, res.text
        payload = res.json()
        assert payload["code"] == 0, payload


@pytest.mark.p2
def test_dataset_get_contract(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_get_success")

    success_res = rest_client.get(f"/datasets/{dataset_id}")
    assert success_res.status_code == 200
    success_payload = success_res.json()
    assert success_payload["code"] == 0, success_payload
    assert success_payload["data"]["id"] == dataset_id, success_payload

    invalid_id_res = rest_client.get("/datasets/invalid_dataset_id")
    assert invalid_id_res.status_code == 200
    invalid_id_payload = invalid_id_res.json()
    assert invalid_id_payload["code"] != 0, invalid_id_payload

    unauthorized_res = RestClient(token=INVALID_API_TOKEN).get(f"/datasets/{dataset_id}")
    assert unauthorized_res.status_code == 401
    unauthorized_payload = unauthorized_res.json()
    assert unauthorized_payload["code"] == 401, unauthorized_payload

    nonexistent_res = rest_client.get(f"/datasets/{'0' * 32}")
    assert nonexistent_res.status_code == 200
    nonexistent_payload = nonexistent_res.json()
    assert nonexistent_payload["code"] != 0, nonexistent_payload


@pytest.mark.p2
def test_dataset_metadata_config_get_and_update_contract(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_metadata_config_contract")

    success_res = rest_client.get(f"/datasets/{dataset_id}/metadata/config")
    assert success_res.status_code == 200
    success_payload = success_res.json()
    assert success_payload["code"] == 0, success_payload
    expected_empty = {"enabled": False, "metadata": [], "built_in_metadata": []}
    assert success_payload["data"] == expected_empty, success_payload

    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        get_res = client.get(f"/datasets/{dataset_id}/metadata/config")
        assert get_res.status_code == 401, (scenario_name, get_res.text)
        get_payload = get_res.json()
        assert_auth_error(get_payload, scenario_name)

        put_res = client.put(
            f"/datasets/{dataset_id}/metadata/config",
            json={"metadata": [], "built_in_metadata": []},
        )
        assert put_res.status_code == 401, (scenario_name, put_res.text)
        put_payload = put_res.json()
        assert_auth_error(put_payload, scenario_name)

    invalid_dataset_res = rest_client.get("/datasets/invalid_dataset_id/metadata/config")
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] == 102, invalid_dataset_payload
    assert "lacks permission for dataset 'invalid_dataset_id'" in invalid_dataset_payload["message"], invalid_dataset_payload

    update_payload = {
        "metadata": [
            {"key": "author", "type": "string", "description": "Author name"},
            {"key": "tags", "type": "list", "description": "Tag list", "enum": ["foo", "bar"]},
        ],
        "built_in_metadata": [
            {"key": "size", "type": "number", "description": "File size"},
        ],
    }
    normalized_update_payload = {
        "metadata": [
            {"key": "author", "type": "string", "description": "Author name", "enum": None},
            {"key": "tags", "type": "list", "description": "Tag list", "enum": ["foo", "bar"]},
        ],
        "built_in_metadata": [
            {"key": "size", "type": "number", "description": "File size", "enum": None},
        ],
    }
    # Go keeps the dataset-level metadata.enabled flag: it is not derived
    # from whether fields are present, so a fresh dataset (flag disabled)
    # stays disabled until enabled explicitly.
    normalized_update_payload["enabled"] = False
    update_res = rest_client.put(f"/datasets/{dataset_id}/metadata/config", json=update_payload)
    assert update_res.status_code == 200
    update_body = update_res.json()
    assert update_body["code"] == 0, update_body
    assert update_body["data"] == normalized_update_payload, update_body

    refetch_res = rest_client.get(f"/datasets/{dataset_id}/metadata/config")
    assert refetch_res.status_code == 200
    refetch_payload = refetch_res.json()
    assert refetch_payload["code"] == 0, refetch_payload
    assert refetch_payload["data"] == normalized_update_payload, refetch_payload

    missing_payload_res = rest_client.put(f"/datasets/{dataset_id}/metadata/config", json={})
    assert missing_payload_res.status_code == 200
    missing_payload = missing_payload_res.json()
    assert missing_payload["code"] == 0, missing_payload
    assert missing_payload["data"] == expected_empty, missing_payload

    invalid_update_dataset_res = rest_client.put(
        "/datasets/invalid_dataset_id/metadata/config",
        json={"metadata": [], "built_in_metadata": []},
    )
    assert invalid_update_dataset_res.status_code == 200
    invalid_update_dataset_payload = invalid_update_dataset_res.json()
    assert invalid_update_dataset_payload["code"] == 102, invalid_update_dataset_payload
    assert "lacks permission for dataset 'invalid_dataset_id'" in invalid_update_dataset_payload["message"], invalid_update_dataset_payload


def test_dataset_metadata_summary_contract(rest_client, create_dataset, tmp_path):
    dataset_id = create_dataset("dataset_metadata_summary")
    document_ids = []
    for i in range(3):
        fp = create_txt_file(tmp_path / f"metadata_summary_{i}.txt")
        with fp.open("rb") as file_obj:
            upload_res = rest_client.post(
                f"/datasets/{dataset_id}/documents",
                files=[("file", (fp.name, file_obj))],
            )
        assert upload_res.status_code == 200
        upload_payload = upload_res.json()
        assert upload_payload["code"] == 0, upload_payload
        document_ids.append(upload_payload["data"][0]["id"])

    payloads = [
        {"tags": ["foo", "bar"], "author": "alice"},
        {"tags": ["foo"], "author": "bob"},
        {"tags": ["bar", "baz"], "author": ""},
    ]
    for document_id, meta_fields in zip(document_ids, payloads):
        update_res = rest_client.patch(
            f"/datasets/{dataset_id}/documents/{document_id}",
            json={"meta_fields": meta_fields},
        )
        assert update_res.status_code == 200
        update_payload = update_res.json()
        assert update_payload["code"] == 0, update_payload

    success_res = rest_client.get(f"/datasets/{dataset_id}/metadata/summary")
    assert success_res.status_code == 200
    success_payload = success_res.json()
    assert success_payload["code"] == 0, success_payload
    assert "summary" in success_payload["data"], success_payload

    summary = success_payload["data"]["summary"]
    counts = {}
    for key, field_data in summary.items():
        counts[key] = {str(k): v for k, v in field_data["values"]}
    assert counts["tags"]["foo"] == 2, counts
    assert counts["tags"]["bar"] == 2, counts
    assert counts["tags"]["baz"] == 1, counts
    assert counts["author"]["alice"] == 1, counts
    assert counts["author"]["bob"] == 1, counts
    assert "None" not in counts["author"], counts

    invalid_dataset_id = f"invalid_{dataset_id}"
    invalid_dataset_res = rest_client.get(f"/datasets/{invalid_dataset_id}/metadata/summary")
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] == 102, invalid_dataset_payload
    assert invalid_dataset_payload["message"] == f"You don't own the dataset {invalid_dataset_id}. ", invalid_dataset_payload

    nonexistent_res = rest_client.get(f"/datasets/{'0' * 32}/metadata/summary")
    assert nonexistent_res.status_code == 200
    nonexistent_payload = nonexistent_res.json()
    assert nonexistent_payload["code"] == 102, nonexistent_payload


@pytest.mark.p3
def test_dataset_search_endpoint(rest_client, ensure_parsed_document):
    dataset_id, _ = ensure_parsed_document()
    res = rest_client.post(
        f"/datasets/{dataset_id}/search",
        json={"question": "test TXT file", "page": 1, "page_size": 10},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert "chunks" in payload["data"], payload


@pytest.mark.p3
@pytest.mark.parametrize(
    "payload",
    [
        {"question": "test TXT file", "page": 1, "page_size": 2},
        {"question": "test TXT file", "similarity_threshold": 0.5},
        {"question": "test TXT file", "vector_similarity_weight": 0.7},
        {"question": "test TXT file", "top_k": 10},
    ],
    ids=["page_size", "similarity_threshold", "vector_similarity_weight", "top_k"],
)
def test_dataset_search_params_and_document_ids_contract(rest_client, ensure_parsed_document, payload):
    dataset_id, document_id = ensure_parsed_document()
    search_payload = dict(payload)
    search_payload["document_ids"] = [document_id]
    res = rest_client.post(f"/datasets/{dataset_id}/search", json=search_payload)
    assert res.status_code == 200
    body = res.json()
    assert body["code"] == 0, body
    assert "chunks" in body["data"], body


@pytest.mark.p2
def test_dataset_search_requires_question(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_search_missing_question")
    res = rest_client.post(f"/datasets/{dataset_id}/search", json={})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] in [101, 102], payload
    assert "question" in payload["message"], payload


@pytest.mark.p2
def test_dataset_tags_and_aggregation(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_tags")
    second_dataset_id = create_dataset("dataset_tags_second")

    # The aggregation route is asserted before the per-dataset tag routes,
    # which the Go API does not serve.
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

    # Go keeps no tag dataset, so the per-dataset tag routes are not served
    # at all; tagging is driven by the dataset's tag source file and only
    # the aggregation route survives. The 404s below replace the old 200
    # responses.
    assert rest_client.get(f"/datasets/{dataset_id}/tags").status_code == 404
    assert rest_client.get("/datasets/invalid_id/tags").status_code == 404


@pytest.mark.p2
def test_dataset_tags_delete_and_rename_validation(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_tag_mutation")

    # With no tag dataset there is nothing to rename or delete, so both the
    # validation cases and the invalid-dataset cases are unrouted instead of
    # argument errors.
    for url in (f"/datasets/{dataset_id}/tags", "/datasets/invalid_id/tags"):
        assert rest_client.delete(url, json={"tags": ["tag1"]}).status_code == 404
        assert rest_client.put(url, json={"from_tag": "old", "to_tag": "new"}).status_code == 404


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

    abnormal_date_filter_res = rest_client.get(
        f"/datasets/{dataset_id}/ingestions",
        params={
            "desc": "false",
            "create_date_from": "2025-02-01",
            "create_date_to": "2025-01-01",
        },
    )
    assert abnormal_date_filter_res.status_code == 200
    abnormal_date_filter_payload = abnormal_date_filter_res.json()
    assert abnormal_date_filter_payload["code"] == 0, abnormal_date_filter_payload
    assert abnormal_date_filter_payload["data"]["logs"] == [], abnormal_date_filter_payload

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

    run_no_docs_raptor = rest_client.post(
        f"/datasets/{dataset_id}/index",
        params={"type": "raptor"},
    )
    assert run_no_docs_raptor.status_code == 200
    run_no_docs_raptor_payload = run_no_docs_raptor.json()
    assert run_no_docs_raptor_payload["code"] == 102, run_no_docs_raptor_payload

    trace_no_task = rest_client.get(
        f"/datasets/{dataset_id}/index",
        params={"type": "graph"},
    )
    assert trace_no_task.status_code == 200
    trace_no_task_payload = trace_no_task.json()
    assert trace_no_task_payload["code"] == 0, trace_no_task_payload
    assert trace_no_task_payload["data"] == {}, trace_no_task_payload

    trace_invalid_type = rest_client.get(
        f"/datasets/{dataset_id}/index",
        params={"type": "invalid_type"},
    )
    assert trace_invalid_type.status_code == 200
    trace_invalid_type_payload = trace_invalid_type.json()
    assert trace_invalid_type_payload["code"] != 0, trace_invalid_type_payload

    delete_graph = rest_client.delete(f"/datasets/{dataset_id}/graph")
    assert delete_graph.status_code == 200
    delete_graph_payload = delete_graph.json()
    assert delete_graph_payload["code"] == 0, delete_graph_payload

    delete_invalid_type = rest_client.delete(f"/datasets/{dataset_id}/invalid_type")
    assert delete_invalid_type.status_code == 200
    delete_invalid_type_payload = delete_invalid_type.json()
    assert delete_invalid_type_payload["code"] != 0, delete_invalid_type_payload


@pytest.mark.p2
def test_dataset_graph_endpoint_contract(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_graph_contract")

    get_graph_res = rest_client.get(f"/datasets/{dataset_id}/graph")
    assert get_graph_res.status_code == 200
    get_graph_payload = get_graph_res.json()
    assert get_graph_payload["code"] == 0, get_graph_payload
    assert "graph" in get_graph_payload["data"], get_graph_payload
    assert "mind_map" in get_graph_payload["data"], get_graph_payload
    assert isinstance(get_graph_payload["data"]["graph"], dict), get_graph_payload
    assert isinstance(get_graph_payload["data"]["mind_map"], dict), get_graph_payload


@pytest.mark.p2
@pytest.mark.parametrize("index_type", ["graph", "raptor", "mindmap"])
def test_dataset_index_trace_and_delete_type_contract(rest_client, create_document, index_type):
    dataset_id, _ = create_document(f"dataset_index_trace_{index_type}.txt")

    run_index_res = rest_client.post(
        f"/datasets/{dataset_id}/index",
        params={"type": index_type},
    )
    assert run_index_res.status_code == 200
    run_index_payload = run_index_res.json()
    assert run_index_payload["code"] == 0, run_index_payload
    assert run_index_payload["data"].get("task_id"), run_index_payload

    trace_index_res = rest_client.get(
        f"/datasets/{dataset_id}/index",
        params={"type": index_type},
    )
    assert trace_index_res.status_code == 200
    trace_index_payload = trace_index_res.json()
    assert trace_index_payload["code"] == 0, trace_index_payload
    assert isinstance(trace_index_payload["data"], dict), trace_index_payload

    delete_index_res = rest_client.delete(f"/datasets/{dataset_id}/{index_type}")
    assert delete_index_res.status_code == 200
    delete_index_payload = delete_index_res.json()
    assert delete_index_payload["code"] == 0, delete_index_payload


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
def test_dataset_embedding_check_missing_embd_id(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_embedding_check_missing_embd_id")

    missing_embd_id_res = rest_client.post(f"/datasets/{dataset_id}/embedding/check", json={})
    assert missing_embd_id_res.status_code == 200
    missing_embd_id_payload = missing_embd_id_res.json()
    assert missing_embd_id_payload["code"] != 0, missing_embd_id_payload
