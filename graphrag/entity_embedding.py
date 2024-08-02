#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
"""

from typing import Any

import numpy as np
import networkx as nx
from graphrag.leiden import stable_largest_connected_component


@dataclass
class NodeEmbeddings:
    """Node embeddings class definition."""

    nodes: list[str]
    embeddings: np.ndarray


def embed_nod2vec(
    graph: nx.Graph | nx.DiGraph,
    dimensions: int = 1536,
    num_walks: int = 10,
    walk_length: int = 40,
    window_size: int = 2,
    iterations: int = 3,
    random_seed: int = 86,
) -> NodeEmbeddings:
    """Generate node embeddings using Node2Vec."""
    # generate embedding
    lcc_tensors = gc.embed.node2vec_embed(  # type: ignore
        graph=graph,
        dimensions=dimensions,
        window_size=window_size,
        iterations=iterations,
        num_walks=num_walks,
        walk_length=walk_length,
        random_seed=random_seed,
    )
    return NodeEmbeddings(embeddings=lcc_tensors[0], nodes=lcc_tensors[1])


def run(graph: nx.Graph, args: dict[str, Any]) -> NodeEmbeddings:
    """Run method definition."""
    if args.get("use_lcc", True):
        graph = stable_largest_connected_component(graph)

    # create graph embedding using node2vec
    embeddings = embed_nod2vec(
        graph=graph,
        dimensions=args.get("dimensions", 1536),
        num_walks=args.get("num_walks", 10),
        walk_length=args.get("walk_length", 40),
        window_size=args.get("window_size", 2),
        iterations=args.get("iterations", 3),
        random_seed=args.get("random_seed", 86),
    )

    pairs = zip(embeddings.nodes, embeddings.embeddings.tolist(), strict=True)
    sorted_pairs = sorted(pairs, key=lambda x: x[0])

    return dict(sorted_pairs)