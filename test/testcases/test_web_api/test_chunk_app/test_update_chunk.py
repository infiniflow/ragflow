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
from random import randint
from time import sleep

import pytest
from common import delete_document, list_chunks, update_chunk
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


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
        res = update_chunk(invalid_auth, {"doc_id": "doc_id", "chunk_id": "chunk_id", "content_with_weight": "test"})
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestUpdateChunk:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content_with_weight": None}, 100, "TypeError('expected string or bytes-like object')"),
            ({"content_with_weight": ""}, 100, """Exception('Error: 413 - {"error":"Input validation error: `inputs` cannot be empty","error_type":"Validation"}')"""),
            ({"content_with_weight": 1}, 100, "TypeError('expected string or bytes-like object')"),
            ({"content_with_weight": "update chunk"}, 0, ""),
            ({"content_with_weight": " "}, 0, ""),
            ({"content_with_weight": "\n!?。；！？\"'"}, 0, ""),
        ],
    )
    def test_content(self, WebApiAuth, add_chunks, payload, expected_code, expected_message):
        _, doc_id, chunk_ids = add_chunks
        chunk_id = chunk_ids[0]
        update_payload = {"doc_id": doc_id, "chunk_id": chunk_id}
        if payload:
            update_payload.update(payload)
        res = update_chunk(WebApiAuth, update_payload)
        assert res["code"] == expected_code, res
        if expected_code != 0:
            assert res["message"] == expected_message, res
        else:
            sleep(1)
            res = list_chunks(WebApiAuth, {"doc_id": doc_id})
            for chunk in res["data"]["chunks"]:
                if chunk["chunk_id"] == chunk_id:
                    assert chunk["content_with_weight"] == payload["content_with_weight"]

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"important_kwd": ["a", "b", "c"]}, 0, ""),
            ({"important_kwd": [""]}, 0, ""),
            ({"important_kwd": [1]}, 100, "TypeError('sequence item 0: expected str instance, int found')"),
            ({"important_kwd": ["a", "a"]}, 0, ""),
            ({"important_kwd": "abc"}, 102, "`important_kwd` should be a list"),
            ({"important_kwd": 123}, 102, "`important_kwd` should be a list"),
        ],
    )
    def test_important_keywords(self, WebApiAuth, add_chunks, payload, expected_code, expected_message):
        _, doc_id, chunk_ids = add_chunks
        chunk_id = chunk_ids[0]
        update_payload = {"doc_id": doc_id, "chunk_id": chunk_id, "content_with_weight": "unchanged content"}  # Add content_with_weight as it's required
        if payload:
            update_payload.update(payload)
        res = update_chunk(WebApiAuth, update_payload)
        assert res["code"] == expected_code, res
        if expected_code != 0:
            assert res["message"] == expected_message, res
        else:
            sleep(1)
            res = list_chunks(WebApiAuth, {"doc_id": doc_id})
            for chunk in res["data"]["chunks"]:
                if chunk["chunk_id"] == chunk_id:
                    assert chunk["important_kwd"] == payload["important_kwd"]

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"question_kwd": ["a", "b", "c"]}, 0, ""),
            ({"question_kwd": [""]}, 100, """Exception('Error: 413 - {"error":"Input validation error: `inputs` cannot be empty","error_type":"Validation"}')"""),
            ({"question_kwd": [1]}, 100, "TypeError('sequence item 0: expected str instance, int found')"),
            ({"question_kwd": ["a", "a"]}, 0, ""),
            ({"question_kwd": "abc"}, 102, "`question_kwd` should be a list"),
            ({"question_kwd": 123}, 102, "`question_kwd` should be a list"),
        ],
    )
    def test_questions(self, WebApiAuth, add_chunks, payload, expected_code, expected_message):
        _, doc_id, chunk_ids = add_chunks
        chunk_id = chunk_ids[0]
        update_payload = {"doc_id": doc_id, "chunk_id": chunk_id, "content_with_weight": "unchanged content"}  # Add content_with_weight as it's required
        if payload:
            update_payload.update(payload)

        res = update_chunk(WebApiAuth, update_payload)
        assert res["code"] == expected_code, res
        if expected_code != 0:
            assert res["message"] == expected_message, res
        else:
            sleep(1)
            res = list_chunks(WebApiAuth, {"doc_id": doc_id})
            for chunk in res["data"]["chunks"]:
                if chunk["chunk_id"] == chunk_id:
                    assert chunk["question_kwd"] == payload["question_kwd"]

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"available_int": 1}, 0, ""),
            ({"available_int": 0}, 0, ""),
        ],
    )
    def test_available(self, WebApiAuth, add_chunks, payload, expected_code, expected_message):
        _, doc_id, chunk_ids = add_chunks
        chunk_id = chunk_ids[0]
        update_payload = {"doc_id": doc_id, "chunk_id": chunk_id, "content_with_weight": "unchanged content"}
        if payload:
            update_payload.update(payload)

        res = update_chunk(WebApiAuth, update_payload)
        assert res["code"] == expected_code, res
        if expected_code != 0:
            assert res["message"] == expected_message, res
        else:
            sleep(1)
            res = list_chunks(WebApiAuth, {"doc_id": doc_id})
            for chunk in res["data"]["chunks"]:
                if chunk["chunk_id"] == chunk_id:
                    assert chunk["available_int"] == payload["available_int"]

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "doc_id_param, expected_code, expected_message",
        [
            ("", 102, "Tenant not found!"),
            ("invalid_doc_id", 102, "Tenant not found!"),
        ],
    )
    def test_invalid_document_id_for_update(self, WebApiAuth, add_chunks, doc_id_param, expected_code, expected_message):
        _, _, chunk_ids = add_chunks
        chunk_id = chunk_ids[0]

        payload = {"doc_id": doc_id_param, "chunk_id": chunk_id, "content_with_weight": "test content"}
        res = update_chunk(WebApiAuth, payload)
        assert res["code"] == expected_code
        assert expected_message in res["message"]

    @pytest.mark.p3
    def test_repeated_update_chunk(self, WebApiAuth, add_chunks):
        _, doc_id, chunk_ids = add_chunks
        payload1 = {"doc_id": doc_id, "chunk_id": chunk_ids[0], "content_with_weight": "chunk test 1"}
        res = update_chunk(WebApiAuth, payload1)
        assert res["code"] == 0

        payload2 = {"doc_id": doc_id, "chunk_id": chunk_ids[0], "content_with_weight": "chunk test 2"}
        res = update_chunk(WebApiAuth, payload2)
        assert res["code"] == 0

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"unknown_key": "unknown_value"}, 0, ""),
            ({}, 0, ""),
            pytest.param(None, 100, """TypeError("int() argument must be a string, a bytes-like object or a real number, not 'NoneType'")""", marks=pytest.mark.skip),
        ],
    )
    def test_invalid_params(self, WebApiAuth, add_chunks, payload, expected_code, expected_message):
        _, doc_id, chunk_ids = add_chunks
        chunk_id = chunk_ids[0]
        update_payload = {"doc_id": doc_id, "chunk_id": chunk_id, "content_with_weight": "unchanged content"}
        if payload is not None:
            update_payload.update(payload)

        res = update_chunk(WebApiAuth, update_payload)
        assert res["code"] == expected_code, res
        if expected_code != 0:
            assert res["message"] == expected_message, res

    @pytest.mark.p3
    @pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="issues/6554")
    def test_concurrent_update_chunk(self, WebApiAuth, add_chunks):
        count = 50
        _, doc_id, chunk_ids = add_chunks

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    update_chunk,
                    WebApiAuth,
                    {"doc_id": doc_id, "chunk_id": chunk_ids[randint(0, 3)], "content_with_weight": f"update chunk test {i}"},
                )
                for i in range(count)
            ]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

    @pytest.mark.p3
    def test_update_chunk_to_deleted_document(self, WebApiAuth, add_chunks):
        _, doc_id, chunk_ids = add_chunks
        delete_document(WebApiAuth, {"doc_id": doc_id})
        payload = {"doc_id": doc_id, "chunk_id": chunk_ids[0], "content_with_weight": "test content"}
        res = update_chunk(WebApiAuth, payload)
        assert res["code"] == 102, res
        assert res["message"] == "Tenant not found!", res
