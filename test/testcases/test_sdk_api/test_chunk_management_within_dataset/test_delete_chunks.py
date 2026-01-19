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
from common import batch_add_chunks


class TestChunksDeletion:
    @pytest.mark.parametrize(
        "payload",
        [
            pytest.param(lambda r: {"ids": ["invalid_id"] + r}, marks=pytest.mark.p3),
            pytest.param(lambda r: {"ids": r[:1] + ["invalid_id"] + r[1:4]}, marks=pytest.mark.p1),
            pytest.param(lambda r: {"ids": r + ["invalid_id"]}, marks=pytest.mark.p3),
        ],
    )
    def test_delete_partial_invalid_id(self, add_chunks_func, payload):
        _, document, chunks = add_chunks_func
        chunk_ids = [chunk.id for chunk in chunks]
        payload = payload(chunk_ids)

        with pytest.raises(Exception) as exception_info:
            document.delete_chunks(**payload)
        assert "rm_chunk deleted chunks" in str(exception_info.value), str(exception_info.value)

        remaining_chunks = document.list_chunks()
        assert len(remaining_chunks) == 1, str(remaining_chunks)

    @pytest.mark.p3
    def test_repeated_deletion(self, add_chunks_func):
        _, document, chunks = add_chunks_func
        chunk_ids = [chunk.id for chunk in chunks]
        document.delete_chunks(ids=chunk_ids)

        with pytest.raises(Exception) as exception_info:
            document.delete_chunks(ids=chunk_ids)
        assert "rm_chunk deleted chunks 0, expect" in str(exception_info.value), str(exception_info.value)

    @pytest.mark.p3
    def test_duplicate_deletion(self, add_chunks_func):
        _, document, chunks = add_chunks_func
        chunk_ids = [chunk.id for chunk in chunks]
        document.delete_chunks(ids=chunk_ids * 2)
        remaining_chunks = document.list_chunks()
        assert len(remaining_chunks) == 1, str(remaining_chunks)

    @pytest.mark.p3
    def test_concurrent_deletion(self, add_document):
        count = 100
        _, document = add_document
        chunks = batch_add_chunks(document, count)
        chunk_ids = [chunk.id for chunk in chunks]

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(document.delete_chunks, ids=[chunk_id]) for chunk_id in chunk_ids]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses

    @pytest.mark.p3
    def test_delete_1k(self, add_document):
        count = 1_000
        _, document = add_document
        chunks = batch_add_chunks(document, count)
        chunk_ids = [chunk.id for chunk in chunks]

        from time import sleep

        sleep(1)

        document.delete_chunks(ids=chunk_ids)
        remaining_chunks = document.list_chunks()
        assert len(remaining_chunks) == 0, str(remaining_chunks)

    @pytest.mark.parametrize(
        "payload, expected_message, remaining",
        [
            pytest.param(None, "TypeError", 5, marks=pytest.mark.skip),
            pytest.param({"ids": ["invalid_id"]}, "rm_chunk deleted chunks 0, expect 1", 5, marks=pytest.mark.p3),
            pytest.param("not json", "UnboundLocalError", 5, marks=pytest.mark.skip(reason="pull/6376")),
            pytest.param(lambda r: {"ids": r[:1]}, "", 4, marks=pytest.mark.p3),
            pytest.param(lambda r: {"ids": r}, "", 1, marks=pytest.mark.p1),
            pytest.param({"ids": []}, "", 0, marks=pytest.mark.p3),
        ],
    )
    def test_basic_scenarios(self, add_chunks_func, payload, expected_message, remaining):
        _, document, chunks = add_chunks_func
        chunk_ids = [chunk.id for chunk in chunks]
        if callable(payload):
            payload = payload(chunk_ids)

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                document.delete_chunks(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            document.delete_chunks(**payload)

        remaining_chunks = document.list_chunks()
        assert len(remaining_chunks) == remaining, str(remaining_chunks)
