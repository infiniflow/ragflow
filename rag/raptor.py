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
        clustering_method="gmm",
    ):
        self._max_cluster = max_cluster
        self._llm_model = llm_model
        self._embd_model = embd_model
        self._threshold = threshold
        self._prompt = prompt
        self._max_token = max_token
        self._max_errors = max(1, max_errors)
        self._error_count = 0
        self._clustering_method = clustering_method

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

    def _get_clusters_ahc(self, embeddings: np.ndarray) -> np.ndarray:
        """Cluster embeddings with Agglomerative Hierarchical Clustering.

        The number of clusters is determined automatically by locating the
        largest gap in the Ward linkage dendrogram (no BIC optimisation
        needed), which avoids the uniform-cluster-size effect of GMM.
        """
        n = len(embeddings)
        logging.debug("RAPTOR AHC clustering: n=%d, max_cluster=%d", n, self._max_cluster)
        if self._max_cluster <= 1:
            logging.info("RAPTOR AHC selected 1 cluster because max_cluster=%d", self._max_cluster)
            return np.zeros(n, dtype=int)
        if n <= 2:
            logging.info("RAPTOR AHC selected %d clusters for small input", n)
            return np.arange(n)

        # Build the full dendrogram to find natural cluster boundaries.
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
            # After (max_gap_idx + 1) merges the next merge would cross the
            # largest gap, so we stop there: n - (max_gap_idx + 1) clusters.
            n_clusters = min(max(2, n - max_gap_idx - 1), self._max_cluster)
            logging.info(
                "RAPTOR AHC selected %d clusters for %d embeddings (max_gap_idx=%d, gap=%.6f)",
                n_clusters,
                n,
                max_gap_idx,
                float(gaps[max_gap_idx]),
            )
        else:
            n_clusters = min(2, self._max_cluster)
            logging.info("RAPTOR AHC selected fallback cluster count=%d for %d embeddings", n_clusters, n)

        clustering = AgglomerativeClustering(n_clusters=n_clusters, linkage="ward")
        return clustering.fit_predict(embeddings)

    def _adjust_tree_nodes(
        self, embeddings: np.ndarray, labels: np.ndarray, max_iter: int = 5
    ) -> np.ndarray:
        """Refine cluster assignments via centroid-based reassignment.

        Inspired by the tree-node adjustment step in Psi-RAG: after the
        initial AHC pass each node is re-assigned to the nearest cluster
        centroid until convergence, preventing "stranded" outliers from
        inflating summaries of the wrong cluster.
        """
        labels = labels.copy()
        for iteration in range(max_iter):
            unique_labels = np.unique(labels)
            centroids = np.stack(
                [embeddings[labels == lbl].mean(axis=0) for lbl in unique_labels]
            )
            diffs = embeddings[:, np.newaxis, :] - centroids[np.newaxis, :, :]
            sq_dists = (diffs ** 2).sum(axis=2)
            new_label_indices = np.argmin(sq_dists, axis=1)
            new_labels = unique_labels[new_label_indices]
            if np.array_equal(new_labels, labels):
                logging.debug("RAPTOR AHC node adjustment converged after %d iteration(s)", iteration + 1)
                break
            # Remap to contiguous ints in case any cluster became empty.
            unique_new = np.unique(new_labels)
            remap = {old: new for new, old in enumerate(unique_new)}
            labels = np.array([remap[int(lbl)] for lbl in new_labels])
        logging.info("RAPTOR AHC node adjustment final clusters=%d", len(np.unique(labels)))
        return labels

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

    async def __call__(self, chunks, random_state, callback=None, task_id: str = ""):
        if len(chunks) <= 1:
            return [], []
        chunks = [(s, a) for s, a in chunks if s and a is not None and len(a) > 0]
        layers = [(0, len(chunks))]
        start, end = 0, len(chunks)
        logging.info("RAPTOR clustering method=%s, max_cluster=%d", self._clustering_method, self._max_cluster)

        @timeout(60 * 20)
        async def summarize(ck_idx: list[int]):
            nonlocal chunks

            self._check_task_canceled(task_id, "summarization")

            texts = [chunks[i][0] for i in ck_idx]
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
                    chunks.append((cnt, embds))
            except TaskCanceledException:
                raise
            except Exception as exc:
                self._error_count += 1
                warn_msg = f"[RAPTOR] Skip cluster ({len(ck_idx)} chunks) due to error: {exc}"
                logging.warning(warn_msg)
                if callback:
                    callback(msg=warn_msg)
                if self._error_count >= self._max_errors:
                    raise RuntimeError(f"RAPTOR aborted after {self._error_count} errors. Last error: {exc}") from exc

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
            if self._clustering_method == "ahc":
                raw_labels = self._get_clusters_ahc(reduced_embeddings)
                if len(np.unique(raw_labels)) > 1:
                    adjusted = self._adjust_tree_nodes(reduced_embeddings, raw_labels)
                else:
                    adjusted = raw_labels
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
