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
from common import update_memory
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from utils import encode_avatar
from utils.file_utils import create_image_file
from utils.hypothesis_utils import valid_names


class TestAuthorization:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
        ids=["empty_auth", "invalid_api_token"]
    )
    def test_auth_invalid(self, invalid_auth, expected_code, expected_message):
        res = update_memory(invalid_auth, "memory_id")
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestMemoryUpdate:

    @pytest.mark.p3
    @given(name=valid_names())
    @example("a" * 128)
    def test_name(self, WebApiAuth, add_memory_func, name):
        memory_ids = add_memory_func
        payload = {"name": name}
        res = update_memory(WebApiAuth, memory_ids[0], payload)
        assert res["code"] == 0, res
        assert res["data"]["name"] == name, res

