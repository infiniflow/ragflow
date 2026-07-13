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

import base64
from unittest.mock import MagicMock, patch

from rag.llm.sequence2txt_model import QWenSeq2txt


def test_fun_asr_flash_uses_native_request_format(tmp_path):
    audio_path = tmp_path / "sample.wav"
    audio_path.write_bytes(b"RIFF-test-audio")
    response = MagicMock()
    response.json.return_value = {"output": {"text": "transcribed text"}}

    with patch("rag.llm.sequence2txt_model.requests.post", return_value=response) as post:
        model = QWenSeq2txt(
            "test-key",
            "fun-asr-flash-2026-06-15",
            base_url="https://workspace.example.com/compatible-mode/v1",
        )
        text, _ = model.transcription(str(audio_path))

    assert text == "transcribed text"
    response.raise_for_status.assert_called_once_with()
    request = post.call_args
    assert request.args[0] == "https://workspace.example.com/api/v1/services/aigc/multimodal-generation/generation"
    assert request.kwargs["headers"]["X-DashScope-SSE"] == "disable"
    assert request.kwargs["json"]["parameters"] == {"format": "wav", "sample_rate": "16000"}
    audio_data = request.kwargs["json"]["input"]["messages"][0]["content"][0]["input_audio"]["data"]
    assert audio_data == f"data:audio/wav;base64,{base64.b64encode(audio_path.read_bytes()).decode('utf-8')}"


def test_qwen_audio_asr_keeps_existing_dashscope_path():
    response = {"output": {"choices": [{"message": MagicMock(content=[{"text": "legacy text"}])}]}}

    with patch("dashscope.MultiModalConversation.call", return_value=response) as call:
        model = QWenSeq2txt("test-key", "qwen-audio-asr")
        text, _ = model.transcription("https://example.com/sample.wav")

    assert text == "legacy text"
    call.assert_called_once_with(
        model="qwen-audio-asr",
        messages=[
            {"role": "system", "content": [{"text": ""}]},
            {"role": "user", "content": [{"audio": "https://example.com/sample.wav"}]},
        ],
        result_format="message",
        asr_options={"enable_lid": True, "enable_itn": False},
    )


def test_fun_asr_flash_stream_uses_native_transcription():
    model = QWenSeq2txt("test-key", "fun-asr-flash-2026-06-15")

    with patch.object(model, "_transcribe_fun_asr_flash", return_value=("stream text", 2)) as transcription:
        events = list(model.stream_transcription("sample.wav"))

    transcription.assert_called_once_with("sample.wav")
    assert events == [
        {"event": "delta", "text": "stream text"},
        {"event": "final", "text": "stream text"},
    ]
