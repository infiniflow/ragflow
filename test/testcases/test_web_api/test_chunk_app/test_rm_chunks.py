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
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from test_common import batch_add_chunks, delete_chunks, list_chunks


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
        res = delete_chunks(invalid_auth, "dataset_id", "document_id", {"chunk_ids": ["1"]})
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestChunksDeletion:
    @pytest.mark.p3
    @pytest.mark.parametrize(
        "document_id, expected_code, expected_message",
        [
            ("invalid_document_id", 100, "Can't find the document with ID invalid_document_id!"),
        ],
    )
    def test_invalid_document_id(self, WebApiAuth, add_chunks_func, document_id, expected_code, expected_message):
        dataset_id, _, chunk_ids = add_chunks_func
        res = delete_chunks(WebApiAuth, dataset_id, document_id, {"chunk_ids": chunk_ids})
        assert res["code"] == expected_code, res
        assert expected_message in res["message"], res

    @pytest.mark.parametrize(
        "payload",
        [
            pytest.param(lambda r: {"chunk_ids": ["invalid_id"] + r}, marks=pytest.mark.p3),
            pytest.param(lambda r: {"chunk_ids": r[:1] + ["invalid_id"] + r[1:4]}, marks=pytest.mark.p1),
            pytest.param(lambda r: {"chunk_ids": r + ["invalid_id"]}, marks=pytest.mark.p3),
        ],
    )
    def test_delete_partial_invalid_id(self, WebApiAuth, add_chunks_func, payload):
        dataset_id, document_id, chunk_ids = add_chunks_func
        payload = payload(chunk_ids)
        res = delete_chunks(WebApiAuth, dataset_id, document_id, payload)
        assert res["code"] == 102, res
        assert "rm_chunk deleted chunks" in res["message"], res

    @pytest.mark.p3
    def test_repeated_deletion(self, WebApiAuth, add_chunks_func):
        dataset_id, document_id, chunk_ids = add_chunks_func
        payload = {"chunk_ids": chunk_ids}
        res = delete_chunks(WebApiAuth, dataset_id, document_id, payload)
        assert res["code"] == 0, res

        res = delete_chunks(WebApiAuth, dataset_id, document_id, payload)
        assert res["code"] == 102, res
        assert res["message"] == f"rm_chunk deleted chunks 0, expect {len(chunk_ids)}", res

    @pytest.mark.p3
    def test_duplicate_deletion(self, WebApiAuth, add_chunks_func):
        dataset_id, document_id, chunk_ids = add_chunks_func
        res = delete_chunks(WebApiAuth, dataset_id, document_id, {"chunk_ids": chunk_ids * 2})
        assert res["code"] == 0, res

        res = list_chunks(WebApiAuth, dataset_id, document_id)
        assert res["code"] == 0, res
        assert len(res["data"]["chunks"]) == 0, res
        assert res["data"]["total"] == 0, res

    @pytest.mark.p2
    def test_delete_duplicate_ids_dedup_behavior(self, WebApiAuth, add_chunks_func):
        dataset_id, document_id, chunk_ids = add_chunks_func
        res = delete_chunks(WebApiAuth, dataset_id, document_id, {"chunk_ids": [chunk_ids[0], chunk_ids[0]]})
        assert res["code"] == 0, res

        res = list_chunks(WebApiAuth, dataset_id, document_id)
        assert res["code"] == 0, res
        assert len(res["data"]["chunks"]) == 3, res
        assert res["data"]["total"] == 3, res

    @pytest.mark.p3
    def test_concurrent_deletion(self, WebApiAuth, add_document):
        count = 100
        dataset_id, document_id = add_document
        chunk_ids = batch_add_chunks(WebApiAuth, dataset_id, document_id, count)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(delete_chunks, WebApiAuth, dataset_id, document_id, {"chunk_ids": chunk_ids[i : i + 1]})
                for i in range(count)
            ]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

    @pytest.mark.p3
    def test_delete_1k(self, WebApiAuth, add_document):
        chunks_num = 1_000
        dataset_id, document_id = add_document
        chunk_ids = batch_add_chunks(WebApiAuth, dataset_id, document_id, chunks_num)

        from time import sleep

        sleep(1)

        res = delete_chunks(WebApiAuth, dataset_id, document_id, {"chunk_ids": chunk_ids})
        assert res["code"] == 0

        res = list_chunks(WebApiAuth, dataset_id, document_id)
        assert res["code"] == 0, res
        assert len(res["data"]["chunks"]) == 0, res
        assert res["data"]["total"] == 0, res

    @pytest.mark.parametrize(
        "payload, expected_code, expected_message, remaining",
        [
            pytest.param({"chunk_ids": ["invalid_id"]}, 102, "rm_chunk deleted chunks 0, expect 1", 4, marks=pytest.mark.p3),
            pytest.param(lambda r: {"chunk_ids": r[:1]}, 0, "", 3, marks=pytest.mark.p3),
            pytest.param(lambda r: {"chunk_ids": r}, 0, "", 0, marks=pytest.mark.p1),
            pytest.param({"chunk_ids": []}, 0, "", 4, marks=pytest.mark.p3),
        ],
    )
    def test_basic_scenarios(self, WebApiAuth, add_chunks_func, payload, expected_code, expected_message, remaining):
        dataset_id, document_id, chunk_ids = add_chunks_func
        if callable(payload):
            payload = payload(chunk_ids)
        res = delete_chunks(WebApiAuth, dataset_id, document_id, payload)
        assert res["code"] == expected_code, res
        if res["code"] != 0:
            assert res["message"] == expected_message, res

        res = list_chunks(WebApiAuth, dataset_id, document_id)
        assert res["code"] == 0, res
        assert len(res["data"]["chunks"]) == remaining, res
        assert res["data"]["total"] == remaining, res
