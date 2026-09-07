#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
import json
import logging
from abc import ABC
from datetime import datetime, timezone
from json.decoder import JSONDecodeError
from typing import ClassVar
from urllib.parse import urlparse

import aiohttp
from botocore import UNSIGNED
from botocore.awsrequest import AWSRequest
from botocore.config import Config
from botocore.exceptions import BotoCoreError, ClientError
from botocore.utils import validate_region_name

from common.aimlapi_utils import attribution_headers
from common.constants import LLMType
from rag.llm.mws_utils import mws_api_url, normalize_mws_project_url, require_mws_token


class Base(ABC):
    def __init__(self, api_key: str, base_url: str = None):
        self.api_key = api_key
        self.base_url = base_url

    def _get_api_key(self):
        # Shared JSON-decode resolution for every model-list verify path in
        # this file. Mirrors the per-site pattern in VolcEngine (line 68),
        # OpenRouter (line 365), NewAPI (line 928), LocalAI (line 221), and
        # Ollama (line 125) -- but in the base class so the 6 providers that
        # do not override this method (Xinference, HuggingFace, FunASR,
        # NVIDIA, GreenPT, VLLM, LMStudio, RAGcon, AIMLAPI) get the same
        # wire-correct behavior for free.
        #
        # When the API service stores the api_key as a JSON dict -- the
        # format used by ``api/apps/services/provider_api_service.py:313``
        # which calls ``json.dumps(api_key)`` for non-string values -- the
        # raw ``self.api_key`` would render as
        # ``Authorization: Bearer {"api_key": "sk-xxx", ...}``, which
        # real-provider endpoints 401. Resolve the same way the per-site
        # overrides do:
        #   * JSON dict with an ``api_key`` field  -> the inner key.
        #   * JSON dict without ``api_key``        -> ``""`` so the resolved
        #     value is falsy and any ``if resolved_key:`` gate in the
        #     subclass's ``get_model_list`` skips the Authorization header
        #     entirely. For real-provider subclasses (NVIDIA, GreenPT,
        #     AIMLAPI) this is no better than the pre-fix malformed Bearer --
        #     they 401 either way -- but the LocalAI/Ollama subclass pattern
        #     of "no-auth when api_key is absent" still works.
        #   * Plain string (the common case)  -> returned as-is.
        #   * JSON parse error, JSON non-object, or non-string api_key  ->
        #     fall back to the pre-fix raw passthrough so we do not
        #     regress any caller that depends on the historical behavior.
        if not self.api_key:
            return ""
        try:
            parsed = json.loads(self.api_key)
        except (JSONDecodeError, TypeError, ValueError):
            return self.api_key
        if isinstance(parsed, dict):
            return parsed.get("api_key", "") if "api_key" in parsed else ""
        return self.api_key

    def _get_model_list_url(self):
        if not self.base_url:
            return None
        if "/v1" in self.base_url:
            return self.base_url.split("/v1")[0].rstrip("/") + "/v1/models"
        return self.base_url.rstrip("/") + "/v1/models"

    async def _get_raw_model_list(self):
        url = self._get_model_list_url()
        if not url:
            return None
        async with aiohttp.ClientSession() as session:
            async with session.get(url, headers={"Authorization": f"Bearer {self._get_api_key()}"}) as resp:
                if resp.status != 200:
                    return None
                return await resp.json()

    def _format_model_list(self, raw_model_list):
        return raw_model_list

    async def get_model_list(self):
        raw_model_list = await self._get_raw_model_list()
        if not raw_model_list:
            return []
        return self._format_model_list(raw_model_list)


class Bedrock(Base):
    _FACTORY_NAME = "Bedrock"

    def _get_config(self) -> tuple[str, str]:
        config = self.api_key
        if isinstance(config, str):
            try:
                config = json.loads(config)
            except JSONDecodeError as error:
                raise ValueError("Bedrock credentials must be a JSON object") from error
        if not isinstance(config, dict) or config.get("auth_mode") != "bedrock_api_key":
            raise ValueError("Bedrock API key authentication is required to list models")

        api_key = config.get("bedrock_api_key")
        if not isinstance(api_key, str) or not api_key.strip():
            raise ValueError("Bedrock API key must be provided")
        region = config.get("bedrock_region")
        if not isinstance(region, str) or not region.strip():
            raise ValueError("Bedrock region must be provided in the key")
        validate_region_name(region)
        return api_key.strip(), region

    def _list_foundation_models(self, api_key: str, region: str) -> dict[str, object]:
        import boto3

        client = boto3.client(
            service_name="bedrock",
            region_name=region,
            config=Config(
                signature_version=UNSIGNED,
                connect_timeout=10,
                read_timeout=10,
                retries={"mode": "standard", "max_attempts": 0},
            ),
        )

        def add_bearer_token(request: AWSRequest, **_kwargs: object) -> None:
            request.headers["Authorization"] = f"Bearer {api_key}"

        client.meta.events.register("before-sign.bedrock.*", add_bearer_token)
        return client.list_foundation_models(byInferenceType="ON_DEMAND")

    def _format_model_list(self, raw_model_list: dict[str, object]) -> list[dict[str, object]]:
        models: list[dict[str, object]] = []
        for summary in raw_model_list.get("modelSummaries", []):
            if not isinstance(summary, dict):
                continue
            model_id = summary.get("modelId")
            input_modalities = summary.get("inputModalities", [])
            output_modalities = summary.get("outputModalities", [])
            inference_types = summary.get("inferenceTypesSupported", [])
            lifecycle_status = (summary.get("modelLifecycle") or {}).get("status")
            if not model_id or "TEXT" not in input_modalities or "rerank" in model_id.lower():
                continue
            if inference_types and "ON_DEMAND" not in inference_types:
                continue
            if lifecycle_status and lifecycle_status != "ACTIVE":
                continue
            if "EMBEDDING" in output_modalities and model_id.startswith(("amazon.titan-embed-text", "cohere.embed-")):
                model_types = [LLMType.EMBEDDING.value]
            elif "TEXT" in output_modalities:
                model_types = [LLMType.CHAT.value]
                if "IMAGE" in input_modalities:
                    model_types.append(LLMType.VISION.value)
            else:
                continue
            models.append({"name": model_id, "model_types": model_types, "max_tokens": 8192, "features": []})
        return models

    async def get_model_list(self) -> list[dict[str, object]]:
        api_key, region = self._get_config()
        try:
            raw_model_list = await asyncio.to_thread(self._list_foundation_models, api_key, region)
        except ClientError as error:
            raise ValueError(f"Failed to list models from Amazon Bedrock in region '{region}'") from error
        except BotoCoreError as error:
            raise ValueError("Failed to list models from Amazon Bedrock") from error
        return self._format_model_list(raw_model_list)


