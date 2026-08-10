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
import logging
import re

import numpy as np

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

from ._common import knowledge_compile_gen_conf


class RecursiveAbstractiveProcessing4TreeOrganizedRetrieval:
    """Build RAPTOR summary layers with the classic or Psi tree strategy."""

    def __init__(
        self,
        max_cluster,
        llm_model,
        embd_model,
        prompt,
        max_token=512,
        small_layer_collapse=8,
        max_errors=3,
        clustering_threshold=0.3,
        clustering_ratio=0.5,
    ):
        """Configure RAPTOR summarization and clustering.

        Args:
            clustering_threshold: Adjacent chunks with cosine similarity
                below this value become cluster boundaries.  Default 0.3.
            clustering_ratio: Maximum number of clusters as a fraction of
                chunk count (e.g. 0.5 means at most 50% of chunks become
                cluster representatives).  If the threshold-based watershed
                produces more clusters than this cap, the threshold is
                lowered using the distribution of recorded adjacent
                similarities.
        """
        self._max_cluster = max_cluster
        self._small_layer_collapse = small_layer_collapse
        self._clustering_threshold = clustering_threshold
        self._clustering_ratio = clustering_ratio
        self._llm_model = llm_model
        self._embd_model = embd_model
        self._prompt = prompt
        self._max_token = min(max(int(max_token or 512), 512), 2048)
        self._max_errors = max(1, max_errors)
        self._error_count = 0

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
                await thread_pool_exec(set_llm_cache, self._llm_model.llm_name, system, response, history, gen_conf)
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

    def _get_clusters_ahc(self, embeddings: np.ndarray, task_id: str = "") -> np.ndarray:
        """1D-watershed segmentation over adjacent cosine similarities.

        Only adjacent embeddings are compared (O(N) instead of O(N²)).

        The split threshold is taken from the ``clustering_threshold``
        percentile of the adjacent-similarity distribution.  If the resulting
        cluster count exceeds the ``clustering_ratio`` cap, the threshold is
        further lowered.
        """
        n = len(embeddings)
        if n <= 1:
            return np.zeros(n, dtype=int)

        self._check_task_canceled(task_id, "_get_clusters_ahc")

        # L2-normalize
        norms = np.linalg.norm(embeddings, axis=1, keepdims=True)
        norms = np.where(norms == 0, 1.0, norms)
        normalized = embeddings / norms

        # Adjacent cosine similarities (n-1 pairs)
        adj_sims = np.sum(normalized[:-1] * normalized[1:], axis=1)
        sorted_sims = np.sort(adj_sims)  # ascending

        # Max clusters allowed by the ratio cap
        max_clusters = max(1, int(round(n * self._clustering_ratio)))

        def _watershed(th: float) -> np.ndarray:
            lbl = np.zeros(n, dtype=int)
            cid = 0
            for i in range(1, n):
                if adj_sims[i - 1] >= th:
                    lbl[i] = cid
                else:
                    cid += 1
                    lbl[i] = cid
            return lbl

        # ---- Phase 1: watershed at percentile-based threshold ----
        # clustering_threshold (e.g. 0.3) denotes the percentile of the
        # adjacent-similarity distribution to use as the split threshold.
        # This adapts to each layer's similarity range automatically.
        pct = max(1, min(99, int(round(self._clustering_threshold * 100))))
        threshold = float(np.percentile(adj_sims, pct))
        labels = _watershed(threshold)
        n_clusters = int(np.unique(labels).size)

        # ---- Phase 2: adjust threshold if we still exceed the cap ----
        if n_clusters > max_clusters and len(sorted_sims) >= max_clusters:
            adjusted = float(sorted_sims[min(max_clusters - 1, len(sorted_sims) - 1)])
            if adjusted < threshold:
                threshold = adjusted
                labels = _watershed(threshold)
                n_clusters = int(np.unique(labels).size)

        logging.info(
            "RAPTOR seq-clus: pct=%d threshold=%.4f n_clusters=%d/%d (%d chunks) cluster_ratio=%.2f",
            pct,
            threshold,
            n_clusters,
            max_clusters,
            n,
            self._clustering_ratio,
        )
        return labels

    def clustering(self, embeddings, random_state: int, task_id: str = "") -> tuple[int, list[int]]:
        """Cluster one RAPTOR layer using 1D-watershed and return contiguous labels."""
        if len(embeddings) == 0:
            return 0, []

        asarray = np.asarray(embeddings, dtype=np.float64)
        labels = self._get_clusters_ahc(asarray, task_id=task_id)

        normalized_labels: list[int] = []
        for label in labels:
            if isinstance(label, np.ndarray):
                normalized_labels.append(int(label[0]) if len(label) else 0)
            else:
                normalized_labels.append(int(label))

        if len(normalized_labels) <= 0:
            return 0, []
        unique_labels = np.unique(normalized_labels)
        if len(unique_labels) <= 1:
            return 1, [0 for _ in normalized_labels]
        label_map = {int(old): idx for idx, old in enumerate(unique_labels)}
        return len(unique_labels), [label_map[label] for label in normalized_labels]

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
                    "You're a helpful assistant.\n\nHelp me with the following task.\n\n%s" % self._prompt.format(cluster_content=cluster_content),
                    [
                        {
                            "role": "user",
                            "content": (
                                "Beside the summarization, give a title at the first line of your summarization. "
                                "Must be in the same language as the paragraphs. "
                                f"Keep the summary concise and target approximately {self._max_token} tokens."
                            ),
                        }
                    ],
                    # ``max_token`` is the target size of the generated node,
                    # not the provider's per-request output ceiling. Keep the
                    # provider budget independent so reasoning tokens cannot
                    # consume the node-size setting and truncate the summary.
                    knowledge_compile_gen_conf(self._llm_model),
                )
                cnt = re.sub(
                    "(······\n由于长度的原因，回答被截断了，要继续吗？|For the content length reason, it stopped, continue?)",
                    "",
                    cnt,
                )
                cnt = str(cnt or "").strip()
                logging.debug(f"SUM: {cnt}")

                self._check_task_canceled(task_id, "before embedding")

                embds = await self._embedding_encode(cnt)
                title = cnt.splitlines()[0].strip() if cnt else ""
                return title, cnt, embds
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

    async def __call__(
        self,
        chunks,
        random_state,
        callback=None,
        task_id: str = "",
        is_tree: bool = False,
    ):
        """Build summary chunks and layer boundaries for RAPTOR retrieval.

        ``chunks`` accepts either the legacy 2-tuple shape
        ``(text, vec)`` or the provenance-carrying 3-tuple shape
        ``(text, vec, source_chunk_ids)`` where ``source_chunk_ids`` is
        the list of original chunk ids that produced this entry. Output
        always uses the 3-tuple shape so every appended summary carries
        its leaves' ids. ``[]`` is left in the slot for a leaf whose id
        was missing — see the caller for the normalization rules.

        Return shapes:
          * ``is_tree=False`` (default) — original behavior: returns
            ``(chunks, layers)`` where ``chunks`` is the flat list
            (originals + summaries) and ``layers`` is the per-level
            index range ``[(start, end), ...]``.
          * ``is_tree=True`` — returns a hierarchical tree dict via
            ``_materialize_tree``. Supported for the classic builder
            only; raises ``NotImplementedError`` for PSI_TREE_BUILDER
            (PSI's hyperedge-driven summarization doesn't form a strict
            parent-of relation). Returns ``None`` when there's nothing
            to materialize.
        """
        if len(chunks) <= 1:
            return (None, None) if is_tree else ([], [])

        # Normalize input to the 3-tuple shape. Reject empties / bad
        # vectors at the same time the legacy path used to.
        def _normalize(item):
            if len(item) >= 3:
                text, vec, src = item[0], item[1], item[2]
            else:
                text, vec = item[0], item[1]
                src = []
            if not text or vec is None or len(vec) <= 0:
                return None
            # Defensive: a leaf should carry a list of strings. Drop
            # falsy entries so we don't propagate empty ids upward.
            if isinstance(src, (list, tuple)):
                src = [s for s in src if s]
            else:
                src = [src] if src else []
            return (text, vec, list(src), "")

        normalized = [t for t in (_normalize(c) for c in chunks) if t is not None]
        if len(normalized) <= 1:
            return (None, None) if is_tree else (normalized, [(0, len(normalized))])
        chunks = normalized

        # ``parent_child_map`` records each summary's immediate
        # children so ``_materialize_tree`` can walk back into a tree
        # when ``is_tree`` is set. Always populated (cheap) so the
        # tree path is just a return-shape choice at the end.
        parent_child_map: dict[int, list[int]] = {}
        n_originals = len(chunks)

        layers = [(0, len(chunks))]
        start, end = 0, len(chunks)

        @timeout(60 * 20)
        async def summarize(ck_idx: list[int]):
            """Summarize one classic RAPTOR cluster into the chunk list.

            On success appends ``(summary_text, summary_vec, src_ids)``
            where ``src_ids`` is the order-preserving deduped union of
            the ``source_chunk_ids`` of every chunk indexed in
            ``ck_idx`` — i.e. the full leaf set that contributed to
            the cluster, even through nested summaries.
            """
            nonlocal chunks

            texts = [chunks[i][0] for i in ck_idx]
            result = await self._summarize_texts(texts, callback, task_id)
            if result is not None:
                # ``dict.fromkeys`` is the cheapest way to de-dup a
                # list of strings while preserving first-seen order.
                merged_ids: list[str] = []
                seen: set[str] = set()
                for i in ck_idx:
                    for src in chunks[i][2]:
                        if src and src not in seen:
                            seen.add(src)
                            merged_ids.append(src)
                summary_ti, summary_text, summary_vec = result
                chunks.append((summary_text, summary_vec, merged_ids, summary_ti))
                # Index of the just-appended summary; map it to its
                # immediate children for the tree materializer below.
                parent_child_map[len(chunks) - 1] = list(ck_idx)

        while end - start > 1:
            self._check_task_canceled(task_id, "layer processing")

            # ``chunks`` is a mix of 3-tuples (layer-0 originals from
            # _normalize) and 4-tuples (summaries appended by
            # summarize). Vector is always at index 1 in both shapes,
            # so use positional access — the older ``_, embd, _, _``
            # form crashed on layer-0 entries.
            embeddings = [entry[1] for entry in chunks[start:end]]
            if end - start <= self._small_layer_collapse:
                # Too few nodes for meaningful sub-clustering. Skip the
                # clustering pass entirely and summarize the whole layer
                # into one parent, so the upper tree doesn't descend one
                # node per layer (N -> N-1 -> N-2 -> ... each a full
                # clustering + summarize pass).
                await summarize(list(range(start, end)))
                produced = len(chunks) - end
                if produced == 0:
                    logging.warning("RAPTOR layer produced no summaries; stopping materialization")
                    break
                logging.info(
                    "RAPTOR small-N collapse: layer of %d node(s) [%d:%d] collapsed into %d summary; stopping at tree top",
                    end - start,
                    start,
                    end,
                    produced,
                )
                layers.append((end, len(chunks)))
                if callback:
                    callback(msg="Cluster one layer: {} -> {} (small-N collapse)".format(end - start, produced))
                break

            n_clusters, lbls = self.clustering(
                embeddings,
                random_state=random_state,
                task_id=task_id,
            )

            # Loop-termination guarantee. The outer ``while end - start > 1``
            # relies on each layer strictly shrinking the input count. If
            # the clusterer degenerates and returns one cluster per input,
            # every "cluster" is a single chunk, ``summarize()`` produces
            # one summary per input, and ``produced == end - start`` —
            # the same count carries into the next iteration and the loop
            # spins forever, logging "Cluster one layer: N -> N".
            #
            # Collapse everything at this level into a single cluster so
            # the layer produces exactly one summary. The tree gets a
            # taller-than-usual "single trunk" segment at this depth
            # instead of an infinite loop; downstream consumers only care
            # that ``layers`` is monotonically shrinking.
            if n_clusters >= len(embeddings):
                logging.warning(
                    "RAPTOR clustering did not reduce input count (%d inputs → %d clusters); collapsing this layer into a single summary to prevent a non-terminating loop",
                    len(embeddings),
                    n_clusters,
                )
                n_clusters = 1
                lbls = [0] * len(embeddings)

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

        if is_tree:
            return self._materialize_tree(chunks, layers, parent_child_map, n_originals), []
        return chunks, layers

    @staticmethod
    def _materialize_tree(chunks, layers, parent_child_map, n_originals):
        """Walk ``parent_child_map`` from the top layer down to layer-1
        and emit the user-facing tree dict. See ``__call__``'s
        ``is_tree=True`` contract for the shape.
        chunks: [(summary_text, summary_vec, merged_ids, summary_ti)]"""
        if not layers or len(chunks) == 0:
            return None
        top_start, top_end = layers[-1]
        if top_end <= top_start:
            return None

        def _title_at(idx: int) -> str:
            # Summary tuples are (text, vec, merged_ids, summary_ti)
            # — title is the 4th slot. Layer-0 originals are 3-tuples
            # and don't appear as tree nodes themselves (they collapse
            # into source_chunk_ids on their layer-1 parent).
            return chunks[idx][3] if len(chunks[idx]) >= 4 else ""

        def _desc_at(idx: int) -> str:
            return chunks[idx][0] if chunks[idx] else ""

        def _build_node(idx: int) -> dict:
            children_idx = parent_child_map.get(idx, [])
            # If every immediate child is a layer-0 original, collapse the
            # cluster into one leaf node and retain all source chunk IDs.
            if children_idx and all(c < n_originals for c in children_idx):
                source_chunk_ids: list[str] = []
                seen: set[str] = set()
                for c in children_idx:
                    for s in chunks[c][2]:
                        if s and s not in seen:
                            seen.add(s)
                            source_chunk_ids.append(s)
                return {"title": _title_at(idx), "source_chunk_ids": source_chunk_ids, "description": _desc_at(idx)}
            return {"children": [_build_node(c) for c in children_idx], "title": _title_at(idx), "description": _desc_at(idx)}

        top_nodes = [_build_node(i) for i in range(top_start, top_end)]
        if len(top_nodes) == 1:
            return top_nodes[0]
        # Multiple top-layer summaries — clustering didn't collapse to
        # a single root. Wrap in a synthetic root so the caller always
        # sees one dict.
        return {"title": "(root)", "children": top_nodes}
