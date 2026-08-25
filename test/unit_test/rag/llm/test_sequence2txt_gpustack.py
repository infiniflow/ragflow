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

from rag.llm.sequence2txt_model import GPUStackSeq2txt


pytestmark = pytest.mark.p2


@pytest.mark.parametrize(
    ("gpustack_base_url", "expected_url"),
    [
        ("http://gpustack:80", "http://gpustack:80/v1/audio/transcriptions"),
        ("http://gpustack:80/v1", "http://gpustack:80/v1/audio/transcriptions"),
    ],
)
def test_gpustack_asr_transcription_uses_openai_compatible_endpoint(tmp_path, gpustack_base_url, expected_url):
    audio_path = tmp_path / "sample.wav"
    audio_path.write_bytes(b"RIFF-test-audio")
    response = MagicMock()
    response.json.return_value = {"text": "  hello from GPUStack  "}

    with patch("rag.llm.sequence2txt_model.requests.post", return_value=response) as post, patch("rag.llm.sequence2txt_model.num_tokens_from_string", return_value=3):
        provider = GPUStackSeq2txt("sk-test", "qwen3-asr", gpustack_base_url)
        text, token_count = provider.transcription(str(audio_path))

    assert text == "hello from GPUStack"
    assert token_count == 3
    post.assert_called_once()
    assert post.call_args.kwargs["url"] == expected_url
    assert post.call_args.kwargs["data"]["model"] == "qwen3-asr"
    assert post.call_args.kwargs["data"]["language"] == "zh"
    assert post.call_args.kwargs["headers"] == {"Authorization": "Bearer sk-test"}


def test_gpustack_asr_check_available_transcribes_generated_wav():
    provider = GPUStackSeq2txt("sk-test", "qwen3-asr", "http://gpustack:80/v1")

    with patch.object(provider, "transcription", return_value=("hello", 1)) as transcription:
        ok, reason = provider.check_available()

    assert ok is True
    assert reason == ""
    transcription.assert_called_once()


def test_gpustack_asr_returns_error_for_non_object_response(tmp_path):
    audio_path = tmp_path / "sample.wav"
    audio_path.write_bytes(b"RIFF-test-audio")
    response = MagicMock()
    response.status_code = 200
    response.json.return_value = []
    provider = GPUStackSeq2txt("sk-test", "qwen3-asr", "http://gpustack:80")

    with patch("rag.llm.sequence2txt_model.requests.post", return_value=response):
        text, token_count = provider.transcription(str(audio_path))

    assert text.startswith("**ERROR**: Invalid transcription response:")
    assert token_count == 0


def test_gpustack_asr_returns_error_for_non_string_text(tmp_path):
    audio_path = tmp_path / "sample.wav"
    audio_path.write_bytes(b"RIFF-test-audio")
    response = MagicMock()
    response.status_code = 200
    response.json.return_value = {"text": 123}
    provider = GPUStackSeq2txt("sk-test", "qwen3-asr", "http://gpustack:80")

    with patch("rag.llm.sequence2txt_model.requests.post", return_value=response):
        text, token_count = provider.transcription(str(audio_path))

    assert text == "**ERROR**: Failed to retrieve transcription."
    assert token_count == 0
