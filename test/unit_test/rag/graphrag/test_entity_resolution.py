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

import networkx as nx
import pytest

from rag.graphrag.entity_resolution import EntityResolution
from rag.graphrag.utils import ENTITY_RESOLUTION_ALIASES_KEY, GraphChange


def _resolver():
    return EntityResolution(llm_invoker=None)


def test_is_similarity_matches_within_half_edit_distance():
    resolver = _resolver()

    assert resolver.is_similarity("microsoft", "microsfot") is True
    assert resolver.is_similarity("apple", "orange") is False


def test_is_similarity_identical_strings():
    resolver = _resolver()

    assert resolver.is_similarity("openai", "openai") is True


@pytest.mark.asyncio
async def test_merge_graph_nodes_preserves_existing_canonical(monkeypatch):
    resolver = _resolver()

    async def keep_description(_name, description, task_id=""):
        return description

    monkeypatch.setattr(resolver, "_handle_entity_relation_summary", keep_description)
    graph = nx.Graph()
    graph.add_node("ALIAS", entity_type="ORG", description="alias", source_id=["doc2"])
    graph.add_node("CANONICAL", entity_type="ORG", description="canonical", source_id=["doc1"])
    graph.graph[ENTITY_RESOLUTION_ALIASES_KEY] = {"OLD_ALIAS": "ALIAS", "ALIAS": "CANONICAL"}

    change = GraphChange()
    await resolver._merge_graph_nodes(graph, ["ALIAS", "CANONICAL"], change)

    assert set(graph.nodes) == {"CANONICAL"}
    assert graph.graph[ENTITY_RESOLUTION_ALIASES_KEY] == {
        "OLD_ALIAS": "CANONICAL",
        "ALIAS": "CANONICAL",
    }
    assert change.removed_nodes == {"ALIAS"}
    assert change.added_updated_nodes == {"CANONICAL"}


@pytest.mark.asyncio
async def test_failed_resolution_attempt_does_not_leak_aliases_to_retry(monkeypatch):
    resolver = _resolver()

    async def keep_description(_name, description, task_id=""):
        return description

    monkeypatch.setattr(resolver, "_handle_entity_relation_summary", keep_description)
    original = nx.Graph()
    for name in ("CANONICAL", "MERGED", "OTHER"):
        original.add_node(name, entity_type="ORG", description=name, source_id=[name])
    original.graph[ENTITY_RESOLUTION_ALIASES_KEY] = {"PREVIOUS": "CANONICAL"}

    # A resolution attempt merges MERGED, then fails before its graph is committed.
    failed_attempt = original.copy()
    await resolver._merge_graph_nodes(failed_attempt, ["CANONICAL", "MERGED"], GraphChange())
    assert failed_attempt.graph[ENTITY_RESOLUTION_ALIASES_KEY] == {
        "PREVIOUS": "CANONICAL",
        "MERGED": "CANONICAL",
    }

    # The next attempt starts from the original and merges a different entity.
    retry = original.copy()
    assert retry.graph[ENTITY_RESOLUTION_ALIASES_KEY] == {"PREVIOUS": "CANONICAL"}
    await resolver._merge_graph_nodes(retry, ["CANONICAL", "OTHER"], GraphChange())
    assert original.graph[ENTITY_RESOLUTION_ALIASES_KEY] == {"PREVIOUS": "CANONICAL"}
    assert retry.graph[ENTITY_RESOLUTION_ALIASES_KEY] == {
        "PREVIOUS": "CANONICAL",
        "OTHER": "CANONICAL",
    }
