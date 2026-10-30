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


def assert_matches(messages, populated, indices):
    """Check exact identities and stored conversation content without ranking ties."""
    assert isinstance(messages, list), messages
    expected = {str(populated["rows"][i]["message_id"]): populated["payloads"][i] for i in indices}
    assert len(messages) == len(expected), messages
    assert {str(row["message_id"]) for row in messages} == set(expected), messages
    for row in messages:
        payload = expected[str(row["message_id"])]
        assert_identity(row, payload, populated["memory"].id)
        assert row["status"] is True, row
        assert row["content"] == f"User Input: {payload['user_input']}\nAgent Response: {payload['agent_response']}", row


@pytest.mark.integration
class TestAuthorization:
    @pytest.mark.parametrize("invalid_auth", [None, "invalid_key_123"])
    def test_auth_invalid(self, integration_settings, invalid_auth):
        client = RAGFlow(invalid_auth, integration_settings["base_url"])
        with pytest.raises(Exception, match="^Invalid access token$"):
            client.search_message("coriander", ["missing-memory"])


@pytest.mark.integration
class TestSearchMessage:
    def test_query(self, client, populated_memory):
        # Disjoint topics make query-ignoring retrieval fail without ordering tied matches.
        for query, relevant_indices in [("coriander cilantro leaves seeds", [0, 1]), ("Mount Everest Himalayas mountain", [4])]:
            relevant_ids = {str(populated_memory["rows"][i]["message_id"]) for i in relevant_indices}
            messages = wait_until(
                lambda: client.search_message(query, [populated_memory["memory"].id], top_n=1),
                lambda rows: len(rows) == 1 and str(rows[0]["message_id"]) in relevant_ids,
            )
            indices = [i for i in relevant_indices if str(populated_memory["rows"][i]["message_id"]) == str(messages[0]["message_id"])]
            assert_matches(messages, populated_memory, indices)

    def test_query_with_agent_filter(self, client, populated_memory):
        messages = wait_until(
            lambda: client.search_message(
                "coriander cilantro leaves",
                [populated_memory["memory"].id],
                agent_id=populated_memory["payloads"][0]["agent_id"],
                top_n=3,
            ),
            lambda rows: str(populated_memory["rows"][0]["message_id"]) in {str(row["message_id"]) for row in rows},
        )
        indices = [i for i in (0, 2, 4) if str(populated_memory["rows"][i]["message_id"]) in {str(row["message_id"]) for row in messages}]
        assert_matches(messages, populated_memory, indices)

    def test_query_with_not_default_params(self, client, populated_memory):
        messages = wait_until(
            lambda: client.search_message(
                "coriander cilantro leaves seeds",
                [populated_memory["memory"].id],
                similarity_threshold=0.1,
                keywords_similarity_weight=0.6,
                top_n=4,
            ),
            lambda rows: str(populated_memory["rows"][0]["message_id"]) in {str(row["message_id"]) for row in rows},
        )
        assert len(messages) <= 4
        indices = [i for i, row in enumerate(populated_memory["rows"]) if str(row["message_id"]) in {str(message["message_id"]) for message in messages}]
        assert_matches(messages, populated_memory, indices)

    @pytest.mark.parametrize(
        "field,index,expected",
        [("agent_id", 0, [0, 2, 4]), ("agent_id", 1, [1, 3]), ("session_id", 0, [0]), ("session_id", 4, [4]), ("user_id", 0, [0, 2, 4]), ("user_id", 1, [1, 3])],
    )
    def test_exact_filters(self, client, populated_memory, field, index, expected):
        # Go's TestSearchMessageFiltersAccessibleMemoryAndDefaultsStatus explicitly supports blank queries.
        messages = wait_until(
            lambda: client.search_message(
                "",
                [populated_memory["memory"].id],
                **{field: populated_memory["payloads"][index][field]},
            ),
            lambda rows: len(rows) == len(expected),
        )
        assert_matches(messages, populated_memory, expected)

    def test_combined_filters(self, client, populated_memory):
        payload = populated_memory["payloads"][1]
        filters = {field: payload[field] for field in ("agent_id", "session_id", "user_id")}
        messages = wait_until(
            lambda: client.search_message("", [populated_memory["memory"].id], **filters),
            lambda rows: len(rows) == 1,
        )
        assert_matches(messages, populated_memory, [1])
        filters["agent_id"] = populated_memory["payloads"][0]["agent_id"]
        assert client.search_message("", [populated_memory["memory"].id], **filters) == []

    @pytest.mark.parametrize("field", ["agent_id", "session_id", "user_id"])
    def test_unknown_filter(self, client, populated_memory, field):
        assert client.search_message("", [populated_memory["memory"].id], **{field: "missing-filter"}) == []

    @pytest.mark.parametrize("top_n", [1, 3, 5, 100])
    def test_top_n(self, client, populated_memory, top_n):
        messages = wait_until(
            lambda: client.search_message("", [populated_memory["memory"].id], top_n=top_n),
            lambda rows: len(rows) == min(top_n, 5),
        )
        assert len({str(row["message_id"]) for row in messages}) == min(top_n, 5), messages
        indices = [i for i, row in enumerate(populated_memory["rows"]) if str(row["message_id"]) in {str(message["message_id"]) for message in messages}]
        assert_matches(messages, populated_memory, indices)

    @pytest.mark.parametrize("top_n", [0, -1, 101, "invalid", 1.5])
    def test_invalid_top_n(self, client, top_n):
        with pytest.raises(Exception, match="^top_n must be between 1 and 100$"):
            client.search_message("", ["missing-memory"], top_n=top_n)

    @pytest.mark.parametrize("field", ["similarity_threshold", "keywords_similarity_weight"])
    @pytest.mark.parametrize("value", [0.0, 1.0])
    def test_similarity_parameter_bounds(self, client, populated_memory, field, value):
        messages = wait_until(
            lambda: client.search_message("", [populated_memory["memory"].id], **{field: value}),
            lambda rows: len(rows) == 5,
        )
        assert_matches(messages, populated_memory, range(5))

    @pytest.mark.parametrize("field", ["similarity_threshold", "keywords_similarity_weight"])
    @pytest.mark.parametrize("value", [-0.1, 1.1, "invalid", "nan", "inf"])
    def test_invalid_similarity_parameters(self, client, populated_memory, field, value):
        response = client.get(
            "/messages/search",
            {
                "query": "",
                "memory_id": [populated_memory["memory"].id],
                field: value,
            },
        ).json()
        assert response["code"] == 101, response
        with pytest.raises(Exception):
            client.search_message("", [populated_memory["memory"].id], **{field: value})

    def test_inactive_and_forgotten_messages_are_excluded(self, client, populated_memory):
        memory = populated_memory["memory"]
        messages = wait_until(lambda: client.search_message("", [memory.id]), lambda rows: len(rows) == 5)
        assert_matches(messages, populated_memory, range(5))
        assert memory.update_message_status(populated_memory["rows"][0]["message_id"], False) is True
        messages = wait_until(lambda: client.search_message("", [memory.id]), lambda rows: len(rows) == 4)
        assert_matches(messages, populated_memory, [1, 2, 3, 4])
        assert memory.forget_message(populated_memory["rows"][1]["message_id"]) is True
        messages = wait_until(lambda: client.search_message("", [memory.id]), lambda rows: len(rows) == 3)
        assert_matches(messages, populated_memory, [2, 3, 4])

    def test_memory_isolation_and_multiple_memories(self, client, populated_memory, memory_factory):
        other = memory_factory()
        assert client.search_message("", [other.id]) == []
        payload = dict(populated_memory["payloads"][0], memory_id=[other.id], session_id=f"{other.id}-session")
        assert client.add_message(**payload) == "All add to task."
        result = wait_until(other.list_memory_messages, lambda value: value["messages"]["total_count"] == 1)
        other_row = result["messages"]["message_list"][0]
        assert_identity(other_row, payload, other.id)
        messages = wait_until(lambda: client.search_message("", [other.id]), lambda rows: len(rows) == 1)
        assert_identity(messages[0], payload, other.id)
        assert str(messages[0]["message_id"]) == str(other_row["message_id"]), messages
        expected_content = f"User Input: {payload['user_input']}\nAgent Response: {payload['agent_response']}"
        assert messages[0]["content"] == expected_content, messages
        messages = wait_until(
            lambda: client.search_message("", [populated_memory["memory"].id, other.id]),
            lambda rows: len(rows) == 6,
        )
        assert_matches([row for row in messages if row["memory_id"] == populated_memory["memory"].id], populated_memory, range(5))
        others = [row for row in messages if row["memory_id"] == other.id]
        assert len(others) == 1, messages
        assert_identity(others[0], payload, other.id)
        assert str(others[0]["message_id"]) == str(other_row["message_id"]), others
        assert others[0]["content"] == expected_content, others
        assert_matches(client.search_message("", [populated_memory["memory"].id]), populated_memory, range(5))

    @pytest.mark.parametrize("memory_ids", [[], [""], ["missing-memory"], ["../invalid-memory"]])
    def test_empty_or_inaccessible_memories(self, client, memory_ids):
        # Search intentionally returns no information about inaccessible memory IDs.
        assert client.search_message("coriander", memory_ids) == []
