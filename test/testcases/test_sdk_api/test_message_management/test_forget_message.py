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
import pytest
from ragflow_sdk import Memory, RAGFlow

from test.testcases.test_sdk_api.test_message_management.message_helpers import assert_content, wait_until

pytestmark = pytest.mark.integration


class TestAuthorization:
    @pytest.mark.parametrize("invalid_auth", [None, "invalid-token"])
    def test_auth_invalid(self, populated_memory, integration_settings, invalid_auth):
        memory = populated_memory["memory"]
        rag = RAGFlow(invalid_auth, integration_settings["base_url"])
        row = populated_memory["rows"][0]
        with pytest.raises(Exception, match="^Invalid access token$"):
            Memory(rag, {"id": memory.id}).forget_message(row["message_id"])
        content = memory.get_message_content(row["message_id"])
        assert content["forget_at"] in (None, "", "None"), content
        assert_content(content, populated_memory["payloads"][0], memory.id)


class TestForgetMessage:
    def test_forget_message(self, client, populated_memory):
        memory = populated_memory["memory"]
        target, *survivors = populated_memory["rows"]
        message_id = target["message_id"]
        assert memory.forget_message(message_id) is True
        forgotten = wait_until(
            lambda: memory.get_message_content(message_id),
            lambda content: content.get("forget_at") not in (None, "", "None", "-"),
        )
        assert_content(forgotten, populated_memory["payloads"][0], memory.id)
        assert forgotten["message_id"] == message_id, forgotten
        assert forgotten["status"] is True, forgotten
        recent = wait_until(
            lambda: client.get_recent_messages([memory.id]),
            lambda rows: {row["message_id"] for row in rows} == {row["message_id"] for row in survivors},
        )
        assert len(recent) == len(survivors)
        assert memory.forget_message(message_id) is True
        assert_content(memory.get_message_content(message_id), populated_memory["payloads"][0], memory.id)

    @pytest.mark.parametrize("message_id", [-1, "bad", "", "1:2"])
    def test_invalid_message_id(self, populated_memory, message_id):
        with pytest.raises(Exception, match="message_id|formatted as memory_id:message_id"):
            populated_memory["memory"].forget_message(message_id)

    def test_nonexistent_message_is_idempotent(self, populated_memory):
        memory = populated_memory["memory"]
        for _ in range(2):
            assert memory.forget_message(9223372036854775807) is True
        for row, payload in zip(populated_memory["rows"], populated_memory["payloads"], strict=True):
            content = memory.get_message_content(row["message_id"])
            assert content["forget_at"] in (None, "", "None"), content
            assert_content(content, payload, memory.id)

    def test_nonexistent_memory(self, client):
        with pytest.raises(Exception, match="^Resource not found$"):
            Memory(client, {"id": "missing-message-test-memory"}).forget_message(0)

    def test_memory_isolation(self, populated_memory, memory_factory):
        other = memory_factory()
        row = populated_memory["rows"][0]
        assert other.forget_message(row["message_id"]) is True
        memory = populated_memory["memory"]
        content = memory.get_message_content(row["message_id"])
        assert content["forget_at"] in (None, "", "None"), content
        assert_content(content, populated_memory["payloads"][0], memory.id)
