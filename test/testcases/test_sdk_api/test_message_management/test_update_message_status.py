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
        client = RAGFlow(invalid_auth, HOST_ADDRESS)
        with pytest.raises(Exception) as exception_info:
            memory = Memory(client, {"id": "empty_memory_id"})
            memory.update_message_status(0, False)
        assert str(exception_info.value) == expected_message, str(exception_info.value)


@pytest.mark.usefixtures("add_memory_with_5_raw_message_func")
class TestUpdateMessageStatus:

    @pytest.mark.p1
    def test_update_to_false(self, client):
        memory_id = self.memory_id
        memory = Memory(client, {"id": memory_id})
        list_res = memory.list_memory_messages()
        assert len(list_res["messages"]["message_list"]) > 0, str(list_res)

        message = random.choice(list_res["messages"]["message_list"])
        res = memory.update_message_status(message["message_id"], False)
        assert res, str(res)

        updated_message_res = memory.get_message_content(message["message_id"])
        assert not updated_message_res["status"], str(updated_message_res)

    @pytest.mark.p1
    def test_update_to_true(self, client):
        memory_id = self.memory_id
        memory = Memory(client, {"id": memory_id})
        list_res = memory.list_memory_messages()
        assert len(list_res["messages"]["message_list"]) > 0, str(list_res)
        # set 1 random message to false first
        message = random.choice(list_res["messages"]["message_list"])
        set_to_false_res = memory.update_message_status(message["message_id"], False)
        assert set_to_false_res, str(set_to_false_res)
        updated_message_res = memory.get_message_content(message["message_id"])
        assert not updated_message_res["status"], updated_message_res
        # set to true
        set_to_true_res = memory.update_message_status(message["message_id"], True)
        assert set_to_true_res, str(set_to_true_res)
        res = memory.get_message_content(message["message_id"])
        assert res["status"], res
