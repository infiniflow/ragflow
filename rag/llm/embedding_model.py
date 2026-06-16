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
import os
import threading
from abc import ABC
from contextlib import contextmanager
from urllib.parse import urljoin
from json.decoder import JSONDecodeError

import dashscope
import numpy as np
import requests
from ollama import Client
from openai import OpenAI
from zhipuai import ZhipuAI

from common import settings
from common.exceptions import ModelException
from common.token_utils import num_tokens_from_string, truncate, total_token_count_from_response
from rag.llm.key_utils import _normalize_replicate_key
import logging
import base64

logger = logging.getLogger(__name__)

# Standard token ceiling for the common 8K-context embedding models (OpenAI
# text-embedding-*, Mistral, Bedrock Titan, ...). Inputs are truncated to this
# many tokens so boundary-sized chunks are not rejected by the provider.
DEFAULT_MAX_TOKENS = 8192


class EmbeddingError(ModelException):
    """Raised when an embedding provider fails to return usable embeddings.

    A single, deterministic exception type for every provider failure path so
    callers see consistent behaviour regardless of which SDK raised underneath.
    Subclasses ``ModelException`` so the API error handler (and its retry
    semantics) treats embedding failures like any other model failure.
    """


def _sorted_by_index(items):
    """Order OpenAI-style SDK embedding items by their ``.index`` so batched
    results stay aligned with input order even if the provider returns them out
    of order. Stable no-op when items carry no ``index`` attribute."""
    return sorted(items, key=lambda d: getattr(d, "index", 0))


def _raise_model_exception_if_failed(resp):
    status_code = resp.status_code
    if status_code >= 400:
        if status_code < 500 and status_code not in [408, 429]:
            raise ModelException(f"status: {resp.status_code}, response: {resp.text}", retryable=False)
        raise ModelException(f"status: {resp.status_code}, response: {resp.text}", retryable=True)


def _dashscope_base_url_for_log(base_url: str) -> str:
    """Log host/path only (no query string) so secrets in URLs are not printed."""
    return base_url.split("?", 1)[0].strip()[:256]


def _dashscope_native_http_api_url(base_url: str | None) -> str | None:
    """
    Resolve the DashScope *native* HTTP API root for Tongyi-Qianwen (Qwen) text embeddings.

    RAGFlow often stores an OpenAI-compatible base URL (e.g. ``.../compatible-mode/v1``) for
    the same provider. The ``dashscope`` Python SDK used by ``TextEmbedding.call`` does *not*
    use that path; it expects ``https://<host>/api/v1`` instead.

    Users outside mainland China are directed to the international endpoint
    (``dashscope-intl.aliyuncs.com``); domestic traffic uses ``dashscope.aliyuncs.com``.
    When ``base_url`` already points at the native API root (ends with ``/api/v1``), it is
    returned unchanged so custom or regional deployments keep working.
    """
    if not base_url:
        return None
    u = base_url.strip().rstrip("/")
    safe = _dashscope_base_url_for_log(u)
    if u.endswith("/api/v1"):
        logger.debug("DashScope Tongyi-Qianwen embedding: using native API base as configured (%s)", safe)
        return u
    # International (Singapore) DashScope — required for overseas Tongyi-Qianwen accounts.
    if "dashscope-intl.aliyuncs.com" in u:
        resolved = "https://dashscope-intl.aliyuncs.com/api/v1"
        logger.info(
            "DashScope Tongyi-Qianwen embedding: mapped configured base_url to intl native API (%s -> %s)",
            safe,
            resolved,
        )
        return resolved
    # China mainland DashScope default host.
    if "dashscope.aliyuncs.com" in u:
        resolved = "https://dashscope.aliyuncs.com/api/v1"
        logger.info(
            "DashScope Tongyi-Qianwen embedding: mapped configured base_url to CN native API (%s -> %s)",
            safe,
            resolved,
        )
        return resolved
    logger.warning(
        "DashScope Tongyi-Qianwen embedding: base_url is set but not recognized as a DashScope host; using SDK default endpoint (%s)",
        safe,
    )
    return None


@contextmanager
def _dashscope_native_api_url_scope(url: str | None):
    """
    Temporarily set ``dashscope.base_http_api_url`` for the duration of a single SDK call,
    then restore the previous value. Narrows the window where concurrent threads see a mismatch.
    """
    if not url:
        yield
        return
    prev = getattr(dashscope, "base_http_api_url", None)
    dashscope.base_http_api_url = url
    try:
        yield
    finally:
        dashscope.base_http_api_url = prev


class Base(ABC):
    def __init__(self, key, model_name, **kwargs):
        """
        Constructor for abstract base class.
        Parameters are accepted for interface consistency but are not stored.
        Subclasses should implement their own initialization as needed.
        """
        pass

    def encode(self, texts: list):
        raise NotImplementedError("Please implement encode method!")

    def encode_queries(self, text: str):
        raise NotImplementedError("Please implement encode method!")

    def _batched_encode(self, texts: list, call_fn, *, batch_size: int, truncate_to: int | None = None):
        """Drive an embedding provider over ``texts`` in batches.

        This is the shared template behind the OpenAI-style providers. It owns:

        * optional per-text truncation to ``truncate_to`` tokens (skipped when
          ``None``) so oversized inputs do not get rejected by the provider;
        * the batch loop, issuing ``ceil(len(texts) / batch_size)`` calls;
        * accumulation of the per-text vectors into a single ``np.ndarray``;
        * summation of the per-batch token counts;
        * one deterministic, informative error path.

        ``call_fn`` is a provider-supplied closure ``call_fn(batch) ->
        (embeddings, token_count)``. It performs the SDK/HTTP request *and*
        parses the response (so a malformed/error response surfaces here), and
        must not assume any particular response shape — the helper never touches
        the raw response object. ``embeddings`` is a sequence of per-text
        vectors; ``token_count`` is the real token usage for that batch.

        Any exception raised by ``call_fn`` is wrapped in a single
        :class:`EmbeddingError` that includes the underlying detail. We log and
        raise here directly instead of relying on ``log_exception``'s implicit
        raise (whose surfaced exception varies by SDK response shape).
        """
        if truncate_to is not None:
            texts = [truncate(t, truncate_to) for t in texts]
        vectors = []
        token_count = 0
        for i in range(0, len(texts), batch_size):
            batch = texts[i : i + batch_size]
            try:
                embeddings, tokens = call_fn(batch)
            except ModelException:
                # Already a structured (and possibly retryable) model error; keep it.
                raise
            except Exception as e:
                logger.exception("%s embedding request failed", type(self).__name__)
                raise EmbeddingError(f"Embedding request failed for {type(self).__name__}. Error: {e}") from e
            vectors.extend(embeddings)
            token_count += tokens
        return np.array(vectors), token_count

    @staticmethod
    def _openai_http_embeddings(response):
        """Parse an OpenAI-compatible HTTP embeddings ``requests`` response.

        Returns ``(embeddings, token_count)``. Raises a retryable-aware
        :class:`ModelException` on a bad HTTP status, or surfaces the response
        body (via :class:`EmbeddingError`) when the payload is not a successful
        ``{"data": [...]}`` response.
        """
        _raise_model_exception_if_failed(response)
        res = response.json()
        if not isinstance(res, dict) or "data" not in res:
            raise ValueError(f"unexpected embeddings response (status {getattr(response, 'status_code', '?')}): {res}")
        # Keep results aligned with input order: OpenAI-compatible responses carry
        # a per-item `index`; sorting by it is a no-op (stable) when it is absent.
        data = sorted(res["data"], key=lambda d: d.get("index", 0))
        return [d["embedding"] for d in data], total_token_count_from_response(res)


