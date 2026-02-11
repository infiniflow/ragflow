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


CANVAS_APP_URL = f"{HOST_ADDRESS}/{VERSION}/canvas"


@pytest.mark.p2
class TestCanvasBasic:
    def test_getsse_invalid_auth_header(self):
        url = f"{CANVAS_APP_URL}/getsse/invalid_canvas_id"
        res = requests.get(url=url, headers={"Authorization": "Bearer"})
        body = res.json()
        assert body.get("code") != 0, body
        message = str(body.get("message", "")).lower()
        assert ("authorization" in message) or ("token" in message) or ("invalid" in message), body

    def test_test_db_connect_unsupported_type(self, WebApiAuth):
        payload = {
            "db_type": "unsupported",
            "database": "db",
            "username": "user",
            "host": "localhost",
            "port": 1234,
            "password": "pwd",
        }
        res = requests.post(url=f"{CANVAS_APP_URL}/test_db_connect", auth=WebApiAuth, json=payload)
        body = res.json()
        assert body.get("code") != 0, body
        message = str(body.get("message", "")).lower()
        assert ("unsupported" in message) or ("attributeerror" in message), body

    def test_prompts_returns_templates(self, WebApiAuth):
        res = requests.get(url=f"{CANVAS_APP_URL}/prompts", auth=WebApiAuth)
        body = res.json()
        assert body.get("code") == 0, body
        data = body.get("data") or {}
        for key in ("task_analysis", "plan_generation", "reflection", "citation_guidelines"):
            assert key in data, body
