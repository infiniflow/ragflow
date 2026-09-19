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

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from common.constants import LLMType
from rag.llm import ModelMeta
from rag.llm.model_meta import Xiaomi

pytestmark = pytest.mark.p2


def test_xiaomi_registered_in_model_meta():
    assert ModelMeta["Xiaomi"] is Xiaomi


@pytest.mark.parametrize(
    "base_url, expected_url",
    [
        ("https://api.xiaomimimo.com/v1", "https://api.xiaomimimo.com/v1/models"),
        ("https://api.xiaomimimo.com/v1/", "https://api.xiaomimimo.com/v1/models"),
        ("http://127.0.0.1:9901/v1", "http://127.0.0.1:9901/v1/models"),
    ],
)
def test_xiaomi_model_list_url(base_url, expected_url):
    assert Xiaomi(api_key="test-key", base_url=base_url)._get_model_list_url() == expected_url


async def test_xiaomi_get_model_list_sends_api_key_header():
    resp = MagicMock()
    resp.status = 200
    resp.json = AsyncMock(
        return_value={
            "object": "list",
            "data": [
                {"id": "mimo-v2.5-pro", "object": "model"},
                {"id": "mimo-v2.5-asr", "object": "model"},
                {"id": "mimo-v2.5-tts", "object": "model"},
            ],
        }
    )

    session = MagicMock()
    session.get.return_value = MagicMock(
        __aenter__=AsyncMock(return_value=resp),
        __aexit__=AsyncMock(return_value=None),
    )
    captured_headers = {}

    def capture_get(url, headers=None):
        captured_headers.update(headers or {})
        return session.get.return_value

    session.get.side_effect = capture_get

    session_cls = MagicMock()
    session_cls.__aenter__ = AsyncMock(return_value=session)
    session_cls.__aexit__ = AsyncMock(return_value=None)

    with patch("rag.llm.model_meta.aiohttp.ClientSession", return_value=session_cls):
        models = await Xiaomi(api_key="test-key", base_url="https://api.xiaomimimo.com/v1").get_model_list()

    assert captured_headers == {"api-key": "test-key"}
    assert [m["name"] for m in models] == ["mimo-v2.5-pro", "mimo-v2.5-asr", "mimo-v2.5-tts"]
    assert models[0]["model_types"] == [LLMType.CHAT.value]
    assert models[1]["model_types"] == [LLMType.ASR.value]
    assert models[2]["model_types"] == [LLMType.TTS.value]
