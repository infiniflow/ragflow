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
from rag.llm.model_meta import FunASR


pytestmark = pytest.mark.p2


def test_funasr_formats_server_models_as_speech_to_text():
    provider = FunASR(api_key="", base_url="http://localhost:8000/v1")

    models = provider._format_model_list(
        {
            "object": "list",
            "data": [
                {"id": "fun-asr-nano", "object": "model"},
                {"id": "sensevoice", "object": "model"},
                {"object": "model"},
            ],
        }
    )

    assert models == [
        {"name": "fun-asr-nano", "model_types": [LLMType.ASR.value], "features": [], "max_tokens": 8192},
        {"name": "sensevoice", "model_types": [LLMType.ASR.value], "features": [], "max_tokens": 8192},
    ]


def test_funasr_rejects_malformed_model_lists():
    provider = FunASR(api_key="", base_url="http://localhost:8000/v1")

    assert provider._format_model_list({}) == []
    assert provider._format_model_list({"data": "sensevoice"}) == []


def test_funasr_uses_openai_compatible_models_endpoint():
    assert FunASR(api_key="", base_url="http://localhost:8000/v1")._get_model_list_url() == "http://localhost:8000/v1/models"
    assert FunASR(api_key="", base_url="http://localhost:8000")._get_model_list_url() == "http://localhost:8000/v1/models"
