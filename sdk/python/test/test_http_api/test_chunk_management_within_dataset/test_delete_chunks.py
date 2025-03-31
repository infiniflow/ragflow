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
from common import INVALID_API_TOKEN, batch_add_chunks, delete_chunks, list_chunks
from libs.auth import RAGFlowHttpApiAuth


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
        res = delete_chunks(auth, "dataset_id", "document_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestChunkstDeletion:
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
    def test_invalid_dataset_id(self, get_http_api_auth, add_chunks_func, dataset_id, expected_code, expected_message):
        _, document_id, chunk_ids = add_chunks_func
        res = delete_chunks(get_http_api_auth, dataset_id, document_id, {"chunk_ids": chunk_ids})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "document_id, expected_code, expected_message",
        [
            ("", 100, "<MethodNotAllowed '405: Method Not Allowed'>"),
            pytest.param(
                "invalid_document_id",
                100,
                "LookupError('Document not found which is supposed to be there')",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="issues/6611"),
            ),
            pytest.param(
                "invalid_document_id",
                100,
                "rm_chunk deleted chunks 0, expect 4",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in [None, "elasticsearch"], reason="issues/6611"),
            ),
        ],
    )
    def test_invalid_document_id(self, get_http_api_auth, add_chunks_func, document_id, expected_code, expected_message):
        dataset_id, _, chunk_ids = add_chunks_func
        res = delete_chunks(get_http_api_auth, dataset_id, document_id, {"chunk_ids": chunk_ids})
        assert res["code"] == expected_code
        #assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "payload",
        [
            lambda r: {"chunk_ids": ["invalid_id"] + r},
            lambda r: {"chunk_ids": r[:1] + ["invalid_id"] + r[1:4]},
            lambda r: {"chunk_ids": r + ["invalid_id"]},
        ],
    )
    def test_delete_partial_invalid_id(self, get_http_api_auth, add_chunks_func, payload):
        dataset_id, document_id, chunk_ids = add_chunks_func
        if callable(payload):
            payload = payload(chunk_ids)
        res = delete_chunks(get_http_api_auth, dataset_id, document_id, payload)
        assert res["code"] == 102
        assert res["message"] == "rm_chunk deleted chunks 4, expect 5"

        res = list_chunks(get_http_api_auth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]["chunks"]) == 1
        assert res["data"]["total"] == 1

    def test_repeated_deletion(self, get_http_api_auth, add_chunks_func):
        dataset_id, document_id, chunk_ids = add_chunks_func
        payload = {"chunk_ids": chunk_ids}
        res = delete_chunks(get_http_api_auth, dataset_id, document_id, payload)
        assert res["code"] == 0

        res = delete_chunks(get_http_api_auth, dataset_id, document_id, payload)
        assert res["code"] == 102
        assert res["message"] == "rm_chunk deleted chunks 0, expect 4"

    def test_duplicate_deletion(self, get_http_api_auth, add_chunks_func):
        dataset_id, document_id, chunk_ids = add_chunks_func
        res = delete_chunks(get_http_api_auth, dataset_id, document_id, {"chunk_ids": chunk_ids * 2})
        assert res["code"] == 0
        assert "Duplicate chunk ids" in res["data"]["errors"][0]
        assert res["data"]["success_count"] == 4

        res = list_chunks(get_http_api_auth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]["chunks"]) == 1
        assert res["data"]["total"] == 1

    @pytest.mark.slow
    def test_concurrent_deletion(self, get_http_api_auth, get_dataset_id_and_document_id):
        chunks_num = 100
        dataset_id, document_id = get_dataset_id_and_document_id
        chunk_ids = batch_add_chunks(get_http_api_auth, dataset_id, document_id, chunks_num)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    delete_chunks,
                    get_http_api_auth,
                    dataset_id,
                    document_id,
                    {"chunk_ids": chunk_ids[i : i + 1]},
                )
                for i in range(chunks_num)
            ]
        responses = [f.result() for f in futures]
        assert all(r["code"] == 0 for r in responses)

    @pytest.mark.slow
    def test_delete_1k(self, get_http_api_auth, get_dataset_id_and_document_id):
        chunks_num = 1_000
        dataset_id, document_id = get_dataset_id_and_document_id
        chunk_ids = batch_add_chunks(get_http_api_auth, dataset_id, document_id, chunks_num)

        # issues/6487
        from time import sleep

        sleep(1)

        res = delete_chunks(get_http_api_auth, dataset_id, document_id, {"chunk_ids": chunk_ids})
        assert res["code"] == 0

        res = list_chunks(get_http_api_auth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]["chunks"]) == 1
        assert res["data"]["total"] == 1

    @pytest.mark.parametrize(
        "payload, expected_code, expected_message, remaining",
        [
            pytest.param(None, 100, """TypeError("argument of type \'NoneType\' is not iterable")""", 5, marks=pytest.mark.skip),
            ({"chunk_ids": ["invalid_id"]}, 102, "rm_chunk deleted chunks 0, expect 1", 5),
            pytest.param(
                "not json",
                100,
                """UnboundLocalError("local variable \'duplicate_messages\' referenced before assignment")""",
                5,
                marks=pytest.mark.skip(reason="pull/6376"),
            ),
            (lambda r: {"chunk_ids": r[:1]}, 0, "", 4),
            (lambda r: {"chunk_ids": r}, 0, "", 1),
            pytest.param({"chunk_ids": []}, 0, "", 5, marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="issues/6607")),
            pytest.param({"chunk_ids": []}, 102, "rm_chunk deleted chunks 5, expect 0", 0, marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in [None, "elasticsearch"], reason="issues/6607")),
        ],
    )
    def test_basic_scenarios(
        self,
        get_http_api_auth,
        add_chunks_func,
        payload,
        expected_code,
        expected_message,
        remaining,
    ):
        dataset_id, document_id, chunk_ids = add_chunks_func
        if callable(payload):
            payload = payload(chunk_ids)
        res = delete_chunks(get_http_api_auth, dataset_id, document_id, payload)
        assert res["code"] == expected_code
        if res["code"] != 0:
            assert res["message"] == expected_message

        res = list_chunks(get_http_api_auth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]["chunks"]) == remaining
        assert res["data"]["total"] == remaining
