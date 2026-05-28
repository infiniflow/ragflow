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

from unittest.mock import patch

import pytest

from common.exceptions import UpstreamProviderError
from rag.llm.embedding_model import QWenEmbed


def test_encode_queries_raises_upstream_message_when_dashscope_output_is_null():
    response = {
        "status_code": 400,
        "request_id": "8ccbaab3-14d7-92c0-95f3-c6ea34eb9bed",
        "code": "Arrearage",
        "message": "Access denied, please make sure your account is in good standing.",
        "output": None,
        "usage": None,
    }

    with patch("rag.llm.embedding_model.dashscope.TextEmbedding.call", return_value=response):
        embed = QWenEmbed("key", "text-embedding-v2")
        with pytest.raises(UpstreamProviderError) as exc_info:
            embed.encode_queries("hello")

    message = str(exc_info.value)
    assert "Embedding model error" in message
    assert "status_code: 400" in message
    assert "code: Arrearage" in message
    assert "Access denied" in message
    assert "request_id" not in message
    assert "NoneType" not in message
