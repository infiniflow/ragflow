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
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
import requests
from common import FILE_API_URL, list_datasets, upload_documents
from configs import DOCUMENT_NAME_LIMIT, HOST_ADDRESS, INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth
from requests_toolbelt import MultipartEncoder
from utils.file_utils import create_txt_file


@pytest.mark.p1
@pytest.mark.usefixtures("clear_datasets")
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
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = upload_documents(invalid_auth, "dataset_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestDocumentsUpload:
    @pytest.mark.p1
    def test_valid_single_upload(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documents(HttpApiAuth, dataset_id, [fp])
        assert res["code"] == 0
        assert res["data"][0]["dataset_id"] == dataset_id
        assert res["data"][0]["name"] == fp.name

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
    def test_file_type_validation(self, HttpApiAuth, add_dataset_func, generate_test_files, request):
        dataset_id = add_dataset_func
        fp = generate_test_files[request.node.callspec.params["generate_test_files"]]
        res = upload_documents(HttpApiAuth, dataset_id, [fp])
        assert res["code"] == 0
        assert res["data"][0]["dataset_id"] == dataset_id
        assert res["data"][0]["name"] == fp.name

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "file_type",
        ["exe", "unknown"],
    )
    def test_unsupported_file_type(self, HttpApiAuth, add_dataset_func, tmp_path, file_type):
        dataset_id = add_dataset_func
        fp = tmp_path / f"ragflow_test.{file_type}"
        fp.touch()
        res = upload_documents(HttpApiAuth, dataset_id, [fp])
        assert res["code"] == 500
        assert res["message"] == f"ragflow_test.{file_type}: This type of file has not been supported yet!"

    @pytest.mark.p2
    def test_missing_file(self, HttpApiAuth, add_dataset_func):
        dataset_id = add_dataset_func
        res = upload_documents(HttpApiAuth, dataset_id)
        assert res["code"] == 101
        assert res["message"] == "No file part!"

    @pytest.mark.p3
    def test_empty_file(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        fp = tmp_path / "empty.txt"
        fp.touch()

        res = upload_documents(HttpApiAuth, dataset_id, [fp])
        assert res["code"] == 0
        assert res["data"][0]["size"] == 0

    @pytest.mark.p3
    def test_filename_empty(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        url = f"{HOST_ADDRESS}{FILE_API_URL}".format(dataset_id=dataset_id)
        with fp.open("rb") as file_obj:
            fields = (("file", ("", file_obj)),)
            m = MultipartEncoder(fields=fields)
            res = requests.post(
                url=url,
                headers={"Content-Type": m.content_type},
                auth=HttpApiAuth,
                data=m,
            )
        assert res.json()["code"] == 101
        assert res.json()["message"] == "No file selected!"

    @pytest.mark.p2
    def test_filename_max_length(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        fp = create_txt_file(tmp_path / f"{'a' * (DOCUMENT_NAME_LIMIT - 4)}.txt")
        res = upload_documents(HttpApiAuth, dataset_id, [fp])
        assert res["code"] == 0
        assert res["data"][0]["name"] == fp.name

    @pytest.mark.p2
    def test_invalid_dataset_id(self, HttpApiAuth, tmp_path):
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documents(HttpApiAuth, "invalid_dataset_id", [fp])
        assert res["code"] == 100
        assert res["message"] == """LookupError("Can\'t find the dataset with ID invalid_dataset_id!")"""

    @pytest.mark.p2
    def test_duplicate_files(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documents(HttpApiAuth, dataset_id, [fp, fp])
        assert res["code"] == 0
        assert len(res["data"]) == 2
        for i in range(len(res["data"])):
            assert res["data"][i]["dataset_id"] == dataset_id
            expected_name = fp.name
            if i != 0:
                expected_name = f"{fp.stem}({i}){fp.suffix}"
            assert res["data"][i]["name"] == expected_name

    @pytest.mark.p2
    def test_same_file_repeat(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        for i in range(3):
            res = upload_documents(HttpApiAuth, dataset_id, [fp])
            assert res["code"] == 0
            assert len(res["data"]) == 1
            assert res["data"][0]["dataset_id"] == dataset_id
            expected_name = fp.name
            if i != 0:
                expected_name = f"{fp.stem}({i}){fp.suffix}"
            assert res["data"][0]["name"] == expected_name

    @pytest.mark.p3
    def test_filename_special_characters(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        illegal_chars = '<>:"/\\|?*'
        translation_table = str.maketrans({char: "_" for char in illegal_chars})
        safe_filename = string.punctuation.translate(translation_table)
        fp = tmp_path / f"{safe_filename}.txt"
        fp.write_text("Sample text content")

        res = upload_documents(HttpApiAuth, dataset_id, [fp])
        assert res["code"] == 0
        assert len(res["data"]) == 1
        assert res["data"][0]["dataset_id"] == dataset_id
        assert res["data"][0]["name"] == fp.name

    @pytest.mark.p1
    def test_multiple_files(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func
        expected_document_count = 20
        fps = []
        for i in range(expected_document_count):
            fp = create_txt_file(tmp_path / f"ragflow_test_{i}.txt")
            fps.append(fp)
        res = upload_documents(HttpApiAuth, dataset_id, fps)
        assert res["code"] == 0

        res = list_datasets(HttpApiAuth, {"id": dataset_id})
        assert res["data"][0]["document_count"] == expected_document_count

    @pytest.mark.p3
    def test_concurrent_upload(self, HttpApiAuth, add_dataset_func, tmp_path):
        dataset_id = add_dataset_func

        count = 20
        fps = []
        for i in range(count):
            fp = create_txt_file(tmp_path / f"ragflow_test_{i}.txt")
            fps.append(fp)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(upload_documents, HttpApiAuth, dataset_id, [fp]) for fp in fps]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

        res = list_datasets(HttpApiAuth, {"id": dataset_id})
        assert res["data"][0]["document_count"] == count
