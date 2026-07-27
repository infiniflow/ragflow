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

import logging
from io import BytesIO
from unittest.mock import Mock, patch

import pytest

from common.constants import LLMType
from rag.llm.model_meta import GreenPT
from rag.llm.rerank_model import GreenPTRerank
from rag.llm.sequence2txt_model import GreenPTSeq2txt


def test_greenpt_model_list_classifies_native_endpoints():
    provider = GreenPT("test", "https://api.greenpt.ai/v1")
    models = provider._format_model_list(
        {
            "data": [
                {"id": "glm-5.2"},
                {"id": "green-embedding"},
                {"id": "green-rerank"},
                {"id": "green-s"},
            ]
        }
    )

    by_name = {model["name"]: model for model in models}
    assert by_name["glm-5.2"]["model_types"] == [LLMType.CHAT.value]
    assert by_name["glm-5.2"]["max_tokens"] == 1_000_000
    assert by_name["green-embedding"]["model_types"] == [LLMType.EMBEDDING.value]
    assert by_name["green-rerank"]["model_types"] == [LLMType.RERANK.value]
    assert by_name["green-s"]["model_types"] == [LLMType.ASR.value]


def test_greenpt_rerank_normalizes_endpoint():
    assert GreenPTRerank("test").base_url == "https://api.greenpt.ai/v1/rerank"
    assert GreenPTRerank("test", base_url="https://example.com/v1").base_url == "https://example.com/v1/rerank"


def test_greenpt_transcription_uses_listen_protocol(caplog):
    caplog.set_level(logging.INFO)
    response = Mock()
    response.status_code = 200
    response.json.return_value = {"results": {"channels": [{"alternatives": [{"transcript": " renewable inference "}]}]}}
    response.raise_for_status.return_value = None

    with (
        patch("builtins.open", return_value=BytesIO(b"audio")),
        patch("rag.llm.sequence2txt_model.requests.post", return_value=response) as post,
    ):
        text, _ = GreenPTSeq2txt("secret").transcription("sample.wav", language="en")

    assert text == "renewable inference"
    assert post.call_args.args[0] == "https://api.greenpt.ai/v1/listen"
    assert post.call_args.kwargs["headers"]["Authorization"] == "Token secret"
    assert post.call_args.kwargs["params"] == {"model": "green-s", "language": "en"}
    assert "status=200" in caplog.text
    assert "secret" not in caplog.text
    assert "sample.wav" not in caplog.text


@pytest.mark.parametrize(
    "payload",
    [
        [],
        {},
        {"results": []},
        {"results": {"channels": "invalid"}},
        {"results": {"channels": [[]]}},
        {"results": {"channels": [{"alternatives": "invalid"}]}},
        {"results": {"channels": [{"alternatives": [[]]}]}},
        {"results": {"channels": [{"alternatives": [{"transcript": 42}]}]}},
        {"results": {"channels": [{"alternatives": [{"transcript": " "}]}]}},
    ],
)
def test_greenpt_transcription_rejects_malformed_responses(payload, caplog):
    caplog.set_level(logging.WARNING)
    response = Mock(status_code=200)
    response.json.return_value = payload

    with (
        patch("builtins.open", return_value=BytesIO(b"audio")),
        patch("rag.llm.sequence2txt_model.requests.post", return_value=response),
        pytest.raises(ValueError, match="contains no valid transcript"),
    ):
        GreenPTSeq2txt("secret").transcription("sample.wav")

    assert "model=green-s status=200" in caplog.text
    assert "secret" not in caplog.text
    assert "sample.wav" not in caplog.text
