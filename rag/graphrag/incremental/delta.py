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
"""GraphDelta: the set of changes between an old subgraph and a new one.

Usage
-----
    old_sg = load_subgraph_from_store(...)   # may be None for brand-new docs
    new_sg = generate_subgraph(...)
    delta  = compute_graph_delta(old_sg, new_sg)
    apply_delta(global_graph, delta)

Design
------
A delta is derived purely from node and edge *names* / ids, not from
attribute content.  This means:
  - A node whose attributes changed (description updated) is treated as
    "added/updated" so its embedding is refreshed.
  - A node that disappeared from the new subgraph is "removed" so the
    global graph can shed it if no other document still references it.

The delta only covers the single document that changed.  Cross-document
entity resolution (merging e.g. "IBM" from two docs) still happens in the
normal resolution pass – but because only the dirty doc's subgraph nodes are
in the ``subgraph_nodes`` anchor set, the resolution candidate-pairing work
is bounded to the new/changed entities.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import FrozenSet, Tuple

import networkx as nx


EdgeKey = Tuple[str, str]


@dataclass
class GraphDelta:
    """Represents the node/edge changes for a single changed document.

    Attributes
    ----------
    doc_id:
        The document whose subgraph changed.
    added_nodes:
        Node names present in new_subgraph but not in old_subgraph.
    removed_nodes:
        Node names present in old_subgraph but not in new_subgraph.
    updated_nodes:
        Node names present in both; their attributes may have changed.
    added_edges:
        (src, tgt) pairs present in new_subgraph but not in old_subgraph.
    removed_edges:
        (src, tgt) pairs present in old_subgraph but not in new_subgraph.
    updated_edges:
        (src, tgt) pairs present in both.
    """
    doc_id: str
    added_nodes: FrozenSet[str] = field(default_factory=frozenset)
    removed_nodes: FrozenSet[str] = field(default_factory=frozenset)
    updated_nodes: FrozenSet[str] = field(default_factory=frozenset)
    added_edges: FrozenSet[EdgeKey] = field(default_factory=frozenset)
    removed_edges: FrozenSet[EdgeKey] = field(default_factory=frozenset)
    updated_edges: FrozenSet[EdgeKey] = field(default_factory=frozenset)

    @property
    def touched_nodes(self) -> FrozenSet[str]:
        """All nodes that are new, changed, or removed."""
        return self.added_nodes | self.updated_nodes | self.removed_nodes

    @property
    def is_empty(self) -> bool:
        return not (
            self.added_nodes or self.removed_nodes or self.updated_nodes
            or self.added_edges or self.removed_edges or self.updated_edges
        )


def _edge_key(src: str, tgt: str) -> EdgeKey:
    a, b = sorted([src, tgt])
    return (a, b)


def compute_graph_delta(
    old_subgraph: nx.Graph | None,
    new_subgraph: nx.Graph | None,
    doc_id: str,
) -> GraphDelta:
    """Diff old_subgraph against new_subgraph and return a GraphDelta.

    Either argument may be None (None old_subgraph → brand-new doc;
    None new_subgraph → doc was deleted).
    """
    old_nodes: set[str] = set(old_subgraph.nodes()) if old_subgraph else set()
    new_nodes: set[str] = set(new_subgraph.nodes()) if new_subgraph else set()

    added_nodes = frozenset(new_nodes - old_nodes)
    removed_nodes = frozenset(old_nodes - new_nodes)
    # Only mark as updated when attributes actually differ.
    updated_nodes = frozenset(
        n for n in (old_nodes & new_nodes)
        if dict(old_subgraph.nodes[n]) != dict(new_subgraph.nodes[n])  # type: ignore[union-attr]
    )

    def _edges(g: nx.Graph | None) -> set[EdgeKey]:
        if g is None:
            return set()
        return {_edge_key(u, v) for u, v in g.edges()}

    old_edges = _edges(old_subgraph)
    new_edges = _edges(new_subgraph)

    added_edges = frozenset(new_edges - old_edges)
    removed_edges = frozenset(old_edges - new_edges)
    # Only mark as updated when edge attributes actually differ.
    updated_edges = frozenset(
        e for e in (old_edges & new_edges)
        if dict(old_subgraph.get_edge_data(*e)) != dict(new_subgraph.get_edge_data(*e))  # type: ignore[union-attr]
    )

    return GraphDelta(
        doc_id=doc_id,
        added_nodes=added_nodes,
        removed_nodes=removed_nodes,
        updated_nodes=updated_nodes,
        added_edges=added_edges,
        removed_edges=removed_edges,
        updated_edges=updated_edges,
    )