class BuiltinEmbed(Base):
    _FACTORY_NAME = "Builtin"
    MAX_TOKENS = {"Qwen/Qwen3-Embedding-0.6B": 30000, "BAAI/bge-m3": 8000, "BAAI/bge-small-en-v1.5": 500}
    _model = None
    _model_name = ""
    _max_tokens = 500
    _model_lock = threading.Lock()

    def __init__(self, key, model_name, **kwargs):
        logging.info(f"Initialize BuiltinEmbed according to settings.EMBEDDING_CFG: {settings.EMBEDDING_CFG}")
        embedding_cfg = settings.EMBEDDING_CFG
        if not BuiltinEmbed._model and "tei-" in os.getenv("COMPOSE_PROFILES", ""):
            with BuiltinEmbed._model_lock:
                BuiltinEmbed._model_name = settings.EMBEDDING_MDL
                BuiltinEmbed._max_tokens = BuiltinEmbed.MAX_TOKENS.get(settings.EMBEDDING_MDL, 500)
                BuiltinEmbed._model = HuggingFaceEmbed(embedding_cfg["api_key"], settings.EMBEDDING_MDL, base_url=embedding_cfg["base_url"])
        self._model = BuiltinEmbed._model
        self._model_name = BuiltinEmbed._model_name
        self._max_tokens = BuiltinEmbed._max_tokens

    def encode(self, texts: list):
        batch_size = 16
        # TEI is able to auto truncate inputs according to https://github.com/huggingface/text-embeddings-inference.
        token_count = 0
        batches = []
        for i in range(0, len(texts), batch_size):
            embeddings, token_count_delta = self._model.encode(texts[i : i + batch_size])
            token_count += token_count_delta
            batches.append(embeddings)
        ress = np.vstack(batches) if batches else np.array([])
        return ress, token_count

    def encode_queries(self, text: str):
        return self._model.encode_queries(text)


class OpenAIEmbed(Base):
    _FACTORY_NAME = "OpenAI"

    def __init__(self, key, model_name="text-embedding-ada-002", base_url="https://api.openai.com/v1"):
        if not base_url:
            base_url = "https://api.openai.com/v1"
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name

    def _call(self, batch):
        res = self.client.embeddings.create(input=batch, model=self.model_name, encoding_format="float", extra_body={"drop_params": True})
        return [d.embedding for d in _sorted_by_index(res.data)], total_token_count_from_response(res)

    def encode(self, texts: list):
        # OpenAI requires batch size <=16; 8191 is the documented per-input token ceiling.
        return self._batched_encode(texts, self._call, batch_size=16, truncate_to=8191)

    def encode_queries(self, text):
        vectors, token_count = self._batched_encode([text], self._call, batch_size=16, truncate_to=8191)
        return vectors[0], token_count


