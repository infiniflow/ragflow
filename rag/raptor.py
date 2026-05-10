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
    PSI_TREE_BUILDER,
    RAPTOR_TREE_BUILDER,
    normalize_raptor_tree_builder,
)


@dataclass
class _PsiTreeNode:
    index: int
    text: str = ""
    embedding: np.ndarray | None = None
    children: list["_PsiTreeNode"] = field(default_factory=list)
    parent: "_PsiTreeNode | None" = None


class _PsiUnionFind:
    def __init__(self, n: int):
        self._rank = [0 for _ in range(n)]
        self._parent_chains = [[] for _ in range(n)]
        self._node_ids = [[i] for i in range(n)]
        self._tree = [-1 for _ in range(max(1, 2 * n - 1))]
        self._next_id = n

    @staticmethod
    def _ordered_extend(target: list[int], values: list[int]):
        seen = set(target)
        for value in values:
            if value not in seen:
                target.append(value)
                seen.add(value)

    def _find(self, i: int) -> list[int]:
        chain = self._parent_chains[i]
        if not chain or (len(chain) == 1 and chain[0] == i):
            return [i]
        if chain[0] == i:
            self._ordered_extend(chain, self._find(chain[1]))
        else:
            self._ordered_extend(chain, self._find(chain[0]))
        return chain

    def _rank_bisect_right(self, chain: list[int], rank: int) -> int:
        idx = 0
        while idx < len(chain) and self._rank[chain[idx]] <= rank:
            idx += 1
        return idx

    def _build(self, i: int, j: int, insert_point: int | None = None):
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
        return self._tree[:self._next_id]


