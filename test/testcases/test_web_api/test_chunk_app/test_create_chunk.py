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
from test_common import add_chunk, delete_document, get_chunk, list_chunks


def validate_chunk_details(auth, dataset_id, document_id, payload, res):
    chunk = res["data"]["chunk"]
    assert chunk["dataset_id"] == dataset_id
    assert chunk["document_id"] == document_id
    assert chunk["content"] == payload["content"]
    if "important_keywords" in payload:
        assert chunk["important_keywords"] == payload["important_keywords"]
    if "questions" in payload:
        expected = [str(q).strip() for q in payload.get("questions", []) if str(q).strip()]
        assert chunk["questions"] == expected
    if "tag_kwd" in payload:
        assert chunk["tag_kwd"] == payload["tag_kwd"]

    fetched = get_chunk(auth, dataset_id, document_id, chunk["id"])
    assert fetched["code"] == 0, fetched
    assert fetched["data"]["id"] == chunk["id"]
    assert fetched["data"]["doc_id"] == document_id


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
        res = add_chunk(invalid_auth, "dataset_id", "document_id", {"content": "chunk test"})
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestAddChunk:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content": None}, 102, "`content` is required"),
            ({"content": ""}, 102, "`content` is required"),
            ({"content": "a"}, 0, ""),
            ({"content": " "}, 102, "`content` is required"),
            ({"content": "\n!?。；！？\"'"}, 0, ""),
        ],
    )
    def test_content(self, WebApiAuth, add_document, payload, expected_code, expected_message):
        dataset_id, document_id = add_document
        chunks_count = list_chunks(WebApiAuth, dataset_id, document_id)["data"]["doc"]["chunk_count"]
        res = add_chunk(WebApiAuth, dataset_id, document_id, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            validate_chunk_details(WebApiAuth, dataset_id, document_id, payload, res)
            res = list_chunks(WebApiAuth, dataset_id, document_id)
            assert res["data"]["doc"]["chunk_count"] == chunks_count + 1, res
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content": "chunk test", "important_keywords": ["a", "b", "c"]}, 0, ""),
            ({"content": "chunk test", "important_keywords": [""]}, 0, ""),
            ({"content": "chunk test", "important_keywords": [1]}, 100, "TypeError('sequence item 0: expected str instance, int found')"),
            ({"content": "chunk test", "important_keywords": ["a", "a"]}, 0, ""),
            ({"content": "chunk test", "important_keywords": "abc"}, 102, "`important_keywords` is required to be a list"),
            ({"content": "chunk test", "important_keywords": 123}, 102, "`important_keywords` is required to be a list"),
        ],
    )
    def test_important_keywords(self, WebApiAuth, add_document, payload, expected_code, expected_message):
        dataset_id, document_id = add_document
        res = add_chunk(WebApiAuth, dataset_id, document_id, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            validate_chunk_details(WebApiAuth, dataset_id, document_id, payload, res)
        else:
            assert res["message"] == expected_message, res

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
    def test_questions(self, WebApiAuth, add_document, payload, expected_code, expected_message):
        dataset_id, document_id = add_document
        res = add_chunk(WebApiAuth, dataset_id, document_id, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            validate_chunk_details(WebApiAuth, dataset_id, document_id, payload, res)
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p2
    def test_add_chunk_with_tag_fields(self, WebApiAuth, add_document):
        dataset_id, document_id = add_document
        payload = {
            "content": "chunk with tags",
            "tag_kwd": ["tag1", "tag2"],
            "important_keywords": ["tag"],
            "questions": ["question"],
        }
        res = add_chunk(WebApiAuth, dataset_id, document_id, payload)
        assert res["code"] == 0, res
        validate_chunk_details(WebApiAuth, dataset_id, document_id, payload, res)

    @pytest.mark.p2
    def test_get_chunk_not_found(self, WebApiAuth, add_document):
        dataset_id, document_id = add_document
        res = get_chunk(WebApiAuth, dataset_id, document_id, "missing_chunk_id")
        assert res["code"] == 102, res
        assert "Chunk not found" in res["message"], res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "document_id, expected_code, expected_message",
        [
            ("invalid_document_id", 102, "You don't own the document invalid_document_id."),
        ],
    )
    def test_invalid_document_id(self, WebApiAuth, add_document, document_id, expected_code, expected_message):
        dataset_id, _ = add_document
        res = add_chunk(WebApiAuth, dataset_id, document_id, {"content": "chunk test"})
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res

    @pytest.mark.p3
    def test_repeated_add_chunk(self, WebApiAuth, add_document):
        payload = {"content": "chunk test"}
        dataset_id, document_id = add_document
        chunks_count = list_chunks(WebApiAuth, dataset_id, document_id)["data"]["doc"]["chunk_count"]

        res = add_chunk(WebApiAuth, dataset_id, document_id, payload)
        assert res["code"] == 0, res
        validate_chunk_details(WebApiAuth, dataset_id, document_id, payload, res)

        res = add_chunk(WebApiAuth, dataset_id, document_id, payload)
        assert res["code"] == 0, res
        validate_chunk_details(WebApiAuth, dataset_id, document_id, payload, res)

        res = list_chunks(WebApiAuth, dataset_id, document_id)
        assert res["data"]["doc"]["chunk_count"] == chunks_count + 2, res

    @pytest.mark.p2
    def test_add_chunk_to_deleted_document(self, WebApiAuth, add_document):
        dataset_id, document_id = add_document
        delete_document(WebApiAuth, dataset_id, {"ids": [document_id]})
        res = add_chunk(WebApiAuth, dataset_id, document_id, {"content": "chunk test"})
        assert res["code"] == 102, res
        assert res["message"] == f"You don't own the document {document_id}.", res

    @pytest.mark.skip(reason="issues/6411")
    @pytest.mark.p3
    def test_concurrent_add_chunk(self, WebApiAuth, add_document):
        count = 50
        dataset_id, document_id = add_document
        chunks_count = list_chunks(WebApiAuth, dataset_id, document_id)["data"]["doc"]["chunk_count"]

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(add_chunk, WebApiAuth, dataset_id, document_id, {"content": f"chunk test {i}"})
                for i in range(count)
            ]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)
        res = list_chunks(WebApiAuth, dataset_id, document_id)
        assert res["data"]["doc"]["chunk_count"] == chunks_count + count
