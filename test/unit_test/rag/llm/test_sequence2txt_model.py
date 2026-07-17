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
import json
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
    assert request.kwargs["json"]["parameters"] == {"format": "wav"}
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


def test_fun_asr_flash_stream_uses_sse():
    response = MagicMock()
    response.iter_lines.return_value = [
        "id:1",
        "event:result",
        f"data:{json.dumps({'output': {'text': 'stream'}})}",
        "",
        "id:2",
        "event:result",
        f"data:{json.dumps({'output': {'text': 'stream text'}})}",
        "",
    ]
    model = QWenSeq2txt("test-key", "fun-asr-flash-2026-06-15")

    with patch("rag.llm.sequence2txt_model.requests.post", return_value=response) as post:
        events = list(model.stream_transcription("data:audio/wav;base64,dGVzdA=="))

    response.raise_for_status.assert_called_once_with()
    assert post.call_args.kwargs["headers"]["X-DashScope-SSE"] == "enable"
    assert post.call_args.kwargs["stream"] is True
    assert events == [
        {"event": "delta", "text": "stream"},
        {"event": "delta", "text": "stream text"},
        {"event": "final", "text": "stream text"},
    ]


def test_fun_asr_flash_stream_closes_response_when_consumer_stops_early():
    response = MagicMock()
    response.iter_lines.return_value = [f"data:{json.dumps({'output': {'text': 'stream'}})}"]
    model = QWenSeq2txt("test-key", "fun-asr-flash-2026-06-15")

    with patch("rag.llm.sequence2txt_model.requests.post", return_value=response):
        stream = model.stream_transcription("data:audio/wav;base64,dGVzdA==")
        assert next(stream) == {"event": "delta", "text": "stream"}
        stream.close()

    response.close.assert_called_once_with()


def test_fun_asr_flash_handles_top_level_text_response():
    response = MagicMock()
    response.json.return_value = {"text": "transcribed text"}

    with patch("rag.llm.sequence2txt_model.requests.post", return_value=response):
        model = QWenSeq2txt("test-key", "fun-asr-flash-2026-06-15")
        text, _ = model.transcription("data:audio/wav;base64,dGVzdA==")

    assert text == "transcribed text"


def test_fun_asr_flash_derives_format_from_data_uri():
    response = MagicMock()
    response.json.return_value = {"output": {"text": "transcribed text"}}
    audio_data = "data:audio/mpeg;base64,dGVzdA=="

    with patch("rag.llm.sequence2txt_model.requests.post", return_value=response) as post:
        model = QWenSeq2txt("test-key", "fun-asr-flash-2026-06-15")
        text, _ = model.transcription(audio_data)

    assert text == "transcribed text"
    assert post.call_args.kwargs["json"]["parameters"] == {"format": "mp3"}
    assert post.call_args.kwargs["json"]["input"]["messages"][0]["content"][0]["input_audio"]["data"] == audio_data


def test_fun_asr_flash_derives_format_from_url_path():
    response = MagicMock()
    response.json.return_value = {"output": {"text": "transcribed text"}}
    audio_url = "https://example.com/sample.opus?signature=test"

    with patch("rag.llm.sequence2txt_model.requests.post", return_value=response) as post:
        model = QWenSeq2txt("test-key", "fun-asr-flash-2026-06-15")
        text, _ = model.transcription(audio_url)

    assert text == "transcribed text"
    assert post.call_args.kwargs["json"]["parameters"] == {"format": "opus"}


def test_fun_asr_flash_rejects_extensionless_url(caplog):
    model = QWenSeq2txt("test-key", "fun-asr-flash-2026-06-15")

    with patch("rag.llm.sequence2txt_model.requests.post") as post:
        text, tokens = model.transcription("https://example.com/audio")

    post.assert_not_called()
    assert text.startswith("**ERROR**: Cannot determine audio format")
    assert tokens == 0
    assert "Fun-ASR-Flash transcription failed" in caplog.text


def test_fun_asr_flash_rejects_local_audio_over_base64_limit():
    model = QWenSeq2txt("test-key", "fun-asr-flash-2026-06-15")
    base64_limit = 8
    largest_allowed_raw_size = (base64_limit // 4) * 3

    with (
        patch.object(QWenSeq2txt, "_FUN_ASR_BASE64_MAX_SIZE", base64_limit),
        patch("rag.llm.sequence2txt_model.os.path.getsize", return_value=largest_allowed_raw_size + 1),
        patch("rag.llm.sequence2txt_model.requests.post") as post,
    ):
        text, tokens = model.transcription("large.wav")

    post.assert_not_called()
    assert text.startswith("**ERROR**: Fun-ASR-Flash Base64 audio exceeds the 10 MB encoded-input limit")
    assert tokens == 0


def test_fun_asr_flash_rejects_data_uri_over_base64_limit():
    model = QWenSeq2txt("test-key", "fun-asr-flash-2026-06-15")
    base64_limit = 8
    audio_data = f"data:audio/wav;base64,{'A' * (base64_limit + 1)}"

    with patch.object(QWenSeq2txt, "_FUN_ASR_BASE64_MAX_SIZE", base64_limit), patch("rag.llm.sequence2txt_model.requests.post") as post:
        text, tokens = model.transcription(audio_data)

    post.assert_not_called()
    assert text.startswith("**ERROR**: Fun-ASR-Flash Base64 audio exceeds the 10 MB encoded-input limit")
    assert tokens == 0


def test_fun_asr_flash_stream_emits_only_error_event_on_failure():
    model = QWenSeq2txt("test-key", "fun-asr-flash-2026-06-15")

    with patch("rag.llm.sequence2txt_model.requests.post", side_effect=RuntimeError("failed")):
        events = list(model.stream_transcription("data:audio/wav;base64,dGVzdA=="))

    assert events == [{"event": "error", "text": "**ERROR**: failed"}]
