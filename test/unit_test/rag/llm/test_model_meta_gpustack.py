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

import pytest

from common.constants import LLMType
from rag.llm.model_meta import GPUStack


pytestmark = pytest.mark.p2


def test_gpustack_formats_openai_compatible_model_list():
    provider = GPUStack(api_key="sk-test", base_url="http://gpustack:80")

    models = provider._format_model_list(
        {
            "object": "list",
            "data": [
                {"id": "qwen3.8-27b-fp8", "object": "model"},
                {"id": "qwen3-embedding-0.6b", "object": "model"},
                {"id": "qwen3-reranker-4b", "object": "model"},
                {"id": "qwen3-asr-0.6b", "object": "model"},
                {"id": "qwen3-tts-12hz-0.6b-customvoice", "object": "model"},
                {"object": "model"},
            ],
        }
    )

    assert models == [
        {"name": "qwen3.8-27b-fp8", "model_types": [LLMType.CHAT.value], "features": [], "max_tokens": 8192},
        {"name": "qwen3-embedding-0.6b", "model_types": [LLMType.EMBEDDING.value], "features": [], "max_tokens": 8192},
        {"name": "qwen3-reranker-4b", "model_types": [LLMType.RERANK.value], "features": [], "max_tokens": 8192},
        {"name": "qwen3-asr-0.6b", "model_types": [LLMType.ASR.value], "features": [], "max_tokens": 8192},
        {"name": "qwen3-tts-12hz-0.6b-customvoice", "model_types": [LLMType.TTS.value], "features": [], "max_tokens": 8192},
    ]


def test_gpustack_uses_models_endpoint_without_duplicate_v1():
    assert GPUStack(api_key="sk-test", base_url="http://gpustack:80")._get_model_list_url() == "http://gpustack:80/v1/models"
    assert GPUStack(api_key="sk-test", base_url="http://gpustack:80/v1")._get_model_list_url() == "http://gpustack:80/v1/models"
