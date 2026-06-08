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
"""apply_delta: patch an in-memory nx.Graph with a GraphDelta.

Semantics
---------
* Removed nodes are pruned from the global graph only when no other document's
  subgraph still references them (source_id list becomes empty).
* Added/updated nodes are merged from the new subgraph, appending description
  and source_id so existing cross-document content is preserved.
* Community labels on *untouched* nodes are preserved; communities that
  contain at least one touched node are flagged in the returned
  ``dirty_communities`` set so the caller can re-run community detection only
  for affected clusters.
* PageRank is recomputed over the whole graph after the patch because the
  topology has changed.
"""

from __future__ import annotations

import logging

import networkx as nx

from rag.graphrag.incremental.delta import GraphDelta
from rag.graphrag.utils import GRAPH_FIELD_SEP


def apply_delta(
    global_graph: nx.Graph,
    new_subgraph: nx.Graph,
    delta: GraphDelta,
    old_subgraph: nx.Graph | None = None,
) -> set[str]:
    """Patch global_graph in place using delta + new_subgraph attributes.

    Returns
    -------
    dirty_communities : set[str]
        Community titles (``community`` node attribute) that contain at least
        one touched node and should be re-extracted.
    """
    doc_id = delta.doc_id

    # ------------------------------------------------------------------
    # 1. Remove nodes that disappeared from this document's subgraph,
    #    but only if no other document's subgraph still references them.
    # ------------------------------------------------------------------
    actually_removed: set[str] = set()
    for node in delta.removed_nodes:
        if not global_graph.has_node(node):
            continue
        attrs = global_graph.nodes[node]
        # source_id is a list of doc_ids that reference this node
        src_ids: list[str] = list(attrs.get("source_id", []))
        try:
            src_ids.remove(doc_id)
        except ValueError:
            pass
        if src_ids:
            # Other documents still reference this node — just update the list.
            global_graph.nodes[node]["source_id"] = src_ids
        else:
            global_graph.remove_node(node)
            actually_removed.add(node)

    # ------------------------------------------------------------------
    # 2. Remove edges from deleted nodes and edges that disappeared.
    # ------------------------------------------------------------------
    for src, tgt in delta.removed_edges:
        if not global_graph.has_edge(src, tgt):
            continue
        edge = global_graph.get_edge_data(src, tgt)
        src_ids = list(edge.get("source_id", []))
        try:
            src_ids.remove(doc_id)
        except ValueError:
            pass
        if src_ids:
            global_graph[src][tgt]["source_id"] = src_ids
        else:
            global_graph.remove_edge(src, tgt)

    # ------------------------------------------------------------------
    # 3. Upsert added and updated nodes from new_subgraph.
    # ------------------------------------------------------------------
    for node in (delta.added_nodes | delta.updated_nodes):
        if not new_subgraph.has_node(node):
            continue
        new_attrs = dict(new_subgraph.nodes[node])
        if global_graph.has_node(node):
            existing = global_graph.nodes[node]
            # Merge description
            existing["description"] = (
                existing.get("description", "") + GRAPH_FIELD_SEP + new_attrs.get("description", "")
            ).strip(GRAPH_FIELD_SEP)
            # Union source_ids
            existing_src = list(existing.get("source_id", []))
            if doc_id not in existing_src:
                existing_src.append(doc_id)
            existing["source_id"] = existing_src
        else:
            new_attrs["source_id"] = [doc_id]
            global_graph.add_node(node, **new_attrs)

    # ------------------------------------------------------------------
    # 4. Upsert added and updated edges from new_subgraph.
    # ------------------------------------------------------------------
    for src, tgt in (delta.added_edges | delta.updated_edges):
        if not new_subgraph.has_edge(src, tgt):
            continue
        new_edge = dict(new_subgraph.get_edge_data(src, tgt))
        if global_graph.has_edge(src, tgt):
            existing_edge = global_graph[src][tgt]
            # For updated edges, subtract the old weight contribution of this doc
            # before adding the new one to prevent drift on repeated reprocessing.
            old_subgraph_edge = old_subgraph.get_edge_data(src, tgt) if old_subgraph else None
            old_weight = dict(old_subgraph_edge).get("weight", 0.0) if old_subgraph_edge else 0.0
            existing_edge["weight"] = (
                existing_edge.get("weight", 1.0) - old_weight + new_edge.get("weight", 0.0)
            )
            existing_edge["description"] = (
                existing_edge.get("description", "") + GRAPH_FIELD_SEP + new_edge.get("description", "")
            ).strip(GRAPH_FIELD_SEP)
            existing_edge["keywords"] = list(set(
                existing_edge.get("keywords", []) + new_edge.get("keywords", [])
            ))
            src_ids = list(existing_edge.get("source_id", []))
            if doc_id not in src_ids:
                src_ids.append(doc_id)
            existing_edge["source_id"] = src_ids
        else:
            new_edge["source_id"] = [doc_id]
            global_graph.add_edge(src, tgt, **new_edge)

    # ------------------------------------------------------------------
    # 5. Update the graph-level source_id list.
    #    Add the doc if it still has nodes in the graph; remove it when all
    #    its nodes were pruned (so stale entries don't accumulate).
    # ------------------------------------------------------------------
    doc_still_present = any(
        doc_id in global_graph.nodes[n].get("source_id", [])
        for n in global_graph.nodes()
    )
    graph_src = list(global_graph.graph.get("source_id", []))
    if doc_still_present:
        if doc_id not in graph_src:
            graph_src.append(doc_id)
    else:
        graph_src = [d for d in graph_src if d != doc_id]
    global_graph.graph["source_id"] = graph_src

    # ------------------------------------------------------------------
    # 6. Recompute node degrees and PageRank over the patched graph.
    # ------------------------------------------------------------------
    for node, degree in global_graph.degree():
        global_graph.nodes[node]["rank"] = int(degree)

    if global_graph.number_of_nodes() > 0:
        try:
            pr = nx.pagerank(global_graph)
            for node_name, pagerank in pr.items():
                global_graph.nodes[node_name]["pagerank"] = pagerank
        except Exception:
            logging.exception("apply_delta: PageRank computation failed, skipping.")

    # ------------------------------------------------------------------
    # 7. Identify dirty communities (those containing at least one touched node).
    # ------------------------------------------------------------------
    dirty_communities: set[str] = set()
    touched = delta.touched_nodes
    for node in global_graph.nodes():
        if node in touched:
            community = global_graph.nodes[node].get("community")
            if community:
                dirty_communities.add(community)

    return dirty_communities
