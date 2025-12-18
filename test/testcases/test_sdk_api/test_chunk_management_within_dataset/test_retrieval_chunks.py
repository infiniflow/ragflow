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


class TestChunksRetrieval:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_page_size, expected_message",
        [
            ({"question": "chunk", "dataset_ids": None}, 4, ""),
            ({"question": "chunk", "document_ids": None}, 0, "missing 1 required positional argument"),
            ({"question": "chunk", "dataset_ids": None, "document_ids": None}, 4, ""),
            ({"question": "chunk"}, 0, "missing 1 required positional argument"),
        ],
    )
    def test_basic_scenarios(self, client, add_chunks, payload, expected_page_size, expected_message):
        dataset, document, _ = add_chunks
        if "dataset_ids" in payload:
            payload["dataset_ids"] = [dataset.id]
        if "document_ids" in payload:
            payload["document_ids"] = [document.id]

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                client.retrieve(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunks = client.retrieve(**payload)
            assert len(chunks) == expected_page_size, str(chunks)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_page_size, expected_message",
        [
            pytest.param(
                {"page": None, "page_size": 2},
                2,
                """TypeError("int() argument must be a string, a bytes-like object or a real number, not \'NoneType\'")""",
                marks=pytest.mark.skip,
            ),
            pytest.param(
                {"page": 0, "page_size": 2},
                0,
                "ValueError('Search does not support negative slicing.')",
                marks=pytest.mark.skip,
            ),
            ({"page": 2, "page_size": 2}, 2, ""),
            ({"page": 3, "page_size": 2}, 0, ""),
            ({"page": "3", "page_size": 2}, 0, ""),
            pytest.param(
                {"page": -1, "page_size": 2},
                0,
                "ValueError('Search does not support negative slicing.')",
                marks=pytest.mark.skip,
            ),
            pytest.param(
                {"page": "a", "page_size": 2},
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.skip,
            ),
        ],
    )
    def test_page(self, client, add_chunks, payload, expected_page_size, expected_message):
        dataset, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset.id]})

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                client.retrieve(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunks = client.retrieve(**payload)
            assert len(chunks) == expected_page_size, str(chunks)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_page_size, expected_message",
        [
            pytest.param(
                {"page_size": None},
                0,
                """TypeError("int() argument must be a string, a bytes-like object or a real number, not \'NoneType\'")""",
                marks=pytest.mark.skip,
            ),
            pytest.param({"page_size": 1}, 1, "", marks=pytest.mark.skip(reason="issues/10692")),
            ({"page_size": 5}, 4, ""),
            pytest.param({"page_size": "1"}, 1, "", marks=pytest.mark.skip(reason="issues/10692")),
            pytest.param(
                {"page_size": "a"},
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.skip,
            ),
        ],
    )
    def test_page_size(self, client, add_chunks, payload, expected_page_size, expected_message):
        dataset, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset.id]})

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                client.retrieve(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunks = client.retrieve(**payload)
            assert len(chunks) == expected_page_size, str(chunks)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_page_size, expected_message",
        [
            ({"vector_similarity_weight": 0}, 4, ""),
            ({"vector_similarity_weight": 0.5}, 4, ""),
            ({"vector_similarity_weight": 10}, 4, ""),
            pytest.param(
                {"vector_similarity_weight": "a"},
                0,
                """ValueError("could not convert string to float: 'a'")""",
                marks=pytest.mark.skip,
            ),
        ],
    )
    def test_vector_similarity_weight(self, client, add_chunks, payload, expected_page_size, expected_message):
        dataset, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset.id]})

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                client.retrieve(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunks = client.retrieve(**payload)
            assert len(chunks) == expected_page_size, str(chunks)

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_page_size, expected_message",
        [
            ({"top_k": 10}, 4, ""),
            pytest.param(
                {"top_k": 1},
                4,
                "",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in ["infinity", "opensearch"], reason="Infinity"),
            ),
            pytest.param(
                {"top_k": 1},
                1,
                "",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in [None, "opensearch", "elasticsearch"], reason="elasticsearch"),
            ),
            pytest.param(
                {"top_k": -1},
                4,
                "must be greater than 0",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in ["infinity", "opensearch"], reason="Infinity"),
            ),
            pytest.param(
                {"top_k": -1},
                4,
                "3014",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in [None, "opensearch", "elasticsearch"], reason="elasticsearch"),
            ),
            pytest.param(
                {"top_k": "a"},
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.skip,
            ),
        ],
    )
    def test_top_k(self, client, add_chunks, payload, expected_page_size, expected_message):
        dataset, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset.id]})

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                client.retrieve(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunks = client.retrieve(**payload)
            assert len(chunks) == expected_page_size, str(chunks)

    @pytest.mark.skip
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ({"rerank_id": "BAAI/bge-reranker-v2-m3"}, ""),
            pytest.param({"rerank_id": "unknown"}, "LookupError('Model(unknown) not authorized')", marks=pytest.mark.skip),
        ],
    )
    def test_rerank_id(self, client, add_chunks, payload, expected_message):
        dataset, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset.id]})

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                client.retrieve(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunks = client.retrieve(**payload)
            assert len(chunks) > 0, str(chunks)

    @pytest.mark.skip
    @pytest.mark.parametrize(
        "payload, expected_page_size, expected_message",
        [
            ({"keyword": True}, 5, ""),
            ({"keyword": "True"}, 5, ""),
            ({"keyword": False}, 5, ""),
            ({"keyword": "False"}, 5, ""),
            ({"keyword": None}, 5, ""),
        ],
    )
    def test_keyword(self, client, add_chunks, payload, expected_page_size, expected_message):
        dataset, _, _ = add_chunks
        payload.update({"question": "chunk test", "dataset_ids": [dataset.id]})

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                client.retrieve(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            chunks = client.retrieve(**payload)
            assert len(chunks) == expected_page_size, str(chunks)

    @pytest.mark.p3
    def test_concurrent_retrieval(self, client, add_chunks):
        dataset, _, _ = add_chunks
        count = 100
        payload = {"question": "chunk", "dataset_ids": [dataset.id]}

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(client.retrieve, **payload) for _ in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
