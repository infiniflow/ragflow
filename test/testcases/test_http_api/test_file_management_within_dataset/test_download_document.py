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
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from common import bulk_upload_documents, download_document, upload_documents
from configs import INVALID_API_TOKEN, INVALID_ID_32
from libs.auth import RAGFlowHttpApiAuth
from requests import codes
from utils import compare_by_hash


@pytest.mark.p1
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 0, "`Authorization` can't be empty"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                109,
                "Authentication error: API key is invalid!",
            ),
        ],
    )
    def test_invalid_auth(self, invalid_auth, tmp_path, expected_code, expected_message):
        res = download_document(invalid_auth, "dataset_id", "document_id", tmp_path / "ragflow_tes.txt")
        assert res.status_code == codes.ok
        with (tmp_path / "ragflow_tes.txt").open("r") as f:
            response_json = json.load(f)
        assert response_json["code"] == expected_code
        assert response_json["message"] == expected_message


@pytest.mark.p1
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
def test_file_type_validation(HttpApiAuth, add_dataset, generate_test_files, request):
    dataset_id = add_dataset
    fp = generate_test_files[request.node.callspec.params["generate_test_files"]]
    res = upload_documents(HttpApiAuth, dataset_id, [fp])
    document_id = res["data"][0]["id"]

    res = download_document(
        HttpApiAuth,
        dataset_id,
        document_id,
        fp.with_stem("ragflow_test_download"),
    )
    assert res.status_code == codes.ok
    assert compare_by_hash(
        fp,
        fp.with_stem("ragflow_test_download"),
    )


class TestDocumentDownload:
    @pytest.mark.p3
    @pytest.mark.parametrize(
        "document_id, expected_code, expected_message",
        [
            (
                INVALID_ID_32,
                102,
                f"The dataset not own the document {INVALID_ID_32}.",
            ),
        ],
    )
    def test_invalid_document_id(self, HttpApiAuth, add_documents, tmp_path, document_id, expected_code, expected_message):
        dataset_id, _ = add_documents
        res = download_document(
            HttpApiAuth,
            dataset_id,
            document_id,
            tmp_path / "ragflow_test_download_1.txt",
        )
        assert res.status_code == codes.ok
        with (tmp_path / "ragflow_test_download_1.txt").open("r") as f:
            response_json = json.load(f)
        assert response_json["code"] == expected_code
        assert response_json["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "dataset_id, expected_code, expected_message",
        [
            (
                INVALID_ID_32,
                102,
                f"You do not own the dataset {INVALID_ID_32}.",
            ),
        ],
    )
    def test_invalid_dataset_id(self, HttpApiAuth, add_documents, tmp_path, dataset_id, expected_code, expected_message):
        _, document_ids = add_documents
        res = download_document(
            HttpApiAuth,
            dataset_id,
            document_ids[0],
            tmp_path / "ragflow_test_download_1.txt",
        )
        assert res.status_code == codes.ok
        with (tmp_path / "ragflow_test_download_1.txt").open("r") as f:
            response_json = json.load(f)
        assert response_json["code"] == expected_code
        assert response_json["message"] == expected_message

    @pytest.mark.p3
    def test_same_file_repeat(self, HttpApiAuth, add_documents, tmp_path, ragflow_tmp_dir):
        num = 5
        dataset_id, document_ids = add_documents
        for i in range(num):
            res = download_document(
                HttpApiAuth,
                dataset_id,
                document_ids[0],
                tmp_path / f"ragflow_test_download_{i}.txt",
            )
            assert res.status_code == codes.ok
            assert compare_by_hash(
                ragflow_tmp_dir / "ragflow_test_upload_0.txt",
                tmp_path / f"ragflow_test_download_{i}.txt",
            )


@pytest.mark.p3
def test_concurrent_download(HttpApiAuth, add_dataset, tmp_path):
    count = 20
    dataset_id = add_dataset
    document_ids = bulk_upload_documents(HttpApiAuth, dataset_id, count, tmp_path)

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [
            executor.submit(
                download_document,
                HttpApiAuth,
                dataset_id,
                document_ids[i],
                tmp_path / f"ragflow_test_download_{i}.txt",
            )
            for i in range(count)
        ]
    responses = list(as_completed(futures))
    assert len(responses) == count, responses
    for i in range(count):
        assert compare_by_hash(
            tmp_path / f"ragflow_test_upload_{i}.txt",
            tmp_path / f"ragflow_test_download_{i}.txt",
        )
