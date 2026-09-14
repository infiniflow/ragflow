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
"""Regression tests for Extractor._merge_graph_nodes concurrency bug.

The historical implementation iterated over ``graph.neighbors(node1)`` directly
while mutating ``graph`` in the loop body (``add_edge`` / ``remove_node``).
Under concurrent merges on overlapping neighbourhoods this raised
``RuntimeError: dictionary keys changed during iteration``.

The fix snapshots the neighbour list. These tests pin that behaviour so the
bug cannot silently regress.
"""

import asyncio
from types import SimpleNamespace

import networkx as nx
import pytest

from rag.graphrag.general.extractor import Extractor
from rag.graphrag.utils import GraphChange


def _stub_extractor() -> Extractor:
    llm = SimpleNamespace(llm_name="test-llm", max_length=4096)
    ext = Extractor.__new__(Extractor)
    ext._llm = llm
    ext._language = "English"

    async def _summary(_name, desc, task_id=""):
        return desc

    ext._handle_entity_relation_summary = _summary  # type: ignore[assignment]
    return ext


def _make_node(graph: nx.Graph, name: str) -> None:
    graph.add_node(
        name,
        description=f"desc-{name}",
        source_id=[name],
        entity_type="person",
    )


def _make_edge(graph: nx.Graph, src: str, tgt: str) -> None:
    graph.add_edge(
        src,
        tgt,
        src_id=src,
        tgt_id=tgt,
        description=f"{src}->{tgt}",
        weight=1.0,
        keywords=[],
        source_id=[src],
    )


@pytest.mark.p1
@pytest.mark.asyncio
async def test_merge_graph_nodes_handles_dense_neighbourhood():
    """A node with many neighbours must merge cleanly without raising."""
    graph = nx.Graph()
    for name in ["A", "B"] + [f"N{i}" for i in range(20)]:
        _make_node(graph, name)
    for i in range(20):
        _make_edge(graph, "A", f"N{i}")
        _make_edge(graph, "B", f"N{i}")

    ext = _stub_extractor()
    change = GraphChange()
    await ext._merge_graph_nodes(graph, ["A", "B"], change)

    assert "B" not in graph.nodes
    assert "A" in graph.nodes
    # All 20 N* neighbours should still be connected to the surviving node A
    assert set(graph.neighbors("A")) == {f"N{i}" for i in range(20)}


@pytest.mark.p1
@pytest.mark.asyncio
async def test_merge_graph_nodes_neighbours_are_snapshotted():
    """Regression: iterating graph.neighbors() must not explode if the
    underlying adjacency dict is mutated during the loop."""
    graph = nx.Graph()
    for name in ["A", "B", "C", "D"]:
        _make_node(graph, name)
    # B and C share neighbour D, so merging {A, B} adds edge A-D while
    # the neighbour iterator for B is live.
    _make_edge(graph, "B", "C")
    _make_edge(graph, "B", "D")
    _make_edge(graph, "A", "D")

    ext = _stub_extractor()
    change = GraphChange()
    await ext._merge_graph_nodes(graph, ["A", "B"], change)

    assert "B" not in graph.nodes
    assert graph.has_edge("A", "C")
    assert graph.has_edge("A", "D")


@pytest.mark.p1
@pytest.mark.asyncio
async def test_concurrent_merges_do_not_raise_under_semaphore():
    """Two concurrent merges on overlapping neighbourhoods must succeed
    when serialized (as entity_resolution now does via Semaphore(1))."""
    graph = nx.Graph()
    for name in ["A1", "A2", "B1", "B2", "X"]:
        _make_node(graph, name)
    _make_edge(graph, "A1", "X")
    _make_edge(graph, "A2", "X")
    _make_edge(graph, "B1", "X")
    _make_edge(graph, "B2", "X")

    ext = _stub_extractor()
    change = GraphChange()
    sem = asyncio.Semaphore(1)

    async def merge(nodes):
        async with sem:
            await ext._merge_graph_nodes(graph, nodes, change)

    await asyncio.gather(merge(["A1", "A2"]), merge(["B1", "B2"]))

    assert "A2" not in graph.nodes and "B2" not in graph.nodes
    # Both survivors must still share neighbour X
    assert graph.has_edge("A1", "X")
    assert graph.has_edge("B1", "X")
