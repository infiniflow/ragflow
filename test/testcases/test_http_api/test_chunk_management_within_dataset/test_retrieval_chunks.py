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
from time import sleep

import pytest
from common import add_chunk, delete_chunks, retrieval_chunks
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth


@pytest.mark.p1
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 0, "`Authorization` can't be empty"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                109,
                "Authentication error: API key is invalid!",
            ),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = retrieval_chunks(invalid_auth)
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestChunksRetrieval:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_page_size, expected_message",
        [
            ({"question": "chunk", "dataset_ids": None}, 0, 4, ""),
            ({"question": "chunk", "document_ids": None}, 102, 0, "`dataset_ids` is required."),
            ({"question": "chunk", "dataset_ids": None, "document_ids": None}, 0, 4, ""),
            ({"question": "chunk"}, 102, 0, "`dataset_ids` is required."),
        ],
    )
    def test_basic_scenarios(self, HttpApiAuth, add_chunks, payload, expected_code, expected_page_size, expected_message):
        dataset_id, document_id, _ = add_chunks
        if "dataset_ids" in payload:
            payload["dataset_ids"] = [dataset_id]
        if "document_ids" in payload:
            payload["document_ids"] = [document_id]
        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_page_size, expected_message",
        [
            pytest.param(
                {"page": None, "page_size": 2},
                100,
                2,
                """TypeError("int() argument must be a string, a bytes-like object or a real number, not \'NoneType\'")""",
                marks=pytest.mark.skip,
            ),
            pytest.param(
                {"page": 0, "page_size": 2},
                100,
                0,
                "ValueError('Search does not support negative slicing.')",
                marks=pytest.mark.skip,
            ),
            ({"page": 2, "page_size": 2}, 0, 2, ""),
            ({"page": 3, "page_size": 2}, 0, 0, ""),
            ({"page": "3", "page_size": 2}, 0, 0, ""),
            pytest.param(
                {"page": -1, "page_size": 2},
                100,
                0,
                "ValueError('Search does not support negative slicing.')",
                marks=pytest.mark.skip,
            ),
            pytest.param(
                {"page": "a", "page_size": 2},
                100,
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.skip,
            ),
        ],
    )
    def test_page(self, HttpApiAuth, add_chunks, payload, expected_code, expected_page_size, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset_id]})
        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_code, expected_page_size, expected_message",
        [
            pytest.param(
                {"page_size": None},
                100,
                0,
                """TypeError("int() argument must be a string, a bytes-like object or a real number, not \'NoneType\'")""",
                marks=pytest.mark.skip,
            ),
            # ({"page_size": 0}, 0, 0, ""),
            pytest.param({"page_size": 1}, 0, 1, "", marks=pytest.mark.skip(reason="issues/10692")),
            ({"page_size": 5}, 0, 4, ""),
            pytest.param({"page_size": "1"}, 0, 1, "", marks=pytest.mark.skip(reason="issues/10692")),
            # ({"page_size": -1}, 0, 0, ""),
            pytest.param(
                {"page_size": "a"},
                100,
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.skip,
            ),
        ],
    )
    def test_page_size(self, HttpApiAuth, add_chunks, payload, expected_code, expected_page_size, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset_id]})

        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_code, expected_page_size, expected_message",
        [
            ({"vector_similarity_weight": 0}, 0, 4, ""),
            ({"vector_similarity_weight": 0.5}, 0, 4, ""),
            ({"vector_similarity_weight": 10}, 0, 4, ""),
            pytest.param(
                {"vector_similarity_weight": "a"},
                100,
                0,
                """ValueError("could not convert string to float: \'a\'")""",
                marks=pytest.mark.skip,
            ),
        ],
    )
    def test_vector_similarity_weight(self, HttpApiAuth, add_chunks, payload, expected_code, expected_page_size, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset_id]})
        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload, expected_code, expected_page_size, expected_message",
        [
            ({"top_k": 10}, 0, 4, ""),
            pytest.param(
                {"top_k": 1},
                0,
                4,
                "",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in ["infinity", "opensearch"], reason="Infinity"),
            ),
            pytest.param(
                {"top_k": 1},
                0,
                1,
                "",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in [None, "opensearch", "elasticsearch"], reason="elasticsearch"),
            ),
            pytest.param(
                {"top_k": -1},
                100,
                4,
                "must be greater than 0",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in ["infinity", "opensearch"], reason="Infinity"),
            ),
            pytest.param(
                {"top_k": -1},
                100,
                4,
                "3014",
                marks=pytest.mark.skipif(os.getenv("DOC_ENGINE") in [None, "opensearch", "elasticsearch"], reason="elasticsearch"),
            ),
            pytest.param(
                {"top_k": "a"},
                100,
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.skip,
            ),
        ],
    )
    def test_top_k(self, HttpApiAuth, add_chunks, payload, expected_code, expected_page_size, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset_id]})
        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert expected_message in res["message"]

    @pytest.mark.skip
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"rerank_id": "BAAI/bge-reranker-v2-m3"}, 0, ""),
            pytest.param({"rerank_id": "unknown"}, 100, "LookupError('Model(unknown) not authorized')", marks=pytest.mark.skip),
        ],
    )
    def test_rerank_id(self, HttpApiAuth, add_chunks, payload, expected_code, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset_id]})
        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) > 0
        else:
            assert expected_message in res["message"]

    @pytest.mark.skip
    @pytest.mark.parametrize(
        "payload, expected_code, expected_page_size, expected_message",
        [
            ({"keyword": True}, 0, 5, ""),
            ({"keyword": "True"}, 0, 5, ""),
            ({"keyword": False}, 0, 5, ""),
            ({"keyword": "False"}, 0, 5, ""),
            ({"keyword": None}, 0, 5, ""),
        ],
    )
    def test_keyword(self, HttpApiAuth, add_chunks, payload, expected_code, expected_page_size, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk test", "dataset_ids": [dataset_id]})
        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["chunks"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_code, expected_highlight, expected_message",
        [
            ({"highlight": True}, 0, True, ""),
            ({"highlight": "True"}, 0, True, ""),
            pytest.param({"highlight": False}, 0, False, "", marks=pytest.mark.skip(reason="issues/6648")),
            ({"highlight": "False"}, 0, False, ""),
            pytest.param({"highlight": None}, 0, False, "", marks=pytest.mark.skip(reason="issues/6648")),
        ],
    )
    def test_highlight(self, HttpApiAuth, add_chunks, payload, expected_code, expected_highlight, expected_message):
        dataset_id, _, _ = add_chunks
        payload.update({"question": "chunk", "dataset_ids": [dataset_id]})
        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == expected_code
        doc_engine = os.environ.get("DOC_ENGINE", "elasticsearch").lower()
        if expected_highlight and doc_engine != "infinity":
            for chunk in res["data"]["chunks"]:
                assert "highlight" in chunk
        else:
            for chunk in res["data"]["chunks"]:
                assert "highlight" not in chunk

        if expected_code != 0:
            assert res["message"] == expected_message

    @pytest.mark.p3
    def test_invalid_params(self, HttpApiAuth, add_chunks):
        dataset_id, _, _ = add_chunks
        payload = {"question": "chunk", "dataset_ids": [dataset_id], "a": "b"}
        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == 0
        assert len(res["data"]["chunks"]) == 4

    @pytest.mark.p3
    def test_concurrent_retrieval(self, HttpApiAuth, add_chunks):
        dataset_id, _, _ = add_chunks
        count = 100
        payload = {"question": "chunk", "dataset_ids": [dataset_id]}

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(retrieval_chunks, HttpApiAuth, payload) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)