class VolcEngine(Base):
    _FACTORY_NAME = "VolcEngine"

    def _get_api_key(self):
        try:
            api_key = json.loads(self.api_key).get("ark_api_key", "")
        except JSONDecodeError:
            api_key = self.api_key
        return api_key

    def _get_model_list_url(self):
        if not self.base_url:
            self.base_url = "https://ark.cn-beijing.volces.com/api/v3"
        parsed = urlparse(self.base_url)
        return f"{parsed.scheme}://{parsed.netloc}/api/v3/models"

    def _format_model_list(self, raw_model_list):
        serving_model = [model for model in raw_model_list["data"] if model.get("status", "") != "Shutdown"]
        res = []
        for model in serving_model:
            model_types = []

            if model.get("domain", "") == "Embedding":
                model_types.append(LLMType.EMBEDDING.value)
            elif set(model.get("task_type", [])) & {"TextEmbedding", "ImageEmbedding"}:
                model_types.append(LLMType.EMBEDDING.value)
            else:
                modalities = model.get("modalities", {})
                input_modalities = modalities.get("input_modalities", [])
                output_modalities = modalities.get("output_modalities", [])

                if "text" in output_modalities:
                    model_types.append(LLMType.CHAT.value)
                if "embeddings" in output_modalities:
                    model_types.append(LLMType.EMBEDDING.value)
                if "image" in input_modalities and "text" in output_modalities:
                    model_types.append(LLMType.VISION.value)
                if "audio" in input_modalities and "text" in output_modalities:
                    model_types.append(LLMType.ASR.value)
                if "audio" in output_modalities:
                    model_types.append(LLMType.TTS.value)

            if not model_types:
                continue

            features = []
            if model.get("features", {}).get("tools", {}).get("function_calling", False):
                features.append("is_tools")
            if model.get("token_limits", {}).get("max_reasoning_token_length", 0) > 0:
                features.append("thinking")

            res.append(
                {"name": model["id"], "model_types": model_types, "features": features, "max_tokens": model.get("token_limits", {}).get("max_input_token_length", 8192), "status": model.get("status")}
            )
        return res


class Ollama(Base):
    _FACTORY_NAME = "Ollama"

    def _get_api_key(self):
        # Ollama typically does not require auth. The model-list verify path
        # in get_model_list() only sends an Authorization header when the
        # resolved key is truthy (see ``if resolved_key:`` in get_model_list).
        # The base default returns self.api_key verbatim, which breaks when
        # the API service stores the key as a JSON dict for consistency with
        # other providers -- the Bearer header would be malformed as
        # ``Authorization: Bearer {"api_key": "sk-xxx", ...}``. Ollama does
        # not validate the token and accepts the malformed Bearer, but
        # downstream Ollama setups that do validate (e.g. behind an
        # authenticating reverse proxy) would reject it.
        #
        # Resolve to a plain string the same way LocalAI / VolcEngine /
        # OpenRouter / NewAPI do for the model-list path:
        #   * JSON dict with an "api_key" field -> the inner key.
        #   * JSON dict without "api_key" -> "" so the no-auth path is kept
        #     (Ollama's normal case; the verify endpoint will then call
        #     /api/tags without an Authorization header, which Ollama
        #     accepts by default).
        #   * Plain string (the common case) -> returned as-is.
        #   * JSON parse error, JSON non-object, or non-string api_key ->
        #     fall back to the base default to avoid regressing any caller
        #     that depends on the historical raw passthrough.
        if not self.api_key:
            return ""
        try:
            parsed = json.loads(self.api_key)
        except (JSONDecodeError, TypeError, ValueError):
            return self.api_key
        if isinstance(parsed, dict):
            return parsed.get("api_key", "") if "api_key" in parsed else ""
        return self.api_key

    def _get_model_tags_url(self):
        return self.base_url.rstrip("/") + "/api/tags"

    def _get_model_detail_url(self):
        return self.base_url.rstrip("/") + "/api/show"

    async def get_model_list(self):
        if not self.base_url:
            return []
        headers = {}
        # Use the resolved key (not raw self.api_key) so a JSON dict that
        # has no inner ``api_key`` field resolves to ``""`` and the no-auth
        # path is taken -- matches Ollama's normal case where no Authorization
        # header is needed. Pre-fix this check used raw self.api_key, so a
        # JSON-dict value (truthy) added a malformed ``Bearer {"api_key": ...}``
        # header.
        resolved_key = self._get_api_key()
        if resolved_key:
            headers.update({"Authorization": f"Bearer {resolved_key}"})
        async with aiohttp.ClientSession() as session:
            async with session.get(self._get_model_tags_url(), headers=headers) as resp:
                if resp.status != 200:
                    return []
                tags = await resp.json()
                models = tags.get("models", [])
                if not models:
                    return []
            res = []
            capability_to_model_type_mapping = {"completion": LLMType.CHAT.value, "vision": LLMType.VISION.value, "embedding": LLMType.EMBEDDING.value}
            capability_to_feature_mapping = {"thinking": "thinking", "tools": "is_tools"}

            for model in models:
                async with session.post(self._get_model_detail_url(), headers=headers, json={"model": model["name"]}) as resp:
                    if resp.status != 200:
                        continue
                    model_info = await resp.json()
                    max_tokens_key = "{}.context_length".format(model_info.get("details", {}).get("family", ""))
                    res.append(
                        {
                            "name": model["name"],
                            "model_types": [capability_to_model_type_mapping[c] for c in model_info.get("capabilities", []) if c in capability_to_model_type_mapping],
                            "features": [capability_to_feature_mapping[c] for c in model_info.get("capabilities", []) if c in capability_to_feature_mapping],
                            "max_tokens": model_info["model_info"].get(max_tokens_key, 8192),
                        }
                    )
        return res


