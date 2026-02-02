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
from common import add_chunk, delete_documents, list_chunks
from configs import INVALID_API_TOKEN, INVALID_ID_32
from libs.auth import RAGFlowHttpApiAuth


def validate_chunk_details(dataset_id, document_id, payload, res):
    chunk = res["data"]["chunk"]
    assert chunk["dataset_id"] == dataset_id
    assert chunk["document_id"] == document_id
    assert chunk["content"] == payload["content"]
    if "important_keywords" in payload:
        assert chunk["important_keywords"] == payload["important_keywords"]
    if "questions" in payload:
        assert chunk["questions"] == [str(q).strip() for q in payload.get("questions", []) if str(q).strip()]


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
        res = add_chunk(invalid_auth, "dataset_id", "document_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestAddChunk:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content": None}, 100, """TypeError("unsupported operand type(s) for +: \'NoneType\' and \'str\'")"""),
            ({"content": ""}, 102, "`content` is required"),
            pytest.param(
                {"content": 1},
                100,
                """TypeError("unsupported operand type(s) for +: \'int\' and \'str\'")""",
                marks=pytest.mark.skip,
            ),
            ({"content": "a"}, 0, ""),
            ({"content": " "}, 102, "`content` is required"),
            ({"content": "\n!?。；！？\"'"}, 0, ""),
        ],
    )
    def test_content(self, HttpApiAuth, add_document, payload, expected_code, expected_message):
        dataset_id, document_id = add_document
        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        chunks_count = res["data"]["doc"]["chunk_count"]
        res = add_chunk(HttpApiAuth, dataset_id, document_id, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            validate_chunk_details(dataset_id, document_id, payload, res)
            res = list_chunks(HttpApiAuth, dataset_id, document_id)
            if res["code"] != 0:
                assert False, res
            assert res["data"]["doc"]["chunk_count"] == chunks_count + 1
        else:
            assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content": "chunk test", "important_keywords": ["a", "b", "c"]}, 0, ""),
            ({"content": "chunk test", "important_keywords": [""]}, 0, ""),
            (
                {"content": "chunk test", "important_keywords": [1]},
                100,
                "TypeError('sequence item 0: expected str instance, int found')",
            ),
            ({"content": "chunk test", "important_keywords": ["a", "a"]}, 0, ""),
            ({"content": "chunk test", "important_keywords": "abc"}, 102, "`important_keywords` is required to be a list"),
            ({"content": "chunk test", "important_keywords": 123}, 102, "`important_keywords` is required to be a list"),
        ],
    )
    def test_important_keywords(self, HttpApiAuth, add_document, payload, expected_code, expected_message):
        dataset_id, document_id = add_document
        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        chunks_count = res["data"]["doc"]["chunk_count"]
        res = add_chunk(HttpApiAuth, dataset_id, document_id, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            validate_chunk_details(dataset_id, document_id, payload, res)
            res = list_chunks(HttpApiAuth, dataset_id, document_id)
            if res["code"] != 0:
                assert False, res
            assert res["data"]["doc"]["chunk_count"] == chunks_count + 1
        else:
            assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content": "chunk test", "questions": ["a", "b", "c"]}, 0, ""),
            ({"content": "chunk test", "questions": [""]}, 0, ""),
            ({"content": "chunk test", "questions": [1]}, 100, "TypeError('sequence item 0: expected str instance, int found')"),
            ({"content": "chunk test", "questions": ["a", "a"]}, 0, ""),
            ({"content": "chunk test", "questions": "abc"}, 102, "`questions` is required to be a list"),
            ({"content": "chunk test", "questions": 123}, 102, "`questions` is required to be a list"),
        ],
    )
    def test_questions(self, HttpApiAuth, add_document, payload, expected_code, expected_message):
        dataset_id, document_id = add_document
        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        chunks_count = res["data"]["doc"]["chunk_count"]
        res = add_chunk(HttpApiAuth, dataset_id, document_id, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            validate_chunk_details(dataset_id, document_id, payload, res)
            if res["code"] != 0:
                assert False, res
            res = list_chunks(HttpApiAuth, dataset_id, document_id)
            assert res["data"]["doc"]["chunk_count"] == chunks_count + 1
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "dataset_id, expected_code, expected_message",
        [
            (INVALID_ID_32, 102, f"You don't own the dataset {INVALID_ID_32}."),
        ],
    )
    def test_invalid_dataset_id(
        self,
        HttpApiAuth,
        add_document,
        dataset_id,
        expected_code,
        expected_message,
    ):
        _, document_id = add_document
        res = add_chunk(HttpApiAuth, dataset_id, document_id, {"content": "a"})
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
    def test_invalid_document_id(self, HttpApiAuth, add_document, document_id, expected_code, expected_message):
        dataset_id, _ = add_document
        res = add_chunk(HttpApiAuth, dataset_id, document_id, {"content": "chunk test"})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.p3
    def test_repeated_add_chunk(self, HttpApiAuth, add_document):
        payload = {"content": "chunk test"}
        dataset_id, document_id = add_document
        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        chunks_count = res["data"]["doc"]["chunk_count"]
        res = add_chunk(HttpApiAuth, dataset_id, document_id, payload)
        assert res["code"] == 0
        validate_chunk_details(dataset_id, document_id, payload, res)
        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        assert res["data"]["doc"]["chunk_count"] == chunks_count + 1

        res = add_chunk(HttpApiAuth, dataset_id, document_id, payload)
        assert res["code"] == 0
        validate_chunk_details(dataset_id, document_id, payload, res)
        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        assert res["data"]["doc"]["chunk_count"] == chunks_count + 2

    @pytest.mark.p3
    def test_add_chunk_to_deleted_document(self, HttpApiAuth, add_document):
        dataset_id, document_id = add_document
        delete_documents(HttpApiAuth, dataset_id, {"ids": [document_id]})
        res = add_chunk(HttpApiAuth, dataset_id, document_id, {"content": "chunk test"})
        assert res["code"] == 102
        assert res["message"] == f"You don't own the document {document_id}."

    @pytest.mark.skip(reason="issues/6411")
    def test_concurrent_add_chunk(self, HttpApiAuth, add_document):
        count = 50
        dataset_id, document_id = add_document
        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        chunks_count = res["data"]["doc"]["chunk_count"]

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    add_chunk,
                    HttpApiAuth,
                    dataset_id,
                    document_id,
                    {"content": f"chunk test {i}"},
                )
                for i in range(count)
            ]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)
        res = list_chunks(HttpApiAuth, dataset_id, document_id)
        if res["code"] != 0:
            assert False, res
        assert res["data"]["doc"]["chunk_count"] == chunks_count + count