class LocalAIEmbed(Base):
    _FACTORY_NAME = "LocalAI"

    def __init__(self, key, model_name, base_url):
        if not base_url:
            raise ValueError("Local embedding model url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key="empty", base_url=base_url)
        self.model_name = model_name.split("___")[0]

    def _call(self, batch):
        res = self.client.embeddings.create(input=batch, model=self.model_name)
        # Local servers (LocalAI / LM Studio) usually omit usage data; fall back
        # to a local tiktoken count rather than fabricating a fixed number.
        tokens = total_token_count_from_response(res)
        if not tokens:
            tokens = sum(num_tokens_from_string(t) for t in batch)
        return [d.embedding for d in _sorted_by_index(res.data)], tokens

    def encode(self, texts: list):
        return self._batched_encode(texts, self._call, batch_size=16)

    def encode_queries(self, text):
        vectors, token_count = self._batched_encode([text], self._call, batch_size=16)
        return vectors[0], token_count


def _resolve_azure_credentials(key):
    try:
        key_obj = json.loads(key)
        if isinstance(key_obj, dict):
            return key_obj.get("api_key", ""), key_obj.get("api_version", "2024-02-01")
        logging.warning(
            "Azure credential payload parsed as JSON but is not an object; using raw api_key string"
        )
    except (json.JSONDecodeError, TypeError):
        logging.warning("Azure credential payload is not valid JSON; using raw api_key string")
    return key, "2024-02-01"


class AzureEmbed(OpenAIEmbed):
    _FACTORY_NAME = "Azure-OpenAI"

    def __init__(self, key, model_name, **kwargs):
        from openai.lib.azure import AzureOpenAI

        api_key, api_version = _resolve_azure_credentials(key)
        self.client = AzureOpenAI(api_key=api_key, azure_endpoint=kwargs["base_url"], api_version=api_version)
        self.model_name = model_name


class AstraflowEmbed(OpenAIEmbed):
    _FACTORY_NAME = "Astraflow"

    def __init__(self, key, model_name, base_url="https://api-us-ca.umodelverse.ai/v1"):
        if not base_url:
            base_url = "https://api-us-ca.umodelverse.ai/v1"
        super().__init__(key, model_name, base_url)


class AstraflowCNEmbed(OpenAIEmbed):
    _FACTORY_NAME = "Astraflow-CN"

    def __init__(self, key, model_name, base_url="https://api.modelverse.cn/v1"):
        if not base_url:
            base_url = "https://api.modelverse.cn/v1"
        super().__init__(key, model_name, base_url)


class FuturMixEmbed(OpenAIEmbed):
    _FACTORY_NAME = "FuturMix"

    def __init__(self, key, model_name="text-embedding-3-small", base_url="https://futurmix.ai/v1"):
        if not base_url:
            base_url = "https://futurmix.ai/v1"
        super().__init__(key, model_name, base_url)
        logging.info("[FuturMix] Embedding initialized with model %s", model_name)


class BaiChuanEmbed(OpenAIEmbed):
    _FACTORY_NAME = "BaiChuan"

    def __init__(self, key, model_name="Baichuan-Text-Embedding", base_url="https://api.baichuan-ai.com/v1"):
        if not base_url:
            base_url = "https://api.baichuan-ai.com/v1"
        super().__init__(key, model_name, base_url)


class QWenEmbed(Base):
    """
    Embeddings for Alibaba Tongyi-Qianwen via the DashScope ``TextEmbedding`` API.

    ``base_url`` comes from the user's embedding-model configuration (often the same host
    as the OpenAI-compatible chat endpoint). This class maps known DashScope hosts to the
    native ``/api/v1`` base URL so international and China endpoints both work.
    """

    _FACTORY_NAME = "Tongyi-Qianwen"

    def __init__(self, key, model_name="text_embedding_v2", base_url=None, **kwargs):
        self.key = key
        self.model_name = model_name
        # Native API root for the SDK; None if base_url is absent or not a known DashScope host.
        self._dashscope_http_api_url = _dashscope_native_http_api_url(base_url)

    def encode(self, texts: list):
        import time

        import dashscope

        batch_size = 4
        res = []
        token_count = 0
        texts = [truncate(t, 2048) for t in texts]
        for i in range(0, len(texts), batch_size):
            retry_max, retry_wait_secs = 5, 10
            for retry in range(retry_max):
                with _dashscope_native_api_url_scope(self._dashscope_http_api_url):
                    resp = dashscope.TextEmbedding.call(model=self.model_name, input=texts[i : i + batch_size], api_key=self.key, text_type="document")
                status_code = resp.status_code
                if status_code >= 400 and status_code < 500 and status_code not in [408, 429]:
                    # No need to retry for 4XX error
                    raise ModelException(f"Error, status: {status_code}, response: {resp}")
                if status_code == 200:
                    break
                if retry < retry_max - 1:
                    logging.warning(f"Got error response from DashScope API (status: {status_code}, response: {resp}). Wait {retry_wait_secs} seconds. Retrying...")
                    time.sleep(retry_wait_secs)
                else:
                    raise ModelException(f"Error after {retry_max} retries, status: {status_code}, response: {resp}")
            try:
                embds = [[] for _ in range(len(resp["output"]["embeddings"]))]
                for e in resp["output"]["embeddings"]:
                    embds[e["text_index"]] = e["embedding"]
                res.extend(embds)
                token_count += total_token_count_from_response(resp)
            except Exception as _e:
                logger.exception("QWenEmbed: failed to parse embedding response")
                raise EmbeddingError(f"Embedding request failed for QWenEmbed. Error: {_e}; response={resp}") from _e
        return np.array(res), token_count

    def encode_queries(self, text):
        with _dashscope_native_api_url_scope(self._dashscope_http_api_url):
            resp = dashscope.TextEmbedding.call(model=self.model_name, input=text[:2048], api_key=self.key, text_type="query")
        status_code = resp.status_code
        if status_code != 200:
            raise ModelException(f"Error: status: {status_code}: code: {resp.get('code')}, message: {resp.get('message')}")
            # No need to retry for 4XX error
        try:
            return np.array(resp["output"]["embeddings"][0]["embedding"]), total_token_count_from_response(resp)
        except Exception as _e:
            logger.exception("QWenEmbed: failed to parse query embedding response")
            raise EmbeddingError(f"Embedding request failed for QWenEmbed. Error: {_e}; response={resp}") from _e


class ZhipuEmbed(Base):
    _FACTORY_NAME = "ZHIPU-AI"

    def __init__(self, key, model_name="embedding-2", **kwargs):
        self.client = ZhipuAI(api_key=key)
        self.model_name = model_name

    def _max_len(self):
        # Per-model input ceilings; fall back to the standard 8K limit for any
        # other model rather than leaving oversized inputs untruncated.
        if self.model_name.lower() == "embedding-2":
            return 512
        if self.model_name.lower() == "embedding-3":
            return 3072
        return DEFAULT_MAX_TOKENS

    def _call(self, batch):
        # Batch like the other OpenAI-style providers: one request per batch
        # instead of one request per text. Sort by index so the batched results
        # stay aligned with input order.
        res = self.client.embeddings.create(input=batch, model=self.model_name)
        return [d.embedding for d in _sorted_by_index(res.data)], total_token_count_from_response(res)

    def encode(self, texts: list):
        return self._batched_encode(texts, self._call, batch_size=16, truncate_to=self._max_len())

    def encode_queries(self, text):
        vectors, token_count = self._batched_encode([text], self._call, batch_size=16, truncate_to=self._max_len())
        return vectors[0], token_count


class OllamaEmbed(Base):
    _FACTORY_NAME = "Ollama"

    _special_tokens = ["<|endoftext|>"]

    def __init__(self, key, model_name, **kwargs):
        self.client = Client(host=kwargs["base_url"]) if not key or key == "x" else Client(host=kwargs["base_url"], headers={"Authorization": f"Bearer {key}"})
        self.model_name = model_name
        self.keep_alive = kwargs.get("ollama_keep_alive", int(os.environ.get("OLLAMA_KEEP_ALIVE", -1)))

    @classmethod
    def _strip_special(cls, text: str) -> str:
        for token in cls._special_tokens:
            text = text.replace(token, "")
        return text

    def _call(self, batch):
        # Batch via client.embed (accepts a list `input`) instead of one
        # client.embeddings request per text. `truncate=True` lets Ollama clip
        # oversized inputs to the model's real context length server-side, which
        # is more accurate than a client-side cl100k estimate.
        cleaned = [self._strip_special(t) for t in batch]
        res = self.client.embed(model=self.model_name, input=cleaned, truncate=True, options={"use_mmap": True}, keep_alive=self.keep_alive)
        # Ollama reports real prompt token usage in `prompt_eval_count`; fall
        # back to a local count only if the server omits it (never a fixed 128).
        tokens = res.get("prompt_eval_count") or 0
        if not tokens:
            tokens = sum(num_tokens_from_string(t) for t in cleaned)
        return res["embeddings"], tokens

    def encode(self, texts: list):
        # No client-side truncation: Ollama truncates to the model context above.
        return self._batched_encode(texts, self._call, batch_size=16)

    def encode_queries(self, text):
        vectors, token_count = self._batched_encode([text], self._call, batch_size=16)
        return vectors[0], token_count


class XinferenceEmbed(Base):
    _FACTORY_NAME = "Xinference"

    def __init__(self, key, model_name="", base_url=""):
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name

    def _call(self, batch):
        res = self.client.embeddings.create(input=batch, model=self.model_name)
        return [d.embedding for d in _sorted_by_index(res.data)], total_token_count_from_response(res)

    def encode(self, texts: list):
        return self._batched_encode(texts, self._call, batch_size=16)

    def encode_queries(self, text):
        vectors, token_count = self._batched_encode([text], self._call, batch_size=16)
        return vectors[0], token_count


class YoudaoEmbed(Base):
    _FACTORY_NAME = "Youdao"
    _client = None

    def __init__(self, key=None, model_name="maidalun1020/bce-embedding-base_v1", **kwargs):
        pass

    def encode(self, texts: list):
        batch_size = 10
        res = []
        token_count = 0
        for t in texts:
            token_count += num_tokens_from_string(t)
        for i in range(0, len(texts), batch_size):
            embds = YoudaoEmbed._client.encode(texts[i : i + batch_size])
            res.extend(embds)
        return np.array(res), token_count

    def encode_queries(self, text):
        embds = YoudaoEmbed._client.encode([text])
        return np.array(embds[0]), num_tokens_from_string(text)


class JinaMultiVecEmbed(Base):
    _FACTORY_NAME = "Jina"

    def __init__(self, key, model_name="jina-embeddings-v4", base_url="https://api.jina.ai/v1/embeddings"):
        self.base_url = "https://api.jina.ai/v1/embeddings"
        self.headers = {"Content-Type": "application/json", "Authorization": f"Bearer {key}"}
        self.model_name = model_name

    @staticmethod
    def _as_input_item(text):
        if isinstance(text, str):
            return {"text": text}
        # bytes -> base64 encoded image
        try:
            base64.b64decode(text, validate=True)
            return {"image": text.decode("utf8")}
        except Exception:
            return {"image": base64.b64encode(text).decode("utf8")}

    def encode(self, texts: list[str | bytes], task="retrieval.passage"):
        def _call(batch):
            data = {"model": self.model_name, "input": [self._as_input_item(t) for t in batch]}
            if "v4" in self.model_name:
                data["return_multivector"] = True
            if "v3" in self.model_name or "v4" in self.model_name:
                data["task"] = task
                data["truncate"] = True  # let Jina truncate oversized inputs server-side
            response = requests.post(self.base_url, headers=self.headers, json=data, timeout=30)
            _raise_model_exception_if_failed(response)
            res = response.json()
            embs = []
            for d in res["data"]:
                if data.get("return_multivector", False):  # v4
                    embs.append(np.asarray(d["embeddings"], dtype=np.float32).mean(axis=0))
                else:  # v2/v3
                    embs.append(np.asarray(d["embedding"], dtype=np.float32))
            return embs, total_token_count_from_response(res)

        # Inputs may be image bytes, so token truncation is left to the server.
        return self._batched_encode(texts, _call, batch_size=16)

    def encode_queries(self, text):
        vectors, token_count = self.encode([text], task="retrieval.query")
        return vectors[0], token_count


class MistralEmbed(Base):
    _FACTORY_NAME = "Mistral"

    def __init__(self, key, model_name="mistral-embed", base_url=None):
        from mistralai.client import MistralClient

        self.client = MistralClient(api_key=key)
        self.model_name = model_name

    def encode(self, texts: list):
        import time
        import random

        texts = [truncate(t, DEFAULT_MAX_TOKENS) for t in texts]
        batch_size = 16
        ress = []
        token_count = 0
        for i in range(0, len(texts), batch_size):
            retry_max = 5
            while retry_max > 0:
                try:
                    res = self.client.embeddings(input=texts[i : i + batch_size], model=self.model_name)
                    ress.extend([d.embedding for d in res.data])
                    token_count += total_token_count_from_response(res)
                    break
                except Exception as _e:
                    if retry_max == 1:
                        logger.exception("MistralEmbed: embedding request failed after retries")
                        raise EmbeddingError(f"Embedding request failed for MistralEmbed. Error: {_e}") from _e
                    delay = random.uniform(20, 60)
                    time.sleep(delay)
                    retry_max -= 1
        return np.array(ress), token_count

    def encode_queries(self, text):
        import time
        import random

        retry_max = 5
        while retry_max > 0:
            try:
                res = self.client.embeddings(input=[truncate(text, DEFAULT_MAX_TOKENS)], model=self.model_name)
                return np.array(res.data[0].embedding), total_token_count_from_response(res)
            except Exception as _e:
                if retry_max == 1:
                    logger.exception("MistralEmbed: query embedding request failed after retries")
                    raise EmbeddingError(f"Embedding request failed for MistralEmbed. Error: {_e}") from _e
                delay = random.randint(20, 60)
                time.sleep(delay)
                retry_max -= 1


class BedrockEmbed(Base):
    _FACTORY_NAME = "Bedrock"

    def __init__(self, key, model_name, **kwargs):
        import boto3

        # `key` protocol (backend stores as JSON string in `api_key`):
        # - Must decode into a dict.
        # - Required: `auth_mode`, `bedrock_region`.
        # - Supported auth modes:
        #   - "access_key_secret": requires `bedrock_ak` + `bedrock_sk`.
        #   - "iam_role": requires `aws_role_arn` and assumes role via STS.
        #   - else: treated as "assume_role" (default AWS credential chain).
        key = json.loads(key)
        mode = key.get("auth_mode")
        if not mode:
            logging.error("Bedrock auth_mode is not provided in the key")
            raise ValueError("Bedrock auth_mode must be provided in the key")

        self.bedrock_region = key.get("bedrock_region")

        self.model_name = model_name
        self.is_amazon = self.model_name.split(".")[0] == "amazon"
        self.is_cohere = self.model_name.split(".")[0] == "cohere"

        if mode == "access_key_secret":
            self.bedrock_ak = key.get("bedrock_ak")
            self.bedrock_sk = key.get("bedrock_sk")
            self.client = boto3.client(service_name="bedrock-runtime", region_name=self.bedrock_region, aws_access_key_id=self.bedrock_ak, aws_secret_access_key=self.bedrock_sk)
        elif mode == "iam_role":
            self.aws_role_arn = key.get("aws_role_arn")
            sts_client = boto3.client("sts", region_name=self.bedrock_region)
            resp = sts_client.assume_role(RoleArn=self.aws_role_arn, RoleSessionName="BedrockSession")
            creds = resp["Credentials"]

            self.client = boto3.client(
                service_name="bedrock-runtime",
                aws_access_key_id=creds["AccessKeyId"],
                aws_secret_access_key=creds["SecretAccessKey"],
                aws_session_token=creds["SessionToken"],
            )
        else:  # assume_role
            self.client = boto3.client("bedrock-runtime", region_name=self.bedrock_region)

    def _extract_vector(self, model_response):
        # Titan returns {"embedding": [...]}; Cohere returns {"embeddings": [[...]]}.
        if self.is_cohere:
            return model_response["embeddings"][0]
        return model_response["embedding"]

    def encode(self, texts: list):
        def _call(batch):
            # Titan accepts a single input per call, so batch_size is 1.
            text = batch[0]
            if self.is_amazon:
                body = {"inputText": text}
            elif self.is_cohere:
                body = {"texts": [text], "input_type": "search_document"}
            response = self.client.invoke_model(modelId=self.model_name, body=json.dumps(body))
            model_response = json.loads(response["body"].read())
            # Bedrock does not report token usage; count locally.
            return [self._extract_vector(model_response)], num_tokens_from_string(text)

        return self._batched_encode(texts, _call, batch_size=1, truncate_to=DEFAULT_MAX_TOKENS)

    def encode_queries(self, text):
        text = truncate(text, DEFAULT_MAX_TOKENS)
        token_count = num_tokens_from_string(text)
        if self.is_amazon:
            body = {"inputText": text}
        elif self.is_cohere:
            body = {"texts": [text], "input_type": "search_query"}
        try:
            response = self.client.invoke_model(modelId=self.model_name, body=json.dumps(body))
            model_response = json.loads(response["body"].read())
            return np.array(self._extract_vector(model_response)), token_count
        except Exception as _e:
            logger.exception("BedrockEmbed: query embedding request failed")
            raise EmbeddingError(f"Embedding request failed for BedrockEmbed. Error: {_e}") from _e


class GeminiEmbed(Base):
    _FACTORY_NAME = "Gemini"

    def __init__(self, key, model_name="gemini-embedding-001", **kwargs):
        from google import genai
        from google.genai import types

        self.key = key
        self.model_name = model_name[7:] if model_name.startswith("models/") else model_name
        self.client = genai.Client(api_key=self.key)
        self.types = types

    @staticmethod
    def _parse_embedding_vector(embedding):
        if isinstance(embedding, dict):
            values = embedding.get("values")
            if values is None:
                values = embedding.get("embedding")
            if values is not None:
                return values

        values = getattr(embedding, "values", None)
        if values is None:
            values = getattr(embedding, "embedding", None)
        if values is not None:
            return values

        raise TypeError(f"Unsupported embedding payload: {type(embedding)}")

    @classmethod
    def _parse_embedding_response(cls, response):
        if response is None:
            raise ValueError("Embedding response is empty")

        embeddings = getattr(response, "embeddings", None)
        if embeddings is None and isinstance(response, dict):
            embeddings = response.get("embeddings")

        if embeddings is None:
            return [cls._parse_embedding_vector(response)]

        return [cls._parse_embedding_vector(item) for item in embeddings]

    def _build_embedding_config(self):
        task_type = "RETRIEVAL_DOCUMENT"
        if hasattr(self.types, "TaskType"):
            task_type = getattr(self.types.TaskType, "RETRIEVAL_DOCUMENT", task_type)
        try:
            return self.types.EmbedContentConfig(task_type=task_type, title="Embedding of single string")
        except TypeError:
            # Compatible with SDK versions that do not accept title in embed config.
            return self.types.EmbedContentConfig(task_type=task_type)

    def encode(self, texts: list):
        config = self._build_embedding_config()

        def _call(batch):
            result = self.client.models.embed_content(model=self.model_name, contents=batch, config=config)
            # Gemini embeddings do not report token usage; count locally.
            return self._parse_embedding_response(result), sum(num_tokens_from_string(t) for t in batch)

        return self._batched_encode(texts, _call, batch_size=16, truncate_to=2048)

    def encode_queries(self, text):
        config = self._build_embedding_config()
        token_count = num_tokens_from_string(text)
        try:
            result = self.client.models.embed_content(
                model=self.model_name,
                contents=[truncate(text, 2048)],
                config=config,
            )
            return np.array(self._parse_embedding_response(result)[0]), token_count
        except Exception as _e:
            logger.exception("GeminiEmbed: query embedding request failed")
            raise EmbeddingError(f"Embedding request failed for GeminiEmbed. Error: {_e}") from _e


class NvidiaEmbed(Base):
    _FACTORY_NAME = "NVIDIA"

    def __init__(self, key, model_name, base_url="https://integrate.api.nvidia.com/v1/embeddings"):
        if not base_url:
            base_url = "https://integrate.api.nvidia.com/v1/embeddings"
        self.api_key = key
        self.base_url = base_url
        self.headers = {
            "accept": "application/json",
            "Content-Type": "application/json",
            "authorization": f"Bearer {self.api_key}",
        }
        self.model_name = model_name
        if model_name == "nvidia/embed-qa-4":
            self.base_url = "https://ai.api.nvidia.com/v1/retrieval/nvidia/embeddings"
            self.model_name = "NV-Embed-QA"
        if model_name == "snowflake/arctic-embed-l":
            self.base_url = "https://ai.api.nvidia.com/v1/retrieval/snowflake/arctic-embed-l/embeddings"

    def _call(self, batch, input_type="query"):
        payload = {
            "input": batch,
            "input_type": input_type,
            "model": self.model_name,
            "encoding_format": "float",
            "truncate": "END",  # NVIDIA truncates oversized inputs server-side.
        }
        response = requests.post(self.base_url, headers=self.headers, json=payload, timeout=30)
        return self._openai_http_embeddings(response)

    def encode(self, texts: list):
        # NVIDIA NIM expects "passage" for documents (indexing) and "query" for retrieval.
        return self._batched_encode(texts, lambda b: self._call(b, "passage"), batch_size=16)

    def encode_queries(self, text):
        vectors, token_count = self._batched_encode([text], lambda b: self._call(b, "query"), batch_size=16)
        return vectors[0], token_count


class LmStudioEmbed(LocalAIEmbed):
    _FACTORY_NAME = "LM-Studio"

    def __init__(self, key, model_name, base_url):
        if not base_url:
            raise ValueError("Local llm url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key="lm-studio", base_url=base_url)
        self.model_name = model_name


class OpenAI_APIEmbed(OpenAIEmbed):
    _FACTORY_NAME = ["VLLM", "OpenAI-API-Compatible"]

    def __init__(self, key, model_name, base_url):
        if not base_url:
            raise ValueError("url cannot be None")
        base_url = urljoin(base_url, "v1")
        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name.split("___")[0]


class CoHereEmbed(Base):
    _FACTORY_NAME = "Cohere"

    def __init__(self, key, model_name, base_url=None):
        from cohere import Client

        self.client = Client(api_key=key)
        self.model_name = model_name

    def _call(self, batch):
        res = self.client.embed(
            texts=batch,
            model=self.model_name,
            input_type="search_document",
            embedding_types=["float"],
            truncate="END",  # let Cohere clip oversized inputs server-side instead of hard-failing
        )
        return list(res.embeddings.float), total_token_count_from_response(res)

    def encode(self, texts: list):
        return self._batched_encode(texts, self._call, batch_size=16)

    def encode_queries(self, text):
        try:
            res = self.client.embed(
                texts=[text],
                model=self.model_name,
                input_type="search_query",
                embedding_types=["float"],
                truncate="END",
            )
            return np.array(res.embeddings.float[0]), int(total_token_count_from_response(res))
        except Exception as _e:
            logger.exception("CoHereEmbed: query embedding request failed")
            raise EmbeddingError(f"Embedding request failed for CoHereEmbed. Error: {_e}") from _e


class TogetherAIEmbed(OpenAIEmbed):
    _FACTORY_NAME = "TogetherAI"

    def __init__(self, key, model_name, base_url="https://api.together.xyz/v1"):
        if not base_url:
            base_url = "https://api.together.xyz/v1"
        super().__init__(key, model_name, base_url=base_url)


class PerfXCloudEmbed(OpenAIEmbed):
    _FACTORY_NAME = "PerfXCloud"

    def __init__(self, key, model_name, base_url="https://cloud.perfxlab.cn/v1"):
        if not base_url:
            base_url = "https://cloud.perfxlab.cn/v1"
        super().__init__(key, model_name, base_url)


class UpstageEmbed(OpenAIEmbed):
    _FACTORY_NAME = "Upstage"

    def __init__(self, key, model_name, base_url="https://api.upstage.ai/v1/solar"):
        if not base_url:
            base_url = "https://api.upstage.ai/v1/solar"
        super().__init__(key, model_name, base_url)


class SILICONFLOWEmbed(Base):
    _FACTORY_NAME = "SILICONFLOW"

    def __init__(self, key, model_name, base_url="https://api.siliconflow.cn/v1/embeddings"):
        normalized_base_url = (base_url or "").strip()
        if not normalized_base_url:
            normalized_base_url = "https://api.siliconflow.cn/v1/embeddings"
        if "/embeddings" not in normalized_base_url:
            normalized_base_url = urljoin(f"{normalized_base_url.rstrip('/')}/", "embeddings").rstrip("/")
        self.headers = {
            "accept": "application/json",
            "content-type": "application/json",
            "authorization": f"Bearer {key}",
        }
        self.base_url = normalized_base_url
        self.model_name = model_name

    def _clean_batch(self, batch):
        if self.model_name in ["BAAI/bge-large-zh-v1.5", "BAAI/bge-large-en-v1.5"]:
            # limit 512, 340 is almost safe
            return [" " if not text.strip() else truncate(text, 256) for text in batch]
        return [" " if not text.strip() else text for text in batch]

    def _call(self, batch):
        payload = {
            "model": self.model_name,
            "input": self._clean_batch(batch),
            "encoding_format": "float",
        }
        response = requests.post(self.base_url, json=payload, headers=self.headers, timeout=30)
        return self._openai_http_embeddings(response)

    def encode(self, texts: list):
        return self._batched_encode(texts, self._call, batch_size=16)

    def encode_queries(self, text):
        vectors, token_count = self._batched_encode([text], self._call, batch_size=16)
        return vectors[0], token_count


class ReplicateEmbed(Base):
    _FACTORY_NAME = "Replicate"

    def __init__(self, key, model_name, base_url=None):
        from replicate.client import Client

        self.model_name = model_name
        self.client = Client(api_token=_normalize_replicate_key(key))

    def encode(self, texts: list):
        batch_size = 16
        token_count = sum([num_tokens_from_string(text) for text in texts])
        ress = []
        for i in range(0, len(texts), batch_size):
            res = self.client.run(self.model_name, input={"texts": texts[i : i + batch_size]})
            ress.extend(res)
        return np.array(ress), token_count

    def encode_queries(self, text):
        res = self.client.embed(self.model_name, input={"texts": [text]})
        return np.array(res), num_tokens_from_string(text)


class BaiduYiyanEmbed(Base):
    _FACTORY_NAME = "BaiduYiyan"

    def __init__(self, key, model_name, base_url=None):
        import qianfan

        key = json.loads(key)
        ak = key.get("yiyan_ak", "")
        sk = key.get("yiyan_sk", "")
        self.client = qianfan.Embedding(ak=ak, sk=sk)
        self.model_name = model_name

    def encode(self, texts: list, batch_size=16):
        try:
            res = self.client.do(model=self.model_name, texts=texts).body
            return (
                np.array([r["embedding"] for r in res["data"]]),
                total_token_count_from_response(res),
            )
        except Exception as _e:
            logger.exception("BaiduYiyanEmbed: embedding request failed")
            raise EmbeddingError(f"Embedding request failed for BaiduYiyanEmbed. Error: {_e}") from _e

    def encode_queries(self, text):
        try:
            res = self.client.do(model=self.model_name, texts=[text]).body
            return (
                np.array([r["embedding"] for r in res["data"]]),
                total_token_count_from_response(res),
            )
        except Exception as _e:
            logger.exception("BaiduYiyanEmbed: query embedding request failed")
            raise EmbeddingError(f"Embedding request failed for BaiduYiyanEmbed. Error: {_e}") from _e


class VoyageEmbed(Base):
    _FACTORY_NAME = "Voyage AI"

    def __init__(self, key, model_name, base_url=None):
        import voyageai

        self.client = voyageai.Client(api_key=key)
        self.model_name = model_name

    def _call(self, batch):
        res = self.client.embed(texts=batch, model=self.model_name, input_type="document")
        # `_batched_encode` accumulates these per-batch vectors and returns a
        # single np.ndarray, so encode() keeps the np.ndarray contract.
        return res.embeddings, res.total_tokens

    def encode(self, texts: list):
        return self._batched_encode(texts, self._call, batch_size=16)

    def encode_queries(self, text):
        try:
            res = self.client.embed(texts=text, model=self.model_name, input_type="query")
            return np.array(res.embeddings)[0], res.total_tokens
        except Exception as _e:
            logger.exception("VoyageEmbed: query embedding request failed")
            raise EmbeddingError(f"Embedding request failed for VoyageEmbed. Error: {_e}") from _e


class HuggingFaceEmbed(Base):
    _FACTORY_NAME = "HuggingFace"

    def __init__(self, key, model_name, base_url=None, **kwargs):
        if not model_name:
            raise ValueError("Model name cannot be None")
        self.key = key
        self.model_name = model_name.split("___")[0]
        self.base_url = base_url or "http://127.0.0.1:8080"

    def encode(self, texts: list):
        response = requests.post(f"{self.base_url}/embed", json={"inputs": texts}, headers={"Content-Type": "application/json"}, timeout=30)
        _raise_model_exception_if_failed(response)
        # TEI auto-truncates oversized inputs, so no client-side truncation is needed.
        return np.array(response.json()), sum([num_tokens_from_string(text) for text in texts])

    def encode_queries(self, text: str):
        response = requests.post(f"{self.base_url}/embed", json={"inputs": text}, headers={"Content-Type": "application/json"}, timeout=30)
        _raise_model_exception_if_failed(response)
        return np.array(response.json()[0]), num_tokens_from_string(text)


class VolcEngineEmbed(Base):
    _FACTORY_NAME = "VolcEngine"

    def __init__(self, key, model_name, base_url="https://ark.cn-beijing.volces.com/api/v3"):
        if not base_url:
            base_url = "https://ark.cn-beijing.volces.com/api/v3"
        self.base_url = base_url

        try:
            cfg = json.loads(key)
            self.ark_api_key = cfg.get("ark_api_key", "")
        except JSONDecodeError:
            self.ark_api_key = key
        self.model_name = model_name

    @staticmethod
    def _extract_embedding(result: dict) -> list[float]:
        if not isinstance(result, dict):
            raise TypeError(f"Unexpected response type: {type(result)}")

        data = result.get("data")
        if data is None:
            raise KeyError("Missing 'data' in response")

        if isinstance(data, list):
            if not data:
                raise ValueError("Empty 'data' in response")
            item = data[0]
        elif isinstance(data, dict):
            item = data
        else:
            raise TypeError(f"Unexpected 'data' type: {type(data)}")

        if not isinstance(item, dict):
            raise TypeError("Unexpected item shape in 'data'")
        if "embedding" not in item:
            raise KeyError("Missing 'embedding' in response item")
        return item["embedding"]

    def _encode_texts(self, texts: list[str]):
        from common.http_client import sync_request

        url = f"{self.base_url}/embeddings/multimodal"
        headers = {"Content-Type": "application/json", "Authorization": f"Bearer {self.ark_api_key}"}

        ress: list[list[float]] = []
        total_tokens = 0
        for text in texts:
            request_body = {"model": self.model_name, "input": [{"type": "text", "text": text}]}
            response = sync_request(method="POST", url=url, headers=headers, json=request_body, timeout=60)
            if response.status_code != 200:
                raise EmbeddingError(f"Embedding request failed for VolcEngineEmbed. Error: {response.status_code} - {response.text}")
            result = response.json()
            try:
                ress.append(self._extract_embedding(result))
                total_tokens += total_token_count_from_response(result)
            except Exception as _e:
                logger.exception("VolcEngineEmbed: failed to parse embedding response")
                raise EmbeddingError(f"Embedding request failed for VolcEngineEmbed. Error: {_e}; response={result}") from _e

        return np.array(ress), total_tokens

    def encode(self, texts: list):
        return self._encode_texts(texts)

    def encode_queries(self, text: str):
        embeddings, tokens = self._encode_texts([text])
        return embeddings[0], tokens


class GPUStackEmbed(OpenAIEmbed):
    _FACTORY_NAME = "GPUStack"

    def __init__(self, key, model_name, base_url):
        if not base_url:
            raise ValueError("url cannot be None")
        base_url = urljoin(base_url, "v1")

        self.client = OpenAI(api_key=key, base_url=base_url)
        self.model_name = model_name


class NovitaEmbed(SILICONFLOWEmbed):
    _FACTORY_NAME = "NovitaAI"

    def __init__(self, key, model_name, base_url="https://api.novita.ai/v3/openai/embeddings"):
        if not base_url:
            base_url = "https://api.novita.ai/v3/openai/embeddings"
        super().__init__(key, model_name, base_url)


class GiteeEmbed(SILICONFLOWEmbed):
    _FACTORY_NAME = "GiteeAI"

    def __init__(self, key, model_name, base_url="https://ai.gitee.com/v1/embeddings"):
        if not base_url:
            base_url = "https://ai.gitee.com/v1/embeddings"
        super().__init__(key, model_name, base_url)


class DeepInfraEmbed(OpenAIEmbed):
    _FACTORY_NAME = "DeepInfra"

    def __init__(self, key, model_name, base_url="https://api.deepinfra.com/v1/openai"):
        if not base_url:
            base_url = "https://api.deepinfra.com/v1/openai"
        super().__init__(key, model_name, base_url)


class Ai302Embed(Base):
    _FACTORY_NAME = "302.AI"

    def __init__(self, key, model_name, base_url="https://api.302.ai/v1/embeddings"):
        if not base_url:
            base_url = "https://api.302.ai/v1/embeddings"
        super().__init__(key, model_name, base_url)


class CometAPIEmbed(OpenAIEmbed):
    _FACTORY_NAME = "CometAPI"

    def __init__(self, key, model_name, base_url="https://api.cometapi.com/v1"):
        if not base_url:
            base_url = "https://api.cometapi.com/v1"
        super().__init__(key, model_name, base_url)


class DeerAPIEmbed(OpenAIEmbed):
    _FACTORY_NAME = "DeerAPI"

    def __init__(self, key, model_name, base_url="https://api.deerapi.com/v1"):
        if not base_url:
            base_url = "https://api.deerapi.com/v1"
        super().__init__(key, model_name, base_url)


class JiekouAIEmbed(OpenAIEmbed):
    _FACTORY_NAME = "Jiekou.AI"

    def __init__(self, key, model_name, base_url="https://api.jiekou.ai/openai/v1/embeddings"):
        if not base_url:
            base_url = "https://api.jiekou.ai/openai/v1/embeddings"
        super().__init__(key, model_name, base_url)


class RAGconEmbed(OpenAIEmbed):
    """
    RAGcon Embedding Provider - routes through LiteLLM proxy

    Default Base URL: https://connect.ragcon.ai/v1
    """

    _FACTORY_NAME = "RAGcon"

    def __init__(self, key, model_name="text-embedding-3-small", base_url=None):
        if not base_url:
            base_url = "https://connect.ragcon.com/v1"

        super().__init__(key, model_name, base_url)


class PerplexityEmbed(Base):
    _FACTORY_NAME = "Perplexity"

    def __init__(self, key, model_name="pplx-embed-v1-0.6b", base_url="https://api.perplexity.ai"):
        if not base_url:
            base_url = "https://api.perplexity.ai"
        self.base_url = base_url.rstrip("/")
        self.api_key = key
        self.model_name = model_name
        self.headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {self.api_key}",
        }

    @staticmethod
    def _decode_base64_int8(b64_str):
        raw = base64.b64decode(b64_str)
        return np.frombuffer(raw, dtype=np.int8).astype(np.float32)

    def _is_contextualized(self):
        return "context" in self.model_name

    def encode(self, texts: list):
        batch_size = 512
        ress = []
        token_count = 0

        if self._is_contextualized():
            url = f"{self.base_url}/v1/contextualizedembeddings"
            for i in range(0, len(texts), batch_size):
                batch = texts[i : i + batch_size]
                payload = {
                    "model": self.model_name,
                    "input": [[chunk] for chunk in batch],
                    "encoding_format": "base64_int8",
                }
                response = requests.post(url, headers=self.headers, json=payload, timeout=30)
                _raise_model_exception_if_failed(response)
                try:
                    res = response.json()
                    for doc in res["data"]:
                        for chunk_emb in doc["data"]:
                            ress.append(self._decode_base64_int8(chunk_emb["embedding"]))
                    token_count += res.get("usage", {}).get("total_tokens", 0)
                except Exception as _e:
                    logger.exception("PerplexityEmbed: failed to parse contextualized embedding response")
                    raise EmbeddingError(f"Embedding request failed for PerplexityEmbed. Error: {response.text}") from _e
        else:
            url = f"{self.base_url}/v1/embeddings"
            for i in range(0, len(texts), batch_size):
                batch = texts[i : i + batch_size]
                payload = {
                    "model": self.model_name,
                    "input": batch,
                    "encoding_format": "base64_int8",
                }
                response = requests.post(url, headers=self.headers, json=payload, timeout=30)
                _raise_model_exception_if_failed(response)
                try:
                    res = response.json()
                    for d in res["data"]:
                        ress.append(self._decode_base64_int8(d["embedding"]))
                    token_count += res.get("usage", {}).get("total_tokens", 0)
                except Exception as _e:
                    logger.exception("PerplexityEmbed: failed to parse embedding response")
                    raise EmbeddingError(f"Embedding request failed for PerplexityEmbed. Error: {response.text}") from _e

        return np.array(ress), token_count

    def encode_queries(self, text):
        embds, cnt = self.encode([text])
        return np.array(embds[0]), cnt