class Xinference(Base):
    _FACTORY_NAME = "Xinference"

    def _get_model_list_url(self):
        if not self.base_url:
            return None
        return self.base_url.rstrip("/") + "/v1/models"

    @staticmethod
    def _xinference_model_type_to_llm_type(model_type_str):
        """Map Xinference model type strings to RAGFlow LLMType values."""
        mapping = {
            "LLM": LLMType.CHAT.value,
            "chat": LLMType.CHAT.value,
            "embedding": LLMType.EMBEDDING.value,
            "rerank": LLMType.RERANK.value,
            "image": LLMType.VISION.value,
            "TTS": LLMType.TTS.value,
            "asr": LLMType.ASR.value,
        }
        return mapping.get(model_type_str, LLMType.CHAT.value)

    def _format_model_list(self, raw_model_list):
        """Xinference /v1/models returns model_type and context_length in addition to OpenAI-standard fields."""
        data = raw_model_list.get("data", [])
        if not data:
            return []
        res = []
        for model in data:
            model_id = model.get("id")
            if not model_id:
                continue
            model_type_str = model.get("model_type", "")
            model_type = self._xinference_model_type_to_llm_type(model_type_str) if model_type_str else LLMType.CHAT.value
            max_tokens = model.get("context_length") or model.get("max_tokens") or 8192
            res.append(
                {
                    "name": model_id,
                    "model_types": [model_type],
                    "features": None,
                    "max_tokens": max_tokens,
                }
            )
        return res


class LocalAI(Base):
    """LocalAI exposes Ollama-compatible /api/tags and /api/show endpoints.

    ``GET /api/tags`` returns model list with capabilities (completion, embedding, vision, tools, thinking).
    ``POST /api/show`` returns ``model_info`` containing ``general.context_length``.
    """

    _FACTORY_NAME = "LocalAI"

    def _get_api_key(self):
        # LocalAI typically does not require auth. The model-list verify path
        # in get_model_list() only sends an Authorization header when the
        # resolved key is truthy (see ``if resolved_key:`` in get_model_list).
        # The base default returns self.api_key verbatim, which breaks when
        # the API service stores the key as a JSON dict for consistency with
        # other providers -- the Bearer header would be malformed as
        # ``Authorization: Bearer {"api_key": "sk-xxx", ...}`` and the
        # LocalAI server would 401, surfacing as
        # "102 no models found for provider LocalAI" (issue #17757).
        #
        # Resolve to a plain string the same way VolcEngine / OpenRouter /
        # NewAPI do for the model-list path:
        #   * JSON dict with an "api_key" field -> the inner key.
        #   * JSON dict without "api_key" -> "" so the no-auth path is kept
        #     (LocalAI's normal case; the verify endpoint will then call
        #     /api/tags without an Authorization header, which LocalAI
        #     accepts by default).
        #   * Plain string (the common case) -> returned as-is.
        #   * JSON parse error, JSON non-object, or non-string api_key ->
        #     fall back to the base default to avoid regressing any caller
        #     that depends on the historical raw passthrough.
        if not self.api_key:
            return ""
        try:
            parsed = json.loads(self.api_key)
        except (JSONDecodeError, TypeError, ValueError):
            return self.api_key
        if isinstance(parsed, dict):
            return parsed.get("api_key", "") if "api_key" in parsed else ""
        return self.api_key

    def _get_model_tags_url(self):
        return self.base_url.rstrip("/") + "/api/tags"

    def _get_model_detail_url(self):
        return self.base_url.rstrip("/") + "/api/show"

    async def get_model_list(self):
        if not self.base_url:
            return []
        headers = {}
        # Use the resolved key (not raw self.api_key) so a JSON dict that
        # has no inner ``api_key`` field resolves to ``""`` and the no-auth
        # path is taken -- matches LocalAI's normal case where no Authorization
        # header is needed. Pre-fix this check used raw self.api_key, so a
        # JSON-dict value (truthy) added a malformed ``Bearer {"api_key": ...}``
        # header and the LocalAI server 401'd, surfacing as
        # "102 no models found for provider LocalAI" (issue #17757).
        resolved_key = self._get_api_key()
        if resolved_key:
            headers.update({"Authorization": f"Bearer {resolved_key}"})
        async with aiohttp.ClientSession() as session:
            async with session.get(self._get_model_tags_url(), headers=headers) as resp:
                if resp.status != 200:
                    return []
                tags = await resp.json()
                models = tags.get("models", [])
                if not models:
                    return []
            res = []
            capability_to_model_type_mapping = {
                "completion": LLMType.CHAT.value,
                "vision": LLMType.VISION.value,
                "embedding": LLMType.EMBEDDING.value,
            }
            capability_to_feature_mapping = {
                "thinking": "thinking",
                "tools": "is_tools",
            }

            for model in models:
                async with session.post(
                    self._get_model_detail_url(),
                    headers=headers,
                    json={"model": model["name"]},
                ) as resp:
                    if resp.status != 200:
                        continue
                    model_info = await resp.json()
                    context_length = model_info.get("model_info", {}).get("general.context_length", 8192)
                    res.append(
                        {
                            "name": model["name"].rsplit(":", 1)[0],
                            "model_types": [capability_to_model_type_mapping[c] for c in model_info.get("capabilities", []) if c in capability_to_model_type_mapping],
                            "features": [capability_to_feature_mapping[c] for c in model_info.get("capabilities", []) if c in capability_to_feature_mapping],
                            "max_tokens": context_length or 8192,
                        }
                    )
        return res


