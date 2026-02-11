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


USER_APP_URL = f"{HOST_ADDRESS}/{VERSION}/user"


@pytest.mark.p2
class TestUserBasic:
    def test_login_missing_body_returns_error(self):
        res = requests.post(url=f"{USER_APP_URL}/login", json={})
        body = res.json()
        assert body.get("code") != 0, body
        message = str(body.get("message", "")).lower()
        assert any(token in message for token in ("unauthor", "missing", "required", "body")), body

    def test_login_channels_returns_list(self):
        res = requests.get(url=f"{USER_APP_URL}/login/channels")
        body = res.json()
        assert body.get("code") == 0, body
        assert isinstance(body.get("data"), list), body

    def test_forget_captcha_missing_email(self):
        res = requests.get(url=f"{USER_APP_URL}/forget/captcha")
        body = res.json()
        assert body.get("code") != 0, body
        message = str(body.get("message", "")).lower()
        assert "email" in message and any(token in message for token in ("required", "lack", "missing")), body

    def test_forget_verify_otp_missing_fields(self):
        res = requests.post(url=f"{USER_APP_URL}/forget/verify-otp", json={})
        body = res.json()
        assert body.get("code") != 0, body
        message = str(body.get("message", "")).lower()
        assert "email" in message and any(token in message for token in ("otp", "code")), body
        assert any(token in message for token in ("required", "missing")), body
