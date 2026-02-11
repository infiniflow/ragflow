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
import uuid

import requests
import pytest
from configs import HOST_ADDRESS, VERSION


FILE_URL = f"{HOST_ADDRESS}/{VERSION}/file"


@pytest.mark.p2
class TestFileApp:
    def test_file_upload_missing_file_part(self, WebApiAuth):
        res = requests.post(
            url=f"{FILE_URL}/upload",
            auth=WebApiAuth,
            data={},
        )
        data = res.json()
        assert data.get("code") != 0, data
        message = str(data.get("message", "")).lower()
        assert "file" in message and any(token in message for token in ("part", "select", "missing")), data

    def test_file_create_duplicate_name_then_cleanup(self, WebApiAuth):
        name = f"test_folder_{uuid.uuid4().hex}"
        create_payload = {"name": name, "type": "folder"}
        res = requests.post(
            url=f"{FILE_URL}/create",
            auth=WebApiAuth,
            headers={"Content-Type": "application/json"},
            json=create_payload,
        )
        data = res.json()
        assert data.get("code") == 0, data
        file_id = data["data"]["id"]
        parent_id = data["data"]["parent_id"]

        try:
            dup_payload = {"name": name, "parent_id": parent_id, "type": "folder"}
            dup_res = requests.post(
                url=f"{FILE_URL}/create",
                auth=WebApiAuth,
                headers={"Content-Type": "application/json"},
                json=dup_payload,
            )
            dup_data = dup_res.json()
            assert dup_data.get("code") != 0, dup_data
            message = str(dup_data.get("message", "")).lower()
            assert "dup" in message or "exist" in message, dup_data
        finally:
            rm_res = requests.post(
                url=f"{FILE_URL}/rm",
                auth=WebApiAuth,
                headers={"Content-Type": "application/json"},
                json={"file_ids": [file_id]},
            )
            rm_data = rm_res.json()
            assert rm_data.get("code") == 0 or rm_data.get("data") is True, rm_data
