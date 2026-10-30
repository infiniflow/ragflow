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
from uuid import uuid4

import pytest
from ragflow_sdk import RAGFlow

from test.testcases.test_sdk_api.test_message_management.message_helpers import assert_content, assert_identity, wait_until

pytestmark = pytest.mark.integration


@pytest.mark.parametrize("invalid_auth", [None, "invalid-token"])
def test_auth_invalid(integration_settings, invalid_auth):
    rag = RAGFlow(invalid_auth, integration_settings["base_url"])
    with pytest.raises(Exception, match="^Invalid access token$"):
        rag.add_message(["missing"], "agent", "session", "question", "answer", user_id="external-user")


def payload_for(memories):
    namespace = uuid4().hex
    return dict(
        memory_id=[memory.id for memory in memories],
        agent_id=f"{namespace}-agent",
        session_id=f"{namespace}-session",
        user_id="external-conversation-user",
        user_input="What is a pineapple?",
        agent_response="A pineapple is a sweet tropical fruit.",
    )


@pytest.mark.parametrize("types", [["raw"], ["raw", "semantic", "episodic", "procedural"]], ids=["raw", "multiple-types"])
def test_add_message(client, memory_factory, types):
    memory = memory_factory(memory_type=types)
    payload = payload_for([memory])
    assert client.add_message(**payload) == "All add to task."
    result = wait_until(memory.list_memory_messages, lambda value: value["messages"]["total_count"] == 1)
    rows = result["messages"]["message_list"]
    assert len(rows) == 1
    assert_identity(rows[0], payload, memory.id)
    assert_content(memory.get_message_content(rows[0]["message_id"]), payload, memory.id)
    recent = wait_until(lambda: client.get_recent_messages([memory.id]), lambda value: any(row["message_id"] == rows[0]["message_id"] for row in value))
    assert_content(next(row for row in recent if row["message_id"] == rows[0]["message_id"]), payload, memory.id)


def test_add_to_multiple_memory(client, memory_factory):
    memories = [memory_factory(), memory_factory()]
    payload = payload_for(memories)
    assert client.add_message(**payload) == "All add to task."
    ids = []
    for memory in memories:
        result = wait_until(memory.list_memory_messages, lambda value: value["messages"]["total_count"] == 1)
        rows = result["messages"]["message_list"]
        assert len(rows) == 1
        assert_content(memory.get_message_content(rows[0]["message_id"]), payload, memory.id)
        ids.append(rows[0]["message_id"])
    # Each association has an independently addressable memory:message identity.
    assert len({(memory.id, str(message_id)) for memory, message_id in zip(memories, ids)}) == 2


@pytest.mark.parametrize("field", ["memory_id", "agent_id", "session_id", "user_input", "agent_response", "user_id"])
@pytest.mark.parametrize("value", [None, "", 12])
def test_invalid_creation_fields(client, memory_factory, field, value):
    memory = memory_factory()
    payload = payload_for([memory])
    payload[field] = value
    with pytest.raises(Exception, match="body arguments is required|memory_id is required|user_id is required"):
        client.add_message(**payload)
    assert memory.list_memory_messages()["messages"] == {"total_count": 0, "message_list": []}


@pytest.mark.parametrize("user_id", ["", "   "])
def test_api_key_requires_external_user(client, memory_factory, user_id):
    memory = memory_factory()
    payload = payload_for([memory])
    payload["user_id"] = user_id
    with pytest.raises(Exception, match="^user_id is required$"):
        client.add_message(**payload)
    assert memory.list_memory_messages()["messages"]["total_count"] == 0


def test_api_key_default_user_is_rejected(client, memory_factory):
    memory = memory_factory()
    payload = payload_for([memory])
    del payload["user_id"]
    with pytest.raises(Exception, match="^user_id is required$"):
        client.add_message(**payload)


def test_external_user_trimmed_and_does_not_control_memory_access(client, memory_factory):
    memory = memory_factory()
    payload = payload_for([memory])
    payload["user_id"] = "  external-subject  "
    assert client.add_message(**payload) == "All add to task."
    result = wait_until(memory.list_memory_messages, lambda value: value["messages"]["total_count"] == 1)
    assert_content(memory.get_message_content(result["messages"]["message_list"][0]["message_id"]), payload, memory.id)


def test_missing_memory_rejected(client):
    payload = payload_for([])
    payload["memory_id"] = [uuid4().hex]
    with pytest.raises(Exception, match="Some messages failed to add.*Memory not found"):
        client.add_message(**payload)


def test_empty_memory_list_rejected(client):
    with pytest.raises(Exception, match="memory_id is required"):
        client.add_message(**payload_for([]))


def test_partial_memory_failure_is_reported_and_valid_save_persists(client, memory_factory):
    memory = memory_factory()
    payload = payload_for([memory])
    payload["memory_id"] = [memory.id, uuid4().hex]
    with pytest.raises(Exception, match="Some messages failed to add"):
        client.add_message(**payload)
    result = wait_until(memory.list_memory_messages, lambda value: value["messages"]["total_count"] == 1)
    assert_content(memory.get_message_content(result["messages"]["message_list"][0]["message_id"]), payload, memory.id)