class BaiduYiyan(Base):
    _FACTORY_NAME = "BaiduYiyan"

    async def get_model_list(self):
        """BaiduYiyan uses the Qianfan SDK which provides static model catalogs.

        The ``models()`` class method returns all supported model names
        without requiring AK/SK credentials.
        ``get_model_info()`` returns ``max_input_tokens`` for each model.
        """
        import qianfan

        res = []
        real = qianfan.ChatCompletion._real_base("1")
        chat_models = real.models()
        for name in chat_models:
            max_tokens = 8192
            try:
                info = real.get_model_info(name)
                if info.max_input_tokens:
                    max_tokens = info.max_input_tokens
            except Exception:
                pass
            res.append(
                {
                    "name": name,
                    "model_types": [LLMType.CHAT.value],
                    "features": None,
                    "max_tokens": max_tokens,
                }
            )

        try:
            embed_models = qianfan.Embedding.models()
            for name in embed_models:
                res.append(
                    {
                        "name": name,
                        "model_types": [LLMType.EMBEDDING.value],
                        "features": None,
                        "max_tokens": 8192,
                    }
                )
        except Exception:
            pass

        return res


class OpenRouter(Base):
    _FACTORY_NAME = "OpenRouter"

    def _get_api_key(self):
        api_key = self.api_key
        if not api_key:
            return ""
        try:
            payload = json.loads(api_key)
        except Exception:
            return api_key
        if isinstance(payload, dict):
            return payload.get("api_key") or api_key
        return api_key

    def _get_model_list_url(self):
        tail = "/api/v1/models?output_modalities=all"
        if not self.base_url:
            return "https://openrouter.ai" + tail
        base_url = self.base_url.rstrip("/")
        if "/api/v1" in base_url:
            return base_url.split("/api/v1")[0].rstrip("/") + tail
        if "/v1" in base_url:
            return base_url.split("/v1")[0].rstrip("/") + tail
        return base_url + tail

    def _format_model_list(self, raw_model_list):
        models = raw_model_list.get("data") if isinstance(raw_model_list, dict) else raw_model_list
        if not isinstance(models, list):
            return []

        model_list = []
        for model in models:
            if not isinstance(model, dict):
                continue

            model_name = model.get("id") or model.get("name") or model.get("canonical_slug")
            if not model_name:
                continue

            architecture = model.get("architecture") or {}
            input_modalities = set(architecture.get("input_modalities") or [])
            output_modalities = set(architecture.get("output_modalities") or [])
            supported_parameters = set(model.get("supported_parameters") or [])

            model_types = []
            if "text" in output_modalities:
                model_types.append(LLMType.CHAT.value)
            if "embeddings" in output_modalities:
                model_types.append(LLMType.EMBEDDING.value)
            if "image" in input_modalities and "text" in output_modalities:
                model_types.append(LLMType.VISION.value)
            if "audio" in input_modalities and "text" in output_modalities:
                model_types.append(LLMType.ASR.value)
            if "audio" in output_modalities:
                model_types.append(LLMType.TTS.value)

            features = []
            if "tools" in supported_parameters:
                features.append("is_tools")
            if supported_parameters & {"reasoning", "include_reasoning"}:
                features.append("thinking")

            max_tokens = (model.get("top_provider") or {}).get("max_completion_tokens") or model.get("context_length") or (model.get("top_provider") or {}).get("context_length") or 8192

            model_list.append(
                {
                    "name": model_name,
                    "model_types": list(dict.fromkeys(model_types)),
                    "features": features,
                    "max_tokens": max_tokens,
                }
            )

        return model_list


