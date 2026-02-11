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


FILE2DOCUMENT_URL = f"{HOST_ADDRESS}/{VERSION}/file2document"


@pytest.mark.p2
class TestFile2Document:
    def test_file2document_rm_missing_and_invalid(self, WebApiAuth):
        res = requests.post(
            url=f"{FILE2DOCUMENT_URL}/rm",
            auth=WebApiAuth,
            headers={"Content-Type": "application/json"},
            json={"file_ids": []},
        )
        data = res.json()
        assert data.get("code") != 0, data
        message = str(data.get("message", "")).lower()
        assert "file" in message and any(token in message for token in ("id", "ids", "lack", "missing", "required")), data

        res = requests.post(
            url=f"{FILE2DOCUMENT_URL}/rm",
            auth=WebApiAuth,
            headers={"Content-Type": "application/json"},
            json={"file_ids": ["invalid_file_id"]},
        )
        data = res.json()
        assert data.get("code") != 0, data
        message = str(data.get("message", "")).lower()
        assert "inform" in message or "not found" in message, data
