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
from rag.llm.model_meta import HuggingFace

pytestmark = pytest.mark.p2


@pytest.mark.parametrize(
    ("model_info", "expected_type", "expected_max_tokens"),
    [
        pytest.param(
            {
                "model_id": "/data/gte-rerank-int8",
                "model_type": {
                    "reranker": {
                        "id2label": {"0": "LABEL_0"},
                        "label2id": {"LABEL_0": 0},
                    }
                },
                "max_input_length": 2048,
                "version": "1.9.3",
            },
            LLMType.RERANK.value,
            2048,
            id="tei-reranker",
        ),
        pytest.param(
            {
                "model_id": "/data/qwen3-embed-int8",
                "model_type": {"embedding": {"pooling": "last_token"}},
                "max_input_length": 2048,
                "version": "1.9.3",
            },
            LLMType.EMBEDDING.value,
            2048,
            id="tei-embedding",
        ),
        pytest.param(
            {
                "model_id": "meta-llama/Llama-2-7b-chat-hf",
                "model_type": "text-generation",
                "max_total_tokens": 4096,
            },
            LLMType.CHAT.value,
            4096,
            id="tgi-text-generation",
        ),
    ],
)
def test_huggingface_formats_discovered_model(model_info, expected_type, expected_max_tokens):
    provider = HuggingFace(api_key="", base_url="http://inference-server")

    assert provider._format_model_list(model_info) == [
        {
            "name": model_info["model_id"],
            "model_types": [expected_type],
            "features": [],
            "max_tokens": expected_max_tokens,
        }
    ]
