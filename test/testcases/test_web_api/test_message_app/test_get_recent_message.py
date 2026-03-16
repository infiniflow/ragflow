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
from test_web_api.common import get_recent_message
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
        res = get_recent_message(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


@pytest.mark.usefixtures("add_memory_with_5_raw_message_func")
class TestGetRecentMessage:

    @pytest.mark.p1
    def test_get_recent_messages(self, WebApiAuth):
        memory_id = self.memory_id
        res = get_recent_message(WebApiAuth, params={"memory_id": memory_id})
        assert res["code"] == 0, res
        assert len(res["data"]) == 5, res

    @pytest.mark.p2
    def test_filter_recent_messages_by_agent(self, WebApiAuth):
        memory_id = self.memory_id
        agent_ids = self.agent_ids
        agent_id = random.choice(agent_ids)
        res = get_recent_message(WebApiAuth, params={"agent_id": agent_id, "memory_id": memory_id})
        assert res["code"] == 0, res
        for message in res["data"]:
            assert message["agent_id"] == agent_id, message

    @pytest.mark.p2
    def test_filter_recent_messages_by_session(self, WebApiAuth):
        memory_id = self.memory_id
        session_ids = self.session_ids
        session_id = random.choice(session_ids)
        res = get_recent_message(WebApiAuth, params={"session_id": session_id, "memory_id": memory_id})
        assert res["code"] == 0, res
        for message in res["data"]:
            assert message["session_id"] == session_id, message

