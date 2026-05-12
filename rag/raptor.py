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
import asyncio
from dataclasses import dataclass, field
import logging
import re

import numpy as np
import umap
from sklearn.cluster import AgglomerativeClustering
from sklearn.mixture import GaussianMixture

from api.db.services.task_service import has_canceled
from common.connection_utils import timeout
from common.exceptions import TaskCanceledException
from common.token_utils import truncate
from rag.graphrag.utils import (
    chat_limiter,
    get_embed_cache,
    get_llm_cache,
    set_embed_cache,
    set_llm_cache,
)
from common.misc_utils import thread_pool_exec
from rag.utils.raptor_utils import (
    AHC_CLUSTERING_METHOD,
    GMM_CLUSTERING_METHOD,
    PSI_TREE_BUILDER,
    RAPTOR_TREE_BUILDER,
    SUPPORTED_CLUSTERING_METHODS,
    SUPPORTED_TREE_BUILDERS,
)


@dataclass
class _PsiTreeNode:
    """Node used to represent the in-memory Psi merge tree."""

    index: int
    text: str = ""
    embedding: np.ndarray | None = None
    children: list["_PsiTreeNode"] = field(default_factory=list)
    parent: "_PsiTreeNode | None" = None


class _PsiUnionFind:
    """Build parent links for the Psi merge tree from ranked leaf pairs."""

    def __init__(self, n: int):
        """Initialize the union-find state for n leaf nodes."""
        self._rank = [0 for _ in range(n)]
        self._parent_chains = [[] for _ in range(n)]
        self._node_ids = [[i] for i in range(n)]
        self._tree = [-1 for _ in range(max(1, 2 * n - 1))]
        self._next_id = n

    @staticmethod
    def _ordered_extend(target: list[int], values: list[int]):
        """Append unseen values while preserving their original order."""
        for value in values:
            if value not in target:
                target.append(value)

    def _find(self, i: int) -> list[int]:
        """Return the parent chain for a leaf, extending it lazily."""
        chain = self._parent_chains[i]
        if not chain or (len(chain) == 1 and chain[0] == i):
            return [i]
        if chain[0] == i:
            self._ordered_extend(chain, self._find(chain[1]))
        else:
            self._ordered_extend(chain, self._find(chain[0]))
        return chain

    def _rank_bisect_right(self, chain: list[int], rank: int) -> int:
        """Return the first chain index whose rank is greater than rank."""
        idx = 0
        while idx < len(chain) and self._rank[chain[idx]] <= rank:
            idx += 1
        return idx

    def _build(self, i: int, j: int, insert_point: int | None = None):
        """Record a merge edge in the compact parent array."""
        if insert_point is not None:
            parent_ids = self._node_ids[insert_point]
            parent_rank_idx = self._rank[i] + 1
            if parent_rank_idx >= len(parent_ids):
                logging.warning(
                    "RAPTOR Psi union fallback: rank index %d is out of bounds for node %d with %d parent ids",
                    parent_rank_idx,
                    insert_point,
                    len(parent_ids),
                )
                parent_rank_idx = len(parent_ids) - 1
            self._tree[self._node_ids[i][-1]] = parent_ids[parent_rank_idx]
            return
        self._tree[self._node_ids[i][-1]] = self._next_id
        self._tree[self._node_ids[j][-1]] = self._next_id
        self._node_ids[i].append(self._next_id)
        self._next_id += 1

    def union(self, i: int, j: int) -> bool:
        """Merge two ranked leaves and return whether a new edge was added."""
        root_i = self._find(i)[-1]
        root_j = self._find(j)[-1]
        if root_i == root_j:
            return False

        if self._rank[root_i] < self._rank[root_j]:
            if not self._parent_chains[root_j]:
                self._parent_chains[root_j].append(root_j)
            chain = self._parent_chains[j]
            higher_rank_idx = self._rank_bisect_right(chain, self._rank[root_i])
            if higher_rank_idx >= len(chain):
                higher_rank_idx = len(chain) - 1
            insert_point = chain[higher_rank_idx]
            self._ordered_extend(self._parent_chains[root_i], chain[higher_rank_idx:])
            self._build(root_i, root_j, insert_point=insert_point)
        elif self._rank[root_i] > self._rank[root_j]:
            if not self._parent_chains[root_i]:
                self._parent_chains[root_i].append(root_i)
            chain = self._parent_chains[i]
            higher_rank_idx = self._rank_bisect_right(chain, self._rank[root_j])
            if higher_rank_idx >= len(chain):
                higher_rank_idx = len(chain) - 1
            insert_point = chain[higher_rank_idx]
            self._ordered_extend(self._parent_chains[root_j], chain[higher_rank_idx:])
            self._build(root_j, root_i, insert_point=insert_point)
        else:
            if not self._parent_chains[root_i]:
                self._parent_chains[root_i].append(root_i)
            self._ordered_extend(self._parent_chains[root_j], self._parent_chains[i][-1:])
            self._rank[root_i] += 1
            self._build(root_i, root_j)
        return True

    @property
    def tree(self) -> list[int]:
        """Return the compact child-to-parent array for constructed nodes."""
        return self._tree[:self._next_id]


