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
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from common import batch_add_chunks, list_chunks
from configs import INVALID_API_TOKEN, INVALID_ID_32
from libs.auth import RAGFlowHttpApiAuth


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
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = list_chunks(invalid_auth, "dataset_id", "document_id")
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
    def test_page(self, HttpApiAuth, add_chunks, params, expected_code, expected_page_size, expected_message):
        dataset_id, document_id, _ = add_chunks
        res = list_chunks(HttpApiAuth, dataset_id, document_id, params=params)
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
            ({"page_size": 1}, 0, 1, ""),
            ({"page_size": 6}, 0, 5, ""),
            ({"page_size": "1"}, 0, 1, ""),
            pytest.param({"page_size": -1}, 0, 5, "", marks=pytest.mark.skip),
            pytest.param({"page_size": "a"}, 100, 0, """ValueError("invalid literal for int() with base 10: \'a\'")""", marks=pytest.mark.skip),
        ],
    )
    def test_page_size(self, HttpApiAuth, add_chunks, params, expected_code, expected_page_size, expected_message):
        dataset_id, document_id, _ = add_chunks
        res = list_chunks(HttpApiAuth, dataset_id, document_id, params=params)
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
            ({"keywords": "chunk"}, 4),
            pytest.param({"keywords": "ragflow"}, 1, marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="issues/6509")),
            pytest.param({"keywords": "ragflow"}, 5, marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") != "infinity", reason="issues/6509")),
            ({"keywords": "unknown"}, 0),
        ],
    )
    def test_keywords(self, HttpApiAuth, add_chunks, params, expected_page_size):
        dataset_id, document_id, _ = add_chunks
        res = list_chunks(HttpApiAuth, dataset_id, document_id, params=params)
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
        HttpApiAuth,
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
        res = list_chunks(HttpApiAuth, dataset_id, document_id, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            if params["id"] in [None, ""]:
                assert len(res["data"]["chunks"]) == expected_page_size
            else:
                assert res["data"]["chunks"][0]["id"] == params["id"]
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    def test_invalid_params(self, HttpApiAuth, add_chunks):
        dataset_id, document_id, _ = add_chunks
        params = {"a": "b"}
        res = list_chunks(HttpApiAuth, dataset_id, document_id, params=params)
        assert res["code"] == 0
        assert len(res["data"]["chunks"]) == 5

    @pytest.mark.p3
    def test_concurrent_list(self, HttpApiAuth, add_chunks):
        dataset_id, document_id, _ = add_chunks
        count = 100
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(list_chunks, HttpApiAuth, dataset_id, document_id) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(len(future.result()["data"]["chunks"]) == 5 for future in futures)

    @pytest.mark.p1
    def test_default(self, HttpApiAuth, add_document):
        dataset_id, document_id = add_document

        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        chunks_count = res["data"]["doc"]["chunk_count"]
        batch_add_chunks(HttpApiAuth, dataset_id, document_id, 31)
        # issues/6487
        from time import sleep

        sleep(3)
        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        assert res["code"] == 0
        assert len(res["data"]["chunks"]) == 30
        assert res["data"]["doc"]["chunk_count"] == chunks_count + 31

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "dataset_id, expected_code, expected_message",
        [
            (INVALID_ID_32, 102, f"You don't own the dataset {INVALID_ID_32}."),
        ],
    )
    def test_invalid_dataset_id(self, HttpApiAuth, add_chunks, dataset_id, expected_code, expected_message):
        _, document_id, _ = add_chunks
        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "document_id, expected_code, expected_message",
        [
            (
                INVALID_ID_32,
                102,
                f"You don't own the document {INVALID_ID_32}.",
            ),
        ],
    )
    def test_invalid_document_id(self, HttpApiAuth, add_chunks, document_id, expected_code, expected_message):
        dataset_id, _, _ = add_chunks
        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        assert res["code"] == expected_code
        assert res["message"] == expected_message
