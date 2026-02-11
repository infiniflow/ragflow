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


MCP_SERVER_APP_URL = f"{HOST_ADDRESS}/{VERSION}/mcp_server"


@pytest.mark.p2
class TestMcpServerBasic:
    def test_mcp_create_invalid_server_type(self, WebApiAuth):
        payload = {"name": "mcp_test", "url": "http://example.com", "server_type": "invalid"}
        res = requests.post(url=f"{MCP_SERVER_APP_URL}/create", auth=WebApiAuth, json=payload)
        body = res.json()
        assert body.get("code") != 0, body
        message = str(body.get("message", "")).lower()
        assert "unsupported" in message and "type" in message, body

    def test_mcp_detail_not_found(self, WebApiAuth):
        res = requests.get(url=f"{MCP_SERVER_APP_URL}/detail", auth=WebApiAuth, params={"mcp_id": "invalid"})
        body = res.json()
        assert body.get("code") != 0, body
        assert body.get("data") is None, body
