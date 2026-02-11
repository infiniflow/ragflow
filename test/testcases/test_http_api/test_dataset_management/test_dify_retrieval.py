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
import requests
import pytest
from configs import HOST_ADDRESS, VERSION, INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth


DIFY_RETRIEVAL_URL = f"{HOST_ADDRESS}/api/{VERSION}/dify/retrieval"


class TestDifyRetrieval:
    @pytest.mark.p2
    def test_dify_retrieval_invalid_api_key(self):
        payload = {"knowledge_id": "invalid_knowledge_id", "query": "hello"}
        res = requests.post(
            url=DIFY_RETRIEVAL_URL,
            headers={"Content-Type": "application/json"},
            auth=RAGFlowHttpApiAuth(INVALID_API_TOKEN),
            json=payload,
        )
        data = res.json()
        assert data.get("code") != 0, data
        message = str(data.get("message", "")).lower()
        assert "api" in message and ("invalid" in message or "unauthor" in message or "forbidden" in message), data
