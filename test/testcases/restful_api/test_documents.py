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
from pathlib import Path
import uuid

from openpyxl import Workbook
import pytest
import requests
from requests_toolbelt import MultipartEncoder
from test.testcases.configs import DEFAULT_PARSER_CONFIG, DOCUMENT_NAME_LIMIT, HOST_ADDRESS, INVALID_API_TOKEN, INVALID_ID_32, VERSION
from test.testcases.restful_api.helpers.client import RestClient
from test.testcases.utils import compare_by_hash
from test.testcases.utils.file_utils import (
    create_docx_file,
    create_eml_file,
    create_excel_file,
    create_html_file,
    create_image_file,
    create_json_file,
    create_md_file,
    create_pdf_file,
    create_ppt_file,
)
from utils import wait_for
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


def _upload_files(rest_client, dataset_id, file_paths, timeout=None):
    with ExitStack() as stack:
        files = [("file", (fp.name, stack.enter_context(fp.open("rb")))) for fp in file_paths]
        kwargs = {"files": files}
        if timeout is not None:
            kwargs["timeout"] = timeout
        return rest_client.post(f"/datasets/{dataset_id}/documents", **kwargs)


def _seed_documents(rest_client, create_dataset, tmp_path, count=5, timeout=None):
    dataset_id = create_dataset("dataset_list_contract")
    file_paths = [create_txt_file(tmp_path / f"ragflow_test_upload_{i}.txt") for i in range(count)]
    res = _upload_files(rest_client, dataset_id, file_paths, timeout=timeout)
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    assert len(payload["data"]) == count, payload
    return dataset_id, payload["data"]


def _seed_documents_for_update(rest_client, create_dataset, tmp_path):
    dataset_id = create_dataset("dataset_update_contract")
    file_paths = [
        create_txt_file(tmp_path / "ragflow_test_upload_0.txt"),
        create_txt_file(tmp_path / "ragflow_test_upload_1.txt"),
    ]
    res = _upload_files(rest_client, dataset_id, file_paths)
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 0, payload
    return dataset_id, payload["data"]


def _assert_docs_sorted(docs, key, reverse):
    values = [doc.get(key) for doc in docs]
    assert values == sorted(values, reverse=reverse)


@wait_for(200, 1, "Document parsing timeout in RESTful document tests")
def _wait_document_runs(rest_client, dataset_id, document_ids, expected_run="DONE"):
    res = rest_client.get(f"/datasets/{dataset_id}/documents", params={"page_size": max(100, len(document_ids))})
    if res.status_code != 200:
        return False
    payload = res.json()
    if payload["code"] != 0:
        return False
    docs = {doc["id"]: doc for doc in payload["data"]["docs"]}
    for doc_id in document_ids:
        doc = docs.get(doc_id)
        if not doc or doc.get("run") != expected_run:
            return False
    return True


def _download_document_to_file(rest_client, dataset_id, document_id, save_path):
    res = rest_client.get(f"/datasets/{dataset_id}/documents/{document_id}", timeout=60)
    if res.status_code == 200 and res.headers.get("Content-Type", "").startswith("application/octet-stream"):
        save_path.write_bytes(res.content)
    return res


