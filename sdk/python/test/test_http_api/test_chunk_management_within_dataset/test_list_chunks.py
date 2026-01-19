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
import os
from concurrent.futures import ThreadPoolExecutor

import pytest
from common import INVALID_API_TOKEN, batch_add_chunks, list_chunks
from libs.auth import RAGFlowHttpApiAuth


@pytest.mark.p1
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
    def test_invalid_auth(self, auth, expected_code, expected_message):
        res = list_chunks(auth, "dataset_id", "document_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestChunksList:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_code, expected_page_size, expected_message",
        [
            ({"page": None, "page_size": 2}, 0, 2, ""),
            pytest.param({"page": 0, "page_size": 2}, 100, 0, "ValueError('Search does not support negative slicing.')", marks=pytest.mark.skip),
            ({"page": 2, "page_size": 2}, 0, 2, ""),
            ({"page": 3, "page_size": 2}, 0, 1, ""),
            ({"page": "3", "page_size": 2}, 0, 1, ""),
            pytest.param({"page": -1, "page_size": 2}, 100, 0, "ValueError('Search does not support negative slicing.')", marks=pytest.mark.skip),
            pytest.param({"page": "a", "page_size": 2}, 100, 0, """ValueError("invalid literal for int() with base 10: \'a\'")""", marks=pytest.mark.skip),
        ],
    )
    def test_page(self, get_http_api_auth, add_chunks, params, expected_code, expected_page_size, expected_message):
        dataset_id, document_id, _ = add_chunks
        res = list_chunks(get_http_api_auth, dataset_id, document_id, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_code, expected_page_size, expected_message",
        [
            ({"page_size": None}, 0, 5, ""),
            pytest.param({"page_size": 0}, 0, 5, ""),
            pytest.param({"page_size": 0}, 100, 0, ""),
            ({"page_size": 1}, 0, 1, ""),
            ({"page_size": 6}, 0, 5, ""),
            ({"page_size": "1"}, 0, 1, ""),
            pytest.param({"page_size": -1}, 0, 5, "", marks=pytest.mark.skip),
            pytest.param({"page_size": "a"}, 100, 0, """ValueError("invalid literal for int() with base 10: \'a\'")""", marks=pytest.mark.skip),
        ],
    )
    def test_page_size(self, get_http_api_auth, add_chunks, params, expected_code, expected_page_size, expected_message):
        dataset_id, document_id, _ = add_chunks
        res = list_chunks(get_http_api_auth, dataset_id, document_id, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_page_size",
        [
            ({"keywords": None}, 5),
            ({"keywords": ""}, 5),
            ({"keywords": "1"}, 1),
            pytest.param({"keywords": "chunk"}, 4, marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="issues/6509")),
            ({"keywords": "ragflow"}, 1),
            ({"keywords": "unknown"}, 0),
        ],
    )
    def test_keywords(self, get_http_api_auth, add_chunks, params, expected_page_size):
        dataset_id, document_id, _ = add_chunks
        res = list_chunks(get_http_api_auth, dataset_id, document_id, params=params)
        assert res["code"] == 0
        assert len(res["data"]["chunks"]) == expected_page_size

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "chunk_id, expected_code, expected_page_size, expected_message",
        [
            (None, 0, 5, ""),
            ("", 0, 5, ""),
            pytest.param(lambda r: r[0], 0, 1, "", marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="issues/6499")),
            pytest.param("unknown", 100, 0, """AttributeError("\'NoneType\' object has no attribute \'keys\'")""", marks=pytest.mark.skip),
        ],
    )
    def test_id(
        self,
        get_http_api_auth,
        add_chunks,
        chunk_id,
        expected_code,
        expected_page_size,
        expected_message,
    ):
        dataset_id, document_id, chunk_ids = add_chunks
        if callable(chunk_id):
            params = {"id": chunk_id(chunk_ids)}
        else:
            params = {"id": chunk_id}
        res = list_chunks(get_http_api_auth, dataset_id, document_id, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            if params["id"] in [None, ""]:
                assert len(res["data"]["chunks"]) == expected_page_size
            else:
                assert res["data"]["chunks"][0]["id"] == params["id"]
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    def test_invalid_params(self, get_http_api_auth, add_chunks):
        dataset_id, document_id, _ = add_chunks
        params = {"a": "b"}
        res = list_chunks(get_http_api_auth, dataset_id, document_id, params=params)
        assert res["code"] == 0
        assert len(res["data"]["chunks"]) == 5

    @pytest.mark.p3
    def test_concurrent_list(self, get_http_api_auth, add_chunks):
        dataset_id, document_id, _ = add_chunks

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(list_chunks, get_http_api_auth, dataset_id, document_id) for i in range(100)]
        responses = [f.result() for f in futures]
        assert all(r["code"] == 0 for r in responses)
        assert all(len(r["data"]["chunks"]) == 5 for r in responses)

    @pytest.mark.p1
    def test_default(self, get_http_api_auth, add_document):
        dataset_id, document_id = add_document

        res = list_chunks(get_http_api_auth, dataset_id, document_id)
        chunks_count = res["data"]["doc"]["chunk_count"]
        batch_add_chunks(get_http_api_auth, dataset_id, document_id, 31)
        # issues/6487
        from time import sleep

        sleep(3)
        res = list_chunks(get_http_api_auth, dataset_id, document_id)
        assert res["code"] == 0
        assert len(res["data"]["chunks"]) == 30
        assert res["data"]["doc"]["chunk_count"] == chunks_count + 31

    @pytest.mark.p3
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
    def test_invalid_dataset_id(self, get_http_api_auth, add_chunks, dataset_id, expected_code, expected_message):
        _, document_id, _ = add_chunks
        res = list_chunks(get_http_api_auth, dataset_id, document_id)
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "document_id, expected_code, expected_message",
        [
            ("", 102, "The dataset not own the document chunks."),
            (
                "invalid_document_id",
                102,
                "You don't own the document invalid_document_id.",
            ),
        ],
    )
    def test_invalid_document_id(self, get_http_api_auth, add_chunks, document_id, expected_code, expected_message):
        dataset_id, _, _ = add_chunks
        res = list_chunks(get_http_api_auth, dataset_id, document_id)
        assert res["code"] == expected_code
        assert res["message"] == expected_message
