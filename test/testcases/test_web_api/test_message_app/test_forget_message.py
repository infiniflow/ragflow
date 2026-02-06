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
from test_web_api.common import forget_message, list_memory_message, get_message_content
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_auth_invalid(self, invalid_auth, expected_code, expected_message):
        res = forget_message(invalid_auth, "empty_memory_id", 0)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


@pytest.mark.usefixtures("add_memory_with_5_raw_message_func")
class TestForgetMessage:

    @pytest.mark.p1
    def test_forget_message(self, WebApiAuth):
        memory_id = self.memory_id
        list_res = list_memory_message(WebApiAuth, memory_id)
        assert list_res["code"] == 0, list_res
        assert len(list_res["data"]["messages"]["message_list"]) > 0

        message = random.choice(list_res["data"]["messages"]["message_list"])
        res = forget_message(WebApiAuth, memory_id, message["message_id"])
        assert res["code"] == 0, res

        forgot_message_res = get_message_content(WebApiAuth, memory_id, message["message_id"])
        assert forgot_message_res["code"] == 0, forgot_message_res
        assert forgot_message_res["data"]["forget_at"] not in ["-", ""], forgot_message_res
