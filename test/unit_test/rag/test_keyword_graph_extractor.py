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

import pytest

from rag.graphrag.keyword_extractor import KeywordGraphExtractor


@pytest.mark.asyncio
@pytest.mark.p2
async def test_keyword_graph_extractor_builds_graph_without_llm_calls():
    class _FailingLLM:
        async def async_chat(self, *_args, **_kwargs):
            raise AssertionError("keyword extraction must not call the LLM")

    extractor = KeywordGraphExtractor(_FailingLLM(), entity_types=["category"])

    entities, relations = await extractor(
        "doc-1",
        [
            "GraphRAG connects Alice and OpenAI. Alice studies GraphRAG retrieval.",
            "OpenAI builds retrieval systems. GraphRAG improves retrieval for Alice.",
        ],
    )

    entity_names = {entity["entity_name"] for entity in entities}
    assert "GraphRAG" in entity_names
    assert "Alice" in entity_names
    assert relations
    assert all(relation["weight"] > 0 for relation in relations)


@pytest.mark.asyncio
@pytest.mark.p2
async def test_keyword_graph_extractor_counts_edges_with_final_term_frequency():
    extractor = KeywordGraphExtractor(None, entity_types=["category"])

    entities, relations = await extractor(
        "doc-1",
        [
            "North Harbor neural",
            "neural systems",
        ],
    )

    entity_names = {entity["entity_name"] for entity in entities}
    relation_pairs = {
        frozenset((relation["src_id"], relation["tgt_id"]))
        for relation in relations
    }

    assert {"North Harbor", "neural"}.issubset(entity_names)
    assert frozenset(("North Harbor", "neural")) in relation_pairs