class OpenAIAPICompatible(Base):
    _FACTORY_NAME = "OpenAI-API-Compatible"

    _EMBEDDING_HINTS = ("embed", "embedding", "bge")
    _RERANK_HINTS = ("rerank", "reranker")
    _ASR_HINTS = ("asr", "stt", "transcribe", "transcriber", "whisper")
    _TTS_HINTS = ("tts", "text-to-speech")
    _VISION_HINTS = (
        "vl",
        "vision",
        "llava",
        "internvl",
        "minicpm-v",
        "gpt-4o",
        "glm-4v",
        "qvq",
        "qwen-vl",
        "pixtral",
    )

    @classmethod
    def _contains_hint(cls, model_name, hints):
        return any(hint in model_name for hint in hints)

    @classmethod
    def _infer_model_types(cls, model_name):
        if cls._contains_hint(model_name, cls._RERANK_HINTS):
            return [LLMType.RERANK.value]
        if cls._contains_hint(model_name, cls._EMBEDDING_HINTS):
            return [LLMType.EMBEDDING.value]
        if cls._contains_hint(model_name, cls._ASR_HINTS):
            return [LLMType.ASR.value]
        if cls._contains_hint(model_name, cls._TTS_HINTS):
            return [LLMType.TTS.value]

        model_types = [LLMType.CHAT.value]
        if cls._contains_hint(model_name, cls._VISION_HINTS):
            model_types.append(LLMType.VISION.value)
        return model_types

    def _format_model_list(self, raw_model_list):
        models = raw_model_list.get("data") if isinstance(raw_model_list, dict) else raw_model_list
        if not isinstance(models, list):
            return []

        model_list = []
        for model in models:
            if not isinstance(model, dict):
                continue

            model_name = model.get("id") or model.get("name")
            if not model_name:
                continue

            model_name_lower = model_name.lower()
            model_list.append(
                {
                    "name": model_name,
                    "model_types": self._infer_model_types(model_name_lower),
                    "features": [],
                    "max_tokens": (model.get("max_tokens") or model.get("max_completion_tokens") or model.get("context_length") or model.get("max_model_len") or 8192),
                }
            )

        return model_list


class MWS(OpenAIAPICompatible):
    """Discover supported MWS deployments through the project models API."""

    _FACTORY_NAME = "MWS"

    def __init__(self, api_key: str, base_url: str = None):
        """Initialize dynamic model discovery for an MWS project."""
        try:
            token = require_mws_token(api_key)
        except ValueError as error:
            logging.warning(
                "mws_model_discovery_validation_failed",
                extra={
                    "provider": self._FACTORY_NAME,
                    "operation": "model_discovery",
                    "validation_target": "token",
                    "error_type": type(error).__name__,
                },
            )
            raise

        try:
            project_url = normalize_mws_project_url(base_url)
        except ValueError as error:
            logging.warning(
                "mws_model_discovery_validation_failed",
                extra={
                    "provider": self._FACTORY_NAME,
                    "operation": "model_discovery",
                    "validation_target": "api_url",
                    "error_type": type(error).__name__,
                },
            )
            raise

        super().__init__(token, project_url)

    def _get_model_list_url(self):
        """Return the OpenAI-compatible models endpoint for the MWS project."""
        return mws_api_url(self.base_url, "openai/v1/models")

    def _format_model_list(self, raw_model_list):
        """Keep only chat, embedding, and reranking MWS deployments."""
        supported_types = {
            LLMType.CHAT.value,
            LLMType.EMBEDDING.value,
            LLMType.RERANK.value,
        }
        return [model for model in super()._format_model_list(raw_model_list) if len(model.get("model_types") or []) == 1 and model["model_types"][0] in supported_types]

    async def get_model_list(self):
        """Discover MWS models while logging safe request and result metadata."""
        try:
            url = self._get_model_list_url()
        except ValueError as error:
            logging.warning(
                "mws_model_discovery_validation_failed",
                extra={
                    "provider": self._FACTORY_NAME,
                    "operation": "model_discovery",
                    "validation_target": "api_url",
                    "error_type": type(error).__name__,
                },
            )
            raise

        log_context = {
            "provider": self._FACTORY_NAME,
            "operation": "model_discovery",
            "url": url,
        }
        logging.info("mws_model_discovery_request", extra=log_context)

        try:
            async with aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=30)) as session:
                async with session.get(url, headers={"Authorization": f"Bearer {self._get_api_key()}"}) as response:
                    if response.status != 200:
                        logging.warning(
                            "mws_model_discovery_request_failed",
                            extra={
                                **log_context,
                                "failure_stage": "http_response",
                                "http_status": response.status,
                            },
                        )
                        logging.info(
                            "mws_model_discovery_completed",
                            extra={**log_context, "result_count": 0},
                        )
                        return []
                    raw_model_list = await response.json()
        except Exception as error:
            logging.warning(
                "mws_model_discovery_request_failed",
                extra={
                    **log_context,
                    "failure_stage": "request",
                    "error_type": type(error).__name__,
                },
            )
            raise

        if not raw_model_list:
            models = []
        else:
            try:
                models = self._format_model_list(raw_model_list)
            except Exception as error:
                logging.warning(
                    "mws_model_discovery_validation_failed",
                    extra={
                        **log_context,
                        "validation_target": "response",
                        "error_type": type(error).__name__,
                    },
                )
                raise

        logging.info(
            "mws_model_discovery_completed",
            extra={**log_context, "result_count": len(models)},
        )
        return models


