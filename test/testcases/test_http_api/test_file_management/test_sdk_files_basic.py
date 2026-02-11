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


FILE_API_URL = f"{HOST_ADDRESS}/api/{VERSION}/file"


@pytest.mark.p2
class TestSdkFiles:
    def test_sdk_file_upload_missing_file_part(self, HttpApiAuth):
        res = requests.post(f"{FILE_API_URL}/upload", auth=HttpApiAuth, data={})
        data = res.json()
        assert data.get("code") != 0, data
        message = str(data.get("message", "")).lower()
        assert "file" in message and any(token in message for token in ("part", "selected")), data

    def test_sdk_file_list_invalid_parent(self, HttpApiAuth):
        res = requests.get(f"{FILE_API_URL}/list", auth=HttpApiAuth, params={"parent_id": "invalid_parent"})
        data = res.json()
        assert data.get("code") != 0, data
        message = str(data.get("message", "")).lower()
        assert "folder" in message and "not found" in message, data
