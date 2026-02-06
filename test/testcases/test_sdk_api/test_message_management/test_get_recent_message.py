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
import random

import pytest
from ragflow_sdk import RAGFlow
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
            client.get_recent_messages(["some_memory_id"])
        assert str(exception_info.value) == expected_message, str(exception_info.value)


@pytest.mark.usefixtures("add_memory_with_5_raw_message_func")
class TestGetRecentMessage:

    @pytest.mark.p1
    def test_get_recent_messages(self, client):
        memory_id = self.memory_id
        res = client.get_recent_messages([memory_id])
        assert len(res) == 5, res

    @pytest.mark.p2
    def test_filter_recent_messages_by_agent(self, client):
        memory_id = self.memory_id
        agent_ids = self.agent_ids
        agent_id = random.choice(agent_ids)
        res = client.get_recent_messages(**{"agent_id": agent_id, "memory_id": [memory_id]})
        for message in res:
            assert message["agent_id"] == agent_id, message

    @pytest.mark.p2
    def test_filter_recent_messages_by_session(self, client):
        memory_id = self.memory_id
        session_ids = self.session_ids
        session_id = random.choice(session_ids)
        res = client.get_recent_messages(**{"session_id": session_id, "memory_id": [memory_id]})
        for message in res:
            assert message["session_id"] == session_id, message