class NVIDIA(OpenAIAPICompatible):
    _FACTORY_NAME = "NVIDIA"
    _HOSTED_API_HOST = "integrate.api.nvidia.com"
    _CATALOG_URL = "https://api.ngc.nvidia.com/v2/search/catalog/resources/ENDPOINT"
    _CATALOG_PAGE_SIZE = 500
    _MODEL_LIST_TIMEOUT = aiohttp.ClientTimeout(total=30)

    @staticmethod
    def _normalized_publisher(value):
        return value.strip().lower().replace("_", "-")

    @classmethod
    def _catalog_resources(cls, catalog):
        catalogs = catalog if isinstance(catalog, list) else [catalog]
        resources = []
        total_count = None
        for catalog_page in catalogs:
            page_resources, page_total_count = cls._catalog_page(catalog_page)
            if page_resources is None:
                return None
            if total_count is None:
                total_count = page_total_count
            elif page_total_count != total_count:
                return None
            resources.extend(page_resources)
        if total_count is None or len(resources) < total_count:
            return None
        return resources

    @staticmethod
    def _catalog_page(catalog):
        if not isinstance(catalog, dict):
            return None, None
        for group in catalog.get("results", []):
            if not isinstance(group, dict) or group.get("groupValue") != "ENDPOINT":
                continue
            resources = group.get("resources")
            total_count = group.get("totalCount")
            if not isinstance(resources, list) or not isinstance(total_count, int) or total_count < 0:
                return None, None
            return resources, total_count
        return None, None

    @staticmethod
    def _label_values(resource, key):
        for label in resource.get("labels", []):
            if isinstance(label, dict) and label.get("key") == key:
                values = label.get("values", [])
                return values if isinstance(values, list) else []
        return []

    @staticmethod
    def _unresolved_label_values(resource, key):
        for label in resource.get("labels", []):
            if isinstance(label, dict) and label.get("key") == key:
                values = label.get("unresolvedValues", [])
                return values if isinstance(values, list) else []
        return []

    @staticmethod
    def _attribute_value(resource, key):
        for attribute in resource.get("attributes", []):
            if isinstance(attribute, dict) and attribute.get("key") == key:
                return attribute.get("value")
        return None

    @classmethod
    def _is_active_free_endpoint(cls, resource, now):
        if "Free Endpoint" not in cls._label_values(resource, "nimType"):
            return False
        deprecation = cls._attribute_value(resource, "DEPRECATION")
        if not deprecation:
            return True
        try:
            cutoff = datetime.strptime(deprecation, "%m/%d/%Y").replace(tzinfo=timezone.utc)
        except (TypeError, ValueError):
            return False
        return cutoff.date() > now.date()

    @classmethod
    def _filter_hosted_models(cls, models, catalog, now=None):
        resources = cls._catalog_resources(catalog)
        if resources is None:
            return []

        now = now or datetime.now(timezone.utc)
        active_endpoints = cls._active_endpoint_keys(resources, now)

        filtered = []
        for model in models:
            publisher, separator, model_name = model["name"].partition("/")
            if separator and (cls._normalized_publisher(publisher), model_name) in active_endpoints:
                filtered.append(model)
        return filtered

    @classmethod
    def _active_endpoint_keys(cls, resources, now):
        active_endpoints = set()
        for resource in resources:
            if not isinstance(resource, dict) or not cls._is_active_free_endpoint(resource, now):
                continue
            publishers = cls._unresolved_label_values(resource, "publisher")
            display_name = resource.get("displayName") or resource.get("name")
            if not publishers or not isinstance(display_name, str) or not display_name:
                continue
            active_endpoints.add((cls._normalized_publisher(publishers[0]), display_name))
        return active_endpoints

    def _uses_hosted_catalog(self):
        model_list_url = self._get_model_list_url()
        return bool(model_list_url and urlparse(model_list_url).hostname == self._HOSTED_API_HOST)

    async def get_model_list(self):
        if not self._uses_hosted_catalog():
            return await super().get_model_list()

        async with aiohttp.ClientSession(timeout=self._MODEL_LIST_TIMEOUT) as session:
            async with session.get(self._get_model_list_url(), headers={"Authorization": f"Bearer {self._get_api_key()}"}) as response:
                response.raise_for_status()
                raw_models = await response.json()

            catalog_pages = []
            resource_count = 0
            total_count = None
            page = 0
            while total_count is None or resource_count < total_count:
                catalog_query = {"page": page, "pageSize": self._CATALOG_PAGE_SIZE}
                async with session.get(
                    self._CATALOG_URL,
                    params={"q": json.dumps(catalog_query, separators=(",", ":")), "group-labels-by-labelset": "true"},
                ) as response:
                    response.raise_for_status()
                    catalog_page = await response.json()

                page_resources, page_total_count = self._catalog_page(catalog_page)
                if page_resources is None:
                    raise ValueError("NVIDIA endpoint catalog response is missing a valid ENDPOINT group")
                if total_count is None:
                    total_count = page_total_count
                elif page_total_count != total_count:
                    raise ValueError(f"NVIDIA endpoint catalog total count changed from {total_count} to {page_total_count}")

                catalog_pages.append(catalog_page)
                resource_count += len(page_resources)
                if resource_count < total_count and len(page_resources) < self._CATALOG_PAGE_SIZE:
                    raise ValueError(f"NVIDIA endpoint catalog returned {resource_count} of {total_count} resources")
                page += 1

        formatted_models = self._format_model_list(raw_models)
        resources = self._catalog_resources(catalog_pages)
        if resources is None:
            raise ValueError("NVIDIA endpoint catalog response is incomplete")
        now = datetime.now(timezone.utc)
        filtered_models = self._filter_hosted_models(formatted_models, catalog_pages, now)
        raw_model_items = raw_models.get("data") if isinstance(raw_models, dict) else raw_models
        raw_model_count = len(raw_model_items) if isinstance(raw_model_items, list) else 0
        logging.info(
            "[NVIDIA] Hosted model discovery succeeded: raw_models=%d active_endpoints=%d filtered_models=%d",
            raw_model_count,
            len(self._active_endpoint_keys(resources, now)),
            len(filtered_models),
        )
        return filtered_models

    def _format_model_list(self, raw_model_list):
        models = super()._format_model_list(raw_model_list)
        unique_models = {}
        for model in models:
            model_name = model["name"].strip()
            if not model_name or model_name in unique_models:
                continue
            model["name"] = model_name
            unique_models[model_name] = model
        return [unique_models[name] for name in sorted(unique_models)]


