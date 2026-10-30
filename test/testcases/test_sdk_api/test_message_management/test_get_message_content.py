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
import math

import pytest
from ragflow_sdk import Memory, RAGFlow

from test.testcases.test_sdk_api.test_message_management.message_helpers import assert_content

pytestmark = pytest.mark.integration


class TestAuthorization:
    @pytest.mark.parametrize("invalid_auth", [None, "invalid-token"])
    def test_auth_invalid(self, populated_memory, integration_settings, invalid_auth):
        memory = populated_memory["memory"]
        rag = RAGFlow(invalid_auth, integration_settings["base_url"])
        unauthorized = Memory(rag, {"id": memory.id})
        with pytest.raises(Exception, match="^Invalid access token$"):
            unauthorized.get_message_content(populated_memory["rows"][0]["message_id"])


class TestGetMessageContent:
    def test_get_message_content(self, populated_memory):
        memory = populated_memory["memory"]
        for row, payload in zip(populated_memory["rows"], populated_memory["payloads"], strict=True):
            content = memory.get_message_content(row["message_id"])
            assert content["message_id"] == row["message_id"], content
            assert_content(content, payload, memory.id)
            assert content["status"] is True, content

    def test_get_message_content_embedding(self, populated_memory, doc_engine):
        memory = populated_memory["memory"]
        content = memory.get_message_content(populated_memory["rows"][0]["message_id"])
        vector = content["content_embed"]
        assert isinstance(vector, list) and vector, content
        assert all(type(value) in (int, float) and math.isfinite(value) for value in vector), content

    @pytest.mark.parametrize("message_id", [-1, "bad", "", "1:2"])
    def test_invalid_message_id(self, populated_memory, message_id):
        with pytest.raises(Exception, match="message_id|formatted as memory_id:message_id"):
            populated_memory["memory"].get_message_content(message_id)

    def test_nonexistent_message(self, populated_memory):
        with pytest.raises(Exception, match="Message .*not found"):
            populated_memory["memory"].get_message_content(9223372036854775807)

    def test_memory_isolation(self, populated_memory, memory_factory):
        other = memory_factory()
        row = populated_memory["rows"][0]
        with pytest.raises(Exception, match="Message .*not found"):
            other.get_message_content(row["message_id"])
        original = populated_memory["memory"].get_message_content(row["message_id"])
        assert_content(original, populated_memory["payloads"][0], populated_memory["memory"].id)

    def test_nonexistent_memory(self, client):
        with pytest.raises(Exception, match="^Resource not found$"):
            Memory(client, {"id": "missing-message-test-memory"}).get_message_content(0)
