# Copyright (c) 2024 Microsoft Corporation.
# Licensed under the MIT License
"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
"""

import logging
import html
from typing import Any, cast
from graspologic.partition import hierarchical_leiden
from graspologic.utils import largest_connected_component
import networkx as nx
from networkx import is_empty


def _stabilize_graph(graph: nx.Graph) -> nx.Graph:
    """Ensure an undirected graph with the same relationships will always be read the same way."""
    fixed_graph = nx.DiGraph() if graph.is_directed() else nx.Graph()

    sorted_nodes = graph.nodes(data=True)
    sorted_nodes = sorted(sorted_nodes, key=lambda x: x[0])

    fixed_graph.add_nodes_from(sorted_nodes)
    edges = list(graph.edges(data=True))

    # If the graph is undirected, we create the edges in a stable way, so we get the same results
    # for example:
    # A -> B
    # in graph theory is the same as
    # B -> A
    # in an undirected graph
    # however, this can lead to downstream issues because sometimes
    # consumers read graph.nodes() which ends up being [A, B] and sometimes it's [B, A]
    # but they base some of their logic on the order of the nodes, so the order ends up being important
    # so we sort the nodes in the edge in a stable way, so that we always get the same order
    if not graph.is_directed():

        def _sort_source_target(edge):
            source, target, edge_data = edge
            if source > target:
                temp = source
                source = target
                target = temp
            return source, target, edge_data

        edges = [_sort_source_target(edge) for edge in edges]

    def _get_edge_key(source: Any, target: Any) -> str:
        return f"{source} -> {target}"

    edges = sorted(edges, key=lambda x: _get_edge_key(x[0], x[1]))

    fixed_graph.add_edges_from(edges)
    return fixed_graph


def normalize_node_names(graph: nx.Graph | nx.DiGraph) -> nx.Graph | nx.DiGraph:
    """Normalize node names."""
    node_mapping = {node: html.unescape(node.upper().strip()) for node in graph.nodes()}  # type: ignore
    return nx.relabel_nodes(graph, node_mapping)


def stable_largest_connected_component(graph: nx.Graph) -> nx.Graph:
    """Return the largest connected component of the graph, with nodes and edges sorted in a stable way."""
    graph = graph.copy()
    graph = cast(nx.Graph, largest_connected_component(graph))
    graph = normalize_node_names(graph)
    return _stabilize_graph(graph)


def _compute_leiden_communities(
        graph: nx.Graph | nx.DiGraph,
        max_cluster_size: int,
        use_lcc: bool,
        seed=0xDEADBEEF,
) -> dict[int, dict[str, int]]:
    """Return Leiden root communities."""
    results: dict[int, dict[str, int]] = {}
    if is_empty(graph):
        return results
    if use_lcc:
        graph = stable_largest_connected_component(graph)

    community_mapping = hierarchical_leiden(
        graph, max_cluster_size=max_cluster_size, random_seed=seed
    )
    for partition in community_mapping:
        results[partition.level] = results.get(partition.level, {})
        results[partition.level][partition.node] = partition.cluster

    return results


def run(graph: nx.Graph, args: dict[str, Any]) -> dict[int, dict[str, dict]]:
    """Run method definition."""
    max_cluster_size = args.get("max_cluster_size", 12)
    use_lcc = args.get("use_lcc", True)
    if args.get("verbose", False):
        logging.debug(
            "Running leiden with max_cluster_size=%s, lcc=%s", max_cluster_size, use_lcc
        )
    if not graph.nodes():
        return {}

    node_id_to_community_map = _compute_leiden_communities(
        graph=graph,
        max_cluster_size=max_cluster_size,
        use_lcc=use_lcc,
        seed=args.get("seed", 0xDEADBEEF),
    )
    levels = args.get("levels")

    # If they don't pass in levels, use them all
    if levels is None:
        levels = sorted(node_id_to_community_map.keys())

    results_by_level: dict[int, dict[str, list[str]]] = {}
    for level in levels:
        result = {}
        results_by_level[level] = result
        for node_id, raw_community_id in node_id_to_community_map[level].items():
            if node_id not in graph.nodes:
                logging.warning(f"Node {node_id} not found in the graph.")
                continue
            community_id = str(raw_community_id)
            if community_id not in result:
                result[community_id] = {"weight": 0, "nodes": []}
            result[community_id]["nodes"].append(node_id)
            result[community_id]["weight"] += graph.nodes[node_id].get("rank", 0) * graph.nodes[node_id].get("weight", 1)
        weights = [comm["weight"] for _, comm in result.items()]
        if not weights:
            continue
        max_weight = max(weights)
        if max_weight == 0:
            continue
        for _, comm in result.items():
            comm["weight"] /= max_weight

    return results_by_level


def add_community_info2graph(graph: nx.Graph, nodes: list[str], community_title):
    for n in nodes:
        if "communities" not in graph.nodes[n]:
            graph.nodes[n]["communities"] = []
        graph.nodes[n]["communities"].append(community_title)
        graph.nodes[n]["communities"] = list(set(graph.nodes[n]["communities"]))
