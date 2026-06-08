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
import aiohttp
from abc import ABC

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

    def get_model_list(self):
        # todo implement access token auth
        raise NotImplementedError


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
                            "name": model["name"],
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


class TencentCloud(Base):
    _FACTORY_NAME = "Tencent Cloud"

    def get_model_list(self):
        # Tencent Cloud uses SDK-based authentication (SID/SK with HMAC signing).
        # Model listing is not available through a simple REST endpoint.
        raise NotImplementedError
