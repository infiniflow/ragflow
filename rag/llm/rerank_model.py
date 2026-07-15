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
import json
import logging
import time
from abc import ABC
from urllib.parse import urljoin
from typing import Tuple, List
from http import HTTPStatus

import numpy as np
import requests
from yarl import URL

from common.log_utils import log_exception
from common.token_utils import num_tokens_from_string, truncate, total_token_count_from_response


class Base(ABC):
    def __init__(self, key, model_name, **kwargs):
        pass

    def similarity(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        """Score ``texts`` against ``query`` and return ``(rank, token_count)``.

        This is the single public entry point shared by every reranker. It
        short-circuits empty input and guarantees the returned scores are
        min-max normalized to ``[0, 1]`` regardless of what the backend emits
        (relevance scores, cosine similarities or raw logits). Downstream
        hybrid scoring blends the reranker output with token similarity on a
        fixed ``[0, 1]`` scale, so an un-normalized provider (e.g. NVIDIA's
        unbounded logits) would otherwise corrupt the final ordering.

        Subclasses implement provider-specific scoring in :meth:`_compute_rank`
        and must not normalize themselves.
        """
        if not query or not texts:
            return np.zeros(len(texts) if texts else 0, dtype=float), 0
        rank, token_count = self._compute_rank(query, texts)
        rank = np.asarray(rank, dtype=float)
        if rank.size:
            logging.debug(
                "Rerank %s scores before normalization: count=%d min=%.4f max=%.4f",
                self.__class__.__name__,
                rank.size,
                float(np.min(rank)),
                float(np.max(rank)),
            )
        return self._normalize_rank(rank), token_count

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        """Provider-specific scoring. ``query`` and ``texts`` are non-empty."""
        raise NotImplementedError("Please implement _compute_rank method!")

    @staticmethod
    def _normalize_rank(rank: np.ndarray) -> np.ndarray:
        """Guarantee scores land in ``[0, 1]`` for the hybrid blend.

        Providers that already emit calibrated relevance scores in ``[0, 1]``
        (Cohere, Jina, Voyage, ...) are returned unchanged, so their absolute
        magnitudes, ``similarity_threshold`` semantics and reported
        ``vector_similarity`` are preserved. Only out-of-range output (e.g.
        NVIDIA's unbounded, often negative logits) is rescaled: a batch with a
        usable spread is min-max mapped onto ``[0, 1]`` (which stops a negative
        logit from dragging a relevant chunk below pure keyword matches once
        weighted by ``vtweight``), while a spreadless batch (including a single
        candidate) has no relative signal and is clamped instead, so a lone
        high score is not silently zeroed.
        """
        if rank.size == 0:
            return rank
        min_rank = float(np.min(rank))
        max_rank = float(np.max(rank))

        if min_rank >= 0.0 and max_rank <= 1.0:
            return rank
        span = max_rank - min_rank
        if span < 1e-3:
            return np.clip(rank, 0.0, 1.0)
        return (rank - min_rank) / span


class JinaRerank(Base):
    _FACTORY_NAME = "Jina"

    def __init__(self, key, model_name="jina-reranker-v2-base-multilingual", base_url="https://api.jina.ai/v1/rerank"):
        self.base_url = base_url or "https://api.jina.ai/v1/rerank"
        self.headers = {"Content-Type": "application/json", "Authorization": f"Bearer {key}"}
        self.model_name = model_name

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        texts = [truncate(t, 8196) for t in texts]
        data = {"model": self.model_name, "query": query, "documents": texts, "top_n": len(texts)}
        response = requests.post(self.base_url, headers=self.headers, json=data, timeout=30)
        response.raise_for_status()
        res = response.json()
        rank = np.zeros(len(texts), dtype=float)
        try:
            for d in res.get("results", []):
                rank[d["index"]] = d["relevance_score"]
        except Exception as _e:
            log_exception(_e, res)
        return rank, total_token_count_from_response(res)


class XInferenceRerank(Base):
    _FACTORY_NAME = "Xinference"

    def __init__(self, key="x", model_name="", base_url=""):
        if base_url.find("/v1") == -1:
            base_url = urljoin(base_url, "/v1/rerank")
        if base_url.find("/rerank") == -1:
            base_url = urljoin(base_url, "/v1/rerank")
        self.model_name = model_name
        self.base_url = base_url
        self.headers = {"Content-Type": "application/json", "accept": "application/json"}
        if key and key != "x":
            self.headers["Authorization"] = f"Bearer {key}"

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        pairs = [(query, truncate(t, 4096)) for t in texts]
        token_count = 0
        for _, t in pairs:
            token_count += num_tokens_from_string(t)
        data = {"model": self.model_name, "query": query, "return_documents": "true", "return_len": "true", "documents": texts}
        response = requests.post(self.base_url, headers=self.headers, json=data, timeout=30)
        response.raise_for_status()
        res = response.json()
        rank = np.zeros(len(texts), dtype=float)
        try:
            for d in res.get("results", []):
                rank[d["index"]] = d["relevance_score"]
        except Exception as _e:
            log_exception(_e, res)
        return rank, token_count


class LocalAIRerank(Base):
    _FACTORY_NAME = "LocalAI"

    def __init__(self, key, model_name, base_url):
        if base_url.find("/rerank") == -1:
            self.base_url = urljoin(base_url, "/rerank")
        else:
            self.base_url = base_url
        self.headers = {"Content-Type": "application/json", "Authorization": f"Bearer {key}"}
        self.model_name = model_name.split("___")[0]

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
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
        response = requests.post(self.base_url, headers=self.headers, json=data, timeout=30)
        response.raise_for_status()
        res = response.json()
        rank = np.zeros(len(texts), dtype=float)
        try:
            for d in res.get("results", []):
                rank[d["index"]] = d["relevance_score"]
        except Exception as _e:
            log_exception(_e, res)
        return rank, token_count


class NvidiaRerank(Base):
    _FACTORY_NAME = "NVIDIA"

    def __init__(self, key, model_name, base_url="https://ai.api.nvidia.com/v1/retrieval/nvidia/"):
        if not base_url:
            base_url = "https://ai.api.nvidia.com/v1/retrieval/nvidia/"
        self.model_name = model_name

        if self.model_name == "nvidia/nv-rerankqa-mistral-4b-v3":
            self.base_url = urljoin(base_url, "nv-rerankqa-mistral-4b-v3/reranking")

        if self.model_name == "nvidia/rerank-qa-mistral-4b":
            self.base_url = urljoin(base_url, "reranking")
            self.model_name = "nv-rerank-qa-mistral-4b:1"

        self.headers = {
            "accept": "application/json",
            "Content-Type": "application/json",
            "Authorization": f"Bearer {key}",
        }

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        token_count = num_tokens_from_string(query) + sum([num_tokens_from_string(t) for t in texts])
        data = {
            "model": self.model_name,
            "query": {"text": query},
            "passages": [{"text": text} for text in texts],
            "truncate": "END",
            "top_n": len(texts),
        }
        response = requests.post(self.base_url, headers=self.headers, json=data, timeout=30)
        response.raise_for_status()
        res = response.json()
        rank = np.zeros(len(texts), dtype=float)
        try:
            for d in res.get("rankings", []):
                rank[d["index"]] = d["logit"]
        except Exception as _e:
            log_exception(_e, res)
        return rank, token_count


class LmStudioRerank(Base):
    _FACTORY_NAME = "LM-Studio"

    def __init__(self, key, model_name, base_url, **kwargs):
        pass

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        raise NotImplementedError("The LmStudioRerank has not been implemented")


class OpenAI_APIRerank(Base):
    _FACTORY_NAME = "OpenAI-API-Compatible"

    def __init__(self, key, model_name, base_url):
        normalized_base_url = (base_url or "").strip()
        if "/rerank" in normalized_base_url:
            self.base_url = normalized_base_url.rstrip("/")
        else:
            self.base_url = urljoin(f"{normalized_base_url.rstrip('/')}/", "rerank").rstrip("/")
        self.headers = {"Content-Type": "application/json", "Authorization": f"Bearer {key}"}
        self.model_name = model_name.split("___")[0]

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
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
        response = requests.post(self.base_url, headers=self.headers, json=data, timeout=30)
        response.raise_for_status()
        res = response.json()
        rank = np.zeros(len(texts), dtype=float)
        try:
            for d in res.get("results", []):
                rank[d["index"]] = d["relevance_score"]
        except Exception as _e:
            log_exception(_e, res)
        return rank, token_count


class CoHereRerank(Base):
    _FACTORY_NAME = ["Cohere", "VLLM"]

    def __init__(self, key, model_name, base_url=None):
        from cohere import Client

        client_kwargs = {"api_key": key, "timeout": 30.0}
        if base_url and base_url.strip():
            client_kwargs["base_url"] = base_url
        self.client = Client(**client_kwargs)
        self.model_name = model_name.split("___")[0]

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        token_count = num_tokens_from_string(query) + sum([num_tokens_from_string(t) for t in texts])
        res = self.client.rerank(
            model=self.model_name,
            query=query,
            documents=texts,
            top_n=len(texts),
            return_documents=False,
        )
        rank = np.zeros(len(texts), dtype=float)
        try:
            for d in res.results:
                rank[d.index] = d.relevance_score
        except Exception as _e:
            log_exception(_e, res)
        return rank, token_count


# Reranker connector for AWS Bedrock, calling the bedrock-agent-runtime Rerank
# API (e.g. amazon.rerank-v1:0, cohere.rerank-v3-5:0). The JSON key protocol
# (auth_mode / bedrock_region / bedrock_ak / bedrock_sk) mirrors BedrockEmbed in
# embedding_model.py.
class BedrockRerank(Base):
    _FACTORY_NAME = "Bedrock"

    # Hard limits of the bedrock-agent-runtime Rerank API: each document text
    # (RerankTextDocument.text) is capped at 32,000 characters, and a single
    # request accepts at most 1,000 sources / numberOfResults.
    _MAX_DOC_CHARS = 32000
    _MAX_SOURCES = 1000

    def __init__(self, key, model_name, **kwargs):
        import boto3

        key = json.loads(key)
        mode = key.get("auth_mode")
        if not mode:
            logging.error("Bedrock auth_mode is not provided in the key")
            raise ValueError("Bedrock auth_mode must be provided in the key")

        self.bedrock_region = key.get("bedrock_region")
        self.model_name = model_name
        # On-demand foundation-model ARN; works for amazon.rerank-v1:0 / cohere.rerank-*.
        self.model_arn = f"arn:aws:bedrock:{self.bedrock_region}::foundation-model/{self.model_name}"
        # Per-document truncation guard sized to the model window. Cohere Rerank
        # v3.5 shares a ~4k window between query and document (~2048 for docs);
        # Amazon Rerank v1 handles 32k, but chunks are small so a generous cap
        # just bounds pathological payloads. Bedrock also truncates internally.
        self.doc_max_tokens = 2048 if self.model_name.split(".")[0] == "cohere" else 8192

        # Rerank lives on the bedrock-agent-runtime service, not bedrock-runtime.
        if mode == "access_key_secret":
            self.client = boto3.client(
                service_name="bedrock-agent-runtime",
                region_name=self.bedrock_region,
                aws_access_key_id=key.get("bedrock_ak"),
                aws_secret_access_key=key.get("bedrock_sk"),
            )
        elif mode == "iam_role":
            sts_client = boto3.client("sts", region_name=self.bedrock_region)
            resp = sts_client.assume_role(RoleArn=key.get("aws_role_arn"), RoleSessionName="BedrockSession")
            creds = resp["Credentials"]
            self.client = boto3.client(
                service_name="bedrock-agent-runtime",
                region_name=self.bedrock_region,
                aws_access_key_id=creds["AccessKeyId"],
                aws_secret_access_key=creds["SecretAccessKey"],
                aws_session_token=creds["SessionToken"],
            )
        else:  # assume_role: default AWS credential chain
            self.client = boto3.client("bedrock-agent-runtime", region_name=self.bedrock_region)

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        # Truncate to the model token window, then enforce the API's hard 32k-char
        # per-text limit (a longer RerankTextQuery / RerankTextDocument is rejected).
        query = query[: self._MAX_DOC_CHARS]
        texts = [truncate(t, self.doc_max_tokens)[: self._MAX_DOC_CHARS] for t in texts]
        # Bedrock does not report token usage; count locally like CoHereRerank.
        token_count = num_tokens_from_string(query) + sum(num_tokens_from_string(t) for t in texts)

        rank = np.zeros(len(texts), dtype=float)
        result_count = 0
        started = time.perf_counter()
        # Both `sources` and `numberOfResults` are capped at 1,000 per request;
        # rerank in batches and map each score back to its global position.
        for offset in range(0, len(texts), self._MAX_SOURCES):
            batch = texts[offset : offset + self._MAX_SOURCES]
            sources = [{"type": "INLINE", "inlineDocumentSource": {"type": "TEXT", "textDocument": {"text": t}}} for t in batch]
            reranking_config = {
                "type": "BEDROCK_RERANKING_MODEL",
                "bedrockRerankingConfiguration": {
                    "numberOfResults": len(batch),
                    "modelConfiguration": {"modelArn": self.model_arn},
                },
            }
            # Drain paginated results: the API may split a batch across nextToken pages.
            next_token = None
            while True:
                request = {"queries": [{"type": "TEXT", "textQuery": {"text": query}}], "sources": sources, "rerankingConfiguration": reranking_config}
                if next_token:
                    request["nextToken"] = next_token
                res = self.client.rerank(**request)
                try:
                    for d in res.get("results", []):
                        rank[offset + d["index"]] = d["relevanceScore"]
                        result_count += 1
                except (KeyError, IndexError, TypeError) as _e:
                    log_exception(_e, res)
                next_token = res.get("nextToken")
                if not next_token:
                    break
        # Safe diagnostics only: no query, document text or credentials.
        logging.debug(
            "BedrockRerank model=%s region=%s sources=%d tokens=%d results=%d elapsed=%.3fs",
            self.model_name,
            self.bedrock_region,
            len(texts),
            token_count,
            result_count,
            time.perf_counter() - started,
        )
        return rank, token_count


class TogetherAIRerank(Base):
    _FACTORY_NAME = "TogetherAI"

    def __init__(self, key, model_name, base_url, **kwargs):
        pass

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        raise NotImplementedError("The api has not been implemented")


class SILICONFLOWRerank(Base):
    _FACTORY_NAME = "SILICONFLOW"

    def __init__(self, key, model_name, base_url="https://api.siliconflow.cn/v1/rerank"):
        normalized_base_url = (base_url or "").strip()
        if not normalized_base_url:
            normalized_base_url = "https://api.siliconflow.cn/v1/rerank"
        if "/rerank" not in normalized_base_url:
            normalized_base_url = urljoin(f"{normalized_base_url.rstrip('/')}/", "rerank").rstrip("/")
        self.model_name = model_name
        self.base_url = normalized_base_url
        self.headers = {
            "accept": "application/json",
            "content-type": "application/json",
            "authorization": f"Bearer {key}",
        }

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        payload = {
            "model": self.model_name,
            "query": query,
            "documents": texts,
            "top_n": len(texts),
            "return_documents": False,
            "max_chunks_per_doc": 1024,
            "overlap_tokens": 80,
        }
        response = requests.post(self.base_url, json=payload, headers=self.headers, timeout=30)
        response.raise_for_status()
        res = response.json()
        rank = np.zeros(len(texts), dtype=float)
        try:
            for d in res.get("results", []):
                rank[d["index"]] = d["relevance_score"]
        except Exception as _e:
            log_exception(_e, response)
        return rank, total_token_count_from_response(res)


class BaiduYiyanRerank(Base):
    _FACTORY_NAME = "BaiduYiyan"

    def __init__(self, key, model_name, base_url=None):
        from qianfan.resources import Reranker

        key = json.loads(key)
        ak = key.get("yiyan_ak", "")
        sk = key.get("yiyan_sk", "")
        self.client = Reranker(ak=ak, sk=sk, request_timeout=30)
        self.model_name = model_name

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        res = self.client.do(
            model=self.model_name,
            query=query,
            documents=texts,
            top_n=len(texts),
        ).body
        rank = np.zeros(len(texts), dtype=float)
        try:
            for d in res.get("results", []):
                rank[d["index"]] = d["relevance_score"]
        except Exception as _e:
            log_exception(_e, res)
        return rank, total_token_count_from_response(res)


class VoyageRerank(Base):
    _FACTORY_NAME = "Voyage AI"

    def __init__(self, key, model_name, base_url=None):
        import voyageai

        self.client = voyageai.Client(api_key=key, timeout=30.0)
        self.model_name = model_name

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        rank = np.zeros(len(texts), dtype=float)

        res = self.client.rerank(query=query, documents=texts, model=self.model_name, top_k=len(texts))
        try:
            for r in res.results:
                rank[r.index] = r.relevance_score
        except Exception as _e:
            log_exception(_e, res)
        return rank, res.total_tokens


class QWenRerank(Base):
    _FACTORY_NAME = "Tongyi-Qianwen"

    def __init__(self, key, model_name="gte-rerank", **kwargs):
        import dashscope

        self.api_key = key
        self.model_name = dashscope.TextReRank.Models.gte_rerank if model_name is None else model_name
        # Remove invalid global timeout, use official SDK per-request timeout parameter
        self.request_timeout = 30.0

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        import dashscope

        # Pass official request_timeout parameter to both API call branches
        if self.model_name.startswith("qwen3-rerank"):
            resp = dashscope.TextReRank.call(api_key=self.api_key, model=self.model_name, query=query, documents=texts, top_n=len(texts), request_timeout=self.request_timeout)
        else:
            resp = dashscope.TextReRank.call(api_key=self.api_key, model=self.model_name, query=query, documents=texts, top_n=len(texts), return_documents=False, request_timeout=self.request_timeout)

        rank = np.zeros(len(texts), dtype=float)
        if resp.status_code == HTTPStatus.OK:
            try:
                for r in resp.output.results:
                    rank[r.index] = r.relevance_score
            except Exception as _e:
                log_exception(_e, resp)
            return rank, total_token_count_from_response(resp)
        else:
            try:
                error_body = resp["text"] if isinstance(resp, dict) and "text" in resp else None
            except Exception:
                error_body = None
            if not error_body:
                try:
                    error_body = json.dumps(dict(resp), ensure_ascii=False)
                except Exception:
                    error_body = str(resp)
            raise ValueError(f"Error calling QWenRerank model {self.model_name}: {resp.status_code} - {error_body}")


class HuggingfaceRerank(Base):
    _FACTORY_NAME = "HuggingFace"

    @staticmethod
    def post(query: str, texts: list, url: str = "http://127.0.0.1"):
        exc = None
        scores = [0 for _ in range(len(texts))]
        batch_size = 8
        # FIX: Robust URL construction to avoid duplicate "/rerank" path suffix
        base_url = url.rstrip("/")
        if not base_url.startswith(("http://", "https://")):
            base_url = f"http://{base_url}"
        # Only append "/rerank" when endpoint does not already end with it
        endpoint = base_url if base_url.endswith("/rerank") else f"{base_url}/rerank"

        for i in range(0, len(texts), batch_size):
            try:
                # Fix: Add request timeout
                res = requests.post(
                    endpoint, headers={"Content-Type": "application/json"}, json={"query": query, "texts": texts[i : i + batch_size], "raw_scores": False, "truncate": True}, timeout=30
                )
                res.raise_for_status()
                for o in res.json():
                    scores[o["index"] + i] = o["score"]
            except Exception as e:
                exc = e

        if exc:
            raise exc
        return np.array(scores)

    def __init__(self, key, model_name="BAAI/bge-reranker-v2-m3", base_url="http://127.0.0.1"):
        self.model_name = model_name.split("___")[0]
        self.base_url = base_url

    def _compute_rank(self, query: str, texts: List) -> tuple[np.ndarray, int]:
        token_count = 0
        for t in texts:
            token_count += num_tokens_from_string(t)
        return HuggingfaceRerank.post(query, texts, self.base_url), token_count


class GPUStackRerank(Base):
    _FACTORY_NAME = "GPUStack"

    def __init__(self, key, model_name, base_url):
        if not base_url:
            raise ValueError("url cannot be None")

        self.model_name = model_name
        self.base_url = str(URL(base_url) / "v1" / "rerank")
        self.headers = {
            "accept": "application/json",
            "content-type": "application/json",
            "authorization": f"Bearer {key}",
        }

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        payload = {
            "model": self.model_name,
            "query": query,
            "documents": texts,
            "top_n": len(texts),
        }

        try:
            response = requests.post(self.base_url, json=payload, headers=self.headers, timeout=30)
            response.raise_for_status()
            response_json = response.json()

            rank = np.zeros(len(texts), dtype=float)
            token_count = sum(num_tokens_from_string(t) for t in texts)
            try:
                for result in response_json.get("results", []):
                    rank[result["index"]] = result["relevance_score"]
            except Exception as _e:
                log_exception(_e, response)

            return (rank, token_count)

        except requests.exceptions.RequestException as e:
            raise ValueError(f"Error calling GPUStackRerank model {self.model_name}: {str(e)}") from e


class NovitaRerank(JinaRerank):
    _FACTORY_NAME = "NovitaAI"

    def __init__(self, key, model_name, base_url="https://api.novita.ai/v3/openai/rerank"):
        if not base_url:
            base_url = "https://api.novita.ai/v3/openai/rerank"
        super().__init__(key, model_name, base_url)


class GiteeRerank(JinaRerank):
    _FACTORY_NAME = "GiteeAI"

    def __init__(self, key, model_name, base_url="https://ai.gitee.com/v1/rerank"):
        if not base_url:
            base_url = "https://ai.gitee.com/v1/rerank"
        super().__init__(key, model_name, base_url)


class Ai302Rerank(Base):
    _FACTORY_NAME = "302.AI"

    def __init__(self, key, model_name, base_url="https://api.302.ai/v1/rerank"):
        self.base_url = base_url or "https://api.302.ai/v1/rerank"
        self.headers = {"Content-Type": "application/json", "Authorization": f"Bearer {key}"}
        self.model_name = model_name

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        texts = [truncate(t, 500) for t in texts]
        data = {"model": self.model_name, "query": query, "documents": texts, "top_n": len(texts)}
        response = requests.post(self.base_url, headers=self.headers, json=data, timeout=30)
        response.raise_for_status()
        res = response.json()
        rank = np.zeros(len(texts), dtype=float)
        try:
            for d in res.get("results", []):
                rank[d["index"]] = d["relevance_score"]
        except Exception as _e:
            log_exception(_e, res)
        return rank, total_token_count_from_response(res)


class JiekouAIRerank(JinaRerank):
    _FACTORY_NAME = "Jiekou.AI"

    def __init__(self, key, model_name, base_url="https://api.jiekou.ai/openai/v1/rerank"):
        if not base_url:
            base_url = "https://api.jiekou.ai/openai/v1/rerank"
        super().__init__(key, model_name, base_url)


class FuturMixRerank(OpenAI_APIRerank):
    _FACTORY_NAME = "FuturMix"

    def __init__(self, key, model_name, base_url="https://futurmix.ai/v1/rerank"):
        if not base_url:
            base_url = "https://futurmix.ai/v1/rerank"
        super().__init__(key, model_name, base_url)
        logging.info("[FuturMix] Rerank initialized with model %s", model_name)


class RAGconRerank(Base):
    _FACTORY_NAME = "RAGcon"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        if not base_url:
            base_url = "https://connect.ragcon.com/v1"

        self._api_key = key
        self._base_url = base_url

        self.headers = {"Content-Type": "application/json", "Authorization": f"Bearer {key}"}
        self.model_name = model_name

    def _compute_rank(self, query: str, texts: List) -> Tuple[np.ndarray, int]:
        texts = [truncate(t, 500) for t in texts]
        data = {
            "model": self.model_name,
            "query": query,
            "documents": texts,
            "top_n": len(texts),
        }
        token_count = sum(num_tokens_from_string(t) for t in texts)
        response = requests.post(self._base_url + "/rerank", headers=self.headers, json=data, timeout=30)
        response.raise_for_status()
        res = response.json()
        rank = np.zeros(len(texts), dtype=float)
        try:
            for d in res.get("results", []):
                rank[d["index"]] = d["relevance_score"]
        except Exception as _e:
            log_exception(_e, res)
        return rank, token_count


class NewAPIRerank(Base):
    _FACTORY_NAME = "New API"

    def __init__(self, key, model_name, base_url):
        normalized_base_url = (base_url or "").strip()
        if "/rerank" in normalized_base_url:
            self.base_url = normalized_base_url.rstrip("/")
        else:
            self.base_url = urljoin(f"{normalized_base_url.rstrip('/')}/", "rerank").rstrip("/")
        self.headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {key}",
        }
        self.model_name = model_name.split("___")[0]

    def _compute_rank(self, query: str, texts: list):
        texts = [truncate(t, 500) for t in texts]
        data = {
            "model": self.model_name,
            "query": query,
            "documents": texts,
            "top_n": len(texts),
        }
        token_count = sum(num_tokens_from_string(t) for t in texts)
        res = requests.post(self.base_url, headers=self.headers, json=data).json()
        rank = np.zeros(len(texts), dtype=float)
        try:
            for d in res["results"]:
                rank[d["index"]] = d["relevance_score"]
        except Exception as _e:
            log_exception(_e, res)
        return rank, token_count
