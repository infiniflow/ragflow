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

from test.testcases.test_sdk_api.test_message_management.message_helpers import assert_identity

pytestmark = pytest.mark.integration


def _assert_messages(result, populated_memory, expected_rows):
    messages = result["messages"]
    rows = messages["message_list"]
    assert result["storage_type"] == populated_memory["memory"].storage_type
    assert messages["total_count"] == len(expected_rows), result
    assert len(rows) == len(expected_rows), result
    assert {str(row["message_id"]) for row in rows} == {str(row["message_id"]) for row in expected_rows}, result
    payloads = {payload["session_id"]: payload for payload in populated_memory["payloads"]}
    for row in rows:
        assert_identity(row, payloads[row["session_id"]], populated_memory["memory"].id)
        assert isinstance(row["extract"], list), row
    timestamps = [row["valid_at"] for row in rows]
    assert timestamps == sorted(timestamps, reverse=True), rows


class TestAuthorization:
    @pytest.mark.parametrize("invalid_auth", [None, "invalid-sdk-message-token"])
    def test_auth_invalid(self, integration_settings, invalid_auth):
        client = RAGFlow(invalid_auth, integration_settings["base_url"])
        with pytest.raises(Exception, match="^Invalid access token$"):
            Memory(client, {"id": "missing-memory"}).list_memory_messages()


class TestMessageList:
    def test_params_unset(self, populated_memory):
        _assert_messages(populated_memory["memory"].list_memory_messages(), populated_memory, populated_memory["rows"])

    @pytest.mark.parametrize(
        "params, expected_page_size",
        [
            ({"page": 1, "page_size": 10}, 5),
            ({"page": 2, "page_size": 10}, 0),
            ({"page": 1, "page_size": 2}, 2),
            ({"page": 3, "page_size": 2}, 1),
            ({"page": 5, "page_size": 10}, 0),
        ],
        ids=["first-full-page", "past-end", "first-small-page", "last-partial-page", "far-past-end"],
    )
    def test_page_size(self, populated_memory, params, expected_page_size):
        memory = populated_memory["memory"]
        result = memory.list_memory_messages(**params)["messages"]
        assert result["total_count"] == 5, result
        assert len(result["message_list"]) == expected_page_size, result
        known = {str(row["message_id"]): row for row in populated_memory["rows"]}
        for row in result["message_list"]:
            assert row["session_id"] == known[str(row["message_id"])]["session_id"], row
            assert row["memory_id"] == memory.id, row
        pages = [memory.list_memory_messages(page=page, page_size=2)["messages"] for page in (1, 2, 3)]
        assert [len(page["message_list"]) for page in pages] == [2, 2, 1], pages
        assert all(page["total_count"] == 5 for page in pages), pages
        rows = [row for page in pages for row in page["message_list"]]
        ids = [str(row["message_id"]) for row in rows]
        assert len(ids) == len(set(ids)) == 5, rows
        assert set(ids) == set(known), rows
        timestamps = [row["valid_at"] for row in rows]
        assert timestamps == sorted(timestamps, reverse=True), rows

    @pytest.mark.parametrize("multiple", [False, True], ids=["scalar", "multiple"])
    def test_filter_agent_id(self, populated_memory, multiple):
        agents = list(dict.fromkeys(row["agent_id"] for row in populated_memory["rows"]))
        selected = agents if multiple else agents[:1]
        result = populated_memory["memory"].list_memory_messages(agent_id=selected if multiple else selected[0])
        expected = [row for row in populated_memory["rows"] if row["agent_id"] in selected]
        _assert_messages(result, populated_memory, expected)

    def test_search_keyword(self, populated_memory):
        expected = populated_memory["rows"][2:3]
        result = populated_memory["memory"].list_memory_messages(keywords=expected[0]["session_id"])
        _assert_messages(result, populated_memory, expected)

    def test_search_keyword_substring(self, populated_memory, doc_engine):
        expected = populated_memory["rows"][2:3]
        keyword = expected[0]["session_id"][4:]
        assert keyword != expected[0]["session_id"]
        result = populated_memory["memory"].list_memory_messages(keywords=keyword)
        _assert_messages(result, populated_memory, expected)

    @pytest.mark.parametrize("filters", [{"agent_id": "missing-agent"}, {"keywords": "missing-session"}])
    def test_unmatched_filter(self, populated_memory, filters):
        result = populated_memory["memory"].list_memory_messages(**filters)
        _assert_messages(result, populated_memory, [])

    def test_combined_filters(self, populated_memory):
        row = populated_memory["rows"][0]
        memory = populated_memory["memory"]
        result = memory.list_memory_messages(agent_id=row["agent_id"], keywords=row["session_id"])
        _assert_messages(result, populated_memory, [row])
        result = memory.list_memory_messages(agent_id=populated_memory["rows"][1]["agent_id"], keywords=row["session_id"])
        _assert_messages(result, populated_memory, [])

    def test_memory_isolation(self, populated_memory, memory_factory):
        result = memory_factory().list_memory_messages()
        assert result["messages"] == {"message_list": [], "total_count": 0}, result
        _assert_messages(populated_memory["memory"].list_memory_messages(), populated_memory, populated_memory["rows"])

    @pytest.mark.parametrize("memory_id", ["missing-memory", "0" * 32])
    def test_nonexistent_memory(self, client, memory_id):
        with pytest.raises(Exception, match="(?i)(not found|does not exist)"):
            Memory(client, {"id": memory_id}).list_memory_messages()

    @pytest.mark.parametrize(
        "params, message",
        [
            ({"page": 0}, "page must be a positive integer"),
            ({"page": -1}, "page must be a positive integer"),
            ({"page": "invalid"}, "page must be a positive integer"),
            ({"page_size": 0}, "page_size must be a positive integer"),
            ({"page_size": -1}, "page_size must be a positive integer"),
            ({"page_size": "invalid"}, "page_size must be a positive integer"),
            ({"page_size": 101}, "page_size must be less than or equal to 100"),
        ],
    )
    def test_invalid_pagination(self, memory_factory, params, message):
        with pytest.raises(Exception, match=f"^{message}$"):
            memory_factory().list_memory_messages(**params)
