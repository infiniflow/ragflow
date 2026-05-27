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
import string
from contextlib import ExitStack

import pytest
import requests
from requests_toolbelt import MultipartEncoder
from test.testcases.configs import DOCUMENT_NAME_LIMIT, HOST_ADDRESS, INVALID_API_TOKEN, VERSION
from test.testcases.restful_api.helpers.client import RestClient
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


def _upload_files(rest_client, dataset_id, file_paths):
    with ExitStack() as stack:
        files = [("file", (fp.name, stack.enter_context(fp.open("rb")))) for fp in file_paths]
        return rest_client.post(f"/datasets/{dataset_id}/documents", files=files)


def _seed_documents(rest_client, create_dataset, tmp_path, count=5):
    dataset_id = create_dataset("dataset_list_contract")
    file_paths = [create_txt_file(tmp_path / f"ragflow_test_upload_{i}.txt") for i in range(count)]
    res = _upload_files(rest_client, dataset_id, file_paths)
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert len(payload["data"]) == count, payload
    return dataset_id, payload["data"]


def _assert_docs_sorted(docs, key, reverse):
    values = [doc.get(key) for doc in docs]
    assert values == sorted(values, reverse=reverse)


@pytest.mark.p1
def test_documents_list_requires_auth(create_dataset):
    dataset_id = create_dataset("dataset_list_auth")
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.get(f"/datasets/{dataset_id}/documents")
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)


@pytest.mark.p1
def test_documents_upload_requires_auth(create_dataset, tmp_path):
    dataset_id = create_dataset("dataset_upload_auth")
    fp = create_txt_file(tmp_path / "upload_auth.txt")
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        with fp.open("rb") as file_obj:
            res = client.post(
                f"/datasets/{dataset_id}/documents",
                files=[("file", (fp.name, file_obj))],
            )
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)


@pytest.mark.p2
def test_documents_list_default_concurrent_and_filters_contract(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents(rest_client, create_dataset, tmp_path)
    first_id = uploaded_docs[0]["id"]
    first_name = uploaded_docs[0]["name"]

    default_res = rest_client.get(f"/datasets/{dataset_id}/documents")
    assert default_res.status_code == 200
    default_payload = default_res.json()
    assert default_payload["code"] == 0, default_payload
    assert default_payload["data"]["total"] == 5, default_payload
    assert len(default_payload["data"]["docs"]) == 5, default_payload

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(rest_client.get, f"/datasets/{dataset_id}/documents") for _ in range(30)]
    responses = list(as_completed(futures))
    assert len(responses) == 30, responses
    assert all(f.result().json()["code"] == 0 for f in futures)

    for params, expected_code, expected_docs, expected_total in (
        ({"create_time_from": "9999999999000"}, 0, 0, 5),
        ({"create_time_to": "1"}, 0, 0, 5),
        ({"create_time_from": "0", "create_time_to": "9999999999000"}, 0, 5, 5),
        ({"keywords": None}, 0, 5, 5),
        ({"keywords": ""}, 0, 5, 5),
        ({"keywords": "0"}, 0, 1, 1),
        ({"keywords": "ragflow_test_upload"}, 0, 5, 5),
        ({"keywords": "unknown"}, 0, 0, 0),
        ({"name": None}, 0, 5, 5),
        ({"name": ""}, 0, 5, 5),
        ({"name": first_name}, 0, 1, 1),
        ({"id": None}, 0, 5, 5),
        ({"id": ""}, 0, 5, 5),
        ({"id": first_id}, 0, 1, 1),
        ({"id": first_id, "name": first_name}, 0, 1, 1),
        ({"id": first_id, "name": "ragflow_test_upload_1.txt"}, 0, 0, 0),
        ({"run": ["UNSTART"]}, 0, 5, 5),
    ):
        res = rest_client.get(f"/datasets/{dataset_id}/documents", params=params)
        assert res.status_code == 200, (params, res.text)
        payload = res.json()
        assert payload["code"] == expected_code, (params, payload)
        assert payload["data"]["total"] == expected_total, (params, payload)
        assert len(payload["data"]["docs"]) == expected_docs, (params, payload)


