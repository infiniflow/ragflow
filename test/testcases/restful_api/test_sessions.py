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


@pytest.mark.p1
def test_session_crud_cycle(rest_client, create_chat):
    chat_id = create_chat("restful_session_crud_chat")

    create_res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "session_a"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    session_id = create_payload["data"]["id"]
    assert create_payload["data"]["chat_id"] == chat_id, create_payload

    list_res = rest_client.get(f"/chats/{chat_id}/sessions")
    assert list_res.status_code == 200
    list_payload = list_res.json()
    assert list_payload["code"] == 0, list_payload
    assert any(item["id"] == session_id for item in list_payload["data"]), list_payload

    get_res = rest_client.get(f"/chats/{chat_id}/sessions/{session_id}")
    assert get_res.status_code == 200
    get_payload = get_res.json()
    assert get_payload["code"] == 0, get_payload
    assert get_payload["data"]["id"] == session_id, get_payload

    patch_res = rest_client.patch(
        f"/chats/{chat_id}/sessions/{session_id}",
        json={"name": "session_a_updated"},
    )
    assert patch_res.status_code == 200
    patch_payload = patch_res.json()
    assert patch_payload["code"] == 0, patch_payload
    assert patch_payload["data"]["name"] == "session_a_updated", patch_payload

    delete_res = rest_client.delete(f"/chats/{chat_id}/sessions", json={"ids": [session_id]})
    assert delete_res.status_code == 200
    delete_payload = delete_res.json()
    assert delete_payload["code"] == 0, delete_payload

    list_after_delete = rest_client.get(f"/chats/{chat_id}/sessions")
    assert list_after_delete.status_code == 200
    list_after_delete_payload = list_after_delete.json()
    assert list_after_delete_payload["code"] == 0, list_after_delete_payload
    assert all(item["id"] != session_id for item in list_after_delete_payload["data"]), list_after_delete_payload


@pytest.mark.p2
def test_session_create_name_validation(rest_client, create_chat):
    chat_id = create_chat("restful_session_name_validation_chat")

    res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": " "})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert "`name` can not be empty." in payload["message"], payload


@pytest.mark.p2
def test_session_update_blocks_messages_and_reference(rest_client, create_chat):
    chat_id = create_chat("restful_session_guard_chat")
    create_res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "session_guard"})
    assert create_res.status_code == 200
    create_payload = create_res.json()
    assert create_payload["code"] == 0, create_payload
    session_id = create_payload["data"]["id"]

    msg_res = rest_client.patch(f"/chats/{chat_id}/sessions/{session_id}", json={"messages": []})
    assert msg_res.status_code == 200
    msg_payload = msg_res.json()
    assert msg_payload["code"] == 102, msg_payload
    assert "`messages` cannot be changed." in msg_payload["message"], msg_payload

    ref_res = rest_client.patch(f"/chats/{chat_id}/sessions/{session_id}", json={"reference": []})
    assert ref_res.status_code == 200
    ref_payload = ref_res.json()
    assert ref_payload["code"] == 102, ref_payload
    assert "`reference` cannot be changed." in ref_payload["message"], ref_payload


@pytest.mark.p2
def test_chat_recommendation_requires_question(rest_client):
    res = rest_client.post("/chat/recommendation", json={})
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 101, payload
    assert "required argument are missing: question" in payload["message"], payload


@pytest.mark.p2
def test_related_questions_compatibility_requires_auth(rest_client_noauth):
    # /api/v1/searchbots/related_questions is an SDK compatibility endpoint.
    res = rest_client_noauth.post(
        "/searchbots/related_questions",
        json={"question": "ragflow"},
        headers={"Authorization": "invalid"},
    )
    assert res.status_code == 200
    payload = res.json()
    assert payload["code"] == 102, payload
    assert "Authorization is not valid!" in payload["message"], payload


@pytest.mark.p2
def test_chat_completion_nonstream_with_session(rest_client, create_chat):
    chat_id = create_chat("restful_completion_nonstream_chat")
    create_session_res = rest_client.post(f"/chats/{chat_id}/sessions", json={"name": "session_for_completion"})
    assert create_session_res.status_code == 200
    create_session_payload = create_session_res.json()
    assert create_session_payload["code"] == 0, create_session_payload
    session_id = create_session_payload["data"]["id"]

    completion_res = rest_client.post(
        "/chat/completions",
        json={
            "chat_id": chat_id,
            "session_id": session_id,
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
        },
        timeout=60,
    )
    assert completion_res.status_code == 200
    completion_payload = completion_res.json()
    assert completion_payload["code"] == 0, completion_payload
    assert isinstance(completion_payload["data"], dict), completion_payload
    assert completion_payload["data"]["session_id"] == session_id, completion_payload
    assert "answer" in completion_payload["data"], completion_payload
    assert "reference" in completion_payload["data"], completion_payload


@pytest.mark.p2
def test_chat_completion_stream_events(rest_client, create_chat):
    chat_id = create_chat("restful_completion_stream_chat")
    stream_res = rest_client.post(
        "/chat/completions",
        json={
            "chat_id": chat_id,
            "messages": [{"role": "user", "content": "hello"}],
            "stream": True,
        },
        timeout=60,
    )
    assert stream_res.status_code == 200
    content_type = stream_res.headers.get("Content-Type", "")
    assert "text/event-stream" in content_type, content_type

    events = _sse_events(stream_res.text)
    assert events, stream_res.text
    parsed_events = []
    for event in events:
        parsed = json.loads(event)
        parsed_events.append(parsed)

    assert any(evt.get("code") == 0 and isinstance(evt.get("data"), dict) for evt in parsed_events), parsed_events
    assert parsed_events[-1].get("data") is True, parsed_events[-1]


@pytest.mark.p2
def test_chat_completion_validation_errors(rest_client, create_chat):
    chat_id = create_chat("restful_completion_validation_chat")

    missing_messages = rest_client.post(
        "/chat/completions",
        json={"chat_id": chat_id, "stream": False},
    )
    assert missing_messages.status_code == 200
    missing_messages_payload = missing_messages.json()
    assert missing_messages_payload["code"] == 101, missing_messages_payload
    assert "required argument are missing: messages" in missing_messages_payload["message"], missing_messages_payload

    missing_chat_for_session = rest_client.post(
        "/chat/completions",
        json={
            "session_id": "some_session",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
        },
    )
    assert missing_chat_for_session.status_code == 200
    missing_chat_for_session_payload = missing_chat_for_session.json()
    assert missing_chat_for_session_payload["code"] == 102, missing_chat_for_session_payload
    assert "`chat_id` is required when `session_id` is provided." in missing_chat_for_session_payload["message"], missing_chat_for_session_payload

    invalid_chat = rest_client.post(
        "/chat/completions",
        json={
            "chat_id": "invalid_chat_id",
            "session_id": "invalid_session",
            "messages": [{"role": "user", "content": "hello"}],
            "stream": False,
        },
    )
    assert invalid_chat.status_code == 200
    invalid_chat_payload = invalid_chat.json()
    assert invalid_chat_payload["code"] == 109, invalid_chat_payload
    assert "No authorization." in invalid_chat_payload["message"], invalid_chat_payload
