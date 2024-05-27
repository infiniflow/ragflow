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
import re
import traceback
from concurrent.futures import ThreadPoolExecutor, ALL_COMPLETED, wait
from threading import Lock
from typing import Tuple
import umap
import numpy as np
from sklearn.mixture import GaussianMixture

from rag.utils import num_tokens_from_string, truncate


class RecursiveAbstractiveProcessing4TreeOrganizedRetrieval:
    def __init__(self, max_cluster, llm_model, embd_model, prompt, max_token=256, threshold=0.1):
        self._max_cluster = max_cluster
        self._llm_model = llm_model
        self._embd_model = embd_model
        self._threshold = threshold
        self._prompt = prompt
        self._max_token = max_token

    def _get_optimal_clusters(self, embeddings: np.ndarray, random_state:int):
        max_clusters = min(self._max_cluster, len(embeddings))
        n_clusters = np.arange(1, max_clusters)
        bics = []
        for n in n_clusters:
            gm = GaussianMixture(n_components=n, random_state=random_state)
            gm.fit(embeddings)
            bics.append(gm.bic(embeddings))
        optimal_clusters = n_clusters[np.argmin(bics)]
        return optimal_clusters

    def __call__(self, chunks: Tuple[str, np.ndarray], random_state, callback=None):
        layers = [(0, len(chunks))]
        start, end = 0, len(chunks)
        if len(chunks) <= 1: return

        def summarize(ck_idx, lock):
            nonlocal chunks
            try:
                texts = [chunks[i][0] for i in ck_idx]
                len_per_chunk = int((self._llm_model.max_length - self._max_token)/len(texts))
                cluster_content = "\n".join([truncate(t, max(1, len_per_chunk)) for t in texts])
                cnt = self._llm_model.chat("You're a helpful assistant.",
                                             [{"role": "user", "content": self._prompt.format(cluster_content=cluster_content)}],
                                             {"temperature": 0.3, "max_tokens": self._max_token}
                                             )
                cnt = re.sub("(······\n由于长度的原因，回答被截断了，要继续吗？|For the content length reason, it stopped, continue?)", "", cnt)
                print("SUM:", cnt)
                embds, _ = self._embd_model.encode([cnt])
                with lock:
                    chunks.append((cnt, embds[0]))
            except Exception as e:
                print(e, flush=True)
                traceback.print_stack(e)
                return e

        labels = []
        while end - start > 1:
            embeddings = [embd for _, embd in chunks[start: end]]
            if len(embeddings) == 2:
                summarize([start, start+1], Lock())
                if callback:
                    callback(msg="Cluster one layer: {} -> {}".format(end-start, len(chunks)-end))
                labels.extend([0,0])
                layers.append((end, len(chunks)))
                start = end
                end = len(chunks)
                continue

            n_neighbors = int((len(embeddings) - 1) ** 0.8)
            reduced_embeddings = umap.UMAP(
                n_neighbors=max(2, n_neighbors), n_components=min(12, len(embeddings)-2), metric="cosine"
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
            lock = Lock()
            with ThreadPoolExecutor(max_workers=12) as executor:
                threads = []
                for c in range(n_clusters):
                    ck_idx = [i+start for i in range(len(lbls)) if lbls[i] == c]
                    threads.append(executor.submit(summarize, ck_idx, lock))
                wait(threads, return_when=ALL_COMPLETED)
                print([t.result() for t in threads])

            assert len(chunks) - end == n_clusters, "{} vs. {}".format(len(chunks) - end, n_clusters)
            labels.extend(lbls)
            layers.append((end, len(chunks)))
            if callback:
                callback(msg="Cluster one layer: {} -> {}".format(end-start, len(chunks)-end))
            start = end
            end = len(chunks)