@pytest.mark.p2
def test_documents_list_error_and_sorting_contract(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents(rest_client, create_dataset, tmp_path)
    first_id = uploaded_docs[0]["id"]

    error_cases = [
        (
            "invalid dataset empty",
            "/datasets//documents",
            None,
            102,
            "lacks permission for dataset 'documents'",
        ),
        (
            "invalid dataset id",
            "/datasets/invalid_dataset_id/documents",
            None,
            102,
            "You don't own the dataset invalid_dataset_id.",
        ),
        (
            "invalid params ignored",
            f"/datasets/{dataset_id}/documents",
            {"a": "b"},
            0,
            "",
        ),
        (
            "metadata json invalid",
            f"/datasets/{dataset_id}/documents",
            {"metadata_condition": "{bad json"},
            102,
            "metadata_condition must be valid JSON",
        ),
        (
            "metadata json non-object",
            f"/datasets/{dataset_id}/documents",
            {"metadata_condition": "[1]"},
            102,
            "metadata_condition must be an object",
        ),
        (
            "name unknown",
            f"/datasets/{dataset_id}/documents",
            {"name": "unknown.txt"},
            102,
            "You don't own the document unknown.txt.",
        ),
        (
            "id unknown",
            f"/datasets/{dataset_id}/documents",
            {"id": "unknown.txt"},
            102,
            "You don't own the document unknown.txt.",
        ),
        (
            "name+id unknown name",
            f"/datasets/{dataset_id}/documents",
            {"id": first_id, "name": "unknown"},
            102,
            "You don't own the document unknown.",
        ),
        (
            "name+id unknown id",
            f"/datasets/{dataset_id}/documents",
            {"id": "id", "name": "ragflow_test_upload_0.txt"},
            102,
            "You don't own the document id.",
        ),
        (
            "run invalid",
            f"/datasets/{dataset_id}/documents",
            {"run": ["INVALID_STATUS"]},
            102,
            "Invalid filter run status conditions: INVALID_STATUS",
        ),
        (
            "orderby invalid",
            f"/datasets/{dataset_id}/documents",
            {"orderby": "unknown"},
            100,
            "Document' has no attribute 'unknown'",
        ),
        (
            "page invalid number",
            f"/datasets/{dataset_id}/documents",
            {"page": -1, "page_size": 2},
            100,
            "1064",
        ),
        (
            "page invalid type",
            f"/datasets/{dataset_id}/documents",
            {"page": "a", "page_size": 2},
            100,
            "invalid literal for int()",
        ),
        (
            "page_size invalid number",
            f"/datasets/{dataset_id}/documents",
            {"page_size": -1},
            100,
            "1064",
        ),
        (
            "page_size invalid type",
            f"/datasets/{dataset_id}/documents",
            {"page_size": "a"},
            100,
            "invalid literal for int()",
        ),
    ]
    for case_name, path, params, expected_code, expected_message in error_cases:
        res = rest_client.get(path, params=params)
        assert res.status_code == 200, (case_name, res.text)
        payload = res.json()
        assert payload["code"] == expected_code, (case_name, payload)
        assert expected_message in payload["message"], (case_name, payload)

    for params, expected_total in (
        ({"page": None, "page_size": 2}, 2),
        ({"page": 1, "page_size": 2}, 2),
        ({"page": 2, "page_size": 2}, 2),
        ({"page": 3, "page_size": 2}, 1),
        ({"page": "3", "page_size": 2}, 1),
        ({"page_size": None}, 5),
        ({"page_size": 1}, 1),
        ({"page_size": 6}, 5),
        ({"page_size": "1"}, 1),
    ):
        res = rest_client.get(f"/datasets/{dataset_id}/documents", params=params)
        assert res.status_code == 200, (params, res.text)
        payload = res.json()
        assert payload["code"] == 0, (params, payload)
        assert len(payload["data"]["docs"]) == expected_total, (params, payload)
        assert payload["data"]["total"] == 5, (params, payload)

    for params, expected_key, expected_desc in (
        ({"orderby": None}, "create_time", True),
        ({"orderby": "create_time"}, "create_time", True),
        ({"orderby": "update_time"}, "update_time", True),
        ({"orderby": "name", "desc": "False"}, "name", False),
        ({"desc": None}, "create_time", True),
        ({"desc": "true"}, "create_time", True),
        ({"desc": "True"}, "create_time", True),
        ({"desc": True}, "create_time", True),
        ({"desc": "false"}, "create_time", False),
        ({"desc": "False"}, "create_time", False),
        ({"desc": False}, "create_time", False),
        ({"desc": "False", "orderby": "update_time"}, "update_time", False),
        ({"desc": "unknown"}, "create_time", True),
    ):
        res = rest_client.get(f"/datasets/{dataset_id}/documents", params=params)
        assert res.status_code == 200, (params, res.text)
        payload = res.json()
        assert payload["code"] == 0, (params, payload)
        _assert_docs_sorted(payload["data"]["docs"], expected_key, expected_desc)


@pytest.mark.p2
def test_documents_upload_missing_file(rest_client, create_dataset):
    dataset_id = create_dataset("dataset_upload_missing")
    res = rest_client.post(f"/datasets/{dataset_id}/documents")
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload
    assert payload["message"] == "No file part!", payload


@pytest.mark.p2
def test_documents_upload_contract_matrix(rest_client, create_dataset, tmp_path):
    dataset_id = create_dataset("dataset_upload_contract")

    valid_res = _upload_files(rest_client, dataset_id, [create_txt_file(tmp_path / "ragflow_test.txt")])
    assert valid_res.status_code == 200
    valid_payload = valid_res.json()
    assert valid_payload["code"] == 0, valid_payload
    assert valid_payload["data"][0]["dataset_id"] == dataset_id, valid_payload
    assert valid_payload["data"][0]["name"] == "ragflow_test.txt", valid_payload

    for ext in ("docx", "xlsx", "pptx", "jpg", "pdf", "txt", "md", "json", "eml", "html"):
        fp = create_txt_file(tmp_path / f"ragflow_test_file_type.{ext}")
        res = _upload_files(rest_client, dataset_id, [fp])
        assert res.status_code == 200, (ext, res.text)
        payload = res.json()
        assert payload["code"] == 0, (ext, payload)
        assert payload["data"][0]["name"] == fp.name, (ext, payload)

    empty_fp = tmp_path / "empty.txt"
    empty_fp.touch()
    empty_res = _upload_files(rest_client, dataset_id, [empty_fp])
    assert empty_res.status_code == 200
    empty_payload = empty_res.json()
    assert empty_payload["code"] == 0, empty_payload
    assert empty_payload["data"][0]["size"] == 0, empty_payload

    duplicate_fp = create_txt_file(tmp_path / "duplicate.txt")
    duplicate_res = _upload_files(rest_client, dataset_id, [duplicate_fp, duplicate_fp])
    assert duplicate_res.status_code == 200
    duplicate_payload = duplicate_res.json()
    assert duplicate_payload["code"] == 0, duplicate_payload
    assert [x["name"] for x in duplicate_payload["data"]] == ["duplicate.txt", "duplicate(1).txt"], duplicate_payload

    for index in range(3):
        repeat_res = _upload_files(rest_client, dataset_id, [duplicate_fp])
        assert repeat_res.status_code == 200
        repeat_payload = repeat_res.json()
        assert repeat_payload["code"] == 0, (index, repeat_payload)
        expected_name = f"duplicate({index + 2}).txt"
        assert repeat_payload["data"][0]["name"] == expected_name, (index, repeat_payload)

    max_name_fp = create_txt_file(tmp_path / f"{'a' * (DOCUMENT_NAME_LIMIT - 4)}.txt")
    max_name_res = _upload_files(rest_client, dataset_id, [max_name_fp])
    assert max_name_res.status_code == 200
    max_name_payload = max_name_res.json()
    assert max_name_payload["code"] == 0, max_name_payload
    assert max_name_payload["data"][0]["name"] == max_name_fp.name, max_name_payload

    illegal_chars = '<>:"/\\|?*'
    safe_filename = string.punctuation.translate(str.maketrans({char: "_" for char in illegal_chars}))
    special_fp = tmp_path / f"{safe_filename}.txt"
    special_fp.write_text("Sample text content")
    special_res = _upload_files(rest_client, dataset_id, [special_fp])
    assert special_res.status_code == 200
    special_payload = special_res.json()
    assert special_payload["code"] == 0, special_payload
    assert special_payload["data"][0]["name"] == special_fp.name, special_payload

    multi_paths = [create_txt_file(tmp_path / f"ragflow_test_multi_{i}.txt") for i in range(20)]
    multi_res = _upload_files(rest_client, dataset_id, multi_paths)
    assert multi_res.status_code == 200
    multi_payload = multi_res.json()
    assert multi_payload["code"] == 0, multi_payload
    assert len(multi_payload["data"]) == 20, multi_payload

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [
            executor.submit(_upload_files, rest_client, dataset_id, [create_txt_file(tmp_path / f"parallel_upload_{i}.txt")])
            for i in range(20)
        ]
    responses = list(as_completed(futures))
    assert len(responses) == 20, responses
    assert all(f.result().json()["code"] == 0 for f in futures)


@pytest.mark.p2
def test_documents_upload_error_contract(rest_client, create_dataset, tmp_path):
    invalid_dataset_res = _upload_files(
        rest_client,
        "invalid_dataset_id",
        [create_txt_file(tmp_path / "invalid_dataset.txt")],
    )
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] == 102, invalid_dataset_payload
    assert invalid_dataset_payload["message"] == "Can't find the dataset with ID invalid_dataset_id!", invalid_dataset_payload

    for file_type in ("exe", "unknown"):
        bad_file = tmp_path / f"ragflow_test.{file_type}"
        bad_file.touch()
        res = _upload_files(rest_client, create_dataset(f"dataset_upload_unsupported_{file_type}"), [bad_file])
        assert res.status_code == 200, (file_type, res.text)
        payload = res.json()
        assert payload["code"] == 500, (file_type, payload)
        assert payload["message"] == f"ragflow_test.{file_type}: This type of file has not been supported yet!", (file_type, payload)

    dataset_id = create_dataset("dataset_upload_missing_empty")
    missing_res = rest_client.post(f"/datasets/{dataset_id}/documents")
    assert missing_res.status_code == 200
    missing_payload = missing_res.json()
    assert missing_payload["code"] == 101, missing_payload
    assert missing_payload["message"] == "No file part!", missing_payload

    fp = create_txt_file(tmp_path / "filename_empty.txt")
    with fp.open("rb") as file_obj:
        m = MultipartEncoder(fields=(("file", ("", file_obj)),))
        filename_empty_res = requests.post(
            url=f"{HOST_ADDRESS}/api/{VERSION}/datasets/{dataset_id}/documents",
            headers={"Content-Type": m.content_type, "Authorization": f"Bearer {rest_client.token}"},
            data=m,
            timeout=30,
        )
    assert filename_empty_res.status_code == 200
    filename_empty_payload = filename_empty_res.json()
    assert filename_empty_payload["code"] == 101, filename_empty_payload
    assert filename_empty_payload["message"] == "No file selected!", filename_empty_payload


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
