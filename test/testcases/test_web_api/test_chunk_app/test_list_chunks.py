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
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from test_common import batch_add_chunks, list_chunks, update_chunk


@pytest.mark.p2
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = list_chunks(invalid_auth, "dataset_id", "document_id")
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


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
            pytest.param({"page": "a", "page_size": 2}, 100, 0, """ValueError("invalid literal for int() with base 10: 'a'")""", marks=pytest.mark.skip),
        ],
    )
    def test_page(self, WebApiAuth, add_chunks, params, expected_code, expected_page_size, expected_message):
        dataset_id, document_id, _ = add_chunks
        res = list_chunks(WebApiAuth, dataset_id, document_id, params=params)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size, res
        else:
            assert res["message"] == expected_message, res

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
            pytest.param({"page_size": "a"}, 100, 0, """ValueError("invalid literal for int() with base 10: 'a'")""", marks=pytest.mark.skip),
        ],
    )
    def test_page_size(self, WebApiAuth, add_chunks, params, expected_code, expected_page_size, expected_message):
        dataset_id, document_id, _ = add_chunks
        res = list_chunks(WebApiAuth, dataset_id, document_id, params=params)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size, res
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p2
    def test_available_filter(self, WebApiAuth, add_chunks):
        dataset_id, document_id, chunk_ids = add_chunks
        chunk_id = chunk_ids[0]

        res = update_chunk(WebApiAuth, dataset_id, document_id, chunk_id, {"content": "unchanged content", "available": False})
        assert res["code"] == 0, res

        from time import sleep

        sleep(1)
        res = list_chunks(WebApiAuth, dataset_id, document_id, params={"available": "false"})
        assert res["code"] == 0, res
        assert len(res["data"]["chunks"]) >= 1, res
        assert all(chunk["available"] is False for chunk in res["data"]["chunks"]), res

        res = update_chunk(WebApiAuth, dataset_id, document_id, chunk_id, {"content": "chunk test 0", "available": True})
        assert res["code"] == 0, res
        sleep(1)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_page_size",
        [
            ({"keywords": None}, 5),
            ({"keywords": ""}, 5),
            ({"keywords": "1"}, 1),
            pytest.param({"keywords": "chunk"}, 4, marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="issues/6509")),
            ({"keywords": "unknown"}, 0),
        ],
    )
    def test_keywords(self, WebApiAuth, add_chunks, params, expected_page_size):
        dataset_id, document_id, _ = add_chunks
        res = list_chunks(WebApiAuth, dataset_id, document_id, params=params)
        assert res["code"] == 0, res
        assert len(res["data"]["chunks"]) == expected_page_size, res

    @pytest.mark.p3
    def test_invalid_params(self, WebApiAuth, add_chunks):
        dataset_id, document_id, _ = add_chunks
        res = list_chunks(WebApiAuth, dataset_id, document_id, params={"a": "b"})
        assert res["code"] == 0, res
        assert len(res["data"]["chunks"]) == 5, res

    @pytest.mark.p3
    def test_concurrent_list(self, WebApiAuth, add_chunks):
        dataset_id, document_id, _ = add_chunks
        count = 100
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(list_chunks, WebApiAuth, dataset_id, document_id) for _ in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(len(future.result()["data"]["chunks"]) == 5 for future in futures)

    @pytest.mark.p1
    def test_default(self, WebApiAuth, add_document):
        dataset_id, document_id = add_document

        res = list_chunks(WebApiAuth, dataset_id, document_id)
        chunks_count = res["data"]["doc"]["chunk_count"]
        batch_add_chunks(WebApiAuth, dataset_id, document_id, 31)

        from time import sleep

        sleep(3)
        res = list_chunks(WebApiAuth, dataset_id, document_id)
        assert res["code"] == 0
        assert len(res["data"]["chunks"]) == 30
        assert res["data"]["doc"]["chunk_count"] == chunks_count + 31
