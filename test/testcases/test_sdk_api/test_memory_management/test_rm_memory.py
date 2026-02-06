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
            client.delete_memory("some_memory_id")
        assert str(exception_info.value) == expected_message, str(exception_info.value)


@pytest.mark.usefixtures("add_memory_func")
class TestMemoryDelete:
    @pytest.mark.p1
    def test_memory_id(self, client):
        memory_ids = self.memory_ids
        client.delete_memory(memory_ids[0])
        res = client.list_memory()
        assert res["total_count"] == 2, res

    @pytest.mark.p2
    def test_id_wrong_uuid(self, client):
        with pytest.raises(Exception) as exception_info:
            client.delete_memory("d94a8dc02c9711f0930f7fbc369eab6d")
        assert exception_info.value, str(exception_info.value)

        res = client.list_memory()
        assert len(res["memory_list"]) == 2, res
