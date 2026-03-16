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
from common import list_kbs, upload_documents
from configs import DOCUMENT_NAME_LIMIT, INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from utils.file_utils import create_txt_file


@pytest.mark.p1
@pytest.mark.usefixtures("clear_datasets")
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = upload_documents(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestDocumentsUpload:
    @pytest.mark.p1
    def test_valid_single_upload(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp])
        assert res["code"] == 0, res
        assert res["data"][0]["kb_id"] == kb_id, res
        assert res["data"][0]["name"] == fp.name, res

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
    def test_file_type_validation(self, WebApiAuth, add_dataset_func, generate_test_files, request):
        kb_id = add_dataset_func
        fp = generate_test_files[request.node.callspec.params["generate_test_files"]]
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp])
        assert res["code"] == 0, res
        assert res["data"][0]["kb_id"] == kb_id, res
        assert res["data"][0]["name"] == fp.name, res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "file_type",
        ["exe", "unknown"],
    )
    def test_unsupported_file_type(self, WebApiAuth, add_dataset_func, tmp_path, file_type):
        kb_id = add_dataset_func
        fp = tmp_path / f"ragflow_test.{file_type}"
        fp.touch()
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp])
        assert res["code"] == 500, res
        assert res["message"] == f"ragflow_test.{file_type}: This type of file has not been supported yet!", res

    @pytest.mark.p2
    def test_missing_file(self, WebApiAuth, add_dataset_func):
        kb_id = add_dataset_func
        res = upload_documents(WebApiAuth, {"kb_id": kb_id})
        assert res["code"] == 101, res
        assert res["message"] == "No file part!", res

    @pytest.mark.p3
    def test_empty_file(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        fp = tmp_path / "empty.txt"
        fp.touch()

        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp])
        assert res["code"] == 0, res
        assert res["data"][0]["size"] == 0, res

    @pytest.mark.p3
    def test_filename_empty(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func

        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp], filename_override="")
        assert res["code"] == 101, res
        assert res["message"] == "No file selected!", res

    @pytest.mark.p3
    def test_filename_exceeds_max_length(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        fp = create_txt_file(tmp_path / f"{'a' * (DOCUMENT_NAME_LIMIT - 4)}.txt")
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp])
        assert res["code"] == 0, res
        assert res["data"][0]["name"] == fp.name, res

    @pytest.mark.p2
    def test_invalid_kb_id(self, WebApiAuth, tmp_path):
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documents(WebApiAuth, {"kb_id": "invalid_kb_id"}, [fp])
        assert res["code"] == 100, res
        assert res["message"] == """LookupError("Can't find this dataset!")""", res

    @pytest.mark.p2
    def test_duplicate_files(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        fp = create_txt_file(tmp_path / "ragflow_test.txt")
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp, fp])
        assert res["code"] == 0, res
        assert len(res["data"]) == 2, res
        for i in range(len(res["data"])):
            assert res["data"][i]["kb_id"] == kb_id, res
            expected_name = fp.name
            if i != 0:
                expected_name = f"{fp.stem}({i}){fp.suffix}"
            assert res["data"][i]["name"] == expected_name, res

    @pytest.mark.p3
    def test_filename_special_characters(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        illegal_chars = '<>:"/\\|?*'
        translation_table = str.maketrans({char: "_" for char in illegal_chars})
        safe_filename = string.punctuation.translate(translation_table)
        fp = tmp_path / f"{safe_filename}.txt"
        fp.write_text("Sample text content")

        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, [fp])
        assert res["code"] == 0, res
        assert len(res["data"]) == 1, res
        assert res["data"][0]["kb_id"] == kb_id, res
        assert res["data"][0]["name"] == fp.name, res

    @pytest.mark.p1
    def test_multiple_files(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        expected_document_count = 20
        fps = []
        for i in range(expected_document_count):
            fp = create_txt_file(tmp_path / f"ragflow_test_{i}.txt")
            fps.append(fp)
        res = upload_documents(WebApiAuth, {"kb_id": kb_id}, fps)
        assert res["code"] == 0, res

        res = list_kbs(WebApiAuth)
        assert res["data"]["kbs"][0]["doc_num"] == expected_document_count, res

    @pytest.mark.p3
    def test_concurrent_upload(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func

        count = 20
        fps = []
        for i in range(count):
            fp = create_txt_file(tmp_path / f"ragflow_test_{i}.txt")
            fps.append(fp)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(upload_documents, WebApiAuth, {"kb_id": kb_id}, fps[i : i + 1]) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures), responses

        res = list_kbs(WebApiAuth)
        assert res["data"]["kbs"][0]["doc_num"] == count, res
