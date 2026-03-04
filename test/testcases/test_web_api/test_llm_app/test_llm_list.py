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
from common import llm_factories, llm_list
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


INVALID_AUTH_CASES = [
    (None, 401, "<Unauthorized '401: Unauthorized'>"),
    (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
]


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_message", INVALID_AUTH_CASES)
    def test_auth_invalid_factories(self, invalid_auth, expected_code, expected_message):
        res = llm_factories(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_message", INVALID_AUTH_CASES)
    def test_auth_invalid_list(self, invalid_auth, expected_code, expected_message):
        res = llm_list(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestLLMList:
    @pytest.mark.p1
    def test_factories(self, WebApiAuth):
        res = llm_factories(WebApiAuth)
        assert res["code"] == 0, res
        assert isinstance(res["data"], list), res

    @pytest.mark.p1
    def test_list(self, WebApiAuth):
        res = llm_list(WebApiAuth)
        assert res["code"] == 0, res
        assert isinstance(res["data"], dict), res
