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
from concurrent.futures import ThreadPoolExecutor

import pytest
from common import INVALID_API_TOKEN, add_chunk, batch_upload_documents, create_datasets, delete_documnet
from libs.auth import RAGFlowHttpApiAuth


def validate_chunk_details(dataset_id, document_id, payload, res):
    chunk = res["data"]["chunk"]
    assert chunk["dataset_id"] == dataset_id
    assert chunk["document_id"] == document_id
    assert chunk["content"] == payload["content"]
    if "important_keywords" in payload:
        assert chunk["important_keywords"] == payload["important_keywords"]
    if "questions" in payload:
        assert chunk["questions"] == payload["questions"]


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
    def test_invalid_auth(self, get_http_api_auth, tmp_path, auth, expected_code, expected_message):
        ids = create_datasets(get_http_api_auth, 1)
        dataset_id = ids[0]
        document_ids = batch_upload_documents(get_http_api_auth, dataset_id, 1, tmp_path)
        res = add_chunk(auth, dataset_id, document_ids[0], {})
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestAddChunk:
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content": None}, 100, """TypeError("unsupported operand type(s) for +: \'NoneType\' and \'str\'")"""),
            ({"content": ""}, 102, "`content` is required"),
            pytest.param(
                {"content": 1},
                100,
                """TypeError("unsupported operand type(s) for +: \'int\' and \'str\'")""",
                marks=pytest.mark.xfail,
            ),
            ({"content": "a"}, 0, ""),
            ({"content": " "}, 102, "`content` is required"),
            ({"content": "\n!?。；！？\"'"}, 0, ""),
        ],
    )
    def test_content(self, get_http_api_auth, tmp_path, payload, expected_code, expected_message):
        ids = create_datasets(get_http_api_auth, 1)
        dataset_id = ids[0]
        document_ids = batch_upload_documents(get_http_api_auth, dataset_id, 1, tmp_path)
        res = add_chunk(get_http_api_auth, dataset_id, document_ids[0], payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            validate_chunk_details(dataset_id, document_ids[0], payload, res)
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content": "a", "important_keywords": ["a", "b", "c"]}, 0, ""),
            ({"content": "a", "important_keywords": [""]}, 0, ""),
            (
                {"content": "a", "important_keywords": [1]},
                100,
                "TypeError('sequence item 0: expected str instance, int found')",
            ),
            ({"content": "a", "important_keywords": ["a", "a"]}, 0, ""),
            ({"content": "a", "important_keywords": "abc"}, 102, "`important_keywords` is required to be a list"),
            ({"content": "a", "important_keywords": 123}, 102, "`important_keywords` is required to be a list"),
        ],
    )
    def test_important_keywords(self, get_http_api_auth, tmp_path, payload, expected_code, expected_message):
        ids = create_datasets(get_http_api_auth, 1)
        dataset_id = ids[0]
        document_ids = batch_upload_documents(get_http_api_auth, dataset_id, 1, tmp_path)
        res = add_chunk(get_http_api_auth, dataset_id, document_ids[0], payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            validate_chunk_details(dataset_id, document_ids[0], payload, res)
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content": "a", "questions": ["a", "b", "c"]}, 0, ""),
            pytest.param(
                {"content": "a", "questions": [""]},
                0,
                "",
                marks=pytest.mark.xfail(reason="issues/6404"),
            ),
            ({"content": "a", "questions": [1]}, 100, "TypeError('sequence item 0: expected str instance, int found')"),
            ({"content": "a", "questions": ["a", "a"]}, 0, ""),
            ({"content": "a", "questions": "abc"}, 102, "`questions` is required to be a list"),
            ({"content": "a", "questions": 123}, 102, "`questions` is required to be a list"),
        ],
    )
    def test_questions(self, get_http_api_auth, tmp_path, payload, expected_code, expected_message):
        ids = create_datasets(get_http_api_auth, 1)
        dataset_id = ids[0]
        document_ids = batch_upload_documents(get_http_api_auth, dataset_id, 1, tmp_path)
        res = add_chunk(get_http_api_auth, dataset_id, document_ids[0], payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            validate_chunk_details(dataset_id, document_ids[0], payload, res)
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "dataset_id, expected_code, expected_message",
        [
            ("", 100, "<NotFound '404: Not Found'>"),
            (
                "invalid_dataset_id",
                102,
                "You don't own the dataset invalid_dataset_id.",
            ),
        ],
    )
    def test_invalid_dataset_id(
        self,
        get_http_api_auth,
        tmp_path,
        dataset_id,
        expected_code,
        expected_message,
    ):
        ids = create_datasets(get_http_api_auth, 1)
        document_ids = batch_upload_documents(get_http_api_auth, ids[0], 1, tmp_path)
        res = add_chunk(get_http_api_auth, dataset_id, document_ids[0], {"content": "a"})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "document_id, expected_code, expected_message",
        [
            ("", 100, "<MethodNotAllowed '405: Method Not Allowed'>"),
            (
                "invalid_document_id",
                102,
                "You don't own the document invalid_document_id.",
            ),
        ],
    )
    def test_invalid_document_id(self, get_http_api_auth, document_id, expected_code, expected_message):
        ids = create_datasets(get_http_api_auth, 1)
        res = add_chunk(get_http_api_auth, ids[0], document_id, {"content": "a"})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    def test_repeated_add_chunk(self, get_http_api_auth, tmp_path):
        payload = {"content": "a"}

        ids = create_datasets(get_http_api_auth, 1)
        dataset_id = ids[0]
        document_ids = batch_upload_documents(get_http_api_auth, dataset_id, 1, tmp_path)
        res = add_chunk(get_http_api_auth, dataset_id, document_ids[0], payload)
        assert res["code"] == 0
        validate_chunk_details(dataset_id, document_ids[0], payload, res)

        res = add_chunk(get_http_api_auth, dataset_id, document_ids[0], payload)
        assert res["code"] == 0
        validate_chunk_details(dataset_id, document_ids[0], payload, res)

    def test_add_chunk_to_deleted_document(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)
        dataset_id = ids[0]
        document_ids = batch_upload_documents(get_http_api_auth, dataset_id, 1, tmp_path)
        delete_documnet(get_http_api_auth, ids[0], {"ids": document_ids})
        res = add_chunk(get_http_api_auth, dataset_id, document_ids[0], {"content": "a"})
        assert res["code"] == 102
        assert res["message"] == f"You don't own the document {document_ids[0]}."

    def test_concurrent_parse(self, get_http_api_auth, tmp_path):
        chunk_num = 50
        ids = create_datasets(get_http_api_auth, 1)
        dataset_id = ids[0]
        document_ids = batch_upload_documents(get_http_api_auth, dataset_id, 1, tmp_path)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    add_chunk,
                    get_http_api_auth,
                    ids[0],
                    document_ids[0],
                    {"content": "a"},
                )
                for i in range(chunk_num)
            ]
        responses = [f.result() for f in futures]
        assert all(r["code"] == 0 for r in responses)
