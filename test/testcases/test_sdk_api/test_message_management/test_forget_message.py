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
        "invalid_auth,  expected_message",
        [
            (None, "<Unauthorized '401: Unauthorized'>"),
            (INVALID_API_TOKEN, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_auth_invalid(self, invalid_auth, expected_message):
        client = RAGFlow(invalid_auth, HOST_ADDRESS)
        with pytest.raises(Exception) as exception_info:
            memory = Memory(client, {"id": "empty_memory_id"})
            memory.forget_message(0)
        assert str(exception_info.value) == expected_message, str(exception_info.value)


@pytest.mark.usefixtures("add_memory_with_5_raw_message_func")
class TestForgetMessage:

    @pytest.mark.p1
    def test_forget_message(self, client):
        memory_id = self.memory_id
        memory = Memory(client, {"id": memory_id})
        list_res = memory.list_memory_messages()
        assert len(list_res["messages"]["message_list"]) > 0

        message = random.choice(list_res["messages"]["message_list"])
        res = memory.forget_message(message["message_id"])
        assert res, str(res)

        forgot_message_res = memory.get_message_content(message["message_id"])
        assert forgot_message_res["forget_at"] not in ["-", ""], forgot_message_res