class TestDeletedChunksNotRetrievable:
    """Regression tests for issue #12520: deleted slices should not appear in retrieval/reference."""

    @pytest.mark.p1
    def test_deleted_chunk_not_in_retrieval(self, HttpApiAuth, add_document):
        """
        Test that a deleted chunk is not returned by the retrieval API.

        Steps:
        1. Add a chunk with unique content
        2. Verify the chunk is retrievable
        3. Delete the chunk
        4. Verify the chunk is no longer retrievable
        """
        dataset_id, document_id = add_document

        # Add a chunk with unique content that we can search for
        unique_content = "UNIQUE_TEST_CONTENT_12520_REGRESSION"
        res = add_chunk(HttpApiAuth, dataset_id, document_id, {"content": unique_content})
        assert res["code"] == 0, f"Failed to add chunk: {res}"
        chunk_id = res["data"]["chunk"]["id"]

        # Wait for indexing to complete
        sleep(2)

        # Verify the chunk is retrievable
        payload = {"question": unique_content, "dataset_ids": [dataset_id]}
        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == 0, f"Retrieval failed: {res}"
        chunk_ids_before = [c["id"] for c in res["data"]["chunks"]]
        assert chunk_id in chunk_ids_before, f"Chunk {chunk_id} should be retrievable before deletion"

        # Delete the chunk
        res = delete_chunks(HttpApiAuth, dataset_id, document_id, {"chunk_ids": [chunk_id]})
        assert res["code"] == 0, f"Failed to delete chunk: {res}"

        # Wait for deletion to propagate
        sleep(1)

        # Verify the chunk is no longer retrievable
        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == 0, f"Retrieval failed after deletion: {res}"
        chunk_ids_after = [c["id"] for c in res["data"]["chunks"]]
        assert chunk_id not in chunk_ids_after, f"Chunk {chunk_id} should NOT be retrievable after deletion"

    @pytest.mark.p2
    def test_deleted_chunks_batch_not_in_retrieval(self, HttpApiAuth, add_document):
        """
        Test that multiple deleted chunks are not returned by retrieval.
        """
        dataset_id, document_id = add_document

        # Add multiple chunks with unique content
        chunk_ids = []
        for i in range(3):
            unique_content = f"BATCH_DELETE_TEST_CHUNK_{i}_12520"
            res = add_chunk(HttpApiAuth, dataset_id, document_id, {"content": unique_content})
            assert res["code"] == 0, f"Failed to add chunk {i}: {res}"
            chunk_ids.append(res["data"]["chunk"]["id"])

        # Wait for indexing
        sleep(2)

        # Verify chunks are retrievable
        payload = {"question": "BATCH_DELETE_TEST_CHUNK", "dataset_ids": [dataset_id]}
        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == 0
        retrieved_ids_before = [c["id"] for c in res["data"]["chunks"]]
        for cid in chunk_ids:
            assert cid in retrieved_ids_before, f"Chunk {cid} should be retrievable before deletion"

        # Delete all chunks
        res = delete_chunks(HttpApiAuth, dataset_id, document_id, {"chunk_ids": chunk_ids})
        assert res["code"] == 0, f"Failed to delete chunks: {res}"

        # Wait for deletion to propagate
        sleep(1)

        # Verify none of the chunks are retrievable
        res = retrieval_chunks(HttpApiAuth, payload)
        assert res["code"] == 0
        retrieved_ids_after = [c["id"] for c in res["data"]["chunks"]]
        for cid in chunk_ids:
            assert cid not in retrieved_ids_after, f"Chunk {cid} should NOT be retrievable after deletion"
