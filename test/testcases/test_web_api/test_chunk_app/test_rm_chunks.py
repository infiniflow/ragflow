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
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from common import batch_add_chunks, delete_chunks, list_chunks
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


@pytest.mark.p1
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = delete_chunks(invalid_auth, {"doc_id": "document_id", "chunk_ids": ["1"]})
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestChunksDeletion:
    @pytest.mark.p3
    @pytest.mark.parametrize(
        "doc_id, expected_code, expected_message",
        [
            ("", 102, "Document not found!"),
            ("invalid_document_id", 102, "Document not found!"),
        ],
    )
    def test_invalid_document_id(self, WebApiAuth, add_chunks_func, doc_id, expected_code, expected_message):
        _, _, chunk_ids = add_chunks_func
        res = delete_chunks(WebApiAuth, {"doc_id": doc_id, "chunk_ids": chunk_ids})
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res

    @pytest.mark.parametrize(
        "payload",
        [
            pytest.param(lambda r: {"chunk_ids": ["invalid_id"] + r}, marks=pytest.mark.p3),
            pytest.param(lambda r: {"chunk_ids": r[:1] + ["invalid_id"] + r[1:4]}, marks=pytest.mark.p1),
            pytest.param(lambda r: {"chunk_ids": r + ["invalid_id"]}, marks=pytest.mark.p3),
        ],
    )
    def test_delete_partial_invalid_id(self, WebApiAuth, add_chunks_func, payload):
        _, doc_id, chunk_ids = add_chunks_func
        if callable(payload):
            payload = payload(chunk_ids)
        payload["doc_id"] = doc_id
        res = delete_chunks(WebApiAuth, payload)
        assert res["code"] == 0, res

        res = list_chunks(WebApiAuth, {"doc_id": doc_id})
        assert res["code"] == 0, res
        assert len(res["data"]["chunks"]) == 0, res
        assert res["data"]["total"] == 0, res

    @pytest.mark.p3
    def test_repeated_deletion(self, WebApiAuth, add_chunks_func):
        _, doc_id, chunk_ids = add_chunks_func
        payload = {"chunk_ids": chunk_ids, "doc_id": doc_id}
        res = delete_chunks(WebApiAuth, payload)
        assert res["code"] == 0, res

        res = delete_chunks(WebApiAuth, payload)
        assert res["code"] == 102, res
        assert res["message"] == "Index updating failure", res

    @pytest.mark.p3
    def test_duplicate_deletion(self, WebApiAuth, add_chunks_func):
        _, doc_id, chunk_ids = add_chunks_func
        payload = {"chunk_ids": chunk_ids * 2, "doc_id": doc_id}
        res = delete_chunks(WebApiAuth, payload)
        assert res["code"] == 0, res

        res = list_chunks(WebApiAuth, {"doc_id": doc_id})
        assert res["code"] == 0, res
        assert len(res["data"]["chunks"]) == 0, res
        assert res["data"]["total"] == 0, res

    @pytest.mark.p3
    def test_concurrent_deletion(self, WebApiAuth, add_document):
        count = 100
        _, doc_id = add_document
        chunk_ids = batch_add_chunks(WebApiAuth, doc_id, count)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    delete_chunks,
                    WebApiAuth,
                    {"doc_id": doc_id, "chunk_ids": chunk_ids[i : i + 1]},
                )
                for i in range(count)
            ]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

    @pytest.mark.p3
    def test_delete_1k(self, WebApiAuth, add_document):
        chunks_num = 1_000
        _, doc_id = add_document
        chunk_ids = batch_add_chunks(WebApiAuth, doc_id, chunks_num)

        from time import sleep

        sleep(1)

        res = delete_chunks(WebApiAuth, {"doc_id": doc_id, "chunk_ids": chunk_ids})
        assert res["code"] == 0

        res = list_chunks(WebApiAuth, {"doc_id": doc_id})
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]["chunks"]) == 0, res
        assert res["data"]["total"] == 0, res

    @pytest.mark.parametrize(
        "payload, expected_code, expected_message, remaining",
        [
            pytest.param(None, 100, """TypeError("argument of type \'NoneType\' is not iterable")""", 5, marks=pytest.mark.skip),
            pytest.param({"chunk_ids": ["invalid_id"]}, 102, "Index updating failure", 4, marks=pytest.mark.p3),
            pytest.param("not json", 100, """UnboundLocalError("local variable \'duplicate_messages\' referenced before assignment")""", 5, marks=pytest.mark.skip(reason="pull/6376")),
            pytest.param(lambda r: {"chunk_ids": r[:1]}, 0, "", 3, marks=pytest.mark.p3),
            pytest.param(lambda r: {"chunk_ids": r}, 0, "", 0, marks=pytest.mark.p1),
            pytest.param({"chunk_ids": []}, 0, "", 0, marks=pytest.mark.p3),
        ],
    )
    def test_basic_scenarios(self, WebApiAuth, add_chunks_func, payload, expected_code, expected_message, remaining):
        _, doc_id, chunk_ids = add_chunks_func
        if callable(payload):
            payload = payload(chunk_ids)
        payload["doc_id"] = doc_id
        res = delete_chunks(WebApiAuth, payload)
        assert res["code"] == expected_code, res
        if res["code"] != 0:
            assert res["message"] == expected_message, res

        res = list_chunks(WebApiAuth, {"doc_id": doc_id})
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]["chunks"]) == remaining, res
        assert res["data"]["total"] == remaining, res
