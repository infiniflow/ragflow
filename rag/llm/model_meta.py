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
import json
import aiohttp
from abc import ABC
from urllib.parse import urlparse
from json.decoder import JSONDecodeError

from common.constants import LLMType


class Base(ABC):
    def __init__(self, api_key: str, base_url: str = None):
        self.api_key = api_key
        self.base_url = base_url

    def _get_api_key(self):
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
                    model_types.append(LLMType.IMAGE2TEXT.value)
                if "audio" in input_modalities and "text" in output_modalities:
                    model_types.append(LLMType.SPEECH2TEXT.value)
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

    def _get_model_tags_url(self):
        return self.base_url.rstrip("/") + "/api/tags"

    def _get_model_detail_url(self):
        return self.base_url.rstrip("/") + "/api/show"

    async def get_model_list(self):
        if not self.base_url:
            return []
        headers = {}
        if self.api_key:
            headers.update({"Authorization": f"Bearer {self._get_api_key()}"})
        async with aiohttp.ClientSession() as session:
            async with session.get(self._get_model_tags_url(), headers=headers) as resp:
                if resp.status != 200:
                    return []
                tags = await resp.json()
                models = tags.get("models", [])
                if not models:
                    return []
            res = []
            capability_to_model_type_mapping = {"completion": LLMType.CHAT.value, "vision": LLMType.IMAGE2TEXT.value, "embedding": LLMType.EMBEDDING.value}
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
            "image": LLMType.IMAGE2TEXT.value,
            "TTS": LLMType.TTS.value,
            "speech2text": LLMType.SPEECH2TEXT.value,
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

    def _get_model_tags_url(self):
        return self.base_url.rstrip("/") + "/api/tags"

    def _get_model_detail_url(self):
        return self.base_url.rstrip("/") + "/api/show"

    async def get_model_list(self):
        if not self.base_url:
            return []
        headers = {}
        if self.api_key:
            headers.update({"Authorization": f"Bearer {self._get_api_key()}"})
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
                "vision": LLMType.IMAGE2TEXT.value,
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
                model_types.append(LLMType.IMAGE2TEXT.value)
            if "audio" in input_modalities and "text" in output_modalities:
                model_types.append(LLMType.SPEECH2TEXT.value)
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
    _SPEECH2TEXT_HINTS = ("asr", "stt", "transcribe", "transcriber", "whisper")
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
        if cls._contains_hint(model_name, cls._SPEECH2TEXT_HINTS):
            return [LLMType.SPEECH2TEXT.value]
        if cls._contains_hint(model_name, cls._TTS_HINTS):
            return [LLMType.TTS.value]

        model_types = [LLMType.CHAT.value]
        if cls._contains_hint(model_name, cls._VISION_HINTS):
            model_types.append(LLMType.IMAGE2TEXT.value)
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


class VLLM(OpenAIAPICompatible):
    _FACTORY_NAME = "VLLM"


class LMStudio(OpenAIAPICompatible):
    _FACTORY_NAME = "LM-Studio"


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