class GreenPT(OpenAIAPICompatible):
    """Discover and classify GreenPT models from the live catalog."""

    _FACTORY_NAME = "GreenPT"

    _MODEL_TYPES: ClassVar[dict[str, list[str]]] = {
        "green-embedding": [LLMType.EMBEDDING.value],
        "qwen3-embedding-8b": [LLMType.EMBEDDING.value],
        "green-rerank": [LLMType.RERANK.value],
        "green-s": [LLMType.ASR.value],
        "green-s-pro": [LLMType.ASR.value],
    }
    _MAX_TOKENS: ClassVar[dict[str, int]] = {
        "glm-5.2": 1_000_000,
        "kimi-k2.7-code": 262_144,
        "green-embedding": 32_768,
        "qwen3-embedding-8b": 32_768,
        "green-rerank": 32_768,
    }

    def _format_model_list(self, raw_model_list):
        """Apply GreenPT capability metadata to discovered models."""
        models = super()._format_model_list(raw_model_list)
        for model in models:
            model["model_types"] = self._MODEL_TYPES.get(model["name"], model["model_types"])
            model["max_tokens"] = self._MAX_TOKENS.get(model["name"], model["max_tokens"])
            if model["model_types"] == [LLMType.CHAT.value]:
                model["features"] = ["is_tools"]
        return models


class Synthorai(OpenAIAPICompatible):
    """Synthorai catalog lister.

    ``/v1/models`` returns the whole catalog, which includes image, audio,
    video and realtime entries alongside chat ones. The inherited formatter
    infers the type from the model id and falls back to ``chat``, so those
    non-chat entries would be offered as chat models and fail at the
    chat-completions endpoint. Only the ids declared in
    ``conf/models/synthorai.json`` are surfaced.
    """

    _FACTORY_NAME = "Synthorai"

    def _format_model_list(self, raw_model_list):
        models = super()._format_model_list(raw_model_list)
        allowed = self._allowed_model_names()
        if not allowed:
            return models
        return [m for m in models if m.get("name") in allowed]

    @staticmethod
    def _allowed_model_names() -> set:
        """Chat model ids declared for this provider, or an empty set."""
        import json
        import os

        path = os.path.join(
            os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))),
            "conf",
            "models",
            "synthorai.json",
        )
        try:
            with open(path, encoding="utf-8") as f:
                cfg = json.load(f)
        except (OSError, ValueError):
            return set()
        return {m["name"] for m in cfg.get("models", []) if isinstance(m, dict) and m.get("name") and "chat" in (m.get("model_types") or [])}


class HuggingFace(Base):
    """Discover models served by Hugging Face inference endpoints.

    Supports Text Embeddings Inference (TEI) and Text Generation Inference (TGI),
    both of which expose a ``GET /info`` endpoint.

    TEI response example::
        {"model_id":"BAAI/bge-base-zh-v1.5","model_type":{"embedding":{"pooling":"cls"}},...}
    TGI response example::
        {"model_id":"meta-llama/Llama-2-7b-chat-hf","model_type":"text-generation",...}
    """

    _FACTORY_NAME = "HuggingFace"

    def _get_model_list_url(self):
        if not self.base_url:
            return None
        return self.base_url.rstrip("/") + "/info"

    @staticmethod
    def _infer_model_types_from_info(info):
        model_type = info.get("model_type")
        # TEI format: {"embedding": {"pooling": "cls"}}
        if isinstance(model_type, dict):
            if "embedding" in model_type:
                return [LLMType.EMBEDDING.value]
            if "reranker" in model_type:
                return [LLMType.RERANK.value]
            return []
        # TGI format: "text-generation" / "text2text-generation"
        if isinstance(model_type, str):
            if "embedding" in model_type.lower():
                return [LLMType.EMBEDDING.value]
            return [LLMType.CHAT.value]
        return []

    def _format_model_list(self, raw_model_list):
        if not isinstance(raw_model_list, dict):
            return []
        model_id = raw_model_list.get("model_id")
        if not model_id:
            return []
        model_types = self._infer_model_types_from_info(raw_model_list)
        if not model_types:
            return []
        max_tokens = raw_model_list.get("max_input_length") or raw_model_list.get("max_total_tokens") or 8192
        return [
            {
                "name": model_id,
                "model_types": model_types,
                "features": [],
                "max_tokens": max_tokens,
            }
        ]


