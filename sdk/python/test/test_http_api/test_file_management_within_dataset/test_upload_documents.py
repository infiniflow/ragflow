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

import string
from concurrent.futures import ThreadPoolExecutor

import pytest
import requests
from common import (
    DOCUMENT_NAME_LIMIT,
    FILE_API_URL,
    HOST_ADDRESS,
    INVALID_API_TOKEN,
    create_datasets,
    list_dataset,
    upload_documnets,
)
from libs.auth import RAGFlowHttpApiAuth
from libs.utils.file_utils import create_txt_file
from requests_toolbelt import MultipartEncoder


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
    def test_invalid_auth(
        self, get_http_api_auth, auth, expected_code, expected_message
    ):
        ids = create_datasets(get_http_api_auth, 1)
        res = upload_documnets(auth, ids[0])
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestUploadDocuments:
    def test_valid_single_upload(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documnets(get_http_api_auth, ids[0], [fp])
        assert res["code"] == 0
        assert res["data"][0]["dataset_id"] == ids[0]
        assert res["data"][0]["name"] == fp.name

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
    def test_file_type_validation(
        self, get_http_api_auth, generate_test_files, request
    ):
        ids = create_datasets(get_http_api_auth, 1)
        fp = generate_test_files[request.node.callspec.params["generate_test_files"]]
        res = upload_documnets(get_http_api_auth, ids[0], [fp])
        assert res["code"] == 0
        assert res["data"][0]["dataset_id"] == ids[0]
        assert res["data"][0]["name"] == fp.name

    @pytest.mark.parametrize(
        "file_type",
        ["exe", "unknown"],
    )
    def test_unsupported_file_type(self, get_http_api_auth, tmp_path, file_type):
        ids = create_datasets(get_http_api_auth, 1)
        fp = tmp_path / f"ragflow_test.{file_type}"
        fp.touch()
        res = upload_documnets(get_http_api_auth, ids[0], [fp])
        assert res["code"] == 500
        assert (
            res["message"]
            == f"ragflow_test.{file_type}: This type of file has not been supported yet!"
        )

    def test_missing_file(self, get_http_api_auth):
        ids = create_datasets(get_http_api_auth, 1)
        res = upload_documnets(get_http_api_auth, ids[0])
        assert res["code"] == 101
        assert res["message"] == "No file part!"

    def test_empty_file(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)
        fp = tmp_path / "empty.txt"
        fp.touch()

        res = upload_documnets(get_http_api_auth, ids[0], [fp])
        assert res["code"] == 0
        assert res["data"][0]["size"] == 0

    def test_filename_empty(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        url = f"{HOST_ADDRESS}{FILE_API_URL}".format(dataset_id=ids[0])
        fields = (("file", ("", fp.open("rb"))),)
        m = MultipartEncoder(fields=fields)
        res = requests.post(
            url=url,
            headers={"Content-Type": m.content_type},
            auth=get_http_api_auth,
            data=m,
        )
        assert res.json()["code"] == 101
        assert res.json()["message"] == "No file selected!"

    def test_filename_exceeds_max_length(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)
        # filename_length = 129
        fp = create_txt_file(tmp_path / f"{'a' * (DOCUMENT_NAME_LIMIT - 3)}.txt")
        res = upload_documnets(get_http_api_auth, ids[0], [fp])
        assert res["code"] == 101
        assert (
            res["message"].find("128") >= 0
        )

    def test_invalid_dataset_id(self, get_http_api_auth, tmp_path):
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documnets(get_http_api_auth, "invalid_dataset_id", [fp])
        assert res["code"] == 100
        assert (
            res["message"]
            == """LookupError("Can\'t find the dataset with ID invalid_dataset_id!")"""
        )

    def test_duplicate_files(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documnets(get_http_api_auth, ids[0], [fp, fp])
        assert res["code"] == 0
        assert len(res["data"]) == 2
        for i in range(len(res["data"])):
            assert res["data"][i]["dataset_id"] == ids[0]
            expected_name = fp.name
            if i != 0:
                expected_name = f"{fp.stem}({i}){fp.suffix}"
            assert res["data"][i]["name"] == expected_name

    def test_same_file_repeat(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        for i in range(10):
            res = upload_documnets(get_http_api_auth, ids[0], [fp])
            assert res["code"] == 0
            assert len(res["data"]) == 1
            assert res["data"][0]["dataset_id"] == ids[0]
            expected_name = fp.name
            if i != 0:
                expected_name = f"{fp.stem}({i}){fp.suffix}"
            assert res["data"][0]["name"] == expected_name

    def test_filename_special_characters(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)
        illegal_chars = '<>:"/\\|?*'
        translation_table = str.maketrans({char: "_" for char in illegal_chars})
        safe_filename = string.punctuation.translate(translation_table)
        fp = tmp_path / f"{safe_filename}.txt"
        fp.write_text("Sample text content")

        res = upload_documnets(get_http_api_auth, ids[0], [fp])
        assert res["code"] == 0
        assert len(res["data"]) == 1
        assert res["data"][0]["dataset_id"] == ids[0]
        assert res["data"][0]["name"] == fp.name

    def test_multiple_files(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)
        expected_document_count = 20
        fps = []
        for i in range(expected_document_count):
            fp = create_txt_file(tmp_path / f"ragflow_test_{i}.txt")
            fps.append(fp)
        res = upload_documnets(get_http_api_auth, ids[0], fps)
        assert res["code"] == 0

        res = list_dataset(get_http_api_auth, {"id": ids[0]})
        assert res["data"][0]["document_count"] == expected_document_count

    @pytest.mark.xfail
    def test_concurrent_upload(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)

        expected_document_count = 20
        fps = []
        for i in range(expected_document_count):
            fp = create_txt_file(tmp_path / f"ragflow_test_{i}.txt")
            fps.append(fp)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    upload_documnets, get_http_api_auth, ids[0], fps[i : i + 1]
                )
                for i in range(expected_document_count)
            ]
        responses = [f.result() for f in futures]
        assert all(r["code"] == 0 for r in responses)

        res = list_dataset(get_http_api_auth, {"id": ids[0]})
        assert res["data"][0]["document_count"] == expected_document_count
