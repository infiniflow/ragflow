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

from rag.llm.sequence2txt_model import OpenAI_APISeq2txt


pytestmark = pytest.mark.p2


@patch("rag.llm.sequence2txt_model.OpenAI")
def test_openai_api_asr_uses_openai_compatible_endpoint(mock_openai):
    provider = OpenAI_APISeq2txt(
        key="compatible-secret",
        model_name="qwen3-asr-1.7b___OpenAI-API",
        base_url="http://gpustack.internal:9090",
    )

    mock_openai.assert_called_once_with(api_key="compatible-secret", base_url="http://gpustack.internal:9090/v1")
    assert provider._FACTORY_NAME == "OpenAI-API-Compatible"
    assert provider.model_name == "qwen3-asr-1.7b"


@patch("rag.llm.sequence2txt_model.OpenAI")
def test_openai_api_asr_transcription_calls_audio_endpoint(mock_openai, tmp_path):
    audio_path = tmp_path / "sample.wav"
    audio_path.write_bytes(b"RIFF-test-audio")

    client = MagicMock()
    client.audio.transcriptions.create.return_value = SimpleNamespace(text="  compatible transcript  ")
    mock_openai.return_value = client
    provider = OpenAI_APISeq2txt(
        key="compatible-secret",
        model_name="whisper-compatible",
        base_url="https://compatible.example.com/v1",
    )

    with patch("rag.llm.sequence2txt_model.num_tokens_from_string", return_value=3):
        text, token_count = provider.transcription(audio_path)

    assert text == "compatible transcript"
    assert token_count == 3
    call = client.audio.transcriptions.create.call_args
    assert call.kwargs["model"] == "whisper-compatible"
    assert call.kwargs["file"].closed


def test_openai_api_asr_requires_base_url():
    with pytest.raises(ValueError, match="url cannot be None"):
        OpenAI_APISeq2txt(key="compatible-secret", model_name="whisper-compatible", base_url="")
