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
import logging
import re

import numpy as np
import trio
import umap
from sklearn.mixture import GaussianMixture

from api.db.services.task_service import has_canceled
from common.connection_utils import timeout
from common.exceptions import TaskCanceledException
from common.token_utils import truncate
from graphrag.utils import (
    chat_limiter,
    get_embed_cache,
    get_llm_cache,
    set_embed_cache,
    set_llm_cache,
)


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
    ):
        self._max_cluster = max_cluster
        self._llm_model = llm_model
        self._embd_model = embd_model
        self._threshold = threshold
        self._prompt = prompt
        self._max_token = max_token
        self._max_errors = max(1, max_errors)
        self._error_count = 0

    @timeout(60 * 20)
    async def _chat(self, system, history, gen_conf):
        cached = await trio.to_thread.run_sync(lambda: get_llm_cache(self._llm_model.llm_name, system, history, gen_conf))
        if cached:
            return cached

        last_exc = None
        for attempt in range(3):
            try:
                response = await trio.to_thread.run_sync(lambda: self._llm_model.chat(system, history, gen_conf))
                response = re.sub(r"^.*</think>", "", response, flags=re.DOTALL)
                if response.find("**ERROR**") >= 0:
                    raise Exception(response)
                await trio.to_thread.run_sync(lambda: set_llm_cache(self._llm_model.llm_name, system, response, history, gen_conf))
                return response
            except Exception as exc:
                last_exc = exc
                logging.warning("RAPTOR LLM call failed on attempt %d/3: %s", attempt + 1, exc)
                if attempt < 2:
                    await trio.sleep(1 + attempt)

        raise last_exc if last_exc else Exception("LLM chat failed without exception")

    @timeout(20)
    async def _embedding_encode(self, txt):
        response = await trio.to_thread.run_sync(lambda: get_embed_cache(self._embd_model.llm_name, txt))
        if response is not None:
            return response
        embds, _ = await trio.to_thread.run_sync(lambda: self._embd_model.encode([txt]))
        if len(embds) < 1 or len(embds[0]) < 1:
            raise Exception("Embedding error: ")
        embds = embds[0]
        await trio.to_thread.run_sync(lambda: set_embed_cache(self._embd_model.llm_name, txt, embds))
        return embds

    def _get_optimal_clusters(self, embeddings: np.ndarray, random_state: int, task_id: str = ""):
        max_clusters = min(self._max_cluster, len(embeddings))
        n_clusters = np.arange(1, max_clusters)
        bics = []
        for n in n_clusters:
            if task_id:
                if has_canceled(task_id):
                    logging.info(f"Task {task_id} cancelled during get optimal clusters.")
                    raise TaskCanceledException(f"Task {task_id} was cancelled")

            gm = GaussianMixture(n_components=n, random_state=random_state)
            gm.fit(embeddings)
            bics.append(gm.bic(embeddings))
        optimal_clusters = n_clusters[np.argmin(bics)]
        return optimal_clusters

    async def __call__(self, chunks, random_state, callback=None, task_id: str = ""):
        if len(chunks) <= 1:
            return []
        chunks = [(s, a) for s, a in chunks if s and a is not None and len(a) > 0]
        layers = [(0, len(chunks))]
        start, end = 0, len(chunks)

        @timeout(60 * 20)
        async def summarize(ck_idx: list[int]):
            nonlocal chunks

            if task_id:
                if has_canceled(task_id):
                    logging.info(f"Task {task_id} cancelled during RAPTOR summarization.")
                    raise TaskCanceledException(f"Task {task_id} was cancelled")

            texts = [chunks[i][0] for i in ck_idx]
            len_per_chunk = int((self._llm_model.max_length - self._max_token) / len(texts))
            cluster_content = "\n".join([truncate(t, max(1, len_per_chunk)) for t in texts])
            try:
                async with chat_limiter:
                    if task_id and has_canceled(task_id):
                        logging.info(f"Task {task_id} cancelled before RAPTOR LLM call.")
                        raise TaskCanceledException(f"Task {task_id} was cancelled")

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

                    if task_id and has_canceled(task_id):
                        logging.info(f"Task {task_id} cancelled before RAPTOR embedding.")
                        raise TaskCanceledException(f"Task {task_id} was cancelled")

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

        labels = []
        while end - start > 1:
            if task_id:
                if has_canceled(task_id):
                    logging.info(f"Task {task_id} cancelled during RAPTOR layer processing.")
                    raise TaskCanceledException(f"Task {task_id} was cancelled")

            embeddings = [embd for _, embd in chunks[start:end]]
            if len(embeddings) == 2:
                await summarize([start, start + 1])
                if callback:
                    callback(msg="Cluster one layer: {} -> {}".format(end - start, len(chunks) - end))
                labels.extend([0, 0])
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

            async with trio.open_nursery() as nursery:
                for c in range(n_clusters):
                    ck_idx = [i + start for i in range(len(lbls)) if lbls[i] == c]
                    assert len(ck_idx) > 0

                    if task_id and has_canceled(task_id):
                        logging.info(f"Task {task_id} cancelled before RAPTOR cluster processing.")
                        raise TaskCanceledException(f"Task {task_id} was cancelled")

                    nursery.start_soon(summarize, ck_idx)

            assert len(chunks) - end == n_clusters, "{} vs. {}".format(len(chunks) - end, n_clusters)
            labels.extend(lbls)
            layers.append((end, len(chunks)))
            if callback:
                callback(msg="Cluster one layer: {} -> {}".format(end - start, len(chunks) - end))
            start = end
            end = len(chunks)

        return chunks
