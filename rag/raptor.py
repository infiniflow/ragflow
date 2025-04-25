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
import umap
import numpy as np
from sklearn.mixture import GaussianMixture
import trio

from graphrag.utils import (
    get_llm_cache,
    get_embed_cache,
    set_embed_cache,
    set_llm_cache,
    chat_limiter,
)
from rag.utils import truncate


class RecursiveAbstractiveProcessing4TreeOrganizedRetrieval:
    def __init__(
        self, max_cluster, llm_model, embd_model, prompt, max_token=512, threshold=0.1
    ):
        self._max_cluster = max_cluster
        self._llm_model = llm_model
        self._embd_model = embd_model
        self._threshold = threshold
        self._prompt = prompt
        self._max_token = max_token

    async def _chat(self, system, history, gen_conf):
        response = get_llm_cache(self._llm_model.llm_name, system, history, gen_conf)
        if response:
            return response
        response = await trio.to_thread.run_sync(
            lambda: self._llm_model.chat(system, history, gen_conf)
        )
        response = re.sub(r"^.*</think>", "", response, flags=re.DOTALL)
        if response.find("**ERROR**") >= 0:
            raise Exception(response)
        set_llm_cache(self._llm_model.llm_name, system, response, history, gen_conf)
        return response

    async def _embedding_encode(self, txt):
        response = get_embed_cache(self._embd_model.llm_name, txt)
        if response is not None:
            return response
        embds, _ = await trio.to_thread.run_sync(lambda: self._embd_model.encode([txt]))
        if len(embds) < 1 or len(embds[0]) < 1:
            raise Exception("Embedding error: ")
        embds = embds[0]
        set_embed_cache(self._embd_model.llm_name, txt, embds)
        return embds

    def _get_optimal_clusters(self, embeddings: np.ndarray, random_state: int):
        max_clusters = min(self._max_cluster, len(embeddings))
        n_clusters = np.arange(1, max_clusters)
        bics = []
        for n in n_clusters:
            gm = GaussianMixture(n_components=n, random_state=random_state)
            gm.fit(embeddings)
            bics.append(gm.bic(embeddings))
        optimal_clusters = n_clusters[np.argmin(bics)]
        return optimal_clusters

    async def __call__(self, chunks, random_state, callback=None):
        if len(chunks) <= 1:
            return []
        chunks = [(s, a) for s, a in chunks if s and len(a) > 0]
        layers = [(0, len(chunks))]
        start, end = 0, len(chunks)

        async def summarize(ck_idx: list[int]):
            nonlocal chunks
            texts = [chunks[i][0] for i in ck_idx]
            len_per_chunk = int(
                (self._llm_model.max_length - self._max_token) / len(texts)
            )
            cluster_content = "\n".join(
                [truncate(t, max(1, len_per_chunk)) for t in texts]
            )
            async with chat_limiter:
                cnt = await self._chat(
                    "You're a helpful assistant.",
                    [
                        {
                            "role": "user",
                            "content": self._prompt.format(
                                cluster_content=cluster_content
                            ),
                        }
                    ],
                    {"temperature": 0.3, "max_tokens": self._max_token},
                )
            cnt = re.sub(
                "(······\n由于长度的原因，回答被截断了，要继续吗？|For the content length reason, it stopped, continue?)",
                "",
                cnt,
            )
            logging.debug(f"SUM: {cnt}")
            embds = await self._embedding_encode(cnt)
            chunks.append((cnt, embds))

        labels = []
        while end - start > 1:
            embeddings = [embd for _, embd in chunks[start:end]]
            if len(embeddings) == 2:
                await summarize([start, start + 1])
                if callback:
                    callback(
                        msg="Cluster one layer: {} -> {}".format(
                            end - start, len(chunks) - end
                        )
                    )
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
            n_clusters = self._get_optimal_clusters(reduced_embeddings, random_state)
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
                    async with chat_limiter:
                        nursery.start_soon(summarize, ck_idx)

            assert len(chunks) - end == n_clusters, "{} vs. {}".format(
                len(chunks) - end, n_clusters
            )
            labels.extend(lbls)
            layers.append((end, len(chunks)))
            if callback:
                callback(
                    msg="Cluster one layer: {} -> {}".format(
                        end - start, len(chunks) - end
                    )
                )
            start = end
            end = len(chunks)

        return chunks
