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
from conftest import wait_ids
from ragflow_sdk import Document

pytestmark = pytest.mark.integration


class TestChunksList:
    @pytest.mark.parametrize(
        "params, size, error",
        [
            ({"page": None, "page_size": 2}, 2, None),
            ({"page": 0}, 0, "page must be a positive integer"),
            ({"page": 2, "page_size": 2}, 2, None),
            ({"page": 3, "page_size": 2}, 0, None),
            ({"page": "3", "page_size": 2}, 0, None),
            ({"page": -1}, 0, "page must be a positive integer"),
            ({"page": "a"}, 0, "page must be a positive integer"),
        ],
    )
    def test_page(self, add_chunks, params, size, error):
        _, document, _ = add_chunks
        if error:
            with pytest.raises(Exception, match=error):
                document.list_chunks(**params)
        else:
            assert len(document.list_chunks(**params)) == size

    @pytest.mark.parametrize(
        "value, size, error",
        [
            (None, 4, None),
            (0, 0, "page_size must be a positive integer"),
            (1, 1, None),
            (6, 4, None),
            ("1", 1, None),
            (-1, 0, "page_size must be a positive integer"),
            ("a", 0, "page_size must be a positive integer"),
        ],
    )
    def test_page_size(self, add_chunks, value, size, error):
        _, document, _ = add_chunks
        if error:
            with pytest.raises(Exception, match=error):
                document.list_chunks(page_size=value)
        else:
            assert len(document.list_chunks(page_size=value)) == size

    @pytest.mark.parametrize("keywords, size", [(None, 4), ("", 4), ("coriander", 4), ("saffron", 1), ("1", 1), ("unfindablequasar", 0)])
    def test_keywords(self, add_chunks, keywords, size):
        _, document, _ = add_chunks
        assert len(document.list_chunks(keywords=keywords)) == size

    @pytest.mark.parametrize("chunk_id", [None, ""])
    def test_id_empty(self, add_chunks, chunk_id):
        _, document, _ = add_chunks
        assert len(document.list_chunks(id=chunk_id)) == 4

    @pytest.mark.skip(reason="SDK sends id but Go ListChunks only reads chunk_ids; Go list also omits SDK id field")
    @pytest.mark.parametrize("selection", ["existing", "unknown"])
    def test_id(self, add_chunks, selection):
        _, document, chunks = add_chunks
        wanted = chunks[0].id if selection == "existing" else "unknown"
        rows = document.list_chunks(id=wanted)
        assert [chunk.id for chunk in rows] == ([wanted] if selection == "existing" else [])

    @pytest.mark.skip(reason="Go List returns chunk_id/content_with_weight/important_kwd/question_kwd without SDK field mapping")
    def test_list_chunk_schema(self, add_chunks):
        dataset, document, chunks = add_chunks
        rows = document.list_chunks(page_size=100)
        assert {chunk.id for chunk in rows} == {chunk.id for chunk in chunks}
        for chunk in rows:
            expected = next(original for original in chunks if original.id == chunk.id)
            assert chunk.content == expected.content
            assert chunk.document_id == document.id
            assert chunk.dataset_id == dataset.id
            assert chunk.important_keywords == expected.important_keywords
            assert chunk.questions == expected.questions

    def test_concurrent_list(self, add_chunks):
        _, document, _ = add_chunks
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(document.list_chunks) for _ in range(20)]
            for future in as_completed(futures):
                assert len(future.result()) == 4

    def test_default(self, add_document):
        _, document = add_document
        chunks = [document.add_chunk(content=f"default pagination {i}") for i in range(31)]
        wait_ids(document, [chunk.id for chunk in chunks])
        assert len(document.list_chunks()) == 30

    def test_list_chunks_invalid_document_id_raises(self, add_chunks):
        dataset, _, _ = add_chunks
        document = Document(dataset.rag, {"id": "missing-document", "dataset_id": dataset.id})
        with pytest.raises(Exception, match="document not found"):
            document.list_chunks()
