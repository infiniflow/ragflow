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

from api.utils import api_utils


def test_get_data_openai_defaults_created_to_current_timestamp(monkeypatch):
    monkeypatch.setattr(api_utils.time, "time", lambda: 1234567890.9)

    data = api_utils.get_data_openai(content="answer")

    assert data["created"] == 1234567890


def test_get_data_openai_preserves_explicit_created_value():
    data = api_utils.get_data_openai(created=0, content="answer")

    assert data["created"] == 0


def test_get_data_openai_stream_response_shape_is_unchanged():
    data = api_utils.get_data_openai(id="chatcmpl-test", model="test-model", content="chunk", finish_reason=None, stream=True)

    assert data == {
        "id": "chatcmpl-test",
        "object": "chat.completion.chunk",
        "model": "test-model",
        "choices": [
            {
                "delta": {"content": "chunk"},
                "finish_reason": None,
                "index": 0,
            }
        ],
    }