class FunASR(Base):
    _FACTORY_NAME = "FunASR"

    def _format_model_list(self, raw_model_list):
        models = raw_model_list.get("data") if isinstance(raw_model_list, dict) else None
        if not isinstance(models, list):
            return []

        model_list = []
        for model in models:
            if not isinstance(model, dict) or not model.get("id"):
                continue
            model_list.append(
                {
                    "name": model["id"],
                    "model_types": [LLMType.ASR.value],
                    "features": [],
                    "max_tokens": 8192,
                }
            )
        return model_list


class VLLM(OpenAIAPICompatible):
    _FACTORY_NAME = "VLLM"


class GPUStack(OpenAIAPICompatible):
    _FACTORY_NAME = "GPUStack"


class LMStudio(OpenAIAPICompatible):
    _FACTORY_NAME = "LM-Studio"


class Llmman(OpenAIAPICompatible):
    _FACTORY_NAME = "llmman"


class NewAPI(OpenAIAPICompatible):
    _FACTORY_NAME = "New API"

    def _get_api_key(self):
        try:
            parsed = json.loads(self.api_key)
            if isinstance(parsed, dict):
                return parsed.get("api_key", self.api_key)
        except (JSONDecodeError, TypeError):
            pass
        return self.api_key


class RAGcon(OpenAIAPICompatible):
    _FACTORY_NAME = "RAGcon"


class AIMLAPI(Base):
    """AIMLAPI (aimlapi.com) aggregates 700+ models behind an OpenAI-compatible
    API. ``GET /v1/models`` returns one record per model *and* endpoint, so a
    single model id repeats under different ``type`` values (e.g.
    ``openai/chat-completions`` and ``openai/responses/submit``); records are
    de-duplicated by id and their RAGFlow model types unioned.

    The ``type`` (endpoint family) field drives classification. Families RAGFlow
    cannot consume — image/video/audio generation, batch, OCR — are intentionally
    left out of the map, so those models are skipped. The listing carries no
    modality flag, so image-capable chat models are detected from the id, the
    same way OpenAIAPICompatible does.
    """

    _FACTORY_NAME = "aimlapi.com"

    _TYPE_TO_MODEL_TYPE = {
        "openai/chat-completions": LLMType.CHAT.value,
        "openai/responses/submit": LLMType.CHAT.value,
        "anthropic/messages": LLMType.CHAT.value,
        "openai/embeddings": LLMType.EMBEDDING.value,
        "internal/text-to-speech": LLMType.TTS.value,
        "internal/speech-to-text/submit": LLMType.ASR.value,
    }

    # Chat models whose id hints at image input also serve VISION (VLM).
    # Heuristic: the /v1/models listing exposes no structured modality field.
    _VISION_HINTS = (
        "gpt-4o",
        "gpt-4.1",
        "gpt-4-turbo",
        "gpt-5",
        "chatgpt-4o",
        "claude-3",
        "claude-opus-4",
        "claude-sonnet-4",
        "claude-haiku-4",
        "gemini",
        "qwen-vl",
        "qwen2-vl",
        "qwen2.5-vl",
        "qwen3-vl",
        "internvl",
        "llava",
        "pixtral",
        "minicpm-v",
        "glm-4v",
        "glm-4.1v",
        "llama-3.2",
        "llama-4",
        "grok-2-vision",
        "grok-4",
        "vision",
        "-vl",
    )

    # aiohttp defaults to a 5-minute total timeout, long enough for a stalled catalog
    # request to hold the task; 60s matches the timeout the other providers here use.
    _MODEL_LIST_TIMEOUT = aiohttp.ClientTimeout(total=60)

    async def _get_raw_model_list(self):
        url = self._get_model_list_url()
        if not url:
            logging.warning("[aimlapi.com] Model list skipped: no base URL configured")
            return None
        headers = {"Authorization": f"Bearer {self._get_api_key()}", **attribution_headers()}
        async with aiohttp.ClientSession(timeout=self._MODEL_LIST_TIMEOUT) as session:
            async with session.get(url, headers=headers) as resp:
                if resp.status != 200:
                    logging.warning("[aimlapi.com] Model list request to %s failed with HTTP %s", url, resp.status)
                    return None
                return await resp.json()

    def _format_model_list(self, raw_model_list):
        models = raw_model_list.get("data") if isinstance(raw_model_list, dict) else raw_model_list
        if not isinstance(models, list):
            return []

        merged = {}
        for model in models:
            if not isinstance(model, dict):
                continue
            model_id = model.get("id")
            if not model_id:
                continue
            model_type = self._TYPE_TO_MODEL_TYPE.get(model.get("type"))
            if not model_type:
                continue

            entry = merged.get(model_id)
            if entry is None:
                info = model.get("info") or {}
                entry = {
                    "name": model_id,
                    "model_types": [],
                    "features": [],
                    "max_tokens": info.get("contextLength") or 8192,
                }
                merged[model_id] = entry

            if model_type not in entry["model_types"]:
                entry["model_types"].append(model_type)
            if model_type == LLMType.CHAT.value and LLMType.VISION.value not in entry["model_types"] and any(hint in model_id.lower() for hint in self._VISION_HINTS):
                entry["model_types"].append(LLMType.VISION.value)

        return list(merged.values())
