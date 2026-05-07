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
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from test_common import delete_document, list_chunks, update_chunk


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
        res = update_chunk(invalid_auth, "dataset_id", "document_id", "chunk_id", {"content": "test"})
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


def _find_chunk(auth, dataset_id, document_id, chunk_id):
    res = list_chunks(auth, dataset_id, document_id, params={"id": chunk_id})
    assert res["code"] == 0, res
    return res["data"]["chunks"][0]


class TestUpdateChunk:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content": None}, 0, ""),
            ({"content": ""}, 102, "`content` is required"),
            pytest.param({"content": 1}, 100, "TypeError('expected string or bytes-like object')", marks=pytest.mark.skip),
            ({"content": "update chunk"}, 0, ""),
            ({"content": " "}, 102, "`content` is required"),
            ({"content": "\n!?。；！？\"'"}, 0, ""),
        ],
    )
    def test_content(self, WebApiAuth, add_chunks, payload, expected_code, expected_message):
        dataset_id, document_id, chunk_ids = add_chunks
        chunk_id = chunk_ids[0]
        res = update_chunk(WebApiAuth, dataset_id, document_id, chunk_id, payload)
        assert res["code"] == expected_code, res
        if expected_code != 0:
            assert res["message"] == expected_message, res
        else:
            sleep(1)
            chunk = _find_chunk(WebApiAuth, dataset_id, document_id, chunk_id)
            if payload["content"] is not None:
                assert chunk["content"] == payload["content"]

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"important_keywords": ["a", "b", "c"]}, 0, ""),
            ({"important_keywords": [""]}, 0, ""),
            ({"important_keywords": [1]}, 100, "TypeError('sequence item 0: expected str instance, int found')"),
            ({"important_keywords": ["a", "a"]}, 0, ""),
            ({"important_keywords": "abc"}, 102, "`important_keywords` should be a list"),
            ({"important_keywords": 123}, 102, "`important_keywords` should be a list"),
        ],
    )
    def test_important_keywords(self, WebApiAuth, add_chunks, payload, expected_code, expected_message):
        dataset_id, document_id, chunk_ids = add_chunks
        chunk_id = chunk_ids[0]
        res = update_chunk(WebApiAuth, dataset_id, document_id, chunk_id, payload)
        assert res["code"] == expected_code, res
        if expected_code != 0:
            assert res["message"] == expected_message, res
        else:
            sleep(1)
            chunk = _find_chunk(WebApiAuth, dataset_id, document_id, chunk_id)
            assert chunk["important_keywords"] == payload["important_keywords"]

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"questions": ["a", "b", "c"]}, 0, ""),
            ({"questions": [""]}, 0, ""),
            ({"questions": [1]}, 100, "TypeError('sequence item 0: expected str instance, int found')"),
            ({"questions": ["a", "a"]}, 0, ""),
            ({"questions": "abc"}, 102, "`questions` should be a list"),
            ({"questions": 123}, 102, "`questions` should be a list"),
        ],
    )
    def test_questions(self, WebApiAuth, add_chunks, payload, expected_code, expected_message):
        dataset_id, document_id, chunk_ids = add_chunks
        chunk_id = chunk_ids[0]
        res = update_chunk(WebApiAuth, dataset_id, document_id, chunk_id, payload)
        assert res["code"] == expected_code, res
        if expected_code != 0:
            assert res["message"] == expected_message, res
        else:
            sleep(1)
            chunk = _find_chunk(WebApiAuth, dataset_id, document_id, chunk_id)
            assert chunk["questions"] == [str(q).strip() for q in payload["questions"] if str(q).strip()]

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"available": True}, 0, ""),
            ({"available": 1}, 0, ""),
            ({"available": False}, 0, ""),
            ({"available": 0}, 0, ""),
        ],
    )
    def test_available(self, WebApiAuth, add_chunks, payload, expected_code, expected_message):
        dataset_id, document_id, chunk_ids = add_chunks
        chunk_id = chunk_ids[0]
        res = update_chunk(WebApiAuth, dataset_id, document_id, chunk_id, payload)
        assert res["code"] == expected_code, res
        if expected_code != 0:
            assert res["message"] == expected_message, res
        else:
            sleep(1)
            chunk = _find_chunk(WebApiAuth, dataset_id, document_id, chunk_id)
            assert chunk["available"] == bool(payload["available"])

    @pytest.mark.p2
    def test_update_chunk_qa_multiline_content(self, WebApiAuth, add_chunks):
        dataset_id, document_id, chunk_ids = add_chunks
        payload = {"content": "Question line\nAnswer line"}
        res = update_chunk(WebApiAuth, dataset_id, document_id, chunk_ids[0], payload)
        assert res["code"] == 0, res

        sleep(1)
        chunk = _find_chunk(WebApiAuth, dataset_id, document_id, chunk_ids[0])
        assert chunk["content"] == payload["content"], chunk

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "document_id, expected_code, expected_message",
        [
            ("invalid_doc_id", 102, "You don't own the document invalid_doc_id."),
        ],
    )
    def test_invalid_document_id_for_update(self, WebApiAuth, add_chunks, document_id, expected_code, expected_message):
        dataset_id, _, chunk_ids = add_chunks
        res = update_chunk(WebApiAuth, dataset_id, document_id, chunk_ids[0], {"content": "test content"})
        assert res["code"] == expected_code
        assert expected_message in res["message"]

    @pytest.mark.p3
    def test_repeated_update_chunk(self, WebApiAuth, add_chunks):
        dataset_id, document_id, chunk_ids = add_chunks
        res = update_chunk(WebApiAuth, dataset_id, document_id, chunk_ids[0], {"content": "chunk test 1"})
        assert res["code"] == 0

        res = update_chunk(WebApiAuth, dataset_id, document_id, chunk_ids[0], {"content": "chunk test 2"})
        assert res["code"] == 0

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"unknown_key": "unknown_value"}, 0, ""),
            ({}, 0, ""),
        ],
    )
    def test_invalid_params(self, WebApiAuth, add_chunks, payload, expected_code, expected_message):
        dataset_id, document_id, chunk_ids = add_chunks
        res = update_chunk(WebApiAuth, dataset_id, document_id, chunk_ids[0], payload)
        assert res["code"] == expected_code, res
        if expected_code != 0:
            assert res["message"] == expected_message, res

    @pytest.mark.p3
    @pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="issues/6554")
    def test_concurrent_update_chunk(self, WebApiAuth, add_chunks):
        count = 50
        dataset_id, document_id, chunk_ids = add_chunks

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    update_chunk,
                    WebApiAuth,
                    dataset_id,
                    document_id,
                    chunk_ids[randint(0, 3)],
                    {"content": f"update chunk test {i}"},
                )
                for i in range(count)
            ]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

    @pytest.mark.p3
    def test_update_chunk_to_deleted_document(self, WebApiAuth, add_chunks):
        dataset_id, document_id, chunk_ids = add_chunks
        delete_document(WebApiAuth, dataset_id, {"ids": [document_id]})
        res = update_chunk(WebApiAuth, dataset_id, document_id, chunk_ids[0], {"content": "test content"})
        assert res["code"] == 102, res
        assert res["message"] in [f"You don't own the document {document_id}.", f"Can't find this chunk {chunk_ids[0]}"]
