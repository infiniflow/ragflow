#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
"""Tests for the opt-in retrieval diagnosis payload builder."""

# See test_search_rerank.py: importing common.settings first reproduces the
# runtime import order and breaks the rag.nlp.query <-> common.settings cycle.
import common.settings  # noqa: F401
from rag.nlp.search import build_retrieval_debug


def _debug(
    *,
    candidate_ids,
    sorted_ids,
    sorted_scores,
    sorted_term_scores,
    sorted_vector_scores,
    valid_ids,
    returned_ids,
    threshold,
    weight=0.3,
    max_dropped=50,
):
    return build_retrieval_debug(
        candidate_ids=candidate_ids,
        sorted_ids=sorted_ids,
        sorted_scores=sorted_scores,
        sorted_term_scores=sorted_term_scores,
        sorted_vector_scores=sorted_vector_scores,
        valid_ids=valid_ids,
        returned_ids=returned_ids,
        similarity_threshold=threshold,
        vector_similarity_weight=weight,
        max_dropped=max_dropped,
    )


class TestRetrievalDebug:
    def test_funnel_counts(self):
        out = _debug(
            candidate_ids=["a", "b", "c"],
            sorted_ids=["a", "b", "c"],
            sorted_scores=[0.9, 0.5, 0.1],
            sorted_term_scores=[0.9, 0.5, 0.1],
            sorted_vector_scores=[0.9, 0.5, 0.1],
            valid_ids=["a", "b"],
            returned_ids=["a"],
            threshold=0.2,
        )
        assert out["funnel"] == {"candidates": 3, "after_threshold": 2, "returned": 1}
        assert out["similarity_threshold"] == 0.2
        assert out["vector_similarity_weight"] == 0.3

    def test_dropped_chunks_reported_with_reason(self):
        out = _debug(
            candidate_ids=["a", "b", "c"],
            sorted_ids=["a", "b", "c"],
            sorted_scores=[0.9, 0.5, 0.1],
            sorted_term_scores=[0.8, 0.4, 0.05],
            sorted_vector_scores=[0.95, 0.55, 0.15],
            valid_ids=["a", "b"],
            returned_ids=["a"],
            threshold=0.2,
        )
        assert out["dropped_total"] == 1
        assert out["dropped"][0]["chunk_id"] == "c"
        assert out["dropped"][0]["similarity"] == 0.1
        assert out["dropped"][0]["term_similarity"] == 0.05
        assert out["dropped"][0]["vector_similarity"] == 0.15
        assert out["dropped"][0]["reason"] == "below_similarity_threshold"

    def test_returned_chunk_never_marked_dropped(self):
        out = _debug(
            candidate_ids=["a", "b"],
            sorted_ids=["a", "b"],
            sorted_scores=[0.9, 0.85],
            sorted_term_scores=[0.9, 0.85],
            sorted_vector_scores=[0.9, 0.85],
            valid_ids=["a", "b"],
            returned_ids=["a", "b"],
            threshold=0.2,
        )
        assert out["dropped"] == []
        assert out["dropped_total"] == 0

    def test_all_dropped_when_threshold_unmet(self):
        out = _debug(
            candidate_ids=["a", "b"],
            sorted_ids=["a", "b"],
            sorted_scores=[0.1, 0.05],
            sorted_term_scores=[0.1, 0.05],
            sorted_vector_scores=[0.1, 0.05],
            valid_ids=[],
            returned_ids=[],
            threshold=0.5,
        )
        assert out["funnel"]["after_threshold"] == 0
        assert out["funnel"]["returned"] == 0
        assert out["dropped_total"] == 2

    def test_max_dropped_truncates(self):
        n = 10
        out = _debug(
            candidate_ids=[f"c{i}" for i in range(n)],
            sorted_ids=[f"c{i}" for i in range(n)],
            sorted_scores=[0.01] * n,
            sorted_term_scores=[0.01] * n,
            sorted_vector_scores=[0.01] * n,
            valid_ids=[],
            returned_ids=[],
            threshold=0.5,
            max_dropped=3,
        )
        assert out["dropped_total"] == n
        assert len(out["dropped"]) == 3

    def test_empty_inputs(self):
        out = _debug(
            candidate_ids=[],
            sorted_ids=[],
            sorted_scores=[],
            sorted_term_scores=[],
            sorted_vector_scores=[],
            valid_ids=[],
            returned_ids=[],
            threshold=0.2,
        )
        assert out["funnel"] == {"candidates": 0, "after_threshold": 0, "returned": 0}
        assert out["dropped"] == []

    def test_scores_are_cast_to_float(self):
        out = _debug(
            candidate_ids=["a"],
            sorted_ids=["a"],
            sorted_scores=[0],
            sorted_term_scores=[0],
            sorted_vector_scores=[0],
            valid_ids=[],
            returned_ids=[],
            threshold=0.2,
        )
        assert isinstance(out["dropped"][0]["similarity"], float)
        assert isinstance(out["similarity_threshold"], float)
