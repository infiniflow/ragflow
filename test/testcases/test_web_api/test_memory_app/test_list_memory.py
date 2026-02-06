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
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from test_web_api.common import list_memory, get_memory_config
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
        res = list_memory(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestCapability:
    @pytest.mark.p3
    def test_capability(self, WebApiAuth):
        count = 100
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(list_memory, WebApiAuth) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

@pytest.mark.usefixtures("add_memory_func")
class TestMemoryList:
    @pytest.mark.p2
    def test_params_unset(self, WebApiAuth):
        res  = list_memory(WebApiAuth, None)
        assert res["code"] == 0, res

    @pytest.mark.p2
    def test_params_empty(self, WebApiAuth):
        res = list_memory(WebApiAuth, {})
        assert res["code"] == 0, res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size",
        [
            ({"page": 1, "page_size": 10}, 3),
            ({"page": 2, "page_size": 10}, 0),
            ({"page": 1, "page_size": 2}, 2),
            ({"page": 2, "page_size": 2}, 1),
            ({"page": 5, "page_size": 10}, 0),
        ],
        ids=["normal_first_page", "beyond_max_page", "normal_last_partial_page" , "normal_middle_page",
             "full_data_single_page"],
    )
    def test_page(self, WebApiAuth, params, expected_page_size):
        # have added 3 memories in fixture
        res = list_memory(WebApiAuth, params)
        assert res["code"] == 0, res
        assert len(res["data"]["memory_list"]) == expected_page_size, res

    @pytest.mark.p2
    def test_filter_memory_type(self, WebApiAuth):
        res = list_memory(WebApiAuth, {"memory_type": ["semantic"]})
        assert res["code"] == 0, res
        for memory in res["data"]["memory_list"]:
            assert "semantic" in memory["memory_type"], res

    @pytest.mark.p2
    def test_filter_multi_memory_type(self, WebApiAuth):
        res = list_memory(WebApiAuth, {"memory_type": ["episodic", "procedural"]})
        assert res["code"] == 0, res
        for memory in res["data"]["memory_list"]:
            assert "episodic" in memory["memory_type"] or "procedural" in memory["memory_type"], res

    @pytest.mark.p2
    def test_filter_storage_type(self, WebApiAuth):
        res = list_memory(WebApiAuth, {"storage_type": "table"})
        assert res["code"] == 0, res
        for memory in res["data"]["memory_list"]:
            assert memory["storage_type"] == "table", res

    @pytest.mark.p2
    def test_match_keyword(self, WebApiAuth):
        res = list_memory(WebApiAuth, {"keywords": "s"})
        assert res["code"] == 0, res
        for memory in res["data"]["memory_list"]:
            assert "s" in memory["name"], res

    @pytest.mark.p1
    def test_get_config(self, WebApiAuth):
        memory_list = list_memory(WebApiAuth, {})
        assert memory_list["code"] == 0, memory_list

        memory_config = get_memory_config(WebApiAuth, memory_list["data"]["memory_list"][0]["id"])
        assert memory_config["code"] == 0, memory_config
        assert memory_config["data"]["id"] == memory_list["data"]["memory_list"][0]["id"], memory_config
        for field in ["name", "avatar", "tenant_id", "owner_name", "memory_type", "storage_type",
                      "embd_id", "llm_id", "permissions", "description", "memory_size", "forgetting_policy",
                      "temperature", "system_prompt", "user_prompt"]:
            assert field in memory_config["data"], memory_config
