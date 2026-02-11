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
import requests
import pytest

from configs import HOST_ADDRESS, VERSION


CONVERSATION_APP_URL = f"{HOST_ADDRESS}/{VERSION}/conversation"


@pytest.mark.p2
class TestConversationBasic:
    def test_getsse_invalid_auth_header(self):
        url = f"{CONVERSATION_APP_URL}/getsse/invalid_dialog_id"
        res = requests.get(url=url, headers={"Authorization": "Bearer"})
        body = res.json()
        assert body.get("code") != 0, body
        message = str(body.get("message", "")).lower()
        assert any(token in message for token in ("authorization", "token", "invalid")), body

    def test_set_conversation_not_found(self, WebApiAuth):
        payload = {"conversation_id": "invalid_conversation_id", "is_new": False, "name": "x"}
        res = requests.post(url=f"{CONVERSATION_APP_URL}/set", auth=WebApiAuth, json=payload)
        body = res.json()
        assert body.get("code") != 0, body
        message = str(body.get("message", "")).lower()
        assert "conversation" in message and "not found" in message, body
