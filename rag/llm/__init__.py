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
#  AFTER UPDATING THIS FILE, PLEASE ENSURE THAT docs/references/supported_models.mdx IS ALSO UPDATED for consistency!
#

import importlib
import inspect

from strenum import StrEnum


class SupportedLiteLLMProvider(StrEnum):
    Tongyi_Qianwen = "Tongyi-Qianwen"
    Dashscope = "Dashscope"
    Bedrock = "Bedrock"
    Moonshot = "Moonshot"
    xAI = "xAI"
    DeepInfra = "DeepInfra"
    Groq = "Groq"
    Cohere = "Cohere"
    Gemini = "Gemini"
    DeepSeek = "DeepSeek"
    Nvidia = "NVIDIA"
    TogetherAI = "TogetherAI"
    Anthropic = "Anthropic"
    Ollama = "Ollama"


FACTORY_DEFAULT_BASE_URL = {
    SupportedLiteLLMProvider.Tongyi_Qianwen: "https://dashscope.aliyuncs.com/compatible-mode/v1",
    SupportedLiteLLMProvider.Dashscope: "https://dashscope.aliyuncs.com/compatible-mode/v1",
    SupportedLiteLLMProvider.Moonshot: "https://api.moonshot.cn/v1",
}


LITELLM_PROVIDER_PREFIX = {
    SupportedLiteLLMProvider.Tongyi_Qianwen: "dashscope/",
    SupportedLiteLLMProvider.Dashscope: "dashscope/",
    SupportedLiteLLMProvider.Bedrock: "bedrock/",
    SupportedLiteLLMProvider.Moonshot: "moonshot/",
    SupportedLiteLLMProvider.xAI: "xai/",
    SupportedLiteLLMProvider.DeepInfra: "deepinfra/",
    SupportedLiteLLMProvider.Groq: "groq/",
    SupportedLiteLLMProvider.Cohere: "",  # don't need a prefix
    SupportedLiteLLMProvider.Gemini: "gemini/",
    SupportedLiteLLMProvider.DeepSeek: "deepseek/",
    SupportedLiteLLMProvider.Nvidia: "nvidia_nim/",
    SupportedLiteLLMProvider.TogetherAI: "together_ai/",
    SupportedLiteLLMProvider.Anthropic: "",  # don't need a prefix
    SupportedLiteLLMProvider.Ollama: "ollama_chat/",
}

ChatModel = globals().get("ChatModel", {})
CvModel = globals().get("CvModel", {})
EmbeddingModel = globals().get("EmbeddingModel", {})
RerankModel = globals().get("RerankModel", {})
Seq2txtModel = globals().get("Seq2txtModel", {})
TTSModel = globals().get("TTSModel", {})


MODULE_MAPPING = {
    "chat_model": ChatModel,
    "cv_model": CvModel,
    "embedding_model": EmbeddingModel,
    "rerank_model": RerankModel,
    "sequence2txt_model": Seq2txtModel,
    "tts_model": TTSModel,
}

package_name = __name__

for module_name, mapping_dict in MODULE_MAPPING.items():
    full_module_name = f"{package_name}.{module_name}"
    module = importlib.import_module(full_module_name)

    base_class = None
    lite_llm_base_class = None
    for name, obj in inspect.getmembers(module):
        if inspect.isclass(obj):
            if name == "Base":
                base_class = obj
            elif name == "LiteLLMBase":
                lite_llm_base_class = obj
                assert hasattr(obj, "_FACTORY_NAME"), "LiteLLMbase should have _FACTORY_NAME field."
                if hasattr(obj, "_FACTORY_NAME"):
                    if isinstance(obj._FACTORY_NAME, list):
                        for factory_name in obj._FACTORY_NAME:
                            mapping_dict[factory_name] = obj
                    else:
                        mapping_dict[obj._FACTORY_NAME] = obj

    if base_class is not None:
        for _, obj in inspect.getmembers(module):
            if inspect.isclass(obj) and issubclass(obj, base_class) and obj is not base_class and hasattr(obj, "_FACTORY_NAME"):
                if isinstance(obj._FACTORY_NAME, list):
                    for factory_name in obj._FACTORY_NAME:
                        mapping_dict[factory_name] = obj
                else:
                    mapping_dict[obj._FACTORY_NAME] = obj


__all__ = [
    "ChatModel",
    "CvModel",
    "EmbeddingModel",
    "RerankModel",
    "Seq2txtModel",
    "TTSModel",
]
