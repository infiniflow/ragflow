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

import pytest
from common import batch_add_chunks


class TestChunksList:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size, expected_message",
        [
            ({"page": None, "page_size": 2}, 2, ""),
            pytest.param({"page": 0, "page_size": 2}, 0, "ValueError('Search does not support negative slicing.')", marks=pytest.mark.skip),
            ({"page": 2, "page_size": 2}, 2, ""),
            ({"page": 3, "page_size": 2}, 1, ""),
            ({"page": "3", "page_size": 2}, 1, ""),
            pytest.param({"page": -1, "page_size": 2}, 0, "ValueError('Search does not support negative slicing.')", marks=pytest.mark.skip),
            pytest.param({"page": "a", "page_size": 2}, 0, """ValueError("invalid literal for int() with base 10: \'a\'")""", marks=pytest.mark.skip),
        ],
    )
    def test_page(self, add_chunks, params, expected_page_size, expected_message):
        _, document, _ = add_chunks

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                document.list_chunks(**params)
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            chunks = document.list_chunks(**params)
            assert len(chunks) == expected_page_size, str(chunks)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size, expected_message",
        [
            ({"page_size": None}, 5, ""),
            pytest.param({"page_size": 0}, 5, ""),
            ({"page_size": 1}, 1, ""),
            ({"page_size": 6}, 5, ""),
            ({"page_size": "1"}, 1, ""),
            pytest.param({"page_size": -1}, 5, "", marks=pytest.mark.skip),
            pytest.param({"page_size": "a"}, 0, """ValueError("invalid literal for int() with base 10: \'a\'")""", marks=pytest.mark.skip),
        ],
    )
    def test_page_size(self, add_chunks, params, expected_page_size, expected_message):
        _, document, _ = add_chunks

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                document.list_chunks(**params)
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            chunks = document.list_chunks(**params)
            assert len(chunks) == expected_page_size, str(chunks)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_page_size",
        [
            ({"keywords": None}, 5),
            ({"keywords": ""}, 5),
            ({"keywords": "1"}, 1),
            pytest.param({"keywords": "chunk"}, 4, marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="issues/6509")),
            ({"keywords": "ragflow"}, 1),
            ({"keywords": "unknown"}, 0),
        ],
    )
    def test_keywords(self, add_chunks, params, expected_page_size):
        _, document, _ = add_chunks
        chunks = document.list_chunks(**params)
        assert len(chunks) == expected_page_size, str(chunks)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "chunk_id, expected_page_size, expected_message",
        [
            (None, 5, ""),
            ("", 5, ""),
            pytest.param(lambda r: r[0], 1, "", marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="issues/6499")),
            pytest.param("unknown", 0, """AttributeError("\'NoneType\' object has no attribute \'keys\'")""", marks=pytest.mark.skip),
        ],
    )
    def test_id(self, add_chunks, chunk_id, expected_page_size, expected_message):
        _, document, chunks = add_chunks
        chunk_ids = [chunk.id for chunk in chunks]
        if callable(chunk_id):
            params = {"id": chunk_id(chunk_ids)}
        else:
            params = {"id": chunk_id}

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                document.list_chunks(**params)
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            chunks = document.list_chunks(**params)
            if params["id"] in [None, ""]:
                assert len(chunks) == expected_page_size, str(chunks)
            else:
                assert chunks[0].id == params["id"], str(chunks)

    @pytest.mark.p3
    def test_concurrent_list(self, add_chunks):
        _, document, _ = add_chunks
        count = 100
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(document.list_chunks) for _ in range(count)]

        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(len(future.result()) == 5 for future in futures)

    @pytest.mark.p1
    def test_default(self, add_document):
        _, document = add_document
        batch_add_chunks(document, 31)

        from time import sleep

        sleep(3)

        chunks = document.list_chunks()
        assert len(chunks) == 30, str(chunks)
