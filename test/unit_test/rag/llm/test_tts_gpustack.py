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

from rag.llm.tts_model import GPUStackTTS


pytestmark = pytest.mark.p2


def test_gpustack_tts_uses_openai_audio_speech_path_without_duplicate_v1():
    provider = GPUStackTTS("sk-test", "qwen3-tts", base_url="http://gpustack:80")

    with patch.object(provider, "_send_request") as send_request, patch.object(provider, "_process_response", return_value=iter([b"audio"])):
        list(provider.tts("hello", voice="aiden"))

    send_request.assert_called_once()
    assert send_request.call_args.args[0] == "/audio/speech"
    assert send_request.call_args.args[1]["voice"] == "aiden"


def test_gpustack_tts_maps_ragflow_default_voice_to_supported_default(monkeypatch):
    monkeypatch.setenv("GPUSTACK_TTS_VOICE", "serena")
    provider = GPUStackTTS("sk-test", "qwen3-tts", base_url="http://gpustack:80")

    with patch.object(provider, "_send_request") as send_request, patch.object(provider, "_process_response", return_value=iter([b"audio"])):
        list(provider.tts("hello"))

    assert send_request.call_args.args[1]["voice"] == "serena"
