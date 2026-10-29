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


import math
import os
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from conftest import raw_chunks, wait_until

pytestmark = pytest.mark.integration


@pytest.fixture
def retrieval(add_chunks, client):
    dataset, document, chunks = add_chunks
    payload = {"dataset_ids": [dataset.id], "question": "coriander botanical archive", "similarity_threshold": 0, "vector_similarity_weight": 0}
    # Listing visibility does not prove that chunks are searchable.
    wait_until(lambda: {chunk.id for chunk in client.retrieve(**payload)} == {chunk.id for chunk in chunks})
    return payload, dataset, document, chunks


def assert_results(rows, dataset, originals, limit=30):
    expected = {chunk.id: chunk for chunk in originals}
    assert len(rows) <= limit
    assert len({row.id for row in rows}) == len(rows)
    for row in rows:
        assert row.id in expected
        assert row.dataset_id == dataset.id
        assert row.document_id == expected[row.id].document_id
        assert row.content == expected[row.id].content
        assert math.isfinite(row.similarity)
    scores = [row.similarity for row in rows]
    assert scores == sorted(scores, reverse=True)


class TestChunksRetrieval:
    @pytest.mark.parametrize("document_filter", [False, True])
    def test_basic_scenarios(self, client, retrieval, document_filter):
        payload, dataset, document, originals = retrieval
        if document_filter:
            payload["document_ids"] = [document.id]
        rows = client.retrieve(**payload)
        assert rows
        assert_results(rows, dataset, originals)

    @pytest.mark.parametrize("page", [None, 1, 2, 3])
    def test_page(self, client, retrieval, page):
        payload, dataset, _, originals = retrieval
        baseline = client.retrieve(**payload)
        rows = client.retrieve(**{**payload, "page": page, "page_size": 2})
        effective_page = page or 1
        assert len(rows) == len(baseline[(effective_page - 1) * 2 : effective_page * 2])
        assert {row.id for row in rows} <= {row.id for row in baseline}
        assert_results(rows, dataset, originals, 2)

    @pytest.mark.parametrize("page_size", [None, 1, 5])
    def test_page_size(self, client, retrieval, page_size):
        payload, dataset, _, originals = retrieval
        rows = client.retrieve(**{**payload, "page_size": page_size})
        baseline = client.retrieve(**payload)
        assert len(rows) == len(baseline[:page_size])
        assert {row.id for row in rows} <= {row.id for row in baseline}
        assert rows
        assert_results(rows, dataset, originals, page_size or 30)

    @pytest.mark.parametrize("weight", [0, 0.5, 1])
    def test_vector_similarity_weight(self, client, retrieval, weight):
        payload, dataset, _, originals = retrieval
        rows = client.retrieve(**{**payload, "vector_similarity_weight": weight})
        assert rows
        assert_results(rows, dataset, originals)

    @pytest.mark.parametrize("top_k", [1, 10, 2048])
    def test_top_k(self, client, retrieval, top_k):
        payload, dataset, _, originals = retrieval
        # top_k limits vector candidates; engines can fuse additional text matches.
        rows = client.retrieve(**{**payload, "top_k": top_k, "page_size": 2, "vector_similarity_weight": 0.5})
        assert rows
        assert_results(rows, dataset, originals, 2)

    @pytest.mark.parametrize(
        "field,value,message",
        [
            ("page", 0, "page must be greater than or equal to 1"),
            ("page", -1, "page must be greater than or equal to 1"),
            ("page", "3", "cannot unmarshal string"),
            ("page", "a", "cannot unmarshal string"),
            ("page_size", 0, "page_size must be greater than or equal to 1"),
            ("page_size", -1, "page_size must be greater than or equal to 1"),
            ("page_size", "1", "cannot unmarshal string"),
            ("page_size", "a", "cannot unmarshal string"),
            ("vector_similarity_weight", 10, "vector_similarity_weight must be between 0 and 1"),
            ("vector_similarity_weight", -0.1, "vector_similarity_weight must be between 0 and 1"),
            ("vector_similarity_weight", "a", "cannot unmarshal string"),
            ("similarity_threshold", -0.1, "similarity_threshold must be between 0 and 1"),
            ("similarity_threshold", 1.1, "similarity_threshold must be between 0 and 1"),
            ("top_k", -1, "must be between 1 and 2048"),
            ("top_k", 0, "must be between 1 and 2048"),
            ("top_k", 2049, "must be between 1 and 2048"),
            ("top_k", "a", "cannot unmarshal string"),
            ("keyword", "True", "cannot unmarshal string"),
            ("keyword", "False", "cannot unmarshal string"),
            ("question", "", "question is required"),
            ("question", " ", "question is required"),
            ("dataset_ids", [], "dataset_ids array cannot be empty"),
            ("dataset_ids", None, "dataset_ids is required"),
        ],
    )
    def test_invalid_parameters(self, client, add_document, field, value, message):
        dataset, _ = add_document
        payload = {"dataset_ids": [dataset.id], "question": "coriander", field: value}
        with pytest.raises(Exception, match=message):
            client.retrieve(**payload)

    def test_unknown_rerank_id(self, client, retrieval):
        payload, _, _, _ = retrieval
        with pytest.raises(Exception, match="failed to get rerank model by rerank_id"):
            client.retrieve(**{**payload, "rerank_id": "sdk-chunks-nonexistent-reranker"})

    def test_rerank_id(self, client, retrieval):
        model = os.getenv("RAGFLOW_SDK_RERANK_ID")
        assert model, "Set RAGFLOW_SDK_RERANK_ID to an authorized reranker for this integration case"
        payload, dataset, _, originals = retrieval
        rows = client.retrieve(**{**payload, "rerank_id": model})
        assert rows
        assert_results(rows, dataset, originals)

    @pytest.mark.parametrize("keyword", [False, None, True])
    def test_keyword(self, client, retrieval, keyword):
        if keyword:
            assert os.getenv("RAGFLOW_SDK_CHAT_CONFIGURED") == "1", "Keyword expansion needs the isolated tenant's configured chat model"
        payload, dataset, _, originals = retrieval
        rows = client.retrieve(**{**payload, "keyword": keyword})
        assert rows
        assert_results(rows, dataset, originals)

    def test_document_filter(self, client, retrieval):
        payload, dataset, document, originals = retrieval
        other = dataset.upload_documents([{"display_name": "filter.txt", "blob": b"Filter fixture"}])[0]
        extra = other.add_chunk(content="Coriander botanical archive excluded document.")
        wait_until(lambda: extra.id in {row.id for row in client.retrieve(**payload)})
        rows = client.retrieve(**{**payload, "document_ids": [document.id]})
        assert rows
        assert_results(rows, dataset, originals)
        assert client.retrieve(**{**payload, "document_ids": ["missing-document"]}) == []

    def test_concurrent_retrieval(self, client, retrieval):
        payload, dataset, _, originals = retrieval
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(client.retrieve, **payload) for _ in range(20)]
            for future in as_completed(futures):
                rows = future.result()
                assert rows
                assert_results(rows, dataset, originals)

    def test_pagination_covers_results_once(self, client, retrieval):
        payload, _, _, _ = retrieval
        baseline = client.retrieve(**payload)
        pages = [client.retrieve(**{**payload, "page": page, "page_size": 1}) for page in range(1, len(baseline) + 1)]
        ids = [row.id for page in pages for row in page]
        assert len(ids) == len(set(ids)) == len(baseline)
        assert set(ids) == {row.id for row in baseline}

    def test_metadata_filter(self, client, retrieval):
        payload, dataset, document, originals = retrieval
        result = dataset.patch(
            f"/datasets/{dataset.id}/documents/metadatas", {"selector": {"document_ids": [document.id]}, "updates": [{"key": "author", "value": "chunk-audit"}], "deletes": []}
        ).json()
        assert result["code"] == 0, result
        assert result["data"]["updated"] == result["data"]["matched_docs"] == 1
        for author, match in [("chunk-audit", True), ("other-author", False)]:
            condition = {"logic": "and", "conditions": [{"name": "author", "comparison_operator": "=", "value": author}]}
            rows = client.retrieve(**{**payload, "metadata_condition": condition})
            if match:
                assert rows
                assert_results(rows, dataset, originals)
            else:
                assert rows == []

    def test_unavailable_chunks_are_excluded(self, client, retrieval):
        payload, dataset, document, originals = retrieval
        hidden = originals[0]
        hidden.update({"available": False})
        wait_until(lambda: next(row for row in raw_chunks(document) if row["chunk_id"] == hidden.id)["available"] is False)

        def visible_results():
            rows = client.retrieve(**payload)
            return rows if hidden.id not in {row.id for row in rows} else None

        rows = wait_until(visible_results)
        assert_results(rows, dataset, originals[1:])