class RecursiveAbstractiveProcessing4TreeOrganizedRetrieval:
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
        psi_exact_max_leaves=4096,
        psi_bucket_size=1024,
    ):
        self._max_cluster = max_cluster
        self._llm_model = llm_model
        self._embd_model = embd_model
        self._threshold = threshold
        self._prompt = prompt
        self._max_token = max_token
        self._max_errors = max(1, max_errors)
        self._error_count = 0
        self._tree_builder = normalize_raptor_tree_builder(tree_builder)
        self._psi_exact_max_leaves = max(2, int(psi_exact_max_leaves or 4096))
        self._psi_bucket_size = min(max(2, int(psi_bucket_size or 1024)), self._psi_exact_max_leaves)

    def _check_task_canceled(self, task_id: str, message: str = ""):
        if task_id and has_canceled(task_id):
            log_msg = f"Task {task_id} cancelled during RAPTOR {message}."
            logging.info(log_msg)
            raise TaskCanceledException(f"Task {task_id} was cancelled")

    @timeout(60 * 20)
    async def _chat(self, system, history, gen_conf):
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

    @timeout(60 * 20)
    async def _summarize_texts(self, texts: list[str], callback=None, task_id: str = ""):
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
        while node.parent is not None:
            node = node.parent
        return node

    def _rank_leaf_pairs(self, leaves: list[_PsiTreeNode]) -> np.ndarray:
        node_embeddings = self._normalize_embeddings(np.asarray([leaf.embedding for leaf in leaves], dtype=np.float64))
        similarities = node_embeddings @ node_embeddings.T
        lower = np.tril_indices(len(leaves), -1)
        ordered = np.argsort(similarities[lower], axis=0)[::-1]
        return np.stack([lower[0][ordered], lower[1][ordered]], axis=-1)

    @staticmethod
    def _normalize_embeddings(node_embeddings: np.ndarray) -> np.ndarray:
        node_embeddings = np.asarray(node_embeddings, dtype=np.float64)
        norms = np.linalg.norm(node_embeddings, axis=1, keepdims=True)
        norms[norms == 0] = 1.0
        return node_embeddings / norms

    def _split_psi_buckets(self, nodes: list[_PsiTreeNode]) -> list[list[_PsiTreeNode]]:
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
        if not node.children:
            return np.asarray(node.embedding, dtype=np.float64)
        embeddings = np.asarray([self._assign_prototype_embeddings(child) for child in node.children], dtype=np.float64)
        node.embedding = embeddings.mean(axis=0)
        return node.embedding

    @staticmethod
    def _iter_nodes(root: _PsiTreeNode):
        stack = [root]
        while stack:
            node = stack.pop()
            yield node
            stack.extend(node.children)

    def _create_psi_parent(self, index: int, children: list[_PsiTreeNode]) -> _PsiTreeNode:
        parent = _PsiTreeNode(index=index, children=children)
        for child in children:
            child.parent = parent
        return parent

    def _rebalance_psi_tree(self, root: _PsiTreeNode, next_index: int) -> tuple[_PsiTreeNode, int]:
        max_children = max(2, int(self._max_cluster or 2))

        def rebalance(node: _PsiTreeNode):
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

        tree = union_find.tree
        local_nodes = {idx: node for idx, node in enumerate(nodes)}
        for child_idx, parent_idx in enumerate(tree):
            if child_idx not in local_nodes:
                local_nodes[child_idx] = _PsiTreeNode(index=next_index)
                next_index += 1
            if parent_idx != -1 and parent_idx not in local_nodes:
                local_nodes[parent_idx] = _PsiTreeNode(index=next_index)
                next_index += 1

        children_by_parent = {}
        for child_idx, parent_idx in enumerate(tree):
            if parent_idx == -1:
                continue
            children_by_parent.setdefault(parent_idx, []).append(child_idx)

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
        if len(nodes) <= self._psi_exact_max_leaves:
            return self._build_exact_psi_structure(nodes, next_index, task_id)
        return self._build_bucketed_psi_structure(nodes, next_index, task_id)

    def _build_psi_structure(self, chunks, task_id: str = "") -> tuple[_PsiTreeNode, list[_PsiTreeNode]]:
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
        layers = {}

        def height(node: _PsiTreeNode) -> int:
            if not node.children:
                return 0
            node_height = max(height(child) for child in node.children) + 1
            layers.setdefault(node_height, []).append(node)
            return node_height

        height(root)
        return layers

    async def _build_psi_layers(self, chunks, callback=None, task_id: str = ""):
        layers = [(0, len(chunks))]
        root, _ = self._build_psi_structure(chunks, task_id=task_id)

        for layer_idx, (_, nodes) in enumerate(sorted(self._psi_layers(root).items()), start=1):
            layer_start = len(chunks)

            async def summarize_node(node: _PsiTreeNode):
                texts = [child.text for child in node.children if child.text]
                if not texts:
                    raise RuntimeError(f"RAPTOR Psi node {node.index} has no child text to summarize")
                result = await self._summarize_texts(texts, callback, task_id)
                if result is None:
                    raise RuntimeError(f"RAPTOR Psi node {node.index} summary failed")
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

        return chunks, layers

    async def __call__(self, chunks, random_state, callback=None, task_id: str = ""):
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
                if callback:
                    callback(msg="Cluster one layer: {} -> {}".format(end - start, len(chunks) - end))
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
            n_clusters = self._get_optimal_clusters(reduced_embeddings, random_state, task_id=task_id)
            if n_clusters == 1:
                lbls = [0 for _ in range(len(reduced_embeddings))]
            else:
                gm = GaussianMixture(n_components=n_clusters, random_state=random_state)
                gm.fit(reduced_embeddings)
                probs = gm.predict_proba(reduced_embeddings)
                lbls = [np.where(prob > self._threshold)[0] for prob in probs]
                lbls = [lbl[0] if isinstance(lbl, np.ndarray) else lbl for lbl in lbls]

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

            assert len(chunks) - end == n_clusters, "{} vs. {}".format(len(chunks) - end, n_clusters)
            layers.append((end, len(chunks)))
            if callback:
                callback(msg="Cluster one layer: {} -> {}".format(end - start, len(chunks) - end))
            start = end
            end = len(chunks)

        return chunks, layers
