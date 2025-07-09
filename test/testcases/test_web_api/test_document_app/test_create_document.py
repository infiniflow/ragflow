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
from common import create_document, list_kbs
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
        res = create_document(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestDocumentCreate:
    @pytest.mark.p3
    def test_filename_empty(self, WebApiAuth, add_dataset_func):
        kb_id = add_dataset_func
        payload = {"name": "", "kb_id": kb_id}
        res = create_document(WebApiAuth, payload)
        assert res["code"] == 101, res
        assert res["message"] == "File name can't be empty.", res

    @pytest.mark.p2
    def test_filename_max_length(self, WebApiAuth, add_dataset_func, tmp_path):
        kb_id = add_dataset_func
        fp = create_txt_file(tmp_path / f"{'a' * (DOCUMENT_NAME_LIMIT - 4)}.txt")
        res = create_document(WebApiAuth, {"name": fp.name, "kb_id": kb_id})
        assert res["code"] == 0, res
        assert res["data"]["name"] == fp.name, res

    @pytest.mark.p2
    def test_invalid_kb_id(self, WebApiAuth):
        res = create_document(WebApiAuth, {"name": "ragflow_test.txt", "kb_id": "invalid_kb_id"})
        assert res["code"] == 102, res
        assert res["message"] == "Can't find this knowledgebase!", res

    @pytest.mark.p3
    def test_filename_special_characters(self, WebApiAuth, add_dataset_func):
        kb_id = add_dataset_func
        illegal_chars = '<>:"/\\|?*'
        translation_table = str.maketrans({char: "_" for char in illegal_chars})
        safe_filename = string.punctuation.translate(translation_table)
        filename = f"{safe_filename}.txt"

        res = create_document(WebApiAuth, {"name": filename, "kb_id": kb_id})
        assert res["code"] == 0, res
        assert res["data"]["kb_id"] == kb_id, res
        assert res["data"]["name"] == filename, f"Expected: {filename}, Got: {res['data']['name']}"

    @pytest.mark.p3
    def test_concurrent_upload(self, WebApiAuth, add_dataset_func):
        kb_id = add_dataset_func

        count = 20
        filenames = [f"ragflow_test_{i}.txt" for i in range(count)]

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(create_document, WebApiAuth, {"name": name, "kb_id": kb_id}) for name in filenames]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures), responses

        res = list_kbs(WebApiAuth, {"id": kb_id})
        assert res["data"]["kbs"][0]["doc_num"] == count, res
