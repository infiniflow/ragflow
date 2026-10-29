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
from conftest import raw_ids, wait_ids

pytestmark = pytest.mark.integration


class TestChunksDeletion:
    @pytest.mark.parametrize("position", [0, 2, 4])
    def test_delete_partial_invalid_id(self, add_chunks, position):
        _, document, chunks = add_chunks
        ids = [chunk.id for chunk in chunks]
        ids.insert(position, "unknown")
        document.delete_chunks(ids=ids)
        wait_ids(document, [])

    def test_repeated_deletion(self, add_chunks):
        _, document, chunks = add_chunks
        ids = [chunk.id for chunk in chunks]
        document.delete_chunks(ids=ids)
        wait_ids(document, [])
        document.delete_chunks(ids=ids)
        assert raw_ids(document) == set()

    def test_duplicate_deletion(self, add_chunks):
        _, document, chunks = add_chunks
        document.delete_chunks(ids=[chunk.id for chunk in chunks] * 2)
        wait_ids(document, [])

    def test_concurrent_deletion(self, add_chunks):
        _, document, chunks = add_chunks
        with ThreadPoolExecutor(max_workers=4) as executor:
            futures = [executor.submit(document.delete_chunks, ids=[chunk.id]) for chunk in chunks]
            for future in as_completed(futures):
                assert future.result() is None
        wait_ids(document, [])

    def test_delete_1k(self, add_document):
        _, document = add_document
        chunks = [document.add_chunk(content=f"bulk chunk {i}") for i in range(1000)]
        ids = [chunk.id for chunk in chunks]
        wait_ids(document, ids)
        document.delete_chunks(ids=ids)
        wait_ids(document, [])

    @pytest.mark.parametrize(
        "payload, removed",
        [
            ({"ids": ["unknown"]}, 0),
            ("single", 1),
            ("all_ids", 4),
            ({"delete_all": True}, 4),
            ({"ids": [], "delete_all": True}, 4),
        ],
    )
    def test_basic_scenarios(self, add_chunks, payload, removed):
        _, document, chunks = add_chunks
        ids = [chunk.id for chunk in chunks]
        if payload == "single":
            payload = {"ids": ids[:1]}
        elif payload == "all_ids":
            payload = {"ids": ids}
        document.delete_chunks(**payload)
        wait_ids(document, ids[removed:])

    @pytest.mark.skip(reason="Go RemoveChunks rejects omitted/empty IDs instead of documented safe no-op")
    @pytest.mark.parametrize("payload", [{}, {"ids": None}, {"ids": []}])
    def test_empty_selection_is_noop(self, add_chunks, payload):
        _, document, chunks = add_chunks
        document.delete_chunks(**payload)
        assert raw_ids(document) == {chunk.id for chunk in chunks}

    def test_delete_all_rejects_ids(self, add_chunks):
        _, document, chunks = add_chunks
        with pytest.raises(Exception, match="mutually exclusive"):
            document.delete_chunks(ids=[chunks[0].id], delete_all=True)
        assert raw_ids(document) == {chunk.id for chunk in chunks}

    def test_delete_all_is_document_scoped(self, add_chunks):
        dataset, document, _ = add_chunks
        other = dataset.upload_documents([{"display_name": "other.txt", "blob": b"Other owned document"}])[0]
        chunk = other.add_chunk(content="Must survive deletion in another document")
        wait_ids(other, [chunk.id])
        document.delete_chunks(delete_all=True)
        wait_ids(document, [])
        assert raw_ids(other) == {chunk.id}
