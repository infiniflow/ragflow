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
from time import sleep

import pytest
from ragflow_sdk import Chunk


def validate_chunk_details(dataset_id: str, document_id: str, payload: dict, chunk: Chunk):
    assert chunk.dataset_id == dataset_id
    assert chunk.document_id == document_id
    assert chunk.content == payload["content"]
    if "important_keywords" in payload:
        assert chunk.important_keywords == payload["important_keywords"]
    if "questions" in payload:
        assert chunk.questions == [str(q).strip() for q in payload.get("questions", []) if str(q).strip()]


class TestAddChunk:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"content": None}, "not instance of"),
            ({"content": ""}, "`content` is required"),
            ({"content": 1}, "not instance of"),
            ({"content": "a"}, ""),
            ({"content": " "}, "`content` is required"),
            ({"content": "\n!?。；！？\"'"}, ""),
        ],
    )
    def test_content(self, add_document, payload, expected_message):
        dataset, document = add_document
        chunks_count = len(document.list_chunks())

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.add_chunk(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunk = document.add_chunk(**payload)
            validate_chunk_details(dataset.id, document.id, payload, chunk)

            sleep(1)
            chunks = document.list_chunks()
            assert len(chunks) == chunks_count + 1, str(chunks)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"content": "chunk test important_keywords 1", "important_keywords": ["a", "b", "c"]}, ""),
            ({"content": "chunk test important_keywords 2", "important_keywords": [""]}, ""),
            ({"content": "chunk test important_keywords 3", "important_keywords": [1]}, "not instance of"),
            ({"content": "chunk test important_keywords 4", "important_keywords": ["a", "a"]}, ""),
            ({"content": "chunk test important_keywords 5", "important_keywords": "abc"}, "not instance of"),
            ({"content": "chunk test important_keywords 6", "important_keywords": 123}, "not instance of"),
        ],
    )
    def test_important_keywords(self, add_document, payload, expected_message):
        dataset, document = add_document
        chunks_count = len(document.list_chunks())

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.add_chunk(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunk = document.add_chunk(**payload)
            validate_chunk_details(dataset.id, document.id, payload, chunk)

            sleep(1)
            chunks = document.list_chunks()
            assert len(chunks) == chunks_count + 1, str(chunks)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"content": "chunk test test_questions 1", "questions": ["a", "b", "c"]}, ""),
            ({"content": "chunk test test_questions 2", "questions": [""]}, ""),
            ({"content": "chunk test test_questions 3", "questions": [1]}, "not instance of"),
            ({"content": "chunk test test_questions 4", "questions": ["a", "a"]}, ""),
            ({"content": "chunk test test_questions 5", "questions": "abc"}, "not instance of"),
            ({"content": "chunk test test_questions 6", "questions": 123}, "not instance of"),
        ],
    )
    def test_questions(self, add_document, payload, expected_message):
        dataset, document = add_document
        chunks_count = len(document.list_chunks())

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.add_chunk(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunk = document.add_chunk(**payload)
            validate_chunk_details(dataset.id, document.id, payload, chunk)

            sleep(1)
            chunks = document.list_chunks()
            assert len(chunks) == chunks_count + 1, str(chunks)

    @pytest.mark.p3
    def test_repeated_add_chunk(self, add_document):
        payload = {"content": "chunk test repeated_add_chunk"}
        dataset, document = add_document
        chunks_count = len(document.list_chunks())

        chunk1 = document.add_chunk(**payload)
        validate_chunk_details(dataset.id, document.id, payload, chunk1)
        sleep(1)
        chunks = document.list_chunks()
        assert len(chunks) == chunks_count + 1, str(chunks)

        chunk2 = document.add_chunk(**payload)
        validate_chunk_details(dataset.id, document.id, payload, chunk2)
        sleep(1)
        chunks = document.list_chunks()
        assert len(chunks) == chunks_count + 1, str(chunks)

    @pytest.mark.p2
    def test_add_chunk_to_deleted_document(self, add_document):
        dataset, document = add_document
        dataset.delete_documents(ids=[document.id])

        with pytest.raises(Exception) as exception_info:
            document.add_chunk(content="chunk test")
        assert f"You don't own the document {document.id}" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.skip(reason="issues/6411")
    @pytest.mark.p3
    def test_concurrent_add_chunk(self, add_document):
        count = 50
        _, document = add_document
        initial_chunk_count = len(document.list_chunks())

        def add_chunk_task(i):
            return document.add_chunk(content=f"chunk test concurrent {i}")

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(add_chunk_task, i) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        sleep(5)
        assert len(document.list_chunks(page_size=100)) == initial_chunk_count + count
