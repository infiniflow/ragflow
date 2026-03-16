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
import os
import random

import pytest
from test_web_api.common import list_memory_message
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
        res = list_memory_message(invalid_auth, "")
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


@pytest.mark.usefixtures("add_memory_with_5_raw_message_func")
class TestMessageList:

    @pytest.mark.p2
    def test_params_unset(self, WebApiAuth):
        memory_id = self.memory_id
        res = list_memory_message(WebApiAuth, memory_id, params=None)
        assert res["code"] == 0, res
        assert len(res["data"]["messages"]["message_list"]) == 5, res

    @pytest.mark.p2
    def test_params_empty(self, WebApiAuth):
        memory_id = self.memory_id
        res = list_memory_message(WebApiAuth, memory_id, params={})
        assert res["code"] == 0, res
        assert len(res["data"]["messages"]["message_list"]) == 5, res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size",
        [
            ({"page": 1, "page_size": 10}, 5),
            ({"page": 2, "page_size": 10}, 0),
            ({"page": 1, "page_size": 2}, 2),
            ({"page": 3, "page_size": 2}, 1),
            ({"page": 5, "page_size": 10}, 0),
        ],
        ids=["normal_first_page", "beyond_max_page", "normal_last_partial_page", "normal_middle_page",
             "full_data_single_page"],
    )
    def test_page_size(self, WebApiAuth, params, expected_page_size):
        # have added 5 messages in fixture
        memory_id = self.memory_id
        res = list_memory_message(WebApiAuth, memory_id, params=params)
        assert res["code"] == 0, res
        assert len(res["data"]["messages"]["message_list"]) == expected_page_size, res

    @pytest.mark.p2
    def test_filter_agent_id(self, WebApiAuth):
        memory_id = self.memory_id
        agent_ids = self.agent_ids
        agent_id = random.choice(agent_ids)
        res = list_memory_message(WebApiAuth, memory_id, params={"agent_id": agent_id})
        assert res["code"] == 0, res
        for message in res["data"]["messages"]["message_list"]:
            assert message["agent_id"] == agent_id, message

    @pytest.mark.p2
    @pytest.mark.skipif(os.getenv("DOC_ENGINE") == "infinity", reason="Not support.")
    def test_search_keyword(self, WebApiAuth):
        memory_id = self.memory_id
        session_ids = self.session_ids
        session_id = random.choice(session_ids)
        slice_start = random.randint(0, len(session_id) - 2)
        slice_end = random.randint(slice_start + 1, len(session_id) - 1)
        keyword = session_id[slice_start:slice_end]
        res = list_memory_message(WebApiAuth, memory_id, params={"keywords": keyword})
        assert res["code"] == 0, res
        assert len(res["data"]["messages"]["message_list"]) > 0, res
        for message in res["data"]["messages"]["message_list"]:
            assert keyword in message["session_id"], message
