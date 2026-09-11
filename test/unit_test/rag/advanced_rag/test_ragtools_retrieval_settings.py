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
"""RAGTools carries the caller's retrieval settings through to the search tools."""

import pytest

from rag.advanced_rag.agentic_rag import RAGTools

pytestmark = pytest.mark.p1


class FakeChatModel:
    max_length = 8192

    def clone(self):
        return self


def test_settings_default_to_unset():
    """None, not a value: the search tools own the fallbacks."""
    tools = RAGTools([], FakeChatModel())

    assert tools.similarity_threshold is None
    assert tools.vector_similarity_weight is None
    assert tools.top_n is None
    assert tools.rerank_candidates_count is None
    assert tools.top_k is None


def test_settings_are_carried_through():
    tools = RAGTools(
        [],
        FakeChatModel(),
        similarity_threshold=0.2,
        vector_similarity_weight=0.8,
        top_n=12,
        rerank_candidates_count=64,
        top_k=4096,
    )

    assert tools.similarity_threshold == 0.2
    assert tools.vector_similarity_weight == 0.8
    assert tools.top_n == 12
    assert tools.rerank_candidates_count == 64
    assert tools.top_k == 4096


def test_zero_is_carried_through_not_treated_as_unset():
    tools = RAGTools([], FakeChatModel(), vector_similarity_weight=0.0, similarity_threshold=0.0)

    assert tools.vector_similarity_weight == 0.0
    assert tools.similarity_threshold == 0.0
