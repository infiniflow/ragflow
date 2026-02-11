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
            client.list_memory()
        assert str(exception_info.value) == expected_message, str(exception_info.value)


class TestCapability:
    @pytest.mark.p3
    def test_capability(self, client):
        count = 100
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(client.list_memory) for _ in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

@pytest.mark.usefixtures("add_memory_func")
class TestMemoryList:
    @pytest.mark.p2
    def test_params_unset(self, client):
        res  = client.list_memory()
        assert len(res["memory_list"]) == 3, str(res)
        assert res["total_count"] == 3, str(res)

    @pytest.mark.p2
    def test_params_empty(self, client):
        res = client.list_memory(**{})
        assert len(res["memory_list"]) == 3, str(res)
        assert res["total_count"] == 3, str(res)

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
    def test_page(self, client, params, expected_page_size):
        # have added 3 memories in fixture
        res = client.list_memory(**params)
        assert len(res["memory_list"]) == expected_page_size, str(res)
        assert res["total_count"] == 3, str(res)

    @pytest.mark.p2
    def test_filter_memory_type(self, client):
        res = client.list_memory(**{"memory_type": ["semantic"]})
        for memory in res["memory_list"]:
            assert "semantic" in memory.memory_type, str(memory)

    @pytest.mark.p2
    def test_filter_multi_memory_type(self, client):
        res = client.list_memory(**{"memory_type": ["episodic", "procedural"]})
        for memory in res["memory_list"]:
            assert "episodic" in memory.memory_type or "procedural" in memory.memory_type, str(memory)

    @pytest.mark.p2
    def test_filter_storage_type(self, client):
        res = client.list_memory(**{"storage_type": "table"})
        for memory in res["memory_list"]:
            assert memory.storage_type == "table", str(memory)

    @pytest.mark.p2
    def test_match_keyword(self, client):
        res = client.list_memory(**{"keywords": "s"})
        for memory in res["memory_list"]:
            assert "s" in memory.name, str(memory)

    @pytest.mark.p1
    def test_get_config(self, client):
        memory_list = client.list_memory()
        assert len(memory_list["memory_list"]) > 0, str(memory_list)
        memory = memory_list["memory_list"][0]
        memory_id = memory.id
        memory_config = memory.get_config()
        assert memory_config.id == memory_id, memory_config
        for field in ["name", "avatar", "tenant_id", "owner_name", "memory_type", "storage_type",
                      "embd_id", "llm_id", "permissions", "description", "memory_size", "forgetting_policy",
                      "temperature", "system_prompt", "user_prompt"]:
            assert hasattr(memory, field), memory_config
