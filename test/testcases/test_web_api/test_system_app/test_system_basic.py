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
from common import (
    system_config,
    system_delete_token,
    system_new_token,
    system_status,
    system_token_list,
    system_version,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


INVALID_AUTH_CASES = [
    (None, 401, "Unauthorized"),
    (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "Unauthorized"),
]


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_status(self, invalid_auth, expected_code, expected_fragment):
        res = system_status(invalid_auth)
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_version(self, invalid_auth, expected_code, expected_fragment):
        res = system_version(invalid_auth)
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_token_list(self, invalid_auth, expected_code, expected_fragment):
        res = system_token_list(invalid_auth)
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_delete_token(self, invalid_auth, expected_code, expected_fragment):
        res = system_delete_token(invalid_auth, "dummy_token")
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res


class TestSystemConfig:
    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth", [None, RAGFlowWebApiAuth(INVALID_API_TOKEN)])
    def test_config_no_auth_required(self, invalid_auth):
        res = system_config(invalid_auth)
        assert res["code"] == 0, res
        assert "registerEnabled" in res["data"], res


class TestSystemEndpoints:
    @pytest.mark.p2
    def test_status(self, WebApiAuth):
        res = system_status(WebApiAuth)
        assert res["code"] == 0, res
        for key in ["doc_engine", "storage", "database", "redis"]:
            assert key in res["data"], res

    @pytest.mark.p2
    def test_version(self, WebApiAuth):
        res = system_version(WebApiAuth)
        assert res["code"] == 0, res
        assert res["data"], res

    @pytest.mark.p2
    def test_token_list(self, WebApiAuth):
        res = system_token_list(WebApiAuth)
        assert res["code"] == 0, res
        assert isinstance(res["data"], list), res

    @pytest.mark.p2
    def test_delete_token(self, WebApiAuth):
        create_res = system_new_token(WebApiAuth)
        assert create_res["code"] == 0, create_res
        token = create_res["data"]["token"]

        delete_res = system_delete_token(WebApiAuth, token)
        assert delete_res["code"] == 0, delete_res
        assert delete_res["data"] is True, delete_res

    @pytest.mark.p3
    def test_delete_missing_token(self, WebApiAuth):
        res = system_delete_token(WebApiAuth, "missing_token")
        assert res["code"] == 0, res
        assert res["data"] is True, res
