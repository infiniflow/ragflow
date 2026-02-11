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


LANGFUSE_API_URL = f"{HOST_ADDRESS}/{VERSION}/langfuse/api_key"


@pytest.mark.p2
class TestLangfuseApiKey:
    def test_langfuse_api_key_missing_fields(self, WebApiAuth):
        res = requests.post(LANGFUSE_API_URL, auth=WebApiAuth, json={})
        data = res.json()
        assert data.get("code") != 0, data
        message = str(data.get("message", "")).lower()
        assert "missing" in message or "required" in message, data

    def test_langfuse_api_key_get_no_record(self, WebApiAuth):
        res = requests.get(LANGFUSE_API_URL, auth=WebApiAuth)
        data = res.json()
        message = str(data.get("message", "")).lower()
        if data.get("code") == 0:
            assert "record" in message or "langfuse" in message or data.get("data"), data
        else:
            assert "langfuse" in message or "invalid" in message or "error" in message, data
