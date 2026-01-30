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
from test_web_api.common import get_message_content, get_recent_message
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
        res = get_message_content(invalid_auth, "empty_memory_id", 0)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


@pytest.mark.usefixtures("add_memory_with_multiple_type_message_func")
class TestGetMessageContent:

    @pytest.mark.p1
    def test_get_message_content(self, WebApiAuth):
        memory_id = self.memory_id
        recent_messages = get_recent_message(WebApiAuth, {"memory_id": memory_id})
        assert len(recent_messages["data"]) > 0, recent_messages
        message = random.choice(recent_messages["data"])
        message_id = message["message_id"]
        content_res = get_message_content(WebApiAuth, memory_id, message_id)
        for field in ["content", "content_embed"]:
            assert field in content_res["data"]
            assert content_res["data"][field] is not None, content_res
