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

from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import pytest

from rag.llm.sequence2txt_model import FunASRSeq2txt


pytestmark = pytest.mark.p2


@patch("rag.llm.sequence2txt_model.OpenAI")
def test_funasr_defaults_to_local_sensevoice(mock_openai):
    provider = FunASRSeq2txt(key="")

    mock_openai.assert_called_once_with(api_key="funasr", base_url="http://localhost:8000/v1")
    assert provider._FACTORY_NAME == "FunASR"
    assert provider.model_name == "sensevoice"
    assert provider.base_url == "http://localhost:8000/v1"


@patch("rag.llm.sequence2txt_model.OpenAI")
def test_funasr_forwards_custom_connection_settings(mock_openai):
    provider = FunASRSeq2txt(
        key="local-secret",
        model_name="paraformer",
        base_url="http://funasr.internal:9000",
    )

    mock_openai.assert_called_once_with(api_key="local-secret", base_url="http://funasr.internal:9000/v1")
    assert provider.model_name == "paraformer"


@patch("rag.llm.sequence2txt_model.OpenAI")
def test_funasr_empty_base_url_uses_local_default(mock_openai):
    FunASRSeq2txt(key=None, base_url="")

    mock_openai.assert_called_once_with(api_key="funasr", base_url="http://localhost:8000/v1")


@patch("rag.llm.sequence2txt_model.OpenAI")
def test_funasr_transcription_uses_openai_compatible_endpoint(mock_openai, tmp_path):
    audio_path = tmp_path / "sample.wav"
    audio_path.write_bytes(b"RIFF-test-audio")

    client = MagicMock()
    client.audio.transcriptions.create.return_value = SimpleNamespace(text="  hello from FunASR  ")
    mock_openai.return_value = client
    provider = FunASRSeq2txt(key="", model_name="sensevoice")

    with patch("rag.llm.sequence2txt_model.num_tokens_from_string", return_value=4):
        text, token_count = provider.transcription(audio_path)

    assert text == "hello from FunASR"
    assert token_count == 4
    call = client.audio.transcriptions.create.call_args
    assert call.kwargs["model"] == "sensevoice"
    assert call.kwargs["file"].closed
