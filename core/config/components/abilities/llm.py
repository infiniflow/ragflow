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

from typing import Dict, Optional, List

from pydantic import BaseModel, Field, model_validator

from core.config.constants.rag import PARSERS


class ModelConfig(BaseModel):
    name: str = Field(default="")
    factory: Optional[str] = Field(default="")
    api_key: Optional[str] = Field(default="", description="The API Key for the embedding model.")
    base_url: Optional[str] = Field(default="", description="The base URL for the embedding service.")

    @model_validator(mode="before")
    @classmethod
    def str_to_model(cls, values):
        if isinstance(values, str):
            return {"name": values}
        return values

    def resolved(self, backup_factory: str = "", backup_api_key: str = "", backup_base_url: str = "") -> Dict[str, str]:
        """
        Resolve per-model config, fallback to back up values if missing,
        and generate the 'name@factory' style name if applicable.
        """
        name = (self.name or "").strip()
        m_factory = self.factory or backup_factory or ""
        m_api_key = self.api_key or backup_api_key or ""
        m_base_url = self.base_url or backup_base_url or ""

        if name and "@" not in name and m_factory:
            name = f"{name}@{m_factory}"

        return {
            "model": name,
            "factory": m_factory,
            "api_key": m_api_key,
            "base_url": m_base_url,
        }

    @property
    def model(self):
        # e.g. Qwen3-4B@Tongyi-Qianwen
        return self.resolved().get("model")


class UserDefaultLLMConfig(BaseModel):
    factory: str = Field(
        default="",
        description="The LLM supplier. Options: OpenAI, DeepSeek, Moonshot, Tongyi-Qianwen, VolcEngine, ZHIPU-AI."
    )
    api_key: Optional[str] = Field(
        default="",
        description="The API key for the specified LLM. Required if factory is set."
    )
    allowed_factories: Optional[List[str]] = Field(
        default=None,
        description="If set, users are only allowed to add factories in this list. "
                    "Options include: OpenAI, DeepSeek, Moonshot."
    )
    parsers: str = Field(default=PARSERS, description="Comma-separated parser id:name list")
    default_models: Dict[str, ModelConfig] = Field(
        default_factory=lambda: {
            "chat_model": ModelConfig(),
            "embedding_model": ModelConfig(),
            "rerank_model": ModelConfig(),
            "asr_model": ModelConfig(),
            "image2text_model": ModelConfig(),
        }
    )

    @property
    def parser_ids(self) -> list[str]:
        """Return list of parser ids from parsers string"""
        if not self.parsers:
            return []
        return [item.split(":", 1)[0] for item in self.parsers.split(",")]

    @property
    def chat_model_cfg(self) -> ModelConfig:
        return self.default_models.get("chat_model", ModelConfig())

    @property
    def embedding_model_cfg(self) -> ModelConfig:
        return self.default_models.get("embedding_model", ModelConfig())

    @property
    def rerank_model_cfg(self) -> ModelConfig:
        return self.default_models.get("rerank_model", ModelConfig())

    @property
    def asr_model_cfg(self) -> ModelConfig:
        return self.default_models.get("asr_model", ModelConfig())

    @property
    def image2text_model_cfg(self) -> ModelConfig:
        return self.default_models.get("image2text_model", ModelConfig())
