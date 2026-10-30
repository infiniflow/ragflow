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
            Memory(rag, {"id": memory.id}).update_message_status(row["message_id"], False)
        content = memory.get_message_content(row["message_id"])
        assert content["status"] is True, content
        assert_content(content, populated_memory["payloads"][0], memory.id)


class TestUpdateMessageStatus:
    def test_update_to_false(self, populated_memory):
        memory = populated_memory["memory"]
        row = populated_memory["rows"][0]
        assert memory.update_message_status(row["message_id"], False) is True
        content = wait_until(
            lambda: memory.get_message_content(row["message_id"]),
            lambda content: content.get("status") is False,
        )
        assert_content(content, populated_memory["payloads"][0], memory.id)
        assert memory.update_message_status(row["message_id"], False) is True
        assert memory.get_message_content(row["message_id"])["status"] is False
        for survivor in populated_memory["rows"][1:]:
            assert memory.get_message_content(survivor["message_id"])["status"] is True

    def test_update_to_true(self, populated_memory):
        memory = populated_memory["memory"]
        row = populated_memory["rows"][0]
        assert memory.update_message_status(row["message_id"], False) is True
        wait_until(
            lambda: memory.get_message_content(row["message_id"]),
            lambda content: content.get("status") is False,
        )
        assert memory.update_message_status(row["message_id"], True) is True
        content = wait_until(
            lambda: memory.get_message_content(row["message_id"]),
            lambda content: content.get("status") is True,
        )
        assert_content(content, populated_memory["payloads"][0], memory.id)
        assert memory.update_message_status(row["message_id"], True) is True
        assert memory.get_message_content(row["message_id"])["status"] is True

    @pytest.mark.parametrize("status", [None, "false", "true", 0, 1, [], {}])
    def test_status_must_be_boolean(self, populated_memory, status):
        memory = populated_memory["memory"]
        row = populated_memory["rows"][0]
        with pytest.raises(Exception, match=r"^Status must be a boolean\.$"):
            memory.update_message_status(row["message_id"], status)
        content = memory.get_message_content(row["message_id"])
        assert content["status"] is True, content
        assert_content(content, populated_memory["payloads"][0], memory.id)

    @pytest.mark.parametrize("message_id", [-1, "bad", "", "1:2"])
    def test_invalid_message_id(self, populated_memory, message_id):
        with pytest.raises(Exception, match="message_id|formatted as memory_id:message_id"):
            populated_memory["memory"].update_message_status(message_id, False)

    def test_nonexistent_message(self, populated_memory, doc_engine):
        with pytest.raises(Exception, match="Message .*not found"):
            populated_memory["memory"].update_message_status(9223372036854775807, False)

    def test_nonexistent_memory(self, client):
        with pytest.raises(Exception, match="^Resource not found$"):
            Memory(client, {"id": "missing-message-test-memory"}).update_message_status(0, False)