class RecursiveAbstractiveProcessing4TreeOrganizedRetrieval:
    """Build RAPTOR summary layers with the classic or Psi tree strategy."""

    def __init__(
        self,
        max_cluster,
        llm_model,
        embd_model,
        prompt,
        max_token=512,
        threshold=0.1,
        max_errors=3,
        tree_builder=RAPTOR_TREE_BUILDER,
        clustering_method=GMM_CLUSTERING_METHOD,
        psi_exact_max_leaves=4096,
        psi_bucket_size=1024,
    ):
        """Configure RAPTOR summarization, clustering, and Psi limits."""
        self._max_cluster = max_cluster
        self._llm_model = llm_model
        self._embd_model = embd_model
        self._threshold = threshold
        self._prompt = prompt
        self._max_token = max_token
        self._max_errors = max(1, max_errors)
        self._error_count = 0
        self._tree_builder = tree_builder or RAPTOR_TREE_BUILDER
        if self._tree_builder not in SUPPORTED_TREE_BUILDERS:
            raise ValueError(f"Unsupported RAPTOR tree builder: {self._tree_builder}")
        self._clustering_method = clustering_method or GMM_CLUSTERING_METHOD
        if self._clustering_method not in SUPPORTED_CLUSTERING_METHODS:
            raise ValueError(f"Unsupported RAPTOR clustering method: {self._clustering_method}")
        self._psi_exact_max_leaves = max(2, int(psi_exact_max_leaves or 4096))
        self._psi_bucket_size = min(max(2, int(psi_bucket_size or 1024)), self._psi_exact_max_leaves)

    def _check_task_canceled(self, task_id: str, message: str = ""):
        """Raise if the current document task was canceled."""
        if task_id and has_canceled(task_id):
            log_msg = f"Task {task_id} cancelled during RAPTOR {message}."
            logging.info(log_msg)
            raise TaskCanceledException(f"Task {task_id} was cancelled")

    @timeout(60 * 20)
    async def _chat(self, system, history, gen_conf):
        """Call the configured LLM with caching and short retries."""
        cached = await thread_pool_exec(get_llm_cache, self._llm_model.llm_name, system, history, gen_conf)
        if cached:
            return cached

        last_exc = None
        for attempt in range(3):
            try:
                response = await self._llm_model.async_chat(system, history, gen_conf)
                response = re.sub(r"^.*</think>", "", response, flags=re.DOTALL)
                if response.find("**ERROR**") >= 0:
                    raise Exception(response)
                await thread_pool_exec(set_llm_cache,self._llm_model.llm_name,system,response,history,gen_conf)
                return response
            except Exception as exc:
                last_exc = exc
                logging.warning("RAPTOR LLM call failed on attempt %d/3: %s", attempt + 1, exc)
                if attempt < 2:
                    await asyncio.sleep(1 + attempt)

        raise last_exc if last_exc else Exception("LLM chat failed without exception")

    @timeout(20)
    async def _embedding_encode(self, txt):
        """Encode text with the configured embedding model and cache result."""
        response = await thread_pool_exec(get_embed_cache, self._embd_model.llm_name, txt)
        if response is not None:
            return response
        embds, _ = await thread_pool_exec(self._embd_model.encode, [txt])
        if len(embds) < 1 or len(embds[0]) < 1:
            raise Exception("Embedding error: empty embeddings returned")
        embds = embds[0]
        await thread_pool_exec(set_embed_cache, self._embd_model.llm_name, txt, embds)
        return embds

    def _get_optimal_clusters(self, embeddings: np.ndarray, random_state: int, task_id: str = ""):
        """Choose the GMM cluster count with the lowest BIC score."""
        max_clusters = min(self._max_cluster, len(embeddings))
        n_clusters = np.arange(1, max_clusters)
        bics = []
        for n in n_clusters:
            self._check_task_canceled(task_id, "get optimal clusters")

            gm = GaussianMixture(n_components=n, random_state=random_state)
            gm.fit(embeddings)
            bics.append(gm.bic(embeddings))
        optimal_clusters = n_clusters[np.argmin(bics)]
        return optimal_clusters

    def _get_clusters_ahc(self, embeddings: np.ndarray, task_id: str = "") -> np.ndarray:
        """Cluster embeddings with Ward-linkage AHC and a dendrogram gap heuristic."""
        n = len(embeddings)
        if n <= 1:
            return np.zeros(n, dtype=int)
        if n == 2:
            return np.arange(n)

        self._check_task_canceled(task_id, "_get_clusters_ahc dendrogram")
        full_clust = AgglomerativeClustering(
            n_clusters=None,
            distance_threshold=0,
            compute_distances=True,
            linkage="ward",
        )
        full_clust.fit(embeddings)

        distances = full_clust.distances_
        if len(distances) > 1:
            gaps = np.diff(distances)
            max_gap_idx = int(np.argmax(gaps))
            n_clusters = max(1, min(n - max_gap_idx - 1, self._max_cluster))
        else:
            n_clusters = max(1, min(n, self._max_cluster))
        if n_clusters <= 1:
            logging.info("RAPTOR AHC: _get_clusters_ahc selected one cluster for %d embeddings", n)
            return np.zeros(n, dtype=int)

        logging.info("RAPTOR AHC: _get_clusters_ahc selected n_clusters=%d for %d embeddings", n_clusters, n)
        self._check_task_canceled(task_id, "_get_clusters_ahc fit")
        clustering = AgglomerativeClustering(n_clusters=n_clusters, linkage="ward")
        return clustering.fit_predict(embeddings)

    def _adjust_tree_nodes(self, embeddings: np.ndarray, labels: np.ndarray, max_iter: int = 5) -> np.ndarray:
        """Refine AHC assignments by reassigning nodes to nearest centroids."""
        labels = labels.copy()
        for _ in range(max_iter):
            unique_labels = np.unique(labels)
            if len(unique_labels) <= 1:
                return labels
            centroids = np.stack([embeddings[labels == lbl].mean(axis=0) for lbl in unique_labels])
            diffs = embeddings[:, np.newaxis, :] - centroids[np.newaxis, :, :]
            sq_dists = (diffs**2).sum(axis=2)
            new_label_indices = np.argmin(sq_dists, axis=1)
            new_labels = unique_labels[new_label_indices]
            if np.array_equal(new_labels, labels):
                break
            unique_new = np.unique(new_labels)
            remap = {old: new for new, old in enumerate(unique_new)}
            labels = np.array([remap[int(lbl)] for lbl in new_labels])
        return labels

    @timeout(60 * 20)
    async def _summarize_texts(self, texts: list[str], callback=None, task_id: str = ""):
        """Summarize a cluster and return text plus embedding when successful."""
        self._check_task_canceled(task_id, "summarization")

        len_per_chunk = int((self._llm_model.max_length - self._max_token) / len(texts))
        cluster_content = "\n".join([truncate(t, max(1, len_per_chunk)) for t in texts])
        try:
            async with chat_limiter:
                self._check_task_canceled(task_id, "before LLM call")

                cnt = await self._chat(
                    "You're a helpful assistant.",
                    [
                        {
                            "role": "user",
                            "content": self._prompt.format(cluster_content=cluster_content),
                        }
                    ],
                    {"max_tokens": max(self._max_token, 512)},  # fix issue:  #10235
                )
                cnt = re.sub(
                    "(······\n由于长度的原因，回答被截断了，要继续吗？|For the content length reason, it stopped, continue?)",
                    "",
                    cnt,
                )
                logging.debug(f"SUM: {cnt}")

                self._check_task_canceled(task_id, "before embedding")

                embds = await self._embedding_encode(cnt)
                return cnt, embds
        except TaskCanceledException:
            raise
        except Exception as exc:
            self._error_count += 1
            warn_msg = f"[RAPTOR] Skip cluster ({len(texts)} chunks) due to error: {exc}"
            logging.warning(warn_msg)
            if callback:
                callback(msg=warn_msg)
            if self._error_count >= self._max_errors:
                raise RuntimeError(f"RAPTOR aborted after {self._error_count} errors. Last error: {exc}") from exc
            return None

    @staticmethod
    def _root(node: _PsiTreeNode) -> _PsiTreeNode:
        """Return the current root for a Psi tree node."""
        while node.parent is not None:
            node = node.parent
        return node

    def _rank_leaf_pairs(self, leaves: list[_PsiTreeNode]) -> np.ndarray:
        """Rank all leaf pairs by original embedding-space cosine similarity."""
        node_embeddings = np.asarray([leaf.embedding for leaf in leaves], dtype=np.float64)
        node_embeddings = self._normalize_embeddings(node_embeddings)
        similarities = node_embeddings @ node_embeddings.T
        lower = np.tril_indices(len(leaves), -1)
        ordered = np.argsort(similarities[lower], axis=0)[::-1]
        return np.stack([lower[0][ordered], lower[1][ordered]], axis=-1)

    @staticmethod
    def _normalize_embeddings(node_embeddings: np.ndarray) -> np.ndarray:
        """Normalize embeddings for cosine operations while tolerating zero vectors."""
        node_embeddings = np.asarray(node_embeddings, dtype=np.float64)
        norms = np.linalg.norm(node_embeddings, axis=1, keepdims=True)
        return node_embeddings / np.maximum(norms, 1e-12)

    def _split_psi_buckets(self, nodes: list[_PsiTreeNode]) -> list[list[_PsiTreeNode]]:
        """Split large Psi inputs so exact pair ranking is bounded per bucket."""
        if len(nodes) <= self._psi_bucket_size:
            return [nodes]

        node_embeddings = self._normalize_embeddings(np.asarray([node.embedding for node in nodes], dtype=np.float64))
        groups = [np.arange(len(nodes), dtype=int)]
        buckets = []

        while groups:
            group = np.asarray(groups.pop(), dtype=int)
            if len(group) <= self._psi_bucket_size:
                buckets.append(group.tolist())
                continue

            fanout = min(max(2, int(np.ceil(len(group) / self._psi_bucket_size))), len(group), 32)
            group_embeddings = node_embeddings[group]
            center_idx = np.linspace(0, len(group_embeddings) - 1, num=fanout, dtype=int)
            centers = group_embeddings[center_idx].copy()

            for _ in range(5):
                labels = np.argmax(group_embeddings @ centers.T, axis=1)
                for center_id in range(fanout):
                    mask = labels == center_id
                    if not np.any(mask):
                        continue
                    center = group_embeddings[mask].mean(axis=0)
                    norm = np.linalg.norm(center)
                    centers[center_id] = center / norm if norm > 0 else center

            labels = np.argmax(group_embeddings @ centers.T, axis=1)
            split_groups = [group[labels == center_id].tolist() for center_id in range(fanout)]
            split_groups = [bucket for bucket in split_groups if bucket]
            if len(split_groups) <= 1:
                split_groups = [
                    group[start:start + self._psi_bucket_size].tolist()
                    for start in range(0, len(group), self._psi_bucket_size)
                ]
            groups.extend(split_groups)

        buckets = [bucket for bucket in buckets if bucket]
        buckets.sort(key=lambda bucket: (len(bucket), bucket[0]))
        return [[nodes[idx] for idx in bucket] for bucket in buckets]

    def _assign_prototype_embeddings(self, node: _PsiTreeNode) -> np.ndarray:
        """Assign mean child embeddings to internal Psi nodes for bucket-level ranking."""
        if not node.children:
            return np.asarray(node.embedding, dtype=np.float64)
        embeddings = np.asarray([self._assign_prototype_embeddings(child) for child in node.children], dtype=np.float64)
        node.embedding = embeddings.mean(axis=0)
        return node.embedding

    @staticmethod
    def _iter_nodes(root: _PsiTreeNode):
        """Yield nodes in a Psi tree using a stack traversal."""
        stack = [root]
        while stack:
            node = stack.pop()
            yield node
            stack.extend(node.children)

    def _create_psi_parent(self, index: int, children: list[_PsiTreeNode]) -> _PsiTreeNode:
        """Create a parent node and attach the provided children to it."""
        parent = _PsiTreeNode(index=index, children=children)
        for child in children:
            child.parent = parent
        return parent

    def _rebalance_psi_tree(self, root: _PsiTreeNode, next_index: int) -> tuple[_PsiTreeNode, int]:
        """Group oversized Psi tree nodes so fanout stays within max_cluster."""
        max_children = max(2, int(self._max_cluster or 2))

        def rebalance(node: _PsiTreeNode):
            """Recursively group children when a Psi node exceeds fanout."""
            nonlocal next_index

            for child in list(node.children):
                rebalance(child)

            while len(node.children) > max_children:
                original_children = len(node.children)
                grouped_children = []
                for start in range(0, len(node.children), max_children):
                    batch = node.children[start:start + max_children]
                    if len(batch) == 1:
                        grouped_children.append(batch[0])
                        batch[0].parent = node
                    else:
                        grouped_children.append(self._create_psi_parent(next_index, batch))
                        grouped_children[-1].parent = node
                        next_index += 1
                node.children = grouped_children
                logging.info(
                    "RAPTOR Psi rebalance: node=%s children=%d grouped_to=%d max_cluster=%d",
                    node.index,
                    original_children,
                    len(grouped_children),
                    max_children,
                )

        rebalance(root)
        return self._root(root), next_index

    def _build_exact_psi_structure(
        self,
        nodes: list[_PsiTreeNode],
        next_index: int,
        task_id: str = "",
    ) -> tuple[_PsiTreeNode, int, int]:
        """Build an exact Psi subtree for a bounded node set."""
        if len(nodes) == 1:
            return nodes[0], next_index, 0

        ranked_pairs = self._rank_leaf_pairs(nodes)
        union_find = _PsiUnionFind(len(nodes))
        merges = 0
        for left_idx, right_idx in ranked_pairs:
            self._check_task_canceled(task_id, "Psi tree construction")
            if union_find.union(int(left_idx), int(right_idx)):
                merges += 1
            if merges == len(nodes) - 1:
                break

        local_nodes = {idx: node for idx, node in enumerate(nodes)}
        tree = union_find.tree
        children_by_parent = {}
        for child_idx, parent_idx in enumerate(tree):
            if child_idx not in local_nodes:
                local_nodes[child_idx] = _PsiTreeNode(index=next_index)
                next_index += 1
            if parent_idx == -1:
                continue
            children_by_parent.setdefault(parent_idx, []).append(child_idx)
            if parent_idx not in local_nodes:
                local_nodes[parent_idx] = _PsiTreeNode(index=next_index)
                next_index += 1

        for parent_idx, child_indices in children_by_parent.items():
            parent = local_nodes[parent_idx]
            parent.children = [local_nodes[child_idx] for child_idx in child_indices]
            for child in parent.children:
                child.parent = parent

        roots = [local_nodes[idx] for idx, parent_idx in enumerate(tree) if parent_idx == -1 and idx in local_nodes]
        root = max(roots, key=lambda node: node.index)
        return root, next_index, merges

    def _build_bucketed_psi_structure(
        self,
        nodes: list[_PsiTreeNode],
        next_index: int,
        task_id: str = "",
    ) -> tuple[_PsiTreeNode, int, int]:
        """Build large Psi trees by exact-ranking bounded buckets, then bucket roots."""
        buckets = self._split_psi_buckets(nodes)
        logging.info(
            "RAPTOR Psi bucketed build: nodes=%d buckets=%d bucket_size=%d exact_max_leaves=%d",
            len(nodes),
            len(buckets),
            self._psi_bucket_size,
            self._psi_exact_max_leaves,
        )

        bucket_roots = []
        merges = 0
        for bucket in buckets:
            bucket_root, next_index, bucket_merges = self._build_psi_structure_from_nodes(bucket, next_index, task_id)
            self._assign_prototype_embeddings(bucket_root)
            bucket_roots.append(bucket_root)
            merges += bucket_merges

        if len(bucket_roots) == 1:
            return bucket_roots[0], next_index, merges

        root, next_index, root_merges = self._build_psi_structure_from_nodes(bucket_roots, next_index, task_id)
        return root, next_index, merges + root_merges

    def _build_psi_structure_from_nodes(
        self,
        nodes: list[_PsiTreeNode],
        next_index: int,
        task_id: str = "",
    ) -> tuple[_PsiTreeNode, int, int]:
        """Build Psi structure exactly for small sets and bucket large sets."""
        if len(nodes) <= self._psi_exact_max_leaves:
            return self._build_exact_psi_structure(nodes, next_index, task_id)
        return self._build_bucketed_psi_structure(nodes, next_index, task_id)

    def _build_psi_structure(self, chunks, task_id: str = "") -> tuple[_PsiTreeNode, list[_PsiTreeNode]]:
        """Build the Psi merge tree from original chunk embeddings."""
        leaves = [
            _PsiTreeNode(index=i, text=text, embedding=np.asarray(embd))
            for i, (text, embd) in enumerate(chunks)
        ]
        if len(leaves) == 1:
            return leaves[0], leaves

        root, next_index, merges = self._build_psi_structure_from_nodes(leaves, len(leaves), task_id)
        root, _ = self._rebalance_psi_tree(root, next_index)
        logging.info(
            "RAPTOR Psi tree built: leaves=%d merges=%d root_fanout=%d",
            len(leaves),
            merges,
            len(root.children),
        )
        return root, leaves

    @staticmethod
    def _psi_layers(root: _PsiTreeNode) -> dict[int, list[_PsiTreeNode]]:
        """Collect non-leaf Psi nodes by height for bottom-up summarization."""
        layers = {}

        def height(node: _PsiTreeNode) -> int:
            """Return node height while collecting internal nodes by layer."""
            if not node.children:
                return 0
            node_height = max(height(child) for child in node.children) + 1
            layers.setdefault(node_height, []).append(node)
            return node_height

        height(root)
        return layers

    async def _build_psi_layers(self, chunks, callback=None, task_id: str = ""):
        """Materialize Psi tree layers as summary chunks."""
        layers = [(0, len(chunks))]
        root, _ = self._build_psi_structure(chunks, task_id=task_id)

        for layer_idx, (_, nodes) in enumerate(sorted(self._psi_layers(root).items()), start=1):
            layer_start = len(chunks)

            async def summarize_node(node: _PsiTreeNode):
                """Summarize one Psi internal node if its children have text."""
                texts = [child.text for child in node.children if child.text]
                if not texts:
                    logging.warning("RAPTOR Psi node %s skipped because it has no child text to summarize", node.index)
                    return None
                result = await self._summarize_texts(texts, callback, task_id)
                if result is None:
                    logging.warning("RAPTOR Psi node %s skipped because summarization failed", node.index)
                    return None
                node.text, node.embedding = result
                return node

            tasks = [asyncio.create_task(summarize_node(node)) for node in nodes]
            try:
                summarized_nodes = await asyncio.gather(*tasks, return_exceptions=False)
            except Exception as e:
                logging.error(f"Error in RAPTOR Psi tree processing: {e}")
                for task in tasks:
                    task.cancel()
                await asyncio.gather(*tasks, return_exceptions=True)
                raise

            summarized_nodes = [node for node in summarized_nodes if node is not None]
            for node in summarized_nodes:
                chunks.append((node.text, node.embedding))

            if len(chunks) > layer_start:
                layers.append((layer_start, len(chunks)))
                logging.info(
                    "RAPTOR Psi layer materialized: layer=%d nodes=%d summaries=%d",
                    layer_idx,
                    len(nodes),
                    len(chunks) - layer_start,
                )
                if callback:
                    callback(msg="Build one Psi-RAG layer: {} -> {}".format(len(nodes), len(chunks) - layer_start))
            else:
                logging.warning("RAPTOR Psi layer %d produced no summaries; stopping materialization", layer_idx)
                break

        return chunks, layers

    async def __call__(self, chunks, random_state, callback=None, task_id: str = ""):
        """Build summary chunks and layer boundaries for RAPTOR retrieval."""
        if len(chunks) <= 1:
            return [], []
        chunks = [(s, a) for s, a in chunks if s and a is not None and len(a) > 0]
        if len(chunks) <= 1:
            return chunks, [(0, len(chunks))]
        if self._tree_builder == PSI_TREE_BUILDER:
            logging.info("RAPTOR: using %s tree builder for %d chunks", self._tree_builder, len(chunks))
            return await self._build_psi_layers(chunks, callback, task_id)

        layers = [(0, len(chunks))]
        start, end = 0, len(chunks)

        @timeout(60 * 20)
        async def summarize(ck_idx: list[int]):
            """Summarize one classic RAPTOR cluster into the chunk list."""
            nonlocal chunks

            texts = [chunks[i][0] for i in ck_idx]
            result = await self._summarize_texts(texts, callback, task_id)
            if result is not None:
                chunks.append(result)

        while end - start > 1:
            self._check_task_canceled(task_id, "layer processing")

            embeddings = [embd for _, embd in chunks[start:end]]
            if len(embeddings) == 2:
                await summarize([start, start + 1])
                produced = len(chunks) - end
                if produced == 0:
                    logging.warning("RAPTOR layer produced no summaries; stopping materialization")
                    break
                if callback:
                    callback(msg="Cluster one layer: {} -> {}".format(end - start, produced))
                layers.append((end, len(chunks)))
                start = end
                end = len(chunks)
                continue

            n_neighbors = int((len(embeddings) - 1) ** 0.8)
            reduced_embeddings = umap.UMAP(
                n_neighbors=max(2, n_neighbors),
                n_components=min(12, len(embeddings) - 2),
                metric="cosine",
            ).fit_transform(embeddings)
            if self._clustering_method == AHC_CLUSTERING_METHOD:
                logging.info("RAPTOR: using clustering_method=%s before _get_clusters_ahc", self._clustering_method)
                raw_labels = self._get_clusters_ahc(reduced_embeddings, task_id=task_id)
                raw_cluster_count = np.unique(raw_labels).size
                logging.info("RAPTOR AHC: _get_clusters_ahc produced n_clusters=%d", raw_cluster_count)
                if raw_cluster_count > 1:
                    adjusted = self._adjust_tree_nodes(reduced_embeddings, raw_labels)
                    adjusted_cluster_count = np.unique(adjusted).size
                    logging.info("RAPTOR AHC: _adjust_tree_nodes adjusted n_clusters=%d", adjusted_cluster_count)
                else:
                    adjusted = raw_labels
                    logging.warning("RAPTOR AHC: _adjust_tree_nodes skipped because _get_clusters_ahc returned one cluster")
                unique_labels = np.unique(adjusted)
                label_map = {old: idx for idx, old in enumerate(unique_labels)}
                lbls = [label_map[int(lbl)] for lbl in adjusted]
                n_clusters = len(unique_labels)
            else:
                n_clusters = self._get_optimal_clusters(reduced_embeddings, random_state, task_id=task_id)
                if n_clusters == 1:
                    lbls = [0 for _ in range(len(reduced_embeddings))]
                else:
                    gm = GaussianMixture(n_components=n_clusters, random_state=random_state)
                    gm.fit(reduced_embeddings)
                    probs = gm.predict_proba(reduced_embeddings)
                    lbls = [np.where(prob > self._threshold)[0] for prob in probs]
                    lbls = [lbl[0] if isinstance(lbl, np.ndarray) else lbl for lbl in lbls]

            if n_clusters == 1:
                lbls = [0 for _ in range(len(reduced_embeddings))]
            else:
                lbls = [int(lbl[0]) if isinstance(lbl, np.ndarray) else int(lbl) for lbl in lbls]

            tasks = []
            for c in range(n_clusters):
                ck_idx = [i + start for i in range(len(lbls)) if lbls[i] == c]
                assert len(ck_idx) > 0
                self._check_task_canceled(task_id, "before cluster processing")
                tasks.append(asyncio.create_task(summarize(ck_idx)))
            try:
                await asyncio.gather(*tasks, return_exceptions=False)
            except Exception as e:
                logging.error(f"Error in RAPTOR cluster processing: {e}")
                for t in tasks:
                    t.cancel()
                await asyncio.gather(*tasks, return_exceptions=True)
                raise

            produced = len(chunks) - end
            assert produced <= n_clusters, "{} vs. {}".format(produced, n_clusters)
            if produced < n_clusters:
                logging.warning(
                    "RAPTOR layer produced %d/%d cluster summaries; skipped %d cluster(s) due to errors",
                    produced,
                    n_clusters,
                    n_clusters - produced,
                )
            if produced == 0:
                logging.warning("RAPTOR layer produced no summaries; stopping materialization")
                break
            layers.append((end, len(chunks)))
            if callback:
                callback(msg="Cluster one layer: {} -> {}".format(end - start, produced))
            start = end
            end = len(chunks)

        return chunks, layers
