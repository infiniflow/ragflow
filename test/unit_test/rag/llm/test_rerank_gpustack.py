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

from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from rag.llm.rerank_model import GPUStackRerank

pytestmark = pytest.mark.p2


@pytest.mark.parametrize(
    ("gpustack_base_url", "expected_url"),
    [
        ("http://gpustack:80", "http://gpustack:80/v1/rerank"),
        ("http://gpustack:80/v1", "http://gpustack:80/v1/rerank"),
        ("http://gpustack:80/v1/rerank", "http://gpustack:80/v1/rerank"),
    ],
)
def test_gpustack_rerank_uses_single_versioned_endpoint(gpustack_base_url, expected_url):
    response = MagicMock()
    response.raise_for_status.return_value = None
    response.json.return_value = {"results": [{"index": 0, "relevance_score": 0.8}, {"index": 1, "relevance_score": 0.2}]}

    with patch("rag.llm.rerank_model.requests.post", return_value=response) as post, patch("rag.llm.rerank_model.num_tokens_from_string", return_value=1):
        rank, token_count = GPUStackRerank("sk-test", "qwen3-reranker-4b", gpustack_base_url).similarity("query", ["first", "second"])

    post.assert_called_once()
    assert post.call_args.args[0] == expected_url
    assert post.call_args.kwargs["json"]["model"] == "qwen3-reranker-4b"
    assert token_count == 2
    assert np.allclose(rank, [0.8, 0.2])
