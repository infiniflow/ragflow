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


def test_gpustack_tts_preserves_non_alias_voice_casing():
    provider = GPUStackTTS("sk-test", "qwen3-tts", base_url="http://gpustack:80")

    with patch.object(provider, "_send_request") as send_request, patch.object(provider, "_process_response", return_value=iter([b"audio"])):
        list(provider.tts("hello", voice="Uncle_Fu"))

    assert send_request.call_args.args[1]["voice"] == "Uncle_Fu"


def test_gpustack_tts_logs_stream_completion_after_audio_consumption():
    provider = GPUStackTTS("sk-test", "qwen3-tts", base_url="http://gpustack:80")
    response = MagicMock()
    response.status_code = 200
    events = []

    def audio_iterator():
        events.append("audio-yielded")
        yield b"audio"
        events.append("audio-exhausted")

    def record_info(message, *args):
        events.append(message)

    with (
        patch.object(provider, "_send_request", return_value=response),
        patch.object(provider, "_process_response", return_value=audio_iterator()),
        patch("rag.llm.tts_model.logger.info", side_effect=record_info),
    ):
        stream = provider.tts("hello", voice="aiden")
        assert "GPUStack TTS response received for model %s with status %s" in events
        assert "GPUStack TTS audio stream completed for model %s with status %s" not in events
        assert next(stream) == b"audio"
        assert "GPUStack TTS audio stream completed for model %s with status %s" not in events
        with pytest.raises(StopIteration):
            next(stream)

    assert events.index("audio-exhausted") < events.index("GPUStack TTS audio stream completed for model %s with status %s")


def test_gpustack_tts_request_failure_log_omits_exception_text():
    provider = GPUStackTTS("sk-test", "qwen3-tts", base_url="http://gpustack:80")

    with patch.object(provider, "_send_request", side_effect=RuntimeError("secret response body")), patch("rag.llm.tts_model.logger.warning") as warning, pytest.raises(RuntimeError):
        list(provider.tts("hello", voice="aiden"))

    warning.assert_called_once_with("GPUStack TTS request failed for model %s before audio streaming", "qwen3-tts")
    logged_parts = " ".join(str(arg) for call in warning.call_args_list for arg in call.args)
    assert "secret response body" not in logged_parts
