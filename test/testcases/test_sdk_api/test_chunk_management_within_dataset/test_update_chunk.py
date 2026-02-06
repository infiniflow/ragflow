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

import pytest


class TestUpdatedChunk:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"content": None}, ""),
            pytest.param(
                {"content": ""},
                """APIRequestFailedError(\'Error code: 400, with error text {"error":{"code":"1213","message":"未正常接收到prompt参数。"}}\')""",
                marks=pytest.mark.skip(reason="issues/6541"),
            ),
            pytest.param(
                {"content": 1},
                "TypeError('expected string or bytes-like object')",
                marks=pytest.mark.skip,
            ),
            ({"content": "update chunk"}, ""),
            pytest.param(
                {"content": " "},
                """APIRequestFailedError(\'Error code: 400, with error text {"error":{"code":"1213","message":"未正常接收到prompt参数。"}}\')""",
                marks=pytest.mark.skip(reason="issues/6541"),
            ),
            ({"content": "\n!?。；！？\"'"}, ""),
        ],
    )
    def test_content(self, add_chunks, payload, expected_message):
        _, _, chunks = add_chunks
        chunk = chunks[0]

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chunk.update(payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunk.update(payload)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"important_keywords": ["a", "b", "c"]}, ""),
            ({"important_keywords": [""]}, ""),
            ({"important_keywords": [1]}, "TypeError('sequence item 0: expected str instance, int found')"),
            ({"important_keywords": ["a", "a"]}, ""),
            ({"important_keywords": "abc"}, "`important_keywords` should be a list"),
            ({"important_keywords": 123}, "`important_keywords` should be a list"),
        ],
    )
    def test_important_keywords(self, add_chunks, payload, expected_message):
        _, _, chunks = add_chunks
        chunk = chunks[0]

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chunk.update(payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunk.update(payload)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"questions": ["a", "b", "c"]}, ""),
            ({"questions": [""]}, ""),
            ({"questions": [1]}, "TypeError('sequence item 0: expected str instance, int found')"),
            ({"questions": ["a", "a"]}, ""),
            ({"questions": "abc"}, "`questions` should be a list"),
            ({"questions": 123}, "`questions` should be a list"),
        ],
    )
    def test_questions(self, add_chunks, payload, expected_message):
        _, _, chunks = add_chunks
        chunk = chunks[0]

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chunk.update(payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunk.update(payload)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"available": True}, ""),
            pytest.param({"available": "True"}, """ValueError("invalid literal for int() with base 10: \'True\'")""", marks=pytest.mark.skip),
            ({"available": 1}, ""),
            ({"available": False}, ""),
            pytest.param({"available": "False"}, """ValueError("invalid literal for int() with base 10: \'False\'")""", marks=pytest.mark.skip),
            ({"available": 0}, ""),
        ],
    )
    def test_available(self, add_chunks, payload, expected_message):
        _, _, chunks = add_chunks
        chunk = chunks[0]

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chunk.update(payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunk.update(payload)

    @pytest.mark.p3
    def test_repeated_update_chunk(self, add_chunks):
        _, _, chunks = add_chunks
        chunk = chunks[0]

        chunk.update({"content": "chunk test 1"})
        chunk.update({"content": "chunk test 2"})

    @pytest.mark.p3
    @pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="issues/6554")
    def test_concurrent_update_chunk(self, add_chunks):
        count = 50
        _, _, chunks = add_chunks

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(chunks[randint(0, 3)].update, {"content": f"update chunk test {i}"}) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses

    @pytest.mark.p3
    def test_update_chunk_to_deleted_document(self, add_chunks):
        dataset, document, chunks = add_chunks
        dataset.delete_documents(ids=[document.id])

        with pytest.raises(Exception) as exception_info:
            chunks[0].update({})
        assert str(exception_info.value) in [f"You don't own the document {chunks[0].document_id}", f"Can't find this chunk {chunks[0].id}"], str(exception_info.value)
