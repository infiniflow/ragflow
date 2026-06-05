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
                        print(resp, flush=True)
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
