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

    def __init__(self, api_key: str, base_url: str=None):
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
            return None
        parsed = urlparse(self.base_url)
        return f"{parsed.scheme}://{parsed.netloc}/api/v3/models"

    def _format_model_list(self, raw_model_list):
        serving_model = [model for model in raw_model_list if model.get("status", "") != "Shutdown"]
        res = []
        for model in serving_model:
            res.append({
                "name": model["id"],
                "model_types": [],
                "features": ["is_tools"] if model.get("features", {}).get("tools", {}).get("function_calling", False) else [],
                "max_tokens": model.get("token_limits", {}).get("max_input_token_length", 8192)
            })
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
            capability_to_model_type_mapping = {
                "completion": LLMType.CHAT.value,
                "vision": LLMType.IMAGE2TEXT.value,
                "embedding": LLMType.EMBEDDING.value
            }
            capability_to_feature_mapping = {
                "thinking": "thinking",
                "tools": "is_tools"
            }

            for model in models:
                async with session.post(self._get_model_detail_url(), headers=headers, json={"model": model["name"]}) as resp:
                    if resp.status != 200:
                        continue
                    model_info = await resp.json()
                    max_tokens_key = "{}.context_length".format(model_info.get("details", {}).get("family", ""))
                    res.append({
                        "name": model["name"],
                        "model_types": [capability_to_model_type_mapping[c] for c in model_info.get("capabilities", []) if c in capability_to_model_type_mapping],
                        "features": [capability_to_feature_mapping[c] for c in model_info.get("capabilities", []) if c in capability_to_feature_mapping],
                        "max_tokens": model_info["model_info"].get(max_tokens_key, 8192)
                    })
        return res


class FishAudio(Base):
    _FACTORY_NAME = "Fish Audio"

    def _get_access_token(self):
        api_key = self._get_api_key()
        if not api_key:
            return ""
        try:
            payload = json.loads(api_key)
        except Exception:
            return api_key
        if isinstance(payload, dict):
            return payload.get("fish_audio_ak") or payload.get("access_token") or payload.get("api_key") or api_key
        return api_key

    def _get_model_list_url(self):
        if not self.base_url:
            return "https://api.fish.audio/model"
        base_url = self.base_url.rstrip("/")
        if "/v1/" in base_url:
            return base_url.split("/v1")[0].rstrip("/") + "/model"
        if base_url.endswith("/v1"):
            return base_url[:-3] + "/model"
        return base_url + "/model"

    async def get_model_list(self):
        url = self._get_model_list_url()
        access_token = self._get_access_token()
        if not url or not access_token:
            return []

        async with aiohttp.ClientSession() as session:
            async with session.get(url, headers={"Authorization": f"Bearer {access_token}"}) as resp:
                if resp.status != 200:
                    return []
                raw_model_list = await resp.json()
                if not isinstance(raw_model_list, dict):
                    return []
                models = raw_model_list.get("items") or []
                if not isinstance(models, list):
                    return []

                model_list = []
                for model in models:
                    if not isinstance(model, dict):
                        continue
                    model_name = model.get("title") or model.get("_id")
                    if not model_name:
                        continue
                    model_list.append({
                        "name": model_name,
                        "model_types": [LLMType.TTS.value],
                        "features": [],
                        "max_tokens": 8192,
                    })
                return model_list

class MinerU(Base):
    _FACTORY_NAME = "MinerU"

    def _get_access_token(self):
        api_key = self._get_api_key()
        if not api_key:
            return ""
        try:
            payload = json.loads(api_key)
        except Exception:
            return api_key
        if isinstance(payload, dict):
            return payload.get("access_token") or payload.get("api_key") or api_key
        return api_key

    async def get_model_list(self):
        url = self._get_model_list_url()
        access_token = self._get_access_token()
        if not url or not access_token:
            return []

        async with aiohttp.ClientSession() as session:
            async with session.get(url, headers={"Authorization": f"Bearer {access_token}"}) as resp:
                if resp.status != 200:
                    return []
                raw_model_list = await resp.json()
                if isinstance(raw_model_list, dict):
                    raw_model_list = raw_model_list.get("data") or raw_model_list.get("models") or raw_model_list.get("items") or []
                if not isinstance(raw_model_list, list):
                    return []

                model_list = []
                for model in raw_model_list:
                    if not isinstance(model, dict):
                        continue
                    model_name = model.get("title") or model.get("name") or model.get("id") or model.get("_id")
                    if not model_name:
                        continue
                    model_list.append({
                        "name": model_name,
                        "model_types": [LLMType.OCR.value],
                        "features": [],
                        "max_tokens": model.get("max_tokens", 8192),
                    })
                return model_list


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

            max_tokens = (
                (model.get("top_provider") or {}).get("max_completion_tokens")
                or model.get("context_length")
                or (model.get("top_provider") or {}).get("context_length")
                or 8192
            )

            model_list.append({
                "name": model_name,
                "model_types": list(dict.fromkeys(model_types)),
                "features": features,
                "max_tokens": max_tokens,
            })

        return model_list


class OpenAIAPICompatible(Base):
    _FACTORY_NAME = "OpenAI-API-Compatible"

    _EMBEDDING_HINTS = ("embed", "embedding")
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
            model_list.append({
                "name": model_name,
                "model_types": self._infer_model_types(model_name_lower),
                "features": [],
                "max_tokens": (
                    model.get("max_tokens")
                    or model.get("max_completion_tokens")
                    or model.get("context_length")
                    or model.get("max_model_len")
                    or 8192
                ),
            })

        return model_list

class VLLM(OpenAIAPICompatible):
    _FACTORY_NAME = "VLLM"

class LMStudio(OpenAIAPICompatible):
    _FACTORY_NAME = "LM-Studio"