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
This module implements a variety of reranking models for RAG (Retrieval-Augmented Generation) systems.

Reranking is a crucial step in the RAG pipeline that takes a set of retrieved documents/passages 
and reorders them based on their relevance to the query, improving the quality of context 
provided to the LLM. This module provides implementations for various reranking approaches,
including local models and API-based services.
"""

import re
import threading
from collections.abc import Iterable
from urllib.parse import urljoin

import requests
import httpx
from huggingface_hub import snapshot_download
import os
from abc import ABC
import numpy as np
from yarl import URL

from api import settings
from api.utils.file_utils import get_home_cache_dir
from rag.utils import num_tokens_from_string, truncate
import json


def sigmoid(x):
    """Apply sigmoid function to input to convert scores to probability range (0-1)"""
    return 1 / (1 + np.exp(-x))


class Base(ABC):
    """
    Abstract base class for all reranking models.
    Defines the interface that all reranker implementations must follow.
    """
    def __init__(self, key, model_name):
        pass

    def similarity(self, query: str, texts: list):
        """
        Calculate similarity scores between query and texts.
        
        Args:
            query: The search query
            texts: List of texts/passages to rerank
            
        Returns:
            Tuple of (similarity scores, token count)
        """
        raise NotImplementedError("Please implement encode method!")

    def total_token_count(self, resp):
        """
        Extract total token count from response.
        Tries different response formats to handle various API returns.
        """
        try:
            return resp.usage.total_tokens
        except Exception:
            pass
        try:
            return resp["usage"]["total_tokens"]
        except Exception:
            pass
        return 0


class DefaultRerank(Base):
    """
    Default reranker implementation using FlagEmbedding FlagReranker.
    Uses a Hugging Face model loaded locally for reranking.
    
    Implements a dynamic batch processing system that adapts to available GPU memory.
    """
    _model = None
    _model_lock = threading.Lock()

    def __init__(self, key, model_name, **kwargs):
        """
        Initialize the DefaultRerank model by loading it from HuggingFace.
        
        Uses a singleton pattern with a lock to ensure the model is loaded only once.
        Supports GPU acceleration when available.
        """
        if not settings.LIGHTEN and not DefaultRerank._model:
            import torch
            from FlagEmbedding import FlagReranker
            with DefaultRerank._model_lock:
                if not DefaultRerank._model:
                    try:
                        DefaultRerank._model = FlagReranker(
                            os.path.join(get_home_cache_dir(), re.sub(r"^[a-zA-Z0-9]+/", "", model_name)),
                            use_fp16=torch.cuda.is_available())
                    except Exception:
                        model_dir = snapshot_download(repo_id=model_name,
                                                      local_dir=os.path.join(get_home_cache_dir(),
                                                                             re.sub(r"^[a-zA-Z0-9]+/", "", model_name)),
                                                      local_dir_use_symlinks=False)
                        DefaultRerank._model = FlagReranker(model_dir, use_fp16=torch.cuda.is_available())
        self._model = DefaultRerank._model
        self._dynamic_batch_size = 8
        self._min_batch_size = 1

    def torch_empty_cache(self):
        """Clear CUDA memory cache to prevent OOM errors"""
        try:
            import torch
            torch.cuda.empty_cache()
        except Exception as e:
            print(f"Error emptying cache: {e}")

    def _process_batch(self, pairs, max_batch_size=None):
        """
        Process query-text pairs in batches with adaptive batch sizing.
        
        If OOM errors occur, reduces batch size and retries until successful.
        This method balances throughput with memory constraints.
        """
        old_dynamic_batch_size = self._dynamic_batch_size
        if max_batch_size is not None:
            self._dynamic_batch_size = max_batch_size
        res = []
        i = 0
        while i < len(pairs):
            current_batch = self._dynamic_batch_size
            max_retries = 5
            retry_count = 0
            while retry_count < max_retries:
                try:
                    # call subclass implemented batch processing calculation
                    batch_scores = self._compute_batch_scores(pairs[i:i + current_batch])
                    res.extend(batch_scores)
                    i += current_batch
                    self._dynamic_batch_size = min(self._dynamic_batch_size * 2, 8)
                    break
                except RuntimeError as e:
                    if "CUDA out of memory" in str(e) and current_batch > self._min_batch_size:
                        current_batch = max(current_batch // 2, self._min_batch_size)
                        self.torch_empty_cache()
                        retry_count += 1
                    else:
                        raise
            if retry_count >= max_retries:
                raise RuntimeError("max retry times, still cannot process batch, please check your GPU memory")
            self.torch_empty_cache()

        self._dynamic_batch_size = old_dynamic_batch_size
        return np.array(res)

    def _compute_batch_scores(self, batch_pairs, max_length=None):
        """
        Compute similarity scores for a batch of query-text pairs.
        
        Applies sigmoid to normalize raw scores to 0-1 range.
        """
        if max_length is None:
            scores = self._model.compute_score(batch_pairs)
        else:
            scores = self._model.compute_score(batch_pairs, max_length=max_length)
        scores = sigmoid(np.array(scores)).tolist()
        if not isinstance(scores, Iterable):
            scores = [scores]
        return scores

    def similarity(self, query: str, texts: list):
        """
        Calculate similarity between query and a list of texts.
        
        Truncates texts to prevent overflow and processes them in batches.
        Returns normalized scores and token count.
        """
        pairs = [(query, truncate(t, 2048)) for t in texts]
        token_count = 0
        for _, t in pairs:
            token_count += num_tokens_from_string(t)
        batch_size = 4096
        res = self._process_batch(pairs, max_batch_size=batch_size)
        return np.array(res), token_count


class JinaRerank(Base):
    """
    Implementation for Jina AI's reranking API service.
    """
    def __init__(self, key, model_name="jina-reranker-v2-base-multilingual",
                 base_url="https://api.jina.ai/v1/rerank"):
        self.base_url = "https://api.jina.ai/v1/rerank"
        self.headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {key}"
        }
        self.model_name = model_name

    def similarity(self, query: str, texts: list):
        """
        Rerank texts using Jina AI's API service.
        
        Makes an API call to Jina's reranking endpoint and processes the response.
        """
        texts = [truncate(t, 8196) for t in texts]
        data = {
            "model": self.model_name,
            "query": query,
            "documents": texts,
            "top_n": len(texts)
        }
        res = requests.post(self.base_url, headers=self.headers, json=data).json()
        rank = np.zeros(len(texts), dtype=float)
        for d in res["results"]:
            rank[d["index"]] = d["relevance_score"]
        return rank, self.total_token_count(res)


class YoudaoRerank(DefaultRerank):
    """
    Reranker using Youdao's BCE (Bi-directional Contrastive Encoder) model.
    
    Extends DefaultRerank and uses RerankerModel from BCEmbedding.
    """
    _model = None
    _model_lock = threading.Lock()

    def __init__(self, key=None, model_name="maidalun1020/bce-reranker-base_v1", **kwargs):
        if not settings.LIGHTEN and not YoudaoRerank._model:
            from BCEmbedding import RerankerModel
            with YoudaoRerank._model_lock:
                if not YoudaoRerank._model:
                    try:
                        YoudaoRerank._model = RerankerModel(model_name_or_path=os.path.join(
                            get_home_cache_dir(),
                            re.sub(r"^[a-zA-Z0-9]+/", "", model_name)))
                    except Exception:
                        YoudaoRerank._model = RerankerModel(
                            model_name_or_path=model_name.replace(
                                "maidalun1020", "InfiniFlow"))

        self._model = YoudaoRerank._model

    def similarity(self, query: str, texts: list):
        """
        Calculate similarity between query and texts using BCE model.
        
        Truncates texts to the model's maximum length and processes in batches.
        """
        pairs = [(query, truncate(t, self._model.max_length)) for t in texts]
        token_count = 0
        for _, t in pairs:
            token_count += num_tokens_from_string(t)
        batch_size = 8
        res = self._process_batch(pairs, max_batch_size=batch_size)
        return np.array(res), token_count


class XInferenceRerank(Base):
    """
    Reranker implementation for generic inference servers with rerank API.
    """
    def __init__(self, key="xxxxxxx", model_name="", base_url=""):
        if base_url.find("/v1") == -1:
            base_url = urljoin(base_url, "/v1/rerank")
        if base_url.find("/rerank") == -1:
            base_url = urljoin(base_url, "/v1/rerank")
        self.model_name = model_name
        self.base_url = base_url
        self.headers = {
            "Content-Type": "application/json",
            "accept": "application/json",
            "Authorization": f"Bearer {key}"
        }

    def similarity(self, query: str, texts: list):
        """
        Rerank texts using an external inference server.
        
        Normalizes scores to 0-1 range and handles empty text lists.
        """
        if len(texts) == 0:
            return np.array([]), 0
        pairs = [(query, truncate(t, 4096)) for t in texts]
        token_count = 0
        for _, t in pairs:
            token_count += num_tokens_from_string(t)
        data = {
            "model": self.model_name,
            "query": query,
            "return_documents": "true",
            "return_len": "true",
            "documents": texts
        }
        res = requests.post(self.base_url, headers=self.headers, json=data).json()
        rank = np.zeros(len(texts), dtype=float)
        for d in res["results"]:
            rank[d["index"]] = d["relevance_score"]
        return rank, token_count


class LocalAIRerank(Base):
    """
    Reranker implementation for LocalAI self-hosted API.
    """
    def __init__(self, key, model_name, base_url):
        if base_url.find("/rerank") == -1:
            self.base_url = urljoin(base_url, "/rerank")
        else:
            self.base_url = base_url
        self.headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {key}"
        }
        self.model_name = model_name.split("___")[0]

    def similarity(self, query: str, texts: list):
        """
        Rerank texts using LocalAI API.
        
        Normalizes the scores to 0-1 range for consistent scoring across different models.
        """
        # noway to config Ragflow , use fix setting
        texts = [truncate(t, 500) for t in texts]
        data = {
            "model": self.model_name,
            "query": query,
            "documents": texts,
            "top_n": len(texts),
        }
        token_count = 0
        for t in texts:
            token_count += num_tokens_from_string(t)
        res = requests.post(self.base_url, headers=self.headers, json=data).json()
        rank = np.zeros(len(texts), dtype=float)
        if 'results' not in res:
            raise ValueError("response not contains results\n" + str(res))
        for d in res["results"]:
            rank[d["index"]] = d["relevance_score"]

        # Normalize the rank values to the range 0 to 1
        min_rank = np.min(rank)
        max_rank = np.max(rank)

        # Avoid division by zero if all ranks are identical
        if max_rank - min_rank != 0:
            rank = (rank - min_rank) / (max_rank - min_rank)
        else:
            rank = np.zeros_like(rank)

        return rank, token_count


class NvidiaRerank(Base):
    """
    Implementation for NVIDIA's AI reranking service.
    """
    def __init__(
            self, key, model_name, base_url="https://ai.api.nvidia.com/v1/retrieval/nvidia/"
    ):
        if not base_url:
            base_url = "https://ai.api.nvidia.com/v1/retrieval/nvidia/"
        self.model_name = model_name

        if self.model_name == "nvidia/nv-rerankqa-mistral-4b-v3":
            self.base_url = os.path.join(
                base_url, "nv-rerankqa-mistral-4b-v3", "reranking"
            )

        if self.model_name == "nvidia/rerank-qa-mistral-4b":
            self.base_url = os.path.join(base_url, "reranking")
            self.model_name = "nv-rerank-qa-mistral-4b:1"

        self.headers = {
            "accept": "application/json",
            "Content-Type": "application/json",
            "Authorization": f"Bearer {key}",
        }

    def similarity(self, query: str, texts: list):
        """
        Rerank texts using NVIDIA's AI reranking API.
        
        Uses logits as scores from the response.
        """
        token_count = num_tokens_from_string(query) + sum(
            [num_tokens_from_string(t) for t in texts]
        )
        data = {
            "model": self.model_name,
            "query": {"text": query},
            "passages": [{"text": text} for text in texts],
            "truncate": "END",
            "top_n": len(texts),
        }
        res = requests.post(self.base_url, headers=self.headers, json=data).json()
        rank = np.zeros(len(texts), dtype=float)
        for d in res["rankings"]:
            rank[d["index"]] = d["logit"]
        return rank, token_count


class LmStudioRerank(Base):
    """
    Placeholder for LM Studio reranking implementation (not implemented).
    """
    def __init__(self, key, model_name, base_url):
        pass

    def similarity(self, query: str, texts: list):
        raise NotImplementedError("The LmStudioRerank has not been implement")


class OpenAI_APIRerank(Base):
    """
    Reranker implementation for OpenAI-compatible reranking APIs.
    """
    def __init__(self, key, model_name, base_url):
        if base_url.find("/rerank") == -1:
            self.base_url = urljoin(base_url, "/rerank")
        else:
            self.base_url = base_url
        self.headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {key}"
        }
        self.model_name = model_name.split("___")[0]

    def similarity(self, query: str, texts: list):
        """
        Rerank texts using OpenAI-compatible reranking API.
        
        Normalizes scores to 0-1 range and handles API response.
        """
        # noway to config Ragflow , use fix setting
        texts = [truncate(t, 500) for t in texts]
        data = {
            "model": self.model_name,
            "query": query,
            "documents": texts,
            "top_n": len(texts),
        }
        token_count = 0
        for t in texts:
            token_count += num_tokens_from_string(t)
        res = requests.post(self.base_url, headers=self.headers, json=data).json()
        rank = np.zeros(len(texts), dtype=float)
        if 'results' not in res:
            raise ValueError("response not contains results\n" + str(res))
        for d in res["results"]:
            rank[d["index"]] = d["relevance_score"]

        # Normalize the rank values to the range 0 to 1
        min_rank = np.min(rank)
        max_rank = np.max(rank)

        # Avoid division by zero if all ranks are identical
        if max_rank - min_rank != 0:
            rank = (rank - min_rank) / (max_rank - min_rank)
        else:
            rank = np.zeros_like(rank)

        return rank, token_count


class CoHereRerank(Base):
    """
    Implementation for Cohere's reranking service.
    """
    def __init__(self, key, model_name, base_url=None):
        from cohere import Client

        self.client = Client(api_key=key, base_url=base_url)
        self.model_name = model_name.split("___")[0]

    def similarity(self, query: str, texts: list):
        """
        Rerank texts using Cohere's reranking API.
        
        Uses the Cohere client for API interaction.
        """
        token_count = num_tokens_from_string(query) + sum(
            [num_tokens_from_string(t) for t in texts]
        )
        res = self.client.rerank(
            model=self.model_name,
            query=query,
            documents=texts,
            top_n=len(texts),
            return_documents=False,
        )
        rank = np.zeros(len(texts), dtype=float)
        for d in res.results:
            rank[d.index] = d.relevance_score
        return rank, token_count


class TogetherAIRerank(Base):
    """
    Placeholder for TogetherAI reranking implementation (not implemented).
    """
    def __init__(self, key, model_name, base_url):
        pass

    def similarity(self, query: str, texts: list):
        raise NotImplementedError("The api has not been implement")


class SILICONFLOWRerank(Base):
    """
    Implementation for SiliconFlow's reranking service.
    """
    def __init__(
            self, key, model_name, base_url="https://api.siliconflow.cn/v1/rerank"
    ):
        if not base_url:
            base_url = "https://api.siliconflow.cn/v1/rerank"
        self.model_name = model_name
        self.base_url = base_url
        self.headers = {
            "accept": "application/json",
            "content-type": "application/json",
            "authorization": f"Bearer {key}",
        }

    def similarity(self, query: str, texts: list):
        """
        Rerank texts using SiliconFlow's API.
        
        Supports chunking for long documents and overlap.
        """
        payload = {
            "model": self.model_name,
            "query": query,
            "documents": texts,
            "top_n": len(texts),
            "return_documents": False,
            "max_chunks_per_doc": 1024,
            "overlap_tokens": 80,
        }
        response = requests.post(
            self.base_url, json=payload, headers=self.headers
        ).json()
        rank = np.zeros(len(texts), dtype=float)
        if "results" not in response:
            return rank, 0

        for d in response["results"]:
            rank[d["index"]] = d["relevance_score"]
        return (
            rank,
            response["meta"]["tokens"]["input_tokens"] + response["meta"]["tokens"]["output_tokens"],
        )


class BaiduYiyanRerank(Base):
    """
    Implementation for Baidu Yiyan (Qianfan) reranking service.
    """
    def __init__(self, key, model_name, base_url=None):
        from qianfan.resources import Reranker

        key = json.loads(key)
        ak = key.get("yiyan_ak", "")
        sk = key.get("yiyan_sk", "")
        self.client = Reranker(ak=ak, sk=sk)
        self.model_name = model_name

    def similarity(self, query: str, texts: list):
        """
        Rerank texts using Baidu Yiyan's reranking API.
        
        Handles the specific response format of Qianfan API.
        """
        res = self.client.do(
            model=self.model_name,
            query=query,
            documents=texts,
            top_n=len(texts),
        ).body
        rank = np.zeros(len(texts), dtype=float)
        for d in res["results"]:
            rank[d["index"]] = d["relevance_score"]
        return rank, self.total_token_count(res)


class VoyageRerank(Base):
    """
    Implementation for Voyage AI reranking service.
    """
    def __init__(self, key, model_name, base_url=None):
        import voyageai

        self.client = voyageai.Client(api_key=key)
        self.model_name = model_name

    def similarity(self, query: str, texts: list):
        """
        Rerank texts using Voyage AI's reranking API.
        
        Returns empty array if texts list is empty.
        """
        rank = np.zeros(len(texts), dtype=float)
        if not texts:
            return rank, 0
        res = self.client.rerank(
            query=query, documents=texts, model=self.model_name, top_k=len(texts)
        )
        for r in res.results:
            rank[r.index] = r.relevance_score
        return rank, res.total_tokens


class QWenRerank(Base):
    """
    Implementation for Alibaba's QWen reranking service via DashScope.
    """
    def __init__(self, key, model_name='gte-rerank', base_url=None, **kwargs):
        import dashscope
        self.api_key = key
        self.model_name = dashscope.TextReRank.Models.gte_rerank if model_name is None else model_name

    def similarity(self, query: str, texts: list):
        """
        Rerank texts using QWen's reranking API through DashScope.
        
        Handles HTTP errors with informative error messages.
        """
        import dashscope
        from http import HTTPStatus
        resp = dashscope.TextReRank.call(
            api_key=self.api_key,
            model=self.model_name,
            query=query,
            documents=texts,
            top_n=len(texts),
            return_documents=False
        )
        rank = np.zeros(len(texts), dtype=float)
        if resp.status_code == HTTPStatus.OK:
            for r in resp.output.results:
                rank[r.index] = r.relevance_score
            return rank, resp.usage.total_tokens
        else:
            raise ValueError(f"Error calling QWenRerank model {self.model_name}: {resp.status_code} - {resp.text}")


class HuggingfaceRerank(DefaultRerank):
    """
    Reranker implementation for local HuggingFace models via REST API.
    """
    @staticmethod
    def post(query: str, texts: list, url="127.0.0.1"):
        """
        POST request helper for Hugging Face reranker API.
        
        Handles batching to prevent oversized requests.
        """
        exc = None
        scores = [0 for _ in range(len(texts))]
        batch_size = 8
        for i in range(0, len(texts), batch_size):
            try:
                res = requests.post(f"http://{url}/rerank", headers={"Content-Type": "application/json"},
                                    json={"query": query, "texts": texts[i: i + batch_size],
                                          "raw_scores": False, "truncate": True})
                for o in res.json():
                    scores[o["index"] + i] = o["score"]
            except Exception as e:
                exc = e

        if exc:
            raise exc
        return np.array(scores)

    def __init__(self, key, model_name="BAAI/bge-reranker-v2-m3", base_url="http://127.0.0.1"):
        self.model_name = model_name
        self.base_url = base_url

    def similarity(self, query: str, texts: list) -> tuple[np.ndarray, int]:
        """
        Calculate similarity between query and texts using HuggingFace model API.
        
        Returns empty array for empty texts list.
        """
        if not texts:
            return np.array([]), 0
        token_count = 0
        for t in texts:
            token_count += num_tokens_from_string(t)
        return HuggingfaceRerank.post(query, texts, self.base_url), token_count


class GPUStackRerank(Base):
    """
    Implementation for GPUStack's reranking service.
    """
    def __init__(
            self, key, model_name, base_url
    ):
        if not base_url:
            raise ValueError("url cannot be None")

        self.model_name = model_name
        self.base_url = str(URL(base_url) / "v1" / "rerank")
        self.headers = {
            "accept": "application/json",
            "content-type": "application/json",
            "authorization": f"Bearer {key}",
        }

    def similarity(self, query: str, texts: list):
        """
        Rerank texts using GPUStack's reranking API.
        
        Provides detailed error messages on API failures.
        """
        payload = {
            "model": self.model_name,
            "query": query,
            "documents": texts,
            "top_n": len(texts),
        }

        try:
            response = requests.post(
                self.base_url, json=payload, headers=self.headers
            )
            response.raise_for_status()
            response_json = response.json()

            rank = np.zeros(len(texts), dtype=float)
            if "results" not in response_json:
                return rank, 0

            token_count = 0
            for t in texts:
                token_count += num_tokens_from_string(t)

            for result in response_json["results"]:
                rank[result["index"]] = result["relevance_score"]

            return (
                rank,
                token_count,
            )

        except httpx.HTTPStatusError as e:
            raise ValueError(
                f"Error calling GPUStackRerank model {self.model_name}: {e.response.status_code} - {e.response.text}")

