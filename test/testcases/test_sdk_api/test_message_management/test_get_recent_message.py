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
from ragflow_sdk import RAGFlow

from test.testcases.test_sdk_api.test_message_management.message_helpers import assert_identity, wait_until

pytestmark = pytest.mark.integration


def _assert_messages(messages, populated_memory, expected_rows):
    assert isinstance(messages, list), messages
    assert len(messages) == len(expected_rows), messages
    assert {str(row["message_id"]) for row in messages} == {str(row["message_id"]) for row in expected_rows}, messages
    payloads = {payload["session_id"]: payload for payload in populated_memory["payloads"]}
    for row in messages:
        payload = payloads[row["session_id"]]
        assert_identity(row, payload, populated_memory["memory"].id)
        assert row["content"] == f"User Input: {payload['user_input']}\nAgent Response: {payload['agent_response']}", row
    timestamps = [row["valid_at"] for row in messages]
    assert timestamps == sorted(timestamps, reverse=True), messages


class TestAuthorization:
    @pytest.mark.parametrize("invalid_auth", [None, "invalid-sdk-message-token"])
    def test_auth_invalid(self, integration_settings, invalid_auth):
        client = RAGFlow(invalid_auth, integration_settings["base_url"])
        with pytest.raises(Exception, match="^Invalid access token$"):
            client.get_recent_messages(["missing-memory"])


class TestGetRecentMessage:
    def test_get_recent_messages(self, client, populated_memory):
        result = client.get_recent_messages([populated_memory["memory"].id])
        _assert_messages(result, populated_memory, populated_memory["rows"])

    def test_filter_recent_messages_by_agent(self, client, populated_memory):
        agent_id = populated_memory["rows"][0]["agent_id"]
        result = client.get_recent_messages([populated_memory["memory"].id], agent_id=agent_id)
        expected = [row for row in populated_memory["rows"] if row["agent_id"] == agent_id]
        _assert_messages(result, populated_memory, expected)

    def test_filter_recent_messages_by_session(self, client, populated_memory):
        expected = populated_memory["rows"][2:3]
        result = client.get_recent_messages([populated_memory["memory"].id], session_id=expected[0]["session_id"])
        _assert_messages(result, populated_memory, expected)

    @pytest.mark.parametrize("limit", [1, 3, 5, 10, 100])
    def test_limit(self, client, populated_memory, limit):
        result = client.get_recent_messages([populated_memory["memory"].id], limit=limit)
        known = {str(row["message_id"]): row for row in populated_memory["rows"]}
        assert len(result) == min(limit, 5), result
        assert len({str(row["message_id"]) for row in result}) == len(result), result
        expected = [known[str(row["message_id"])] for row in result]
        _assert_messages(result, populated_memory, expected)
        # Equal timestamps do not guarantee a stable order of message IDs.
        assert [row["valid_at"] for row in result] == sorted((row["valid_at"] for row in populated_memory["rows"]), reverse=True)[:limit], result

    @pytest.mark.parametrize("filters", [{"agent_id": "missing-agent"}, {"session_id": "missing-session"}])
    def test_unmatched_filter(self, client, populated_memory, filters):
        assert client.get_recent_messages([populated_memory["memory"].id], **filters) == []

    def test_combined_filters(self, client, populated_memory):
        row = populated_memory["rows"][0]
        memory_id = populated_memory["memory"].id
        result = client.get_recent_messages([memory_id], agent_id=row["agent_id"], session_id=row["session_id"])
        _assert_messages(result, populated_memory, [row])
        assert client.get_recent_messages([memory_id], agent_id=populated_memory["rows"][1]["agent_id"], session_id=row["session_id"]) == []

    def test_memory_isolation_and_multiple_memories(self, client, populated_memory, memory_factory):
        other = memory_factory()
        assert client.get_recent_messages([other.id]) == []
        payload = dict(populated_memory["payloads"][0], memory_id=[other.id], session_id=f"{other.id}-session")
        assert client.add_message(**payload) == "All add to task."
        other_rows = wait_until(lambda: client.get_recent_messages([other.id]), lambda rows: len(rows) == 1)
        assert_identity(other_rows[0], payload, other.id)
        assert other_rows[0]["content"] == f"User Input: {payload['user_input']}\nAgent Response: {payload['agent_response']}"
        result = client.get_recent_messages([populated_memory["memory"].id])
        _assert_messages(result, populated_memory, populated_memory["rows"])
        result = client.get_recent_messages([populated_memory["memory"].id, other.id])
        assert len(result) == 6, result
        assert {(row["memory_id"], str(row["message_id"])) for row in result} == {(row["memory_id"], str(row["message_id"])) for row in populated_memory["rows"] + other_rows}, result
        timestamps = [row["valid_at"] for row in result]
        assert timestamps == sorted(timestamps, reverse=True), result
        own = [row for row in result if row["memory_id"] == populated_memory["memory"].id]
        _assert_messages(own, populated_memory, populated_memory["rows"])

    @pytest.mark.parametrize("memory_id", ["missing-memory", "0" * 32])
    def test_nonexistent_memory(self, client, memory_id):
        assert client.get_recent_messages([memory_id]) == []

    def test_empty_memory_ids(self, client):
        with pytest.raises(Exception, match="^memory_ids is required[.]$"):
            client.get_recent_messages([])

    @pytest.mark.parametrize("limit", [0, -1, 101, "invalid"])
    def test_invalid_limit(self, client, memory_factory, limit):
        with pytest.raises(Exception, match="^limit must be between 1 and 100$"):
            client.get_recent_messages([memory_factory().id], limit=limit)
