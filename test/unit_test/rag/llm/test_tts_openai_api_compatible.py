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

from rag.llm.tts_model import OpenAIAPITTS, OpenAITTS

pytestmark = pytest.mark.p2


def test_openai_api_compatible_tts_registers_under_factory_name():
    assert OpenAIAPITTS._FACTORY_NAME == "OpenAI-API-Compatible"
    assert issubclass(OpenAIAPITTS, OpenAITTS)


def test_openai_api_compatible_tts_requires_base_url():
    with pytest.raises(ValueError):
        OpenAIAPITTS("sk-test", "some-tts")


def test_openai_api_compatible_tts_posts_to_audio_speech_endpoint():
    provider = OpenAIAPITTS("sk-test", "some-tts", base_url="http://gateway:8080/v1")

    with patch.object(provider, "_send_request") as send_request, patch.object(provider, "_process_response", return_value=iter([b"audio"])):
        list(provider.tts("hello"))

    send_request.assert_called_once()
    assert send_request.call_args.args[0] == "/audio/speech"
    payload = send_request.call_args.args[1]
    assert payload["model"] == "some-tts"
    assert payload["input"] == "hello"
    assert payload["voice"] == "alloy"
