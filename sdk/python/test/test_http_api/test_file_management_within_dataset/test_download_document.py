#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

import json
from concurrent.futures import ThreadPoolExecutor

import pytest
from common import INVALID_API_TOKEN, batch_create_datasets, bulk_upload_documents, download_document, upload_documnets
from libs.auth import RAGFlowHttpApiAuth
from libs.utils import compare_by_hash
from requests import codes


class TestAuthorization:
    @pytest.mark.parametrize(
        "auth, expected_code, expected_message",
        [
            (None, 0, "`Authorization` can't be empty"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                109,
                "Authentication error: API key is invalid!",
            ),
        ],
    )
    def test_invalid_auth(self, get_dataset_id_and_document_ids, tmp_path, auth, expected_code, expected_message):
        dataset_id, document_ids = get_dataset_id_and_document_ids
        res = download_document(auth, dataset_id, document_ids[0], tmp_path / "ragflow_tes.txt")
        assert res.status_code == codes.ok
        with (tmp_path / "ragflow_tes.txt").open("r") as f:
            response_json = json.load(f)
        assert response_json["code"] == expected_code
        assert response_json["message"] == expected_message


@pytest.mark.usefixtures("clear_datasets")
@pytest.mark.parametrize(
    "generate_test_files",
    [
        "docx",
        "excel",
        "ppt",
        "image",
        "pdf",
        "txt",
        "md",
        "json",
        "eml",
        "html",
    ],
    indirect=True,
)
def test_file_type_validation(get_http_api_auth, generate_test_files, request):
    ids = batch_create_datasets(get_http_api_auth, 1)
    fp = generate_test_files[request.node.callspec.params["generate_test_files"]]
    res = upload_documnets(get_http_api_auth, ids[0], [fp])
    document_id = res["data"][0]["id"]

    res = download_document(
        get_http_api_auth,
        ids[0],
        document_id,
        fp.with_stem("ragflow_test_download"),
    )
    assert res.status_code == codes.ok
    assert compare_by_hash(
        fp,
        fp.with_stem("ragflow_test_download"),
    )


class TestDocumentDownload:
    @pytest.mark.parametrize(
        "document_id, expected_code, expected_message",
        [
            (
                "invalid_document_id",
                102,
                "The dataset not own the document invalid_document_id.",
            ),
        ],
    )
    def test_invalid_document_id(self, get_http_api_auth, get_dataset_id_and_document_ids, tmp_path, document_id, expected_code, expected_message):
        dataset_id, _ = get_dataset_id_and_document_ids
        res = download_document(
            get_http_api_auth,
            dataset_id,
            document_id,
            tmp_path / "ragflow_test_download_1.txt",
        )
        assert res.status_code == codes.ok
        with (tmp_path / "ragflow_test_download_1.txt").open("r") as f:
            response_json = json.load(f)
        assert response_json["code"] == expected_code
        assert response_json["message"] == expected_message

    @pytest.mark.parametrize(
        "dataset_id, expected_code, expected_message",
        [
            ("", 100, "<NotFound '404: Not Found'>"),
            (
                "invalid_dataset_id",
                102,
                "You do not own the dataset invalid_dataset_id.",
            ),
        ],
    )
    def test_invalid_dataset_id(self, get_http_api_auth, get_dataset_id_and_document_ids, tmp_path, dataset_id, expected_code, expected_message):
        _, document_ids = get_dataset_id_and_document_ids
        res = download_document(
            get_http_api_auth,
            dataset_id,
            document_ids[0],
            tmp_path / "ragflow_test_download_1.txt",
        )
        assert res.status_code == codes.ok
        with (tmp_path / "ragflow_test_download_1.txt").open("r") as f:
            response_json = json.load(f)
        assert response_json["code"] == expected_code
        assert response_json["message"] == expected_message

    def test_same_file_repeat(self, get_http_api_auth, get_dataset_id_and_document_ids, tmp_path, file_management_tmp_dir):
        num = 5
        dataset_id, document_ids = get_dataset_id_and_document_ids
        for i in range(num):
            res = download_document(
                get_http_api_auth,
                dataset_id,
                document_ids[0],
                tmp_path / f"ragflow_test_download_{i}.txt",
            )
            assert res.status_code == codes.ok
            assert compare_by_hash(
                file_management_tmp_dir / "ragflow_test_upload_0.txt",
                tmp_path / f"ragflow_test_download_{i}.txt",
            )


@pytest.mark.usefixtures("clear_datasets")
def test_concurrent_download(get_http_api_auth, tmp_path):
    document_count = 20
    ids = batch_create_datasets(get_http_api_auth, 1)
    document_ids = bulk_upload_documents(get_http_api_auth, ids[0], document_count, tmp_path)

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [
            executor.submit(
                download_document,
                get_http_api_auth,
                ids[0],
                document_ids[i],
                tmp_path / f"ragflow_test_download_{i}.txt",
            )
            for i in range(document_count)
        ]
    responses = [f.result() for f in futures]
    assert all(r.status_code == codes.ok for r in responses)
    for i in range(document_count):
        assert compare_by_hash(
            tmp_path / f"ragflow_test_upload_{i}.txt",
            tmp_path / f"ragflow_test_download_{i}.txt",
        )
