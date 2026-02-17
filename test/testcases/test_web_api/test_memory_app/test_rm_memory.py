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
from test_web_api.common import (list_memory, delete_memory)
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
        res = delete_memory(invalid_auth, "some_memory_id")
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestMemoryDelete:
    @pytest.mark.p1
    def test_memory_id(self, WebApiAuth, add_memory_func):
        memory_ids = add_memory_func
        res = delete_memory(WebApiAuth, memory_ids[0])
        assert res["code"] == 0, res

        res = list_memory(WebApiAuth)
        assert res["data"]["total_count"] == 2, res

    @pytest.mark.p2
    @pytest.mark.usefixtures("add_memory_func")
    def test_id_wrong_uuid(self, WebApiAuth):
        res = delete_memory(WebApiAuth, "d94a8dc02c9711f0930f7fbc369eab6d")
        assert res["code"] == 404, res

        res = list_memory(WebApiAuth)
        assert len(res["data"]["memory_list"]) == 3, res
