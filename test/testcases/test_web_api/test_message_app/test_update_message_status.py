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
import random

import pytest
import requests
from test_web_api.common import update_message_status, list_memory_message, get_message_content
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from configs import HOST_ADDRESS, VERSION


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
        res = update_message_status(invalid_auth, "empty_memory_id", 0, False)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


@pytest.mark.usefixtures("add_memory_with_5_raw_message_func")
class TestUpdateMessageStatus:

    @pytest.mark.p1
    def test_update_to_false(self, WebApiAuth):
        memory_id = self.memory_id
        list_res = list_memory_message(WebApiAuth, memory_id)
        assert list_res["code"] == 0, list_res
        assert len(list_res["data"]["messages"]["message_list"]) > 0

        message = random.choice(list_res["data"]["messages"]["message_list"])
        res = update_message_status(WebApiAuth, memory_id, message["message_id"], False)
        assert res["code"] == 0, res

        updated_message_res = get_message_content(WebApiAuth, memory_id, message["message_id"])
        assert updated_message_res["code"] == 0, res
        assert not updated_message_res["data"]["status"], res

    @pytest.mark.p1
    def test_update_to_true(self, WebApiAuth):
        memory_id = self.memory_id
        list_res = list_memory_message(WebApiAuth, memory_id)
        assert list_res["code"] == 0, list_res
        assert len(list_res["data"]["messages"]["message_list"]) > 0
        # set 1 random message to false first
        message = random.choice(list_res["data"]["messages"]["message_list"])
        set_to_false_res = update_message_status(WebApiAuth, memory_id, message["message_id"], False)
        assert set_to_false_res["code"] == 0, set_to_false_res
        updated_message_res = get_message_content(WebApiAuth, memory_id, message["message_id"])
        assert updated_message_res["code"] == 0, set_to_false_res
        assert not updated_message_res["data"]["status"], updated_message_res
        # set to true
        set_to_true_res = update_message_status(WebApiAuth, memory_id, message["message_id"], True)
        assert set_to_true_res["code"] == 0, set_to_true_res
        res = get_message_content(WebApiAuth, memory_id, message["message_id"])
        assert res["code"] == 0, res
        assert res["data"]["status"], res

    @pytest.mark.p2
    def test_update_invalid_status_type(self, WebApiAuth):
        memory_id = self.memory_id
        list_res = list_memory_message(WebApiAuth, memory_id)
        assert list_res["code"] == 0, list_res
        message_id = list_res["data"]["messages"]["message_list"][0]["message_id"]

        url = f"{HOST_ADDRESS}/api/{VERSION}/messages/{memory_id}:{message_id}"
        res = requests.put(url=url, headers={"Content-Type": "application/json"}, auth=WebApiAuth, json={"status": "false"}).json()
        assert res["code"] == 101, res
        assert "Status must be a boolean." in res["message"], res

    @pytest.mark.p2
    def test_update_invalid_memory_id(self, WebApiAuth):
        res = update_message_status(WebApiAuth, "missing_memory_id", 1, False)
        assert res["code"] == 404, res
        assert "not found" in res["message"].lower(), res

    @pytest.mark.p2
    def test_update_invalid_message_id(self, WebApiAuth):
        memory_id = self.memory_id
        url = f"{HOST_ADDRESS}/api/{VERSION}/messages/{memory_id}:invalid_message_id"
        res = requests.put(
            url=url,
            headers={"Content-Type": "application/json"},
            auth=WebApiAuth,
            json={"status": True},
        ).json()
        assert res["code"] == 500, res
        assert "Internal server error" in res["message"], res
