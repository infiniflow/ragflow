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


TENANT_APP_URL = f"{HOST_ADDRESS}/{VERSION}/tenant"


@pytest.mark.p2
class TestTenantBasic:
    def test_tenant_user_list_unauthorized(self, WebApiAuth):
        res = requests.get(url=f"{TENANT_APP_URL}/invalid/user/list", auth=WebApiAuth)
        body = res.json()
        assert body.get("code") != 0, body
        message = str(body.get("message", "")).lower()
        assert "authorization" in message, body

    def test_tenant_create_unauthorized(self, WebApiAuth):
        payload = {"email": "user@example.com"}
        res = requests.post(url=f"{TENANT_APP_URL}/invalid/user", auth=WebApiAuth, json=payload)
        body = res.json()
        assert body.get("code") != 0, body
        message = str(body.get("message", "")).lower()
        assert "authorization" in message, body
