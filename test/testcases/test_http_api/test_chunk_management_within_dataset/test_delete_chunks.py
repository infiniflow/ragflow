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
        res = delete_chunks(invalid_auth, "dataset_id", "document_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestChunksDeletion:
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
    def test_invalid_dataset_id(self, HttpApiAuth, add_chunks_func, dataset_id, expected_code, expected_message):
        _, document_id, chunk_ids = add_chunks_func
        res = delete_chunks(HttpApiAuth, dataset_id, document_id, {"chunk_ids": chunk_ids})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "document_id, expected_code, expected_message",
        [
            ("", 100, "<MethodNotAllowed '405: Method Not Allowed'>"),
            ("invalid_document_id", 100, """LookupError("Can't find the document with ID invalid_document_id!")"""),
        ],
    )
    def test_invalid_document_id(self, HttpApiAuth, add_chunks_func, document_id, expected_code, expected_message):
        dataset_id, _, chunk_ids = add_chunks_func
        res = delete_chunks(HttpApiAuth, dataset_id, document_id, {"chunk_ids": chunk_ids})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "payload",
        [
            pytest.param(lambda r: {"chunk_ids": ["invalid_id"] + r}, marks=pytest.mark.p3),
            pytest.param(lambda r: {"chunk_ids": r[:1] + ["invalid_id"] + r[1:4]}, marks=pytest.mark.p1),
            pytest.param(lambda r: {"chunk_ids": r + ["invalid_id"]}, marks=pytest.mark.p3),
        ],
    )
    def test_delete_partial_invalid_id(self, HttpApiAuth, add_chunks_func, payload):
        dataset_id, document_id, chunk_ids = add_chunks_func
        if callable(payload):
            payload = payload(chunk_ids)
        res = delete_chunks(HttpApiAuth, dataset_id, document_id, payload)
        assert res["code"] == 102
        assert res["message"] == "rm_chunk deleted chunks 4, expect 5"

        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]["chunks"]) == 1
        assert res["data"]["total"] == 1

    @pytest.mark.p3
    def test_repeated_deletion(self, HttpApiAuth, add_chunks_func):
        dataset_id, document_id, chunk_ids = add_chunks_func
        payload = {"chunk_ids": chunk_ids}
        res = delete_chunks(HttpApiAuth, dataset_id, document_id, payload)
        assert res["code"] == 0

        res = delete_chunks(HttpApiAuth, dataset_id, document_id, payload)
        assert res["code"] == 102
        assert res["message"] == "rm_chunk deleted chunks 0, expect 4"

    @pytest.mark.p3
    def test_duplicate_deletion(self, HttpApiAuth, add_chunks_func):
        dataset_id, document_id, chunk_ids = add_chunks_func
        res = delete_chunks(HttpApiAuth, dataset_id, document_id, {"chunk_ids": chunk_ids * 2})
        assert res["code"] == 0
        assert "Duplicate chunk ids" in res["data"]["errors"][0]
        assert res["data"]["success_count"] == 4

        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]["chunks"]) == 1
        assert res["data"]["total"] == 1

    @pytest.mark.p3
    def test_concurrent_deletion(self, HttpApiAuth, add_document):
        count = 100
        dataset_id, document_id = add_document
        chunk_ids = batch_add_chunks(HttpApiAuth, dataset_id, document_id, count)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    delete_chunks,
                    HttpApiAuth,
                    dataset_id,
                    document_id,
                    {"chunk_ids": chunk_ids[i : i + 1]},
                )
                for i in range(count)
            ]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

    @pytest.mark.p3
    def test_delete_1k(self, HttpApiAuth, add_document):
        chunks_num = 1_000
        dataset_id, document_id = add_document
        chunk_ids = batch_add_chunks(HttpApiAuth, dataset_id, document_id, chunks_num)

        # issues/6487
        from time import sleep

        sleep(1)

        res = delete_chunks(HttpApiAuth, dataset_id, document_id, {"chunk_ids": chunk_ids})
        assert res["code"] == 0

        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]["chunks"]) == 0
        assert res["data"]["total"] == 0

    @pytest.mark.parametrize(
        "payload, expected_code, expected_message, remaining",
        [
            pytest.param(None, 100, """TypeError("argument of type \'NoneType\' is not iterable")""", 5, marks=pytest.mark.skip),
            pytest.param({"chunk_ids": ["invalid_id"]}, 102, "rm_chunk deleted chunks 0, expect 1", 5, marks=pytest.mark.p3),
            pytest.param("not json", 100, """UnboundLocalError("local variable \'duplicate_messages\' referenced before assignment")""", 5, marks=pytest.mark.skip(reason="pull/6376")),
            pytest.param(lambda r: {"chunk_ids": r[:1]}, 0, "", 4, marks=pytest.mark.p3),
            pytest.param(lambda r: {"chunk_ids": r}, 0, "", 1, marks=pytest.mark.p1),
            pytest.param({"chunk_ids": []}, 0, "", 0, marks=pytest.mark.p3),
        ],
    )
    def test_basic_scenarios(
        self,
        HttpApiAuth,
        add_chunks_func,
        payload,
        expected_code,
        expected_message,
        remaining,
    ):
        dataset_id, document_id, chunk_ids = add_chunks_func
        if callable(payload):
            payload = payload(chunk_ids)
        res = delete_chunks(HttpApiAuth, dataset_id, document_id, payload)
        assert res["code"] == expected_code
        if res["code"] != 0:
            assert res["message"] == expected_message

        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]["chunks"]) == remaining
        assert res["data"]["total"] == remaining
