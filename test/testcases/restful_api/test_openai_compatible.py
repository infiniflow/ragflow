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

import json

import pytest


def _sse_events(response_text: str) -> list[str]:
    return [line[5:] for line in response_text.splitlines() if line.startswith("data:")]


@pytest.mark.p2
@pytest.mark.parametrize(
    "payload, expected_message",
    [
        (
            {
                "model": "model",
                "messages": [{"role": "user", "content": "hello"}],
                "extra_body": "invalid_extra_body",
            },
            "extra_body must be an object.",
        ),
        (
            {
                "model": "model",
                "messages": [{"role": "user", "content": "hello"}],
                "extra_body": {"reference_metadata": "invalid_reference_metadata"},
            },
            "reference_metadata must be an object.",
        ),
        (
            {
                "model": "model",
                "messages": [{"role": "user", "content": "hello"}],
                "extra_body": {"reference_metadata": {"fields": "author"}},
            },
            "reference_metadata.fields must be an array.",
        ),
        (
            {
                "model": "model",
                "messages": [],
            },
            "You have to provide messages.",
        ),
        (
            {
                "model": "model",
                "messages": [{"role": "assistant", "content": "hello"}],
            },
            "The last content of this conversation is not from user.",
        ),
    ],
)
def test_openai_compatible_validation_payloads(rest_client, create_chat, payload, expected_message):
    chat_id = create_chat("restful_openai_validation_chat")
    res = rest_client.post(f"/openai/{chat_id}/chat/completions", json=payload)
    assert res.status_code == 200
    data = res.json()
    assert data["code"] != 0, data
    assert expected_message in data.get("message", ""), data


@pytest.mark.p2
def test_openai_compatible_metadata_condition_requires_object(rest_client, create_chat):
    chat_id = create_chat("restful_openai_metadata_condition_chat")
    res = rest_client.post(
        f"/openai/{chat_id}/chat/completions",
        json={
            "model": "model",
            "messages": [{"role": "user", "content": "hello"}],
            "extra_body": {"metadata_condition": "invalid"},
        },
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert "metadata_condition must be an object." in payload["message"], payload


@pytest.mark.p2
def test_openai_compatible_invalid_chat(rest_client):
    res = rest_client.post(
        "/openai/invalid_chat_id/chat/completions",
        json={
            "model": "model",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
        },
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] != 0, payload
    assert "don't own the chat" in payload["message"], payload


@pytest.mark.p2
def test_openai_compatible_nonstream_shape(rest_client, create_chat):
    chat_id = create_chat("restful_openai_nonstream_chat")
    res = rest_client.post(
        f"/openai/{chat_id}/chat/completions",
        json={
            "model": "model",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
        },
        timeout=60,
    )
    assert res.status_code == 200
    payload = res.json()

    assert payload["object"] == "chat.completion", payload
    assert isinstance(payload["choices"], list) and payload["choices"], payload
    first_choice = payload["choices"][0]
    assert first_choice.get("finish_reason") == "stop", payload
    assert first_choice.get("message", {}).get("role") == "assistant", payload
    assert "content" in first_choice.get("message", {}), payload

    usage = payload.get("usage", {})
    assert "prompt_tokens" in usage, usage
    assert "completion_tokens" in usage, usage
    assert "total_tokens" in usage, usage
    assert usage["total_tokens"] == usage["prompt_tokens"] + usage["completion_tokens"], usage


@pytest.mark.p2
def test_openai_compatible_nonstream_with_reference_output_shape(rest_client, create_chat):
    chat_id = create_chat("restful_openai_reference_chat")
    res = rest_client.post(
        f"/openai/{chat_id}/chat/completions",
        json={
            "model": "model",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
            "extra_body": {
                "reference": True,
                "reference_metadata": {"include": True, "fields": ["author"]},
            },
        },
        timeout=60,
    )
    assert res.status_code == 200
    payload = res.json()
    choice_msg = payload["choices"][0]["message"]
    assert "reference" in choice_msg, payload
    assert isinstance(choice_msg["reference"], list), payload


@pytest.mark.p2
def test_openai_compatible_stream_shape_and_done_semantics(rest_client, create_chat):
    chat_id = create_chat("restful_openai_stream_chat")
    res = rest_client.post(
        f"/openai/{chat_id}/chat/completions",
        json={
            "model": "model",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": True,
            "extra_body": {"reference": True},
        },
        timeout=60,
    )
    assert res.status_code == 200
    content_type = res.headers.get("Content-Type", "")
    assert "text/event-stream" in content_type, content_type

    events = _sse_events(res.text)
    assert events, res.text
    assert events[-1].strip() == "[DONE]", events[-1]

    json_events = [json.loads(evt) for evt in events if evt.strip() != "[DONE]"]
    assert json_events, events
    assert any(evt.get("object") == "chat.completion.chunk" for evt in json_events), json_events
    assert any(evt.get("choices", [{}])[0].get("finish_reason") == "stop" for evt in json_events), json_events


@pytest.mark.p2
def test_openai_compatible_reference_metadata_fields_filter_accepts_array(rest_client, create_chat):
    chat_id = create_chat("restful_openai_reference_fields_array_chat")
    res = rest_client.post(
        f"/openai/{chat_id}/chat/completions",
        json={
            "model": "model",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
            "extra_body": {
                "reference": True,
                "reference_metadata": {"include": True, "fields": ["author", "year"]},
            },
        },
        timeout=60,
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload.get("choices"), payload
    choice_msg = payload["choices"][0]["message"]
    assert "reference" in choice_msg, payload
    assert isinstance(choice_msg["reference"], list), payload
