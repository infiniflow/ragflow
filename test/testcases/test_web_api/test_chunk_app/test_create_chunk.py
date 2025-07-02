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
from common import add_chunk, delete_document, get_chunk, list_chunks
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


def validate_chunk_details(auth, kb_id, doc_id, payload, res):
    chunk_id = res["data"]["chunk_id"]
    res = get_chunk(auth, {"chunk_id": chunk_id})
    assert res["code"] == 0, res
    chunk = res["data"]
    assert chunk["doc_id"] == doc_id
    assert chunk["kb_id"] == kb_id
    assert chunk["content_with_weight"] == payload["content_with_weight"]
    if "important_kwd" in payload:
        assert chunk["important_kwd"] == payload["important_kwd"]
    if "question_kwd" in payload:
        expected = [str(q).strip() for q in payload.get("question_kwd", [])]
        assert chunk["question_kwd"] == expected


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
        res = add_chunk(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestAddChunk:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content_with_weight": None}, 100, """TypeError("unsupported operand type(s) for +: 'NoneType' and 'str'")"""),
            ({"content_with_weight": ""}, 0, ""),
            pytest.param(
                {"content_with_weight": 1},
                100,
                """TypeError("unsupported operand type(s) for +: 'int' and 'str'")""",
                marks=pytest.mark.skip,
            ),
            ({"content_with_weight": "a"}, 0, ""),
            ({"content_with_weight": " "}, 0, ""),
            ({"content_with_weight": "\n!?。；！？\"'"}, 0, ""),
        ],
    )
    def test_content(self, WebApiAuth, add_document, payload, expected_code, expected_message):
        kb_id, doc_id = add_document
        res = list_chunks(WebApiAuth, {"doc_id": doc_id})
        if res["code"] == 0:
            chunks_count = res["data"]["doc"]["chunk_num"]
        else:
            chunks_count = 0
        res = add_chunk(WebApiAuth, {**payload, "doc_id": doc_id})
        assert res["code"] == expected_code, res
        if expected_code == 0:
            validate_chunk_details(WebApiAuth, kb_id, doc_id, payload, res)
            res = list_chunks(WebApiAuth, {"doc_id": doc_id})
            assert res["code"] == 0, res
            assert res["data"]["doc"]["chunk_num"] == chunks_count + 1, res
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content_with_weight": "chunk test", "important_kwd": ["a", "b", "c"]}, 0, ""),
            ({"content_with_weight": "chunk test", "important_kwd": [""]}, 0, ""),
            (
                {"content_with_weight": "chunk test", "important_kwd": [1]},
                100,
                "TypeError('sequence item 0: expected str instance, int found')",
            ),
            ({"content_with_weight": "chunk test", "important_kwd": ["a", "a"]}, 0, ""),
            ({"content_with_weight": "chunk test", "important_kwd": "abc"}, 102, "`important_kwd` is required to be a list"),
            ({"content_with_weight": "chunk test", "important_kwd": 123}, 102, "`important_kwd` is required to be a list"),
        ],
    )
    def test_important_keywords(self, WebApiAuth, add_document, payload, expected_code, expected_message):
        kb_id, doc_id = add_document
        res = list_chunks(WebApiAuth, {"doc_id": doc_id})
        if res["code"] == 0:
            chunks_count = res["data"]["doc"]["chunk_num"]
        else:
            chunks_count = 0
        res = add_chunk(WebApiAuth, {**payload, "doc_id": doc_id})
        assert res["code"] == expected_code, res
        if expected_code == 0:
            validate_chunk_details(WebApiAuth, kb_id, doc_id, payload, res)
            res = list_chunks(WebApiAuth, {"doc_id": doc_id})
            assert res["code"] == 0, res
            assert res["data"]["doc"]["chunk_num"] == chunks_count + 1, res
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"content_with_weight": "chunk test", "question_kwd": ["a", "b", "c"]}, 0, ""),
            ({"content_with_weight": "chunk test", "question_kwd": [""]}, 0, ""),
            ({"content_with_weight": "chunk test", "question_kwd": [1]}, 100, "TypeError('sequence item 0: expected str instance, int found')"),
            ({"content_with_weight": "chunk test", "question_kwd": ["a", "a"]}, 0, ""),
            ({"content_with_weight": "chunk test", "question_kwd": "abc"}, 102, "`question_kwd` is required to be a list"),
            ({"content_with_weight": "chunk test", "question_kwd": 123}, 102, "`question_kwd` is required to be a list"),
        ],
    )
    def test_questions(self, WebApiAuth, add_document, payload, expected_code, expected_message):
        kb_id, doc_id = add_document
        res = list_chunks(WebApiAuth, {"doc_id": doc_id})
        if res["code"] == 0:
            chunks_count = res["data"]["doc"]["chunk_num"]
        else:
            chunks_count = 0
        res = add_chunk(WebApiAuth, {**payload, "doc_id": doc_id})
        assert res["code"] == expected_code, res
        if expected_code == 0:
            validate_chunk_details(WebApiAuth, kb_id, doc_id, payload, res)
            res = list_chunks(WebApiAuth, {"doc_id": doc_id})
            assert res["code"] == 0, res
            assert res["data"]["doc"]["chunk_num"] == chunks_count + 1, res
        else:
            assert res["message"] == expected_message, res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "doc_id, expected_code, expected_message",
        [
            ("", 102, "Document not found!"),
            ("invalid_document_id", 102, "Document not found!"),
        ],
    )
    def test_invalid_document_id(self, WebApiAuth, add_document, doc_id, expected_code, expected_message):
        _, _ = add_document
        res = add_chunk(WebApiAuth, {"doc_id": doc_id, "content_with_weight": "chunk test"})
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res

    @pytest.mark.p3
    def test_repeated_add_chunk(self, WebApiAuth, add_document):
        payload = {"content_with_weight": "chunk test"}
        kb_id, doc_id = add_document
        res = list_chunks(WebApiAuth, {"doc_id": doc_id})
        if res["code"] != 0:
            assert False, res
        chunks_count = res["data"]["doc"]["chunk_num"]

        res = add_chunk(WebApiAuth, {**payload, "doc_id": doc_id})
        assert res["code"] == 0, res
        validate_chunk_details(WebApiAuth, kb_id, doc_id, payload, res)
        res = list_chunks(WebApiAuth, {"doc_id": doc_id})
        if res["code"] != 0:
            assert False, res
        assert res["data"]["doc"]["chunk_num"] == chunks_count + 1, res

        res = add_chunk(WebApiAuth, {**payload, "doc_id": doc_id})
        assert res["code"] == 0, res
        validate_chunk_details(WebApiAuth, kb_id, doc_id, payload, res)
        res = list_chunks(WebApiAuth, {"doc_id": doc_id})
        if res["code"] != 0:
            assert False, res
        assert res["data"]["doc"]["chunk_num"] == chunks_count + 2, res

    @pytest.mark.p2
    def test_add_chunk_to_deleted_document(self, WebApiAuth, add_document):
        _, doc_id = add_document
        delete_document(WebApiAuth, {"doc_id": doc_id})
        res = add_chunk(WebApiAuth, {"doc_id": doc_id, "content_with_weight": "chunk test"})
        assert res["code"] == 102, res
        assert res["message"] == "Document not found!", res

    @pytest.mark.skip(reason="issues/6411")
    @pytest.mark.p3
    def test_concurrent_add_chunk(self, WebApiAuth, add_document):
        count = 50
        _, doc_id = add_document
        res = list_chunks(WebApiAuth, {"doc_id": doc_id})
        if res["code"] == 0:
            chunks_count = res["data"]["doc"]["chunk_num"]
        else:
            chunks_count = 0

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    add_chunk,
                    WebApiAuth,
                    {"doc_id": doc_id, "content_with_weight": f"chunk test {i}"},
                )
                for i in range(count)
            ]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)
        res = list_chunks(WebApiAuth, {"doc_id": doc_id})
        assert res["code"] == 0, res
        assert res["data"]["doc"]["chunk_num"] == chunks_count + count
