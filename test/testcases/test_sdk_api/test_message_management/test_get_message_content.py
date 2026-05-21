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
        client = RAGFlow(INVALID_API_TOKEN, HOST_ADDRESS)
        with pytest.raises(Exception) as exception_info:
            memory = Memory(client, {"id": "empty_memory_id"})
            memory.get_message_content(0)
        assert str(exception_info.value) == expected_message, str(exception_info.value)


@pytest.mark.usefixtures("add_memory_with_multiple_type_message_func")
class TestGetMessageContent:

    @pytest.mark.p1
    def test_get_message_content(self,client):
        memory_id = self.memory_id
        recent_messages = client.get_recent_messages([memory_id])
        assert len(recent_messages) > 0, recent_messages
        message = random.choice(recent_messages)
        message_id = message["message_id"]
        memory = Memory(client, {"id": memory_id})
        content_res = memory.get_message_content(message_id)
        for field in ["content", "content_embed"]:
            assert field in content_res
            assert content_res[field] is not None, content_res
