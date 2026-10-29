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
from conftest import raw_ids, wait_ids

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
    if "tag_kwd" in payload:
        assert chunk.tag_kwd == payload["tag_kwd"]


pytestmark = pytest.mark.integration


class TestAddChunk:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"content": None}, "`content` is required"),
            ({"content": ""}, "`content` is required"),
            ({"content": 1}, "`content` must be a string"),
            ({"content": "a"}, ""),
            ({"content": " "}, "`content` is required"),
            ({"content": "\n!?。；！？\"'"}, ""),
        ],
    )
    def test_content(self, add_document, payload, expected_message):
        dataset, document = add_document
        initial_ids = raw_ids(document)

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.add_chunk(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
            assert raw_ids(document) == initial_ids
        else:
            chunk = document.add_chunk(**payload)
            validate_chunk_details(dataset.id, document.id, payload, chunk)

            wait_ids(document, initial_ids | {chunk.id})

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"content": "chunk test important_keywords 1", "important_keywords": ["a", "b", "c"]}, ""),
            ({"content": "chunk test important_keywords 2", "important_keywords": [""]}, ""),
            ({"content": "chunk test important_keywords 3", "important_keywords": [1]}, "`important_keywords` must be a list of strings"),
            ({"content": "chunk test important_keywords 4", "important_keywords": ["a", "a"]}, ""),
            ({"content": "chunk test important_keywords 5", "important_keywords": "abc"}, "`important_keywords` is required to be a list"),
            ({"content": "chunk test important_keywords 6", "important_keywords": 123}, "`important_keywords` is required to be a list"),
        ],
    )
    def test_important_keywords(self, add_document, payload, expected_message):
        dataset, document = add_document
        initial_ids = raw_ids(document)

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.add_chunk(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
            assert raw_ids(document) == initial_ids
        else:
            chunk = document.add_chunk(**payload)
            validate_chunk_details(dataset.id, document.id, payload, chunk)

            wait_ids(document, initial_ids | {chunk.id})

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"content": "chunk test test_questions 1", "questions": ["a", "b", "c"]}, ""),
            ({"content": "chunk test test_questions 2", "questions": [""]}, ""),
            ({"content": "chunk test test_questions 3", "questions": [1]}, "`questions` must be a list of strings"),
            ({"content": "chunk test test_questions 4", "questions": ["a", "a"]}, ""),
            ({"content": "chunk test test_questions 5", "questions": "abc"}, "`questions` is required to be a list"),
            ({"content": "chunk test test_questions 6", "questions": 123}, "`questions` is required to be a list"),
        ],
    )
    def test_questions(self, add_document, payload, expected_message):
        dataset, document = add_document
        initial_ids = raw_ids(document)

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.add_chunk(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
            assert raw_ids(document) == initial_ids
        else:
            chunk = document.add_chunk(**payload)
            validate_chunk_details(dataset.id, document.id, payload, chunk)

            wait_ids(document, initial_ids | {chunk.id})

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"content": "chunk test test_tag_kwd 1", "tag_kwd": ["tag1", "tag2"]}, ""),
            ({"content": "chunk test test_tag_kwd 2", "tag_kwd": [""]}, ""),
            ({"content": "chunk test test_tag_kwd 3", "tag_kwd": [1]}, "`tag_kwd` must be a list of strings"),
            ({"content": "chunk test test_tag_kwd 4", "tag_kwd": ["tag", "tag"]}, ""),
            ({"content": "chunk test test_tag_kwd 5", "tag_kwd": "abc"}, "`tag_kwd` is required to be a list"),
            ({"content": "chunk test test_tag_kwd 6", "tag_kwd": 123}, "`tag_kwd` is required to be a list"),
        ],
    )
    def test_tag_kwd(self, add_document, payload, expected_message):
        dataset, document = add_document
        initial_ids = raw_ids(document)

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.add_chunk(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
            assert raw_ids(document) == initial_ids
        else:
            chunk = document.add_chunk(**payload)
            validate_chunk_details(dataset.id, document.id, payload, chunk)

            wait_ids(document, initial_ids | {chunk.id})

    @pytest.mark.p3
    def test_repeated_add_chunk(self, add_document):
        payload = {"content": "chunk test repeated_add_chunk"}
        dataset, document = add_document
        initial_ids = raw_ids(document)

        chunk1 = document.add_chunk(**payload)
        validate_chunk_details(dataset.id, document.id, payload, chunk1)
        wait_ids(document, initial_ids | {chunk1.id})

        chunk2 = document.add_chunk(**payload)
        validate_chunk_details(dataset.id, document.id, payload, chunk2)
        assert chunk2.id == chunk1.id
        wait_ids(document, initial_ids | {chunk1.id})

    @pytest.mark.p2
    def test_add_chunk_to_deleted_document(self, add_document):
        dataset, document = add_document
        dataset.delete_documents(ids=[document.id])

        with pytest.raises(Exception) as exception_info:
            document.add_chunk(content="chunk test")
        assert f"you don't own the document {document.id}" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p3
    def test_concurrent_add_chunk(self, add_document):
        count = 50
        _, document = add_document
        initial_ids = raw_ids(document)

        def add_chunk_task(i):
            return document.add_chunk(content=f"chunk test concurrent {i}")

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(add_chunk_task, i) for i in range(count)]
        responses = [future.result() for future in as_completed(futures)]
        assert len({chunk.id for chunk in responses}) == count
        wait_ids(document, initial_ids | {chunk.id for chunk in responses})

    @pytest.mark.parametrize("field", ["important_keywords", "questions", "tag_kwd"])
    @pytest.mark.parametrize("value", [[], None])
    def test_empty_list_and_null(self, add_document, field, value):
        dataset, document = add_document
        payload = {"content": "List boundary", field: value}
        if value is None:
            with pytest.raises(Exception, match=f"`{field}` is required to be a list"):
                document.add_chunk(**payload)
            assert raw_ids(document) == set()
        else:
            chunk = document.add_chunk(**payload)
            validate_chunk_details(dataset.id, document.id, payload, chunk)
            wait_ids(document, [chunk.id])

    @pytest.mark.parametrize(
        "image, error",
        [
            (None, None),
            ("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC", None),
            ("", "`image_base64` must be a non-empty string"),
            (" ", "`image_base64` must be a non-empty string"),
            ("not-base64", "invalid `image_base64`"),
            (123, "`image_base64` must be a string"),
        ],
    )
    def test_image(self, add_document, image, error):
        dataset, document = add_document
        if error:
            with pytest.raises(Exception, match=error):
                document.add_chunk(content="Image boundary", image_base64=image)
            assert raw_ids(document) == set()
        else:
            chunk = document.add_chunk(content="Image boundary", image_base64=image)
            assert chunk.image_id == (f"{dataset.id}-{chunk.id}" if image else "")
            wait_ids(document, [chunk.id])
