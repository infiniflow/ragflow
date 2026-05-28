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

import pytest

from common.exceptions import UpstreamProviderError
from rag.llm.embedding_model import SILICONFLOWEmbed


def test_encode_queries_raises_upstream_message_when_siliconflow_data_is_null():
    response_body = {
        "code": 30001,
        "message": "Sorry, your account balance is insufficient",
        "data": None,
    }
    response = MagicMock()
    response.status_code = 200
    response.json.return_value = response_body

    with patch("rag.llm.embedding_model.requests.post", return_value=response):
        embed = SILICONFLOWEmbed("key", "BAAI/bge-m3")
        with pytest.raises(UpstreamProviderError) as exc_info:
            embed.encode_queries("hello")

    message = str(exc_info.value)
    assert "Embedding model error" in message
    assert "message: Sorry, your account balance is insufficient" in message
    assert "code: 30001" in message
    assert "NoneType" not in message
    assert "request_id" not in message


def test_encode_queries_raises_upstream_message_when_siliconflow_http_status_fails():
    response = MagicMock()
    response.status_code = 500
    response.text = '{"code":"ServerError","message":"upstream embedding service failed"}'

    with patch("rag.llm.embedding_model.requests.post", return_value=response):
        embed = SILICONFLOWEmbed("key", "BAAI/bge-m3")
        with pytest.raises(UpstreamProviderError) as exc_info:
            embed.encode_queries("hello")

    message = str(exc_info.value)
    assert "Embedding model error" in message
    assert "status_code: 500" in message
    assert "code: ServerError" in message
    assert "message: upstream embedding service failed" in message
    response.json.assert_not_called()
