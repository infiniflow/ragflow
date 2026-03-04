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
from test_web_api.common import search_message, list_memory_message
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
        res = search_message(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


@pytest.mark.usefixtures("add_memory_with_multiple_type_message_func")
class TestSearchMessage:

    @pytest.mark.p1
    def test_query(self, WebApiAuth):
        memory_id = self.memory_id
        list_res = list_memory_message(WebApiAuth, memory_id)
        assert list_res["code"] == 0, list_res
        assert list_res["data"]["messages"]["total_count"] > 0

        query = "Coriander is a versatile herb with two main edible parts. What's its name can refer to?"
        res = search_message(WebApiAuth, {"memory_id": memory_id, "query": query})
        assert res["code"] == 0, res
        assert len(res["data"]) > 0

    @pytest.mark.p2
    def test_query_with_agent_filter(self, WebApiAuth):
        memory_id = self.memory_id
        list_res = list_memory_message(WebApiAuth, memory_id)
        assert list_res["code"] == 0, list_res
        assert list_res["data"]["messages"]["total_count"] > 0

        agent_id = self.agent_id
        query = "Coriander is a versatile herb with two main edible parts. What's its name can refer to?"
        res = search_message(WebApiAuth, {"memory_id": memory_id, "query": query, "agent_id": agent_id})
        assert res["code"] == 0, res
        assert len(res["data"]) > 0
        for message in res["data"]:
            assert message["agent_id"] == agent_id, message

    @pytest.mark.p2
    def test_query_with_not_default_params(self, WebApiAuth):
        memory_id = self.memory_id
        list_res = list_memory_message(WebApiAuth, memory_id)
        assert list_res["code"] == 0, list_res
        assert list_res["data"]["messages"]["total_count"] > 0

        query = "Coriander is a versatile herb with two main edible parts. What's its name can refer to?"
        params = {
            "similarity_threshold": 0.1,
            "keywords_similarity_weight": 0.6,
            "top_n": 4
        }
        res = search_message(WebApiAuth, {"memory_id": memory_id, "query": query, **params})
        assert res["code"] == 0, res
        assert len(res["data"]) > 0
        assert len(res["data"]) <= params["top_n"]