def _create_table_excel(path, rows):
    wb = Workbook()
    ws = wb.active
    for ridx, row in enumerate(rows, start=1):
        for cidx, value in enumerate(row, start=1):
            ws.cell(row=ridx, column=cidx, value=value)
    wb.save(path)
    return path


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
def test_documents_update_requires_auth(create_document):
    dataset_id, document_id = create_document("update_auth_target.txt")
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.patch(
            f"/datasets/{dataset_id}/documents/{document_id}",
            json={"name": "updated_auth_target.txt"},
        )
        assert res.status_code == 401, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == 401, (scenario_name, body)
        assert body["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, body)


@pytest.mark.p2
def test_documents_update_name_contract(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents_for_update(rest_client, create_dataset, tmp_path)
    first_document_id = uploaded_docs[0]["id"]

    long_name = f"{'a' * (DOCUMENT_NAME_LIMIT - 4)}.txt"
    name_cases = [
        ("new_name.txt", 0, ""),
        (long_name, 0, ""),
        (0, 102, "Field: <name> - Message: <Input should be a valid string> - Value: <0>"),
        (None, 100, "AttributeError('NoneType' object has no attribute 'encode')"),
        ("", 101, "The extension of file can't be changed"),
        ("ragflow_test_upload_0", 101, "The extension of file can't be changed"),
        ("ragflow_test_upload_1.txt", 102, "Duplicated document name in the same dataset."),
        ("RAGFLOW_TEST_UPLOAD_1.TXT", 0, ""),
    ]
    for name, expected_code, expected_message in name_cases:
        res = rest_client.patch(
            f"/datasets/{dataset_id}/documents/{first_document_id}",
            json={"name": name},
        )
        assert res.status_code == 200, (name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (name, body)
        if expected_code == 0:
            assert body["data"]["name"] == name, (name, body)
            list_res = rest_client.get(f"/datasets/{dataset_id}/documents", params={"id": first_document_id})
            assert list_res.status_code == 200, (name, list_res.text)
            list_body = list_res.json()
            assert list_body["code"] == 0, (name, list_body)
            assert list_body["data"]["docs"][0]["name"] == name, (name, list_body)
        else:
            assert body["message"] == expected_message, (name, body)


@pytest.mark.p2
def test_documents_update_invalid_dataset_and_document_contract(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents_for_update(rest_client, create_dataset, tmp_path)
    first_document_id = uploaded_docs[0]["id"]

    invalid_dataset_res = rest_client.patch(
        f"/datasets/{INVALID_ID_32}/documents/{first_document_id}",
        json={"name": "new_name.txt"},
    )
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_body = invalid_dataset_res.json()
    assert invalid_dataset_body["code"] == 102, invalid_dataset_body
    assert "You don't own the dataset." in invalid_dataset_body["message"], invalid_dataset_body

    invalid_document_res = rest_client.patch(
        f"/datasets/{dataset_id}/documents/{INVALID_ID_32}",
        json={"name": "new_name.txt"},
    )
    assert invalid_document_res.status_code == 200
    invalid_document_body = invalid_document_res.json()
    assert invalid_document_body["code"] == 102, invalid_document_body
    assert invalid_document_body["message"] == "The dataset doesn't own the document.", invalid_document_body


@pytest.mark.p2
def test_documents_update_chunk_method_contract(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents_for_update(rest_client, create_dataset, tmp_path)
    first_document_id = uploaded_docs[0]["id"]

    chunk_method_cases = [
        ("naive", 0, ""),
        ("manual", 0, ""),
        ("qa", 0, ""),
        ("table", 0, ""),
        ("paper", 0, ""),
        ("book", 0, ""),
        ("laws", 0, ""),
        ("presentation", 0, ""),
        ("picture", 0, ""),
        ("one", 0, ""),
        ("knowledge_graph", 0, ""),
        ("email", 0, ""),
        ("tag", 0, ""),
        ("", 102, "`chunk_method` (empty string) is not valid"),
        (
            "other_chunk_method",
            102,
            "Field: <chunk_method> - Message: <`chunk_method` other_chunk_method doesn't exist> - Value: <other_chunk_method>",
        ),
    ]
    for chunk_method, expected_code, expected_message in chunk_method_cases:
        res = rest_client.patch(
            f"/datasets/{dataset_id}/documents/{first_document_id}",
            json={"chunk_method": chunk_method},
        )
        assert res.status_code == 200, (chunk_method, res.text)
        body = res.json()
        assert body["code"] == expected_code, (chunk_method, body)
        if expected_code == 0:
            list_res = rest_client.get(f"/datasets/{dataset_id}/documents", params={"id": first_document_id})
            assert list_res.status_code == 200, (chunk_method, list_res.text)
            list_body = list_res.json()
            assert list_body["code"] == 0, (chunk_method, list_body)
            assert list_body["data"]["docs"][0]["chunk_method"] == chunk_method, (chunk_method, list_body)
        else:
            assert body["message"] == expected_message, (chunk_method, body)


@pytest.mark.p2
def test_documents_update_meta_fields_contract(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents_for_update(rest_client, create_dataset, tmp_path)
    first_document_id = uploaded_docs[0]["id"]

    meta_fields_cases = [
        ({"test": "test"}, 0, ""),
        ({"author": "alice", "year": 2024}, 0, ""),
        ({"tags": ["tag1", "tag2"]}, 0, ""),
        ({"count": 42, "price": 19.99}, 0, ""),
        ("test", 102, "Field: <meta_fields> - Message: <Input should be a valid dictionary> - Value: <test>"),
        ([], 102, "Field: <meta_fields> - Message: <Input should be a valid dictionary> - Value: <[]>"),
        ({"tags": [{"x": {"a": "b"}}]}, 102, "Field: <meta_fields> - Message: <The type is not supported in list: [{'x': {'a': 'b'}}]> - Value: <{'tags': [{'x': {'a': 'b'}}]}>"),
        ({"tags": [{"x": 1}]}, 102, "Field: <meta_fields> - Message: <The type is not supported in list: [{'x': 1}]> - Value: <{'tags': [{'x': 1}]}>"),
        ({"obj": {"x": 1}}, 102, "Field: <meta_fields> - Message: <The type is not supported: {'x': 1}> - Value: <{'obj': {'x': 1}}>"),
        ({"tags": [2, 1]}, 0, ""),
    ]
    for meta_fields, expected_code, expected_message in meta_fields_cases:
        res = rest_client.patch(
            f"/datasets/{dataset_id}/documents/{first_document_id}",
            json={"meta_fields": meta_fields},
        )
        assert res.status_code == 200, (meta_fields, res.text)
        body = res.json()
        assert body["code"] == expected_code, (meta_fields, body)
        if expected_code == 0:
            list_res = rest_client.get(f"/datasets/{dataset_id}/documents", params={"id": first_document_id})
            assert list_res.status_code == 200, (meta_fields, list_res.text)
            list_body = list_res.json()
            assert list_body["code"] == 0, (meta_fields, list_body)
            assert list_body["data"]["docs"][0]["meta_fields"] == meta_fields, (meta_fields, list_body)
        else:
            assert expected_message in body["message"] or body["message"] == expected_message, (meta_fields, body)

    invalid_meta_doc_res = rest_client.patch(
        f"/datasets/{dataset_id}/documents/invalid_doc_id_12345678901234567890",
        json={"meta_fields": {"author": "alice"}},
    )
    assert invalid_meta_doc_res.status_code == 200
    invalid_meta_doc_body = invalid_meta_doc_res.json()
    assert invalid_meta_doc_body["code"] == 102, invalid_meta_doc_body
    assert "The dataset doesn't own the document." in invalid_meta_doc_body["message"], invalid_meta_doc_body


@pytest.mark.p2
def test_documents_update_invalid_field_and_guard_contract(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents_for_update(rest_client, create_dataset, tmp_path)
    first_document_id = uploaded_docs[0]["id"]

    strict_guard_cases = [
        ({"chunk_count": 1}, 102, "Can't change `chunk_count`."),
        ({"token_count": 1}, 102, "Can't change `token_count`."),
        ({"chunk_count": 100}, 102, "Can't change `chunk_count`."),
        ({"token_count": 100}, 102, "Can't change `token_count`."),
        ({"progress": 2.0}, 102, "Field: <progress> - Message: <Input should be less than or equal to 1> - Value: <2.0>"),
        ({"progress": 1.0}, 102, "Can't change `progress`."),
        ({"meta_fields": []}, 102, "Field: <meta_fields> - Message: <Input should be a valid dictionary> - Value: <[]>"),
    ]
    for payload, expected_code, expected_message in strict_guard_cases:
        res = rest_client.patch(
            f"/datasets/{dataset_id}/documents/{first_document_id}",
            json=payload,
        )
        assert res.status_code == 200, (payload, res.text)
        body = res.json()
        assert body["code"] == expected_code, (payload, body)
        assert expected_message in body["message"] or body["message"] == expected_message, (payload, body)

    legacy_invalid_field_cases = [
        {"create_date": "Fri, 14 Mar 2025 16:53:42 GMT"},
        {"create_time": 1},
        {"created_by": "ragflow_test"},
        {"dataset_id": "ragflow_test"},
        {"id": "ragflow_test"},
        {"location": "ragflow_test.txt"},
        {"process_begin_at": 1},
        {"process_duration": 1.0},
        {"progress_msg": "ragflow_test"},
        {"run": "ragflow_test"},
        {"size": 1},
        {"source_type": "ragflow_test"},
        {"thumbnail": "ragflow_test"},
        {"type": "ragflow_test"},
        {"update_date": "Fri, 14 Mar 2025 16:33:17 GMT"},
        {"update_time": 1},
    ]
    for payload in legacy_invalid_field_cases:
        res = rest_client.patch(
            f"/datasets/{dataset_id}/documents/{first_document_id}",
            json=payload,
        )
        assert res.status_code == 200, (payload, res.text)
        body = res.json()
        assert body["code"] in (0, 102), (payload, body)
        if body["code"] == 102:
            assert "invalid" in body["message"].lower(), (payload, body)
        else:
            assert "data" in body, (payload, body)


@pytest.mark.p2
def test_documents_update_parser_config_contract(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents_for_update(rest_client, create_dataset, tmp_path)
    first_document_id = uploaded_docs[0]["id"]
    default_parser_config_for_test = {
        "layout_recognize": "DeepDOC",
        "chunk_token_num": 512,
        "delimiter": "\n",
        "auto_keywords": 0,
        "auto_questions": 0,
        "html4excel": False,
        "topn_tags": 3,
        "raptor": {
            "use_raptor": True,
            "prompt": "Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize.",
            "max_token": 256,
            "threshold": 0.1,
            "max_cluster": 64,
            "random_seed": 0,
        },
        "graphrag": {
            "use_graphrag": True,
            "entity_types": ["organization", "person", "geo", "event", "category"],
            "method": "light",
            "batch_chunk_token_size": 4096,
        },
    }

    parser_cases = [
        ({}, 0, ""),
        (default_parser_config_for_test, 0, ""),
        ({"chunk_token_num": -1}, 102, "Field: <parser_config.chunk_token_num> - Message: <Input should be greater than or equal to 1> - Value: <-1>"),
        ({"chunk_token_num": 0}, 102, "Field: <parser_config.chunk_token_num> - Message: <Input should be greater than or equal to 1> - Value: <0>"),
        ({"chunk_token_num": 100000000}, 102, "Field: <parser_config.chunk_token_num> - Message: <Input should be less than or equal to 2048> - Value: <100000000>"),
        ({"chunk_token_num": 3.14}, 102, "Field: <parser_config.chunk_token_num> - Message: <Input should be a valid integer> - Value: <3.14>"),
        ({"chunk_token_num": "1024"}, 102, "Field: <parser_config.chunk_token_num> - Message: <Input should be a valid integer> - Value: <1024>"),
        ({"layout_recognize": "DeepDOC"}, 0, ""),
        ({"layout_recognize": "Naive"}, 0, ""),
        ({"html4excel": True}, 0, ""),
        ({"html4excel": False}, 0, ""),
        ({"html4excel": 1}, 102, "Field: <parser_config.html4excel> - Message: <Input should be a valid boolean> - Value: <1>"),
        ({"delimiter": ""}, 102, "Field: <parser_config.delimiter> - Message: <String should have at least 1 character> - Value: <>"),
        ({"delimiter": "`##`"}, 0, ""),
        ({"delimiter": 1}, 102, "Field: <parser_config.delimiter> - Message: <Input should be a valid string> - Value: <1>"),
        ({"task_page_size": -1}, 102, "Field: <parser_config.task_page_size> - Message: <Input should be greater than or equal to 1> - Value: <-1>"),
        ({"task_page_size": 0}, 102, "Field: <parser_config.task_page_size> - Message: <Input should be greater than or equal to 1> - Value: <0>"),
        ({"task_page_size": 100000000}, 0, ""),
        ({"task_page_size": 3.14}, 102, "Field: <parser_config.task_page_size> - Message: <Input should be a valid integer> - Value: <3.14>"),
        ({"task_page_size": "1024"}, 102, "Field: <parser_config.task_page_size> - Message: <Input should be a valid integer> - Value: <1024>"),
        ({"raptor": {"use_raptor": {"a": "b"}}}, 102, "Field: <parser_config.raptor.use_raptor> - Message: <Input should be a valid boolean> - Value: <{'a': 'b'}>"),
        ({"raptor": {"use_raptor": False}}, 0, ""),
        ({"invalid_key": "invalid_value"}, 102, "Field: <parser_config.invalid_key> - Message: <Extra inputs are not permitted> - Value: <invalid_value>"),
        ({"auto_keywords": -1}, 102, "Field: <parser_config.auto_keywords> - Message: <Input should be greater than or equal to 0> - Value: <-1>"),
        ({"auto_keywords": 32}, 0, ""),
        ({"auto_keywords": "1024"}, 102, "Field: <parser_config.auto_keywords> - Message: <Input should be a valid integer> - Value: <1024>"),
        ({"auto_keywords": 3.14}, 102, "Field: <parser_config.auto_keywords> - Message: <Input should be a valid integer> - Value: <3.14>"),
        ({"auto_questions": -1}, 102, "Field: <parser_config.auto_questions> - Message: <Input should be greater than or equal to 0> - Value: <-1>"),
        ({"auto_questions": 10}, 0, ""),
        ({"auto_questions": 3.14}, 102, "Field: <parser_config.auto_questions> - Message: <Input should be a valid integer> - Value: <3.14>"),
        ({"auto_questions": "1024"}, 102, "Field: <parser_config.auto_questions> - Message: <Input should be a valid integer> - Value: <1024>"),
        ({"topn_tags": -1}, 102, "Field: <parser_config.topn_tags> - Message: <Input should be greater than or equal to 1> - Value: <-1>"),
        ({"topn_tags": 10}, 0, ""),
        ({"topn_tags": 3.14}, 102, "Field: <parser_config.topn_tags> - Message: <Input should be a valid integer> - Value: <3.14>"),
        ({"topn_tags": "1024"}, 102, "Field: <parser_config.topn_tags> - Message: <Input should be a valid integer> - Value: <1024>"),
    ]
    for parser_config, expected_code, expected_message in parser_cases:
        res = rest_client.patch(
            f"/datasets/{dataset_id}/documents/{first_document_id}",
            json={"chunk_method": "naive", "parser_config": parser_config},
        )
        assert res.status_code == 200, (parser_config, res.text)
        body = res.json()
        assert body["code"] == expected_code, (parser_config, body)
        if expected_code == 0:
            list_res = rest_client.get(f"/datasets/{dataset_id}/documents", params={"id": first_document_id})
            assert list_res.status_code == 200, (parser_config, list_res.text)
            list_body = list_res.json()
            assert list_body["code"] == 0, (parser_config, list_body)
            doc_parser_config = list_body["data"]["docs"][0]["parser_config"]
            if parser_config == {}:
                assert doc_parser_config == DEFAULT_PARSER_CONFIG, (parser_config, list_body)
            else:
                for key, value in parser_config.items():
                    if isinstance(value, dict):
                        for sub_key, sub_value in value.items():
                            assert doc_parser_config[key][sub_key] == sub_value, (parser_config, list_body)
                    else:
                        assert doc_parser_config[key] == value, (parser_config, list_body)
        else:
            assert body["message"] == expected_message, (parser_config, body)


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
def test_documents_metadata_batch_update_contract(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents(rest_client, create_dataset, tmp_path, count=5)
    document_ids = [doc["id"] for doc in uploaded_docs]

    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.patch(
            f"/datasets/{dataset_id}/documents/metadatas",
            json={"selector": {"document_ids": document_ids[:1]}, "updates": [], "deletes": []},
        )
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)

    invalid_dataset_res = rest_client.patch(
        "/datasets/invalid_dataset_id/documents/metadatas",
        json={"selector": {"document_ids": []}, "updates": [], "deletes": []},
    )
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] == 102, invalid_dataset_payload
    assert invalid_dataset_payload["message"] == "You don't own the dataset invalid_dataset_id.", invalid_dataset_payload

    validation_cases = [
        ("selector not object", {"selector": [1], "updates": [], "deletes": []}, 102, "selector must be an object."),
        ("updates not list", {"selector": {}, "updates": {"key": "value"}, "deletes": []}, 102, "updates and deletes must be lists."),
        ("metadata condition not object", {"selector": {"metadata_condition": [1]}, "updates": [], "deletes": []}, 102, "metadata_condition must be an object."),
        ("document ids not list", {"selector": {"document_ids": "doc-1"}, "updates": [], "deletes": []}, 102, "document_ids must be a list."),
        ("update missing key", {"selector": {}, "updates": [{"key": ""}], "deletes": []}, 102, "Each update requires key and value."),
        ("delete missing key", {"selector": {}, "updates": [], "deletes": [{"x": "y"}]}, 102, "Each delete requires key."),
        (
            "document ids wrong dataset",
            {"selector": {"document_ids": ["doc-does-not-exist-1", "doc-does-not-exist-2"]}, "updates": [{"key": "author", "value": "test"}], "deletes": []},
            102,
            f"These documents do not belong to dataset {dataset_id}: ",
        ),
    ]
    for scenario_name, payload, expected_code, expected_message in validation_cases:
        res = rest_client.patch(f"/datasets/{dataset_id}/documents/metadatas", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if scenario_name == "document ids wrong dataset":
            assert body["message"].startswith(expected_message), (scenario_name, body)
            invalid_ids = set(body["message"][len(expected_message) :].split(", "))
            assert invalid_ids == {"doc-does-not-exist-1", "doc-does-not-exist-2"}, (scenario_name, body)
        else:
            assert body["message"] == expected_message, (scenario_name, body)

    update_by_ids_res = rest_client.patch(
        f"/datasets/{dataset_id}/documents/metadatas",
        json={
            "selector": {"document_ids": document_ids},
            "updates": [{"key": "author", "value": "test_author"}, {"key": "status", "value": "processed"}],
            "deletes": [],
        },
    )
    assert update_by_ids_res.status_code == 200
    update_by_ids_payload = update_by_ids_res.json()
    assert update_by_ids_payload["code"] == 0, update_by_ids_payload
    assert update_by_ids_payload["data"] == {"updated": 5, "matched_docs": 5}, update_by_ids_payload

    filtered_update_res = rest_client.patch(
        f"/datasets/{dataset_id}/documents/metadatas",
        json={
            "selector": {
                "document_ids": document_ids,
                "metadata_condition": {"conditions": [{"comparison_operator": "is", "name": "status", "value": "processed"}]},
            },
            "updates": [{"key": "author", "value": "filtered_author"}],
            "deletes": [],
        },
    )
    assert filtered_update_res.status_code == 200
    filtered_update_payload = filtered_update_res.json()
    assert filtered_update_payload["code"] == 0, filtered_update_payload
    assert filtered_update_payload["data"] == {"updated": 5, "matched_docs": 5}, filtered_update_payload

    delete_metadata_res = rest_client.patch(
        f"/datasets/{dataset_id}/documents/metadatas",
        json={
            "selector": {"document_ids": document_ids},
            "updates": [],
            "deletes": [{"key": "author"}],
        },
    )
    assert delete_metadata_res.status_code == 200
    delete_metadata_payload = delete_metadata_res.json()
    assert delete_metadata_payload["code"] == 0, delete_metadata_payload
    assert delete_metadata_payload["data"] == {"updated": 5, "matched_docs": 5}, delete_metadata_payload

    combined_res = rest_client.patch(
        f"/datasets/{dataset_id}/documents/metadatas",
        json={
            "selector": {"document_ids": document_ids},
            "updates": [{"key": "author", "value": "new_author"}],
            "deletes": [{"key": "status"}],
        },
    )
    assert combined_res.status_code == 200
    combined_payload = combined_res.json()
    assert combined_payload["code"] == 0, combined_payload
    assert combined_payload["data"] == {"updated": 5, "matched_docs": 5}, combined_payload

    empty_ids_res = rest_client.patch(
        f"/datasets/{dataset_id}/documents/metadatas",
        json={"selector": {"document_ids": []}, "updates": [{"key": "author", "value": "test"}], "deletes": []},
    )
    assert empty_ids_res.status_code == 200
    empty_ids_payload = empty_ids_res.json()
    assert empty_ids_payload["code"] == 0, empty_ids_payload
    assert empty_ids_payload["data"] == {"updated": 0, "matched_docs": 0}, empty_ids_payload

    no_match_res = rest_client.patch(
        f"/datasets/{dataset_id}/documents/metadatas",
        json={
            "selector": {
                "document_ids": document_ids,
                "metadata_condition": {"conditions": [{"comparison_operator": "is", "name": "nonexistent_key", "value": "nonexistent_value"}]},
            },
            "updates": [{"key": "author", "value": "test"}],
            "deletes": [],
        },
    )
    assert no_match_res.status_code == 200
    no_match_payload = no_match_res.json()
    assert no_match_payload["code"] == 0, no_match_payload
    assert no_match_payload["data"] == {"updated": 0, "matched_docs": 0}, no_match_payload


@pytest.mark.p2
def test_document_metadata_config_contract(rest_client, create_document):
    dataset_id, document_id = create_document("document_metadata_config_contract.txt")

    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.put(
            f"/datasets/{dataset_id}/documents/{document_id}/metadata/config",
            json={"metadata": {"author": "alice"}},
        )
        assert res.status_code == 401, (scenario_name, res.text)
        payload = res.json()
        assert payload["code"] == 401, (scenario_name, payload)
        assert payload["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, payload)

    missing_payload_res = rest_client.put(f"/datasets/{dataset_id}/documents/{document_id}/metadata/config", json={})
    assert missing_payload_res.status_code == 200
    missing_payload = missing_payload_res.json()
    assert missing_payload["code"] == 101, missing_payload
    assert missing_payload["message"] == "metadata is required", missing_payload

    invalid_dataset_res = rest_client.put(
        f"/datasets/{INVALID_ID_32}/documents/{document_id}/metadata/config",
        json={"metadata": {"author": "alice"}},
    )
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] == 102, invalid_dataset_payload
    assert invalid_dataset_payload["message"] == "You don't own the dataset.", invalid_dataset_payload

    invalid_document_res = rest_client.put(
        f"/datasets/{dataset_id}/documents/{INVALID_ID_32}/metadata/config",
        json={"metadata": {"author": "alice"}},
    )
    assert invalid_document_res.status_code == 200
    invalid_document_payload = invalid_document_res.json()
    assert invalid_document_payload["code"] == 102, invalid_document_payload
    assert invalid_document_payload["message"] == f"Document {INVALID_ID_32} not found in dataset {dataset_id}", invalid_document_payload

    update_payload = {"metadata": {"author": "alice", "tags": ["one", "two"]}}
    update_res = rest_client.put(
        f"/datasets/{dataset_id}/documents/{document_id}/metadata/config",
        json=update_payload,
    )
    assert update_res.status_code == 200
    update_body = update_res.json()
    assert update_body["code"] == 0, update_body
    parser_config = update_body["data"]["parser_config"]
    assert parser_config["metadata"] == update_payload["metadata"], update_body


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


@pytest.mark.p2
def test_documents_delete_contract_matrix(rest_client, create_dataset, tmp_path):
    scenarios = [
        ("empty object", lambda ids: {}, 102, "should either provide doc ids or set delete_all(true)", 3),
        ("empty ids", lambda ids: {"ids": []}, 102, "should either provide doc ids or set delete_all(true)", 3),
        ("invalid id only", lambda ids: {"ids": ["invalid_id"]}, 102, "These documents do not belong to dataset", 3),
        ("not json object", lambda ids: "not json", 101, "Invalid request payload: expected object, got str", 3),
        ("delete one", lambda ids: {"ids": ids[:1]}, 0, "", 2),
        ("delete all by ids", lambda ids: {"ids": ids}, 0, "", 0),
        ("delete_all flag", lambda ids: {"delete_all": True}, 0, "", 0),
    ]
    for scenario_name, payload_builder, expected_code, expected_message, expected_total in scenarios:
        dataset_id, uploaded_docs = _seed_documents(rest_client, create_dataset, tmp_path, count=3)
        document_ids = [doc["id"] for doc in uploaded_docs]
        payload = payload_builder(document_ids)

        res = rest_client.delete(f"/datasets/{dataset_id}/documents", json=payload)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_code != 0:
            assert expected_message in body["message"], (scenario_name, body)
        else:
            assert body["data"]["deleted"] in (len(document_ids), len(document_ids[:1])), (scenario_name, body)

        list_res = rest_client.get(f"/datasets/{dataset_id}/documents", params={"page_size": 10})
        assert list_res.status_code == 200, (scenario_name, list_res.text)
        list_payload = list_res.json()
        assert list_payload["code"] == 0, (scenario_name, list_payload)
        assert list_payload["data"]["total"] == expected_total, (scenario_name, list_payload)


@pytest.mark.p2
def test_documents_delete_requires_auth(rest_client, create_dataset, tmp_path):
    dataset_id = create_dataset("dataset_delete_auth")
    file_path = create_txt_file(tmp_path / "delete_auth_target.txt")
    with file_path.open("rb") as file_obj:
        upload_res = rest_client.post(f"/datasets/{dataset_id}/documents", files=[("file", (file_path.name, file_obj))])
    assert upload_res.status_code == 200
    upload_payload = upload_res.json()
    assert upload_payload["code"] == 0, upload_payload
    document_id = upload_payload["data"][0]["id"]

    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.delete(f"/datasets/{dataset_id}/documents", json={"ids": [document_id]})
        assert res.status_code == 401, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == 401, (scenario_name, body)
        assert body["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, body)


@pytest.mark.p2
def test_documents_delete_invalid_dataset_partial_duplicate_repeat_and_cross_dataset(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents(rest_client, create_dataset, tmp_path, count=3)
    document_ids = [doc["id"] for doc in uploaded_docs]
    other_dataset_id, other_docs = _seed_documents(rest_client, create_dataset, tmp_path, count=1)
    other_document_id = other_docs[0]["id"]

    invalid_dataset_res = rest_client.delete(
        "/datasets/invalid_dataset_id/documents",
        json={"ids": document_ids[:1]},
    )
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] == 102, invalid_dataset_payload
    assert "You don't own the dataset invalid_dataset_id." in invalid_dataset_payload["message"], invalid_dataset_payload

    partial_invalid_payloads = [
        {"ids": ["invalid_id"] + document_ids},
        {"ids": document_ids[:1] + ["invalid_id"] + document_ids[1:]},
        {"ids": document_ids + ["invalid_id"]},
    ]
    for payload in partial_invalid_payloads:
        res = rest_client.delete(f"/datasets/{dataset_id}/documents", json=payload)
        assert res.status_code == 200, (payload, res.text)
        body = res.json()
        assert body["code"] == 102, (payload, body)
        assert "These documents do not belong to dataset" in body["message"], (payload, body)

    cross_dataset_res = rest_client.delete(f"/datasets/{dataset_id}/documents", json={"ids": [other_document_id]})
    assert cross_dataset_res.status_code == 200
    cross_dataset_payload = cross_dataset_res.json()
    assert cross_dataset_payload["code"] == 102, cross_dataset_payload
    assert f"These documents do not belong to dataset {dataset_id}" in cross_dataset_payload["message"], cross_dataset_payload

    duplicate_res = rest_client.delete(f"/datasets/{dataset_id}/documents", json={"ids": document_ids + document_ids})
    assert duplicate_res.status_code == 200
    duplicate_payload = duplicate_res.json()
    assert duplicate_payload["code"] == 101, duplicate_payload
    assert "Field: <ids> - Message: <Duplicate ids:" in duplicate_payload["message"], duplicate_payload

    delete_once_res = rest_client.delete(f"/datasets/{dataset_id}/documents", json={"ids": document_ids})
    assert delete_once_res.status_code == 200
    delete_once_payload = delete_once_res.json()
    assert delete_once_payload["code"] == 0, delete_once_payload

    delete_twice_res = rest_client.delete(f"/datasets/{dataset_id}/documents", json={"ids": document_ids})
    assert delete_twice_res.status_code == 200
    delete_twice_payload = delete_twice_res.json()
    assert delete_twice_payload["code"] == 102, delete_twice_payload
    assert "or Document not found" in delete_twice_payload["message"], delete_twice_payload

    other_list_res = rest_client.get(f"/datasets/{other_dataset_id}/documents")
    assert other_list_res.status_code == 200
    other_list_payload = other_list_res.json()
    assert other_list_payload["code"] == 0, other_list_payload
    assert other_list_payload["data"]["total"] == 1, other_list_payload


@pytest.mark.p2
def test_documents_delete_concurrent_and_bulk_contract(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents(
        rest_client, create_dataset, tmp_path, count=60, timeout=120
    )
    document_ids = [doc["id"] for doc in uploaded_docs]

    with ThreadPoolExecutor(max_workers=8) as executor:
        futures = [
            executor.submit(
                rest_client.delete,
                f"/datasets/{dataset_id}/documents",
                json={"ids": [doc_id]},
            )
            for doc_id in document_ids
        ]
    _responses = list(as_completed(futures))
    assert len(_responses) == len(document_ids), _responses
    for future in futures:
        response = future.result()
        assert response.status_code == 200, response.text
        payload = response.json()
        assert payload["code"] == 0, payload

    list_after_concurrent = rest_client.get(f"/datasets/{dataset_id}/documents")
    assert list_after_concurrent.status_code == 200
    list_after_payload = list_after_concurrent.json()
    assert list_after_payload["code"] == 0, list_after_payload
    assert list_after_payload["data"]["total"] == 0, list_after_payload

    bulk_dataset_id, bulk_docs = _seed_documents(
        rest_client, create_dataset, tmp_path, count=120, timeout=120
    )
    bulk_ids = [doc["id"] for doc in bulk_docs]
    bulk_delete_res = rest_client.delete(
        f"/datasets/{bulk_dataset_id}/documents",
        json={"ids": bulk_ids},
        timeout=120,
    )
    assert bulk_delete_res.status_code == 200
    bulk_delete_payload = bulk_delete_res.json()
    assert bulk_delete_payload["code"] == 0, bulk_delete_payload
    assert bulk_delete_payload["data"]["deleted"] == 120, bulk_delete_payload

    bulk_list_res = rest_client.get(f"/datasets/{bulk_dataset_id}/documents")
    assert bulk_list_res.status_code == 200
    bulk_list_payload = bulk_list_res.json()
    assert bulk_list_payload["code"] == 0, bulk_list_payload
    assert bulk_list_payload["data"]["total"] == 0, bulk_list_payload


@pytest.mark.p2
def test_documents_parse_requires_auth(create_document):
    dataset_id, document_id = create_document("parse_auth_target.txt")
    payload = {"document_ids": [document_id]}
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.post(f"/datasets/{dataset_id}/documents/parse", json=payload)
        assert res.status_code == 401, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == 401, (scenario_name, body)
        assert body["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, body)


@pytest.mark.p2
def test_documents_parse_contract_matrix(rest_client, create_dataset, tmp_path):
    scenarios = [
        ("empty ids", lambda ids: {"document_ids": []}, 102, "`document_ids` is required"),
        ("invalid id", lambda ids: {"document_ids": ["invalid_id"]}, 102, "Documents not found: ['invalid_id']"),
        ("special invalid id", lambda ids: {"document_ids": ["\\n!?。；！？\"'"]}, 102, "Documents not found:"),
        ("not json object", lambda ids: "not json", 100, "object has no attribute"),
        ("parse one", lambda ids: {"document_ids": ids[:1]}, 0, ""),
        ("parse all", lambda ids: {"document_ids": ids}, 0, ""),
    ]
    for scenario_name, payload_builder, expected_code, expected_message in scenarios:
        dataset_id, uploaded_docs = _seed_documents(rest_client, create_dataset, tmp_path, count=3)
        doc_ids = [doc["id"] for doc in uploaded_docs]
        payload = payload_builder(doc_ids)

        res = rest_client.post(f"/datasets/{dataset_id}/documents/parse", json=payload, timeout=60)
        assert res.status_code == 200, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (scenario_name, body)
        if expected_code != 0:
            assert expected_message in body["message"], (scenario_name, body)
        else:
            target_ids = payload["document_ids"]
            _wait_document_runs(rest_client, dataset_id, target_ids, expected_run="DONE")
            detail_res = rest_client.get(f"/datasets/{dataset_id}/documents", params={"page_size": 10})
            detail_payload = detail_res.json()
            docs = {doc["id"]: doc for doc in detail_payload["data"]["docs"]}
            for doc_id in target_ids:
                doc = docs[doc_id]
                assert doc["run"] == "DONE", (scenario_name, doc)
                assert doc["process_begin_at"], (scenario_name, doc)
                assert doc["process_duration"] >= 0, (scenario_name, doc)
                assert doc["progress"] >= 0, (scenario_name, doc)
                assert "Task done" in doc["progress_msg"], (scenario_name, doc)


@pytest.mark.p2
def test_documents_parse_invalid_dataset_partial_duplicate_and_repeated(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents(rest_client, create_dataset, tmp_path, count=3)
    doc_ids = [doc["id"] for doc in uploaded_docs]

    for bad_dataset in ("", "invalid_dataset_id"):
        path = f"/datasets/{bad_dataset}/documents/parse" if bad_dataset else "/datasets//documents/parse"
        res = rest_client.post(path, json={"document_ids": doc_ids})
        assert res.status_code == 200, (bad_dataset, res.text)
        body = res.json()
        if bad_dataset == "":
            assert body["code"] == 100, (bad_dataset, body)
            assert "Method Not Allowed" in body["message"], (bad_dataset, body)
        else:
            assert body["code"] == 102, (bad_dataset, body)
            assert "You don't own the dataset" in body["message"], (bad_dataset, body)

    for payload in (
        {"document_ids": ["invalid_id"] + doc_ids},
        {"document_ids": doc_ids[:1] + ["invalid_id"] + doc_ids[1:]},
        {"document_ids": doc_ids + ["invalid_id"]},
    ):
        res = rest_client.post(f"/datasets/{dataset_id}/documents/parse", json=payload, timeout=60)
        assert res.status_code == 200, (payload, res.text)
        body = res.json()
        assert body["code"] == 102, (payload, body)
        assert body["message"] == "Documents not found: ['invalid_id']", (payload, body)

    duplicate_res = rest_client.post(
        f"/datasets/{dataset_id}/documents/parse",
        json={"document_ids": doc_ids + doc_ids},
        timeout=60,
    )
    assert duplicate_res.status_code == 200
    duplicate_payload = duplicate_res.json()
    assert duplicate_payload["code"] == 0, duplicate_payload
    assert duplicate_payload["data"]["success_count"] == len(doc_ids), duplicate_payload
    assert any("Duplicate document ids:" in err for err in duplicate_payload["data"].get("errors", [])), duplicate_payload
    _wait_document_runs(rest_client, dataset_id, doc_ids, expected_run="DONE")

    repeated_res = rest_client.post(f"/datasets/{dataset_id}/documents/parse", json={"document_ids": doc_ids}, timeout=60)
    assert repeated_res.status_code == 200
    repeated_payload = repeated_res.json()
    assert repeated_payload["code"] == 0, repeated_payload
    _wait_document_runs(rest_client, dataset_id, doc_ids, expected_run="DONE")


@pytest.mark.p2
def test_documents_parse_chunks_and_scaled_bulk_contract(rest_client, create_dataset, tmp_path):
    single_dataset_id, single_docs = _seed_documents(rest_client, create_dataset, tmp_path, count=1)
    single_doc_id = single_docs[0]["id"]
    parse_single_res = rest_client.post(
        f"/datasets/{single_dataset_id}/documents/parse",
        json={"document_ids": [single_doc_id]},
        timeout=60,
    )
    assert parse_single_res.status_code == 200
    parse_single_payload = parse_single_res.json()
    assert parse_single_payload["code"] == 0, parse_single_payload
    _wait_document_runs(rest_client, single_dataset_id, [single_doc_id], expected_run="DONE")

    chunk_res = rest_client.get(f"/datasets/{single_dataset_id}/documents/{single_doc_id}/chunks")
    assert chunk_res.status_code == 200, chunk_res.text
    chunk_payload = chunk_res.json()
    assert chunk_payload["code"] == 0, chunk_payload
    assert chunk_payload["data"]["doc"]["chunk_count"] > 0, chunk_payload
    assert len(chunk_payload["data"]["chunks"]) > 0, chunk_payload

    parse_bulk_dataset, parse_bulk_docs = _seed_documents(rest_client, create_dataset, tmp_path, count=20)
    parse_bulk_ids = [doc["id"] for doc in parse_bulk_docs]
    parse_bulk_res = rest_client.post(
        f"/datasets/{parse_bulk_dataset}/documents/parse",
        json={"document_ids": parse_bulk_ids},
        timeout=60,
    )
    assert parse_bulk_res.status_code == 200
    parse_bulk_payload = parse_bulk_res.json()
    assert parse_bulk_payload["code"] == 0, parse_bulk_payload
    _wait_document_runs(rest_client, parse_bulk_dataset, parse_bulk_ids, expected_run="DONE")

    concurrent_dataset, concurrent_docs = _seed_documents(rest_client, create_dataset, tmp_path, count=20)
    concurrent_ids = [doc["id"] for doc in concurrent_docs]
    with ThreadPoolExecutor(max_workers=8) as executor:
        futures = [
            executor.submit(
                rest_client.post,
                f"/datasets/{concurrent_dataset}/documents/parse",
                json={"document_ids": [doc_id]},
                timeout=60,
            )
            for doc_id in concurrent_ids
        ]
    _responses = list(as_completed(futures))
    assert len(_responses) == len(concurrent_ids), _responses
    for future in futures:
        response = future.result()
        assert response.status_code == 200, response.text
        payload = response.json()
        assert payload["code"] == 0, payload
    _wait_document_runs(rest_client, concurrent_dataset, concurrent_ids, expected_run="DONE")


@pytest.mark.p2
def test_documents_stop_parse_requires_auth(rest_client, create_document):
    dataset_id, document_id = create_document("stop_parse_auth_target.txt")
    parse_res = rest_client.post(
        f"/datasets/{dataset_id}/documents/parse",
        json={"document_ids": [document_id]},
    )
    assert parse_res.status_code == 200
    assert parse_res.json()["code"] == 0, parse_res.json()
    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.post(f"/datasets/{dataset_id}/documents/stop", json={"document_ids": [document_id]})
        assert res.status_code == 401, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == 401, (scenario_name, body)
        assert body["message"] == "<Unauthorized '401: Unauthorized'>", (scenario_name, body)


@pytest.mark.p2
def test_documents_stop_parse_contract_matrix(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents(rest_client, create_dataset, tmp_path, count=6)
    doc_ids = [doc["id"] for doc in uploaded_docs]

    parse_res = rest_client.post(f"/datasets/{dataset_id}/documents/parse", json={"document_ids": doc_ids}, timeout=60)
    assert parse_res.status_code == 200
    parse_payload = parse_res.json()
    assert parse_payload["code"] == 0, parse_payload

    invalid_payloads = [
        ("empty ids", {"document_ids": []}, 102, "`document_ids` is required"),
        ("invalid id", {"document_ids": ["invalid_id"]}, 102, "Documents not found: ['invalid_id']"),
        ("special invalid id", {"document_ids": ["\\n!?。；！？\"'"]}, 102, "Documents not found:"),
        ("not json object", "not json", 100, "object has no attribute"),
    ]
    for case_name, payload, expected_code, expected_message in invalid_payloads:
        res = rest_client.post(f"/datasets/{dataset_id}/documents/stop", json=payload, timeout=60)
        assert res.status_code == 200, (case_name, res.text)
        body = res.json()
        assert body["code"] == expected_code, (case_name, body)
        assert expected_message in body["message"], (case_name, body)

    stop_subset_res = rest_client.post(f"/datasets/{dataset_id}/documents/stop", json={"document_ids": doc_ids[:3]}, timeout=60)
    assert stop_subset_res.status_code == 200
    stop_subset_payload = stop_subset_res.json()
    assert stop_subset_payload["code"] == 0, stop_subset_payload
    assert stop_subset_payload["data"]["success_count"] >= 0, stop_subset_payload

    duplicate_stop_res = rest_client.post(
        f"/datasets/{dataset_id}/documents/stop",
        json={"document_ids": doc_ids[:3] + doc_ids[:3]},
        timeout=60,
    )
    assert duplicate_stop_res.status_code == 200
    duplicate_stop_payload = duplicate_stop_res.json()
    assert duplicate_stop_payload["code"] == 0, duplicate_stop_payload
    assert any("Duplicate document ids:" in err for err in duplicate_stop_payload["data"].get("errors", [])), duplicate_stop_payload

    repeated_stop_res = rest_client.post(f"/datasets/{dataset_id}/documents/stop", json={"document_ids": doc_ids[:3]}, timeout=60)
    assert repeated_stop_res.status_code == 200
    repeated_stop_payload = repeated_stop_res.json()
    assert repeated_stop_payload["code"] in (0, 102), repeated_stop_payload
    if repeated_stop_payload["code"] == 102:
        assert "Can't stop parsing document that has not started or already completed" in repeated_stop_payload["message"], repeated_stop_payload


@pytest.mark.p2
def test_documents_stop_parse_invalid_dataset_partial_and_scaled_concurrency(rest_client, create_dataset, tmp_path):
    dataset_id, uploaded_docs = _seed_documents(rest_client, create_dataset, tmp_path, count=25)
    doc_ids = [doc["id"] for doc in uploaded_docs]

    parse_res = rest_client.post(f"/datasets/{dataset_id}/documents/parse", json={"document_ids": doc_ids}, timeout=60)
    assert parse_res.status_code == 200
    assert parse_res.json()["code"] == 0, parse_res.json()

    for bad_dataset in ("", "invalid_dataset_id"):
        path = f"/datasets/{bad_dataset}/documents/stop" if bad_dataset else "/datasets//documents/stop"
        res = rest_client.post(path, json={"document_ids": doc_ids[:1]})
        assert res.status_code == 200, (bad_dataset, res.text)
        body = res.json()
        if bad_dataset == "":
            assert body["code"] == 100, (bad_dataset, body)
            assert "Method Not Allowed" in body["message"], (bad_dataset, body)
        else:
            assert body["code"] == 102, (bad_dataset, body)
            assert "You don't own the dataset" in body["message"], (bad_dataset, body)

    for payload in (
        {"document_ids": ["invalid_id"] + doc_ids[:3]},
        {"document_ids": doc_ids[:1] + ["invalid_id"] + doc_ids[1:3]},
        {"document_ids": doc_ids[:3] + ["invalid_id"]},
    ):
        res = rest_client.post(f"/datasets/{dataset_id}/documents/stop", json=payload, timeout=60)
        assert res.status_code == 200, (payload, res.text)
        body = res.json()
        assert body["code"] == 102, (payload, body)
        assert body["message"] == "Documents not found: ['invalid_id']", (payload, body)

    with ThreadPoolExecutor(max_workers=8) as executor:
        futures = [
            executor.submit(
                rest_client.post,
                f"/datasets/{dataset_id}/documents/stop",
                json={"document_ids": [doc_id]},
                timeout=60,
            )
            for doc_id in doc_ids
        ]
    responses = [future.result() for future in futures]
    assert len(responses) == len(doc_ids), responses
    assert all(res.status_code == 200 for res in responses)
    assert all(res.json()["code"] == 0 for res in responses)

    stop_all_res = rest_client.post(f"/datasets/{dataset_id}/documents/stop", json={"document_ids": doc_ids}, timeout=60)
    assert stop_all_res.status_code == 200
    stop_all_payload = stop_all_res.json()
    assert stop_all_payload["code"] == 0, stop_all_payload
    assert stop_all_payload["data"]["success_count"] >= 0, stop_all_payload


@pytest.mark.p2
def test_documents_download_requires_auth_and_invalid_id_contract(rest_client, create_document, tmp_path):
    dataset_id, document_id = create_document("download_target.txt")

    for scenario_name, client in (("missing token", RestClient(token=None)), ("invalid token", RestClient(token=INVALID_API_TOKEN))):
        res = client.get(f"/datasets/{dataset_id}/documents/{document_id}")
        assert res.status_code == 401, (scenario_name, res.text)
        body = res.json()
        assert body["code"] == 401, (scenario_name, body)

    invalid_doc_path = tmp_path / "invalid_doc_download.txt"
    invalid_doc_res = _download_document_to_file(rest_client, dataset_id, "invalid_document_id", invalid_doc_path)
    assert invalid_doc_res.status_code == 200
    invalid_doc_payload = invalid_doc_res.json()
    assert invalid_doc_payload["code"] == 102, invalid_doc_payload
    assert "The dataset not own the document invalid_document_id." in invalid_doc_payload["message"], invalid_doc_payload

    invalid_dataset_path = tmp_path / "invalid_dataset_download.txt"
    invalid_dataset_res = _download_document_to_file(rest_client, "invalid_dataset_id", document_id, invalid_dataset_path)
    assert invalid_dataset_res.status_code == 200
    invalid_dataset_payload = invalid_dataset_res.json()
    assert invalid_dataset_payload["code"] == 102, invalid_dataset_payload
    assert f"The dataset not own the document {document_id}." in invalid_dataset_payload["message"], invalid_dataset_payload


@pytest.mark.p2
def test_documents_download_filetype_repeat_and_concurrent_contract(rest_client, create_dataset, tmp_path):
    dataset_id = create_dataset("dataset_download_contract")

    creators = {
        "docx": create_docx_file,
        "xlsx": create_excel_file,
        "pptx": create_ppt_file,
        "png": create_image_file,
        "pdf": create_pdf_file,
        "txt": create_txt_file,
        "md": create_md_file,
        "json": create_json_file,
        "eml": create_eml_file,
        "html": create_html_file,
    }

    uploaded = []
    for ext, creator in creators.items():
        source_path = Path(creator(tmp_path / f"download_type_{ext}.{ext}"))
        with source_path.open("rb") as file_obj:
            upload_res = rest_client.post(
                f"/datasets/{dataset_id}/documents",
                files=[("file", (source_path.name, file_obj))],
            )
        assert upload_res.status_code == 200, (ext, upload_res.text)
        upload_payload = upload_res.json()
        assert upload_payload["code"] == 0, (ext, upload_payload)
        uploaded.append((source_path, upload_payload["data"][0]["id"]))

    for source_path, document_id in uploaded:
        target_path = tmp_path / f"download_once_{source_path.name}"
        download_res = _download_document_to_file(rest_client, dataset_id, document_id, target_path)
        assert download_res.status_code == 200, (source_path.name, download_res.text)
        assert compare_by_hash(source_path, target_path), source_path.name

    first_source, first_document_id = uploaded[0]
    for index in range(5):
        repeated_path = tmp_path / f"download_repeat_{index}_{first_source.name}"
        repeated_res = _download_document_to_file(rest_client, dataset_id, first_document_id, repeated_path)
        assert repeated_res.status_code == 200, (index, repeated_res.text)
        assert compare_by_hash(first_source, repeated_path), index

    with ThreadPoolExecutor(max_workers=8) as executor:
        futures = [
            executor.submit(
                _download_document_to_file,
                rest_client,
                dataset_id,
                document_id,
                tmp_path / f"download_concurrent_{i}_{source_path.name}",
            )
            for i, (source_path, document_id) in enumerate(uploaded)
        ]
    _responses = list(as_completed(futures))
    assert len(_responses) == len(uploaded), _responses
    for i, (source_path, _document_id) in enumerate(uploaded):
        downloaded_path = tmp_path / f"download_concurrent_{i}_{source_path.name}"
        assert downloaded_path.exists(), source_path.name
        assert compare_by_hash(source_path, downloaded_path), source_path.name


@pytest.mark.p2
def test_documents_table_parser_chat_patterns(rest_client, clear_datasets, tmp_path):
    create_dataset_res = rest_client.post(
        "/datasets",
        json={"name": f"table_parser_dataset_contract_{uuid.uuid4().hex[:8]}", "chunk_method": "table"},
    )
    assert create_dataset_res.status_code == 200
    create_dataset_payload = create_dataset_res.json()
    assert create_dataset_payload["code"] == 0, create_dataset_payload
    dataset_id = create_dataset_payload["data"]["id"]

    excel_a = _create_table_excel(
        tmp_path / "table_a.xlsx",
        [
            ["employee_id", "name", "department", "salary"],
            ["E001", "Alice Johnson", "Engineering", "95000"],
            ["E002", "Bob Smith", "Marketing", "65000"],
            ["E003", "Carol Williams", "Engineering", "88000"],
        ],
    )
    excel_b = _create_table_excel(
        tmp_path / "table_b.xlsx",
        [
            ["product", "price", "category"],
            ["Laptop", "999", "Electronics"],
            ["Keyboard", "79", "Electronics"],
            ["Desk", "299", "Furniture"],
        ],
    )
    with ExitStack() as stack:
        files = [
            ("file", (excel_a.name, stack.enter_context(excel_a.open("rb")))),
            ("file", (excel_b.name, stack.enter_context(excel_b.open("rb")))),
        ]
        upload_res = rest_client.post(f"/datasets/{dataset_id}/documents", files=files)
    assert upload_res.status_code == 200
    upload_payload = upload_res.json()
    assert upload_payload["code"] == 0, upload_payload
    document_ids = [doc["id"] for doc in upload_payload["data"]]

    parse_res = rest_client.post(
        f"/datasets/{dataset_id}/documents/parse",
        json={"document_ids": document_ids},
        timeout=60,
    )
    assert parse_res.status_code == 200
    parse_payload = parse_res.json()
    assert parse_payload["code"] == 0, parse_payload
    _wait_document_runs(rest_client, dataset_id, document_ids, expected_run="DONE")

    chat_payload = {
        "name": f"table_parser_chat_{uuid.uuid4().hex[:8]}",
        "dataset_ids": [dataset_id],
        "prompt_config": {
            "system": "Use table knowledge to answer questions.",
            "parameters": [{"key": "knowledge", "optional": True, "value": "Answer with table evidence."}],
        },
    }
    create_chat_res = rest_client.post("/chats", json=chat_payload)
    assert create_chat_res.status_code == 200
    create_chat_payload = create_chat_res.json()
    assert create_chat_payload["code"] == 0, create_chat_payload
    chat_id = create_chat_payload["data"]["id"]

    create_session_res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "table_parser_session"})
    assert create_session_res.status_code == 200
    create_session_payload = create_session_res.json()
    assert create_session_payload["code"] == 0, create_session_payload
    session_id = create_session_payload["data"]["id"]

    questions = [
        "show me column of product",
        "which product has price 79",
        "How many rows in the dataset?",
        "Show me all employees in Engineering department",
    ]
    for question in questions:
        completion_res = rest_client.post(
            "/chat/completions",
            json={
                "chat_id": chat_id,
                "session_id": session_id,
                "messages": [{"role": "user", "content": question}],
                "stream": False,
            },
            timeout=60,
        )
        assert completion_res.status_code == 200, (question, completion_res.text)
        completion_payload = completion_res.json()
        assert completion_payload["code"] == 0, (question, completion_payload)
        answer = completion_payload["data"]["answer"]
        assert isinstance(answer, str) and answer.strip(), (question, completion_payload)
