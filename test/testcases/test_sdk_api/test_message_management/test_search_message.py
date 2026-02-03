#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
from ragflow_sdk import RAGFlow, Memory
from configs import INVALID_API_TOKEN, HOST_ADDRESS

class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "invalid_auth, expected_message",
        [
            (None, "<Unauthorized '401: Unauthorized'>"),
            (INVALID_API_TOKEN, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_auth_invalid(self, invalid_auth, expected_message):
        client = RAGFlow(invalid_auth, HOST_ADDRESS)
        with pytest.raises(Exception) as exception_info:
            client.search_message("", ["empty_memory_id"])
        assert str(exception_info.value) == expected_message, str(exception_info.value)


@pytest.mark.usefixtures("add_memory_with_multiple_type_message_func")
class TestSearchMessage:

    @pytest.mark.p1
    def test_query(self, client):
        memory_id = self.memory_id
        memory = Memory(client, {"id": memory_id})
        list_res = memory.list_memory_messages()
        assert list_res["messages"]["total_count"] > 0

        query = "Coriander is a versatile herb with two main edible parts. What's its name can refer to?"
        res = client.search_message(**{"memory_id": [memory_id], "query": query})
        assert len(res) > 0

    @pytest.mark.p2
    def test_query_with_agent_filter(self, client):
        memory_id = self.memory_id
        memory = Memory(client, {"id": memory_id})
        list_res = memory.list_memory_messages()
        assert list_res["messages"]["total_count"] > 0

        agent_id = self.agent_id
        query = "Coriander is a versatile herb with two main edible parts. What's its name can refer to?"
        res = client.search_message(**{"memory_id": [memory_id], "query": query, "agent_id": agent_id})
        assert len(res) > 0
        for message in res:
            assert message["agent_id"] == agent_id, message

    @pytest.mark.p2
    def test_query_with_not_default_params(self, client):
        memory_id = self.memory_id
        memory = Memory(client, {"id": memory_id})
        list_res = memory.list_memory_messages()
        assert list_res["messages"]["total_count"] > 0

        query = "Coriander is a versatile herb with two main edible parts. What's its name can refer to?"
        params = {
            "similarity_threshold": 0.1,
            "keywords_similarity_weight": 0.6,
            "top_n": 4
        }
        res = client.search_message(**{"memory_id": [memory_id], "query": query, **params})
        assert len(res) > 0
        assert len(res) <= params["top_n"]
