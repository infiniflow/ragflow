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
from conftest import raw_chunks, wait_until
from ragflow_sdk.modules.chunk import ChunkUpdateError

pytestmark = pytest.mark.integration


def assert_updated(document, chunk, field, expected):
    def persisted():
        rows = raw_chunks(document)
        row = next((row for row in rows if row["chunk_id"] == chunk.id), None)
        return row is not None and row.get(field) == expected

    wait_until(persisted)


class TestUpdatedChunk:
    @pytest.mark.parametrize("content", ["update chunk", "\n!?。；！？\"'", "", " "])
    def test_content(self, add_chunks, content):
        _, document, chunks = add_chunks
        chunk = chunks[0]
        assert chunk.update({"content": content}) is None
        assert_updated(document, chunk, "content_with_weight", content)

    @pytest.mark.parametrize(
        "content",
        [
            pytest.param(None, marks=pytest.mark.skip(reason="Go UpdateChunk silently ignores non-string content")),
            pytest.param(1, marks=pytest.mark.skip(reason="Go UpdateChunk silently ignores non-string content")),
        ],
    )
    def test_content_invalid(self, add_chunks, content):
        _, _, chunks = add_chunks
        with pytest.raises(ChunkUpdateError) as error:
            chunks[0].update({"content": content})
        assert error.value.code in (101, 102, 400)
        assert "content" in error.value.message

    @pytest.mark.parametrize("field, stored", [("important_keywords", "important_kwd"), ("questions", "question_kwd"), ("tag_kwd", "tag_kwd")])
    @pytest.mark.parametrize("value", [["a", "b", "c"], [""], ["a", "a"], []])
    def test_list_fields(self, add_chunks, field, stored, value):
        _, document, chunks = add_chunks
        # Seed first so [] must clear existing values rather than preserve defaults.
        chunks[0].update({field: ["seed"]})
        assert_updated(document, chunks[0], stored, ["seed"])
        chunks[0].update({field: value})
        expected = [item.strip() for item in value if item.strip()] if field == "questions" else value
        assert_updated(document, chunks[0], stored, expected)

    @pytest.mark.parametrize(
        "field",
        [
            pytest.param("important_keywords", marks=pytest.mark.skip(reason="Go UpdateChunk ignores invalid keyword types or replaces non-string elements with empty strings")),
            pytest.param("questions", marks=pytest.mark.skip(reason="Go UpdateChunk ignores invalid question types or replaces non-string elements with empty strings")),
            "tag_kwd",
        ],
    )
    @pytest.mark.parametrize("value", [[1], "abc", 123, None])
    def test_list_fields_invalid(self, add_chunks, field, value):
        _, _, chunks = add_chunks
        with pytest.raises(ChunkUpdateError) as error:
            chunks[0].update({field: value})
        assert error.value.code == 102
        assert field in error.value.message
        assert "list" in error.value.message

    @pytest.mark.parametrize("available", [True, False])
    def test_available(self, add_chunks, available):
        _, document, chunks = add_chunks
        chunks[0].update({"available": not available})
        assert_updated(document, chunks[0], "available", not available)
        chunks[0].update({"available": available})
        assert_updated(document, chunks[0], "available", available)

    @pytest.mark.skip(reason="Go UpdateChunk only reads bool availability and silently ignores other types")
    @pytest.mark.parametrize("available", ["True", "False", 1, 0])
    def test_available_invalid(self, add_chunks, available):
        _, _, chunks = add_chunks
        with pytest.raises(ChunkUpdateError) as error:
            chunks[0].update({"available": available})
        assert error.value.code in (101, 102, 400)
        assert "available" in error.value.message

    def test_repeated_update_chunk(self, add_chunks):
        _, document, chunks = add_chunks
        for content in ("chunk test 1", "chunk test 2", "chunk test 2"):
            chunks[0].update({"content": content})
            assert_updated(document, chunks[0], "content_with_weight", content)

    def test_concurrent_update_chunk(self, add_chunks):
        _, document, chunks = add_chunks
        values = {chunk.id: set() for chunk in chunks}
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = []
            for i in range(50):
                chunk = chunks[i % len(chunks)]
                content = f"concurrent update {i}"
                values[chunk.id].add(content)
                futures.append(executor.submit(chunk.update, {"content": content}))
            for future in as_completed(futures):
                assert future.result() is None
        # Completion order is unspecified; each final value must be a submitted write.
        wait_until(lambda: all(row["content_with_weight"] in values[row["chunk_id"]] for row in raw_chunks(document)))
        assert len(raw_chunks(document)) == len(chunks)

    def test_update_chunk_to_deleted_document(self, add_chunks):
        dataset, document, chunks = add_chunks
        dataset.delete_documents(ids=[document.id])
        with pytest.raises(ChunkUpdateError, match="document not found") as error:
            chunks[0].update({})
        assert error.value.code == 500

    def test_unsupported_field(self, add_chunks):
        _, _, chunks = add_chunks
        with pytest.raises(ChunkUpdateError, match="not supported") as error:
            chunks[0].update({"unknown": True})
        assert error.value.code == 400
