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


def test_get_data_openai_stream_chunk_matches_openai_shape(monkeypatch):
    monkeypatch.setattr(api_utils.time, "time", lambda: 1234567890.9)

    data = api_utils.get_data_openai(id="chatcmpl-test", model="test-model", content="chunk", stream=True)

    assert data == {
        "id": "chatcmpl-test",
        "object": "chat.completion.chunk",
        "created": 1234567890,
        "model": "test-model",
        "system_fingerprint": "",
        "usage": None,
        "choices": [
            {
                "delta": {
                    "content": "chunk",
                    "role": "assistant",
                    "function_call": None,
                    "tool_calls": None,
                },
                "finish_reason": None,
                "index": 0,
                "logprobs": None,
            }
        ],
    }


def test_get_data_openai_stream_preserves_explicit_created_value():
    data = api_utils.get_data_openai(created=0, content="chunk", stream=True)

    assert data["created"] == 0


def test_get_data_openai_terminal_stream_chunk_includes_usage():
    data = api_utils.get_data_openai(prompt_tokens=3, completion_tokens=5, content=None, finish_reason="stop", stream=True)

    assert data["usage"] == {
        "prompt_tokens": 3,
        "completion_tokens": 5,
        "total_tokens": 8,
        "completion_tokens_details": {
            "reasoning_tokens": 0,
            "accepted_prediction_tokens": 0,
            "rejected_prediction_tokens": 0,
        },
    }
    assert data["choices"][0]["finish_reason"] == "stop"


def test_get_data_openai_stream_delta_allows_reference_payload():
    data = api_utils.get_data_openai(content="chunk", stream=True)

    data["choices"][0]["delta"]["reference"] = {"chunks": []}

    assert data["choices"][0]["delta"]["reference"] == {"chunks": []}


def _build_error_result_in_app_context(code):
    import asyncio

    from quart import Quart

    app = Quart(__name__)

    async def run():
        async with app.app_context():
            return api_utils.build_error_result(code=code, message="boom")

    return asyncio.run(run())


def test_build_error_result_never_uses_1xx_http_status():
    # RetCode.ARGUMENT_ERROR is 101. Sending it as the HTTP status makes h11
    # reject the final response, so Hypercorn closes the connection and the
    # client sees an empty reply instead of the JSON error (#17980).
    from common.constants import RetCode

    resp = _build_error_result_in_app_context(RetCode.ARGUMENT_ERROR)

    assert resp.status_code == 400

    # Plain ints must hit the same mapping (RetCode is an IntEnum).
    assert _build_error_result_in_app_context(101).status_code == 400


def test_build_error_result_maps_internal_ret_codes():
    from common.constants import RetCode

    expected = {
        RetCode.EXCEPTION_ERROR: 500,
        RetCode.DATA_ERROR: 400,
        RetCode.OPERATING_ERROR: 400,
        RetCode.CONNECTION_ERROR: 500,
        RetCode.RUNNING: 500,
        RetCode.PERMISSION_ERROR: 403,
        RetCode.AUTHENTICATION_ERROR: 403,
    }
    for code, http_status in expected.items():
        resp = _build_error_result_in_app_context(code)
        assert resp.status_code == http_status, code
        assert resp.status_code >= 200


def test_build_error_result_keeps_valid_http_codes_and_body():
    import asyncio

    from common.constants import RetCode

    resp = _build_error_result_in_app_context(RetCode.NOT_FOUND)

    assert resp.status_code == 404
    body = asyncio.run(resp.get_json())
    assert body == {"code": RetCode.NOT_FOUND, "message": "boom"}
