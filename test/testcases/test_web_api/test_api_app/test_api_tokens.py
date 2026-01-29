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
from common import api_new_token, api_rm_token, api_stats, api_token_list, batch_create_dialogs
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


INVALID_AUTH_CASES = [
    (None, 401, "Unauthorized"),
    (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "Unauthorized"),
]


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_new_token(self, invalid_auth, expected_code, expected_fragment):
        res = api_new_token(invalid_auth, {"dialog_id": "dummy_dialog_id"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_token_list(self, invalid_auth, expected_code, expected_fragment):
        res = api_token_list(invalid_auth, {"dialog_id": "dummy_dialog_id"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_rm(self, invalid_auth, expected_code, expected_fragment):
        res = api_rm_token(invalid_auth, {"tokens": ["dummy_token"], "tenant_id": "dummy_tenant"})
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res

    @pytest.mark.p2
    @pytest.mark.parametrize("invalid_auth, expected_code, expected_fragment", INVALID_AUTH_CASES)
    def test_auth_invalid_stats(self, invalid_auth, expected_code, expected_fragment):
        res = api_stats(invalid_auth)
        assert res["code"] == expected_code, res
        assert expected_fragment in res["message"], res


@pytest.mark.usefixtures("clear_dialogs")
class TestApiTokens:
    @pytest.mark.p2
    def test_token_lifecycle(self, WebApiAuth):
        dialog_id = batch_create_dialogs(WebApiAuth, 1)[0]
        create_res = api_new_token(WebApiAuth, {"dialog_id": dialog_id})
        assert create_res["code"] == 0, create_res
        token = create_res["data"]["token"]
        tenant_id = create_res["data"]["tenant_id"]

        list_res = api_token_list(WebApiAuth, {"dialog_id": dialog_id})
        assert list_res["code"] == 0, list_res
        assert any(item["token"] == token for item in list_res["data"]), list_res

        rm_res = api_rm_token(WebApiAuth, {"tokens": [token], "tenant_id": tenant_id})
        assert rm_res["code"] == 0, rm_res
        assert rm_res["data"] is True, rm_res

    @pytest.mark.p2
    def test_stats_basic(self, WebApiAuth):
        res = api_stats(WebApiAuth)
        assert res["code"] == 0, res
        for key in ["pv", "uv", "speed", "tokens", "round", "thumb_up"]:
            assert key in res["data"], res

    @pytest.mark.p3
    def test_rm_missing_tokens(self, WebApiAuth):
        res = api_rm_token(WebApiAuth, {"tenant_id": "dummy_tenant"})
        assert res["code"] == 101, res
        assert "required argument are missing" in res["message"], res
