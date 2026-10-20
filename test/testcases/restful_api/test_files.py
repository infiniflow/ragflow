#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

import pytest

from test.testcases.utils.file_utils import create_txt_file


def _assert_ok(res, *, expect_list=False):
    assert res.status_code == 200, res.text
    payload = res.json()
    assert payload["code"] == 0, payload
    data = payload["data"]
    if expect_list:
        assert isinstance(data, dict), payload
        assert "files" in data and "total" in data, payload
    return data


@pytest.mark.p2
def test_files_list_create_folder_and_upload_to_root(rest_client, tmp_path):
    """Exercise GET/POST /files so connector offload paths run on the live server."""
    created_ids: list[str] = []
    suffix = uuid.uuid4().hex[:12]
    folder_name = f"rf_folder_{suffix}"
    upload_name = f"rf_upload_{suffix}.txt"

    try:
        list_data = _assert_ok(rest_client.get("/files"), expect_list=True)
        assert isinstance(list_data["files"], list), list_data

        folder = _assert_ok(rest_client.post("/files", json={"name": folder_name, "type": "folder"}))
        folder_id = folder.get("id")
        assert folder_id, folder
        created_ids.append(folder_id)
        assert folder.get("name") == folder_name

        fp = create_txt_file(tmp_path / upload_name)
        with fp.open("rb") as file_obj:
            upload_res = rest_client.post("/files", files=[("file", (fp.name, file_obj))])
        uploaded = _assert_ok(upload_res)
        assert isinstance(uploaded, list) and uploaded, uploaded
        file_id = uploaded[0].get("id")
        assert file_id, uploaded
        created_ids.append(file_id)
        assert uploaded[0].get("name") == upload_name
    finally:
        if created_ids:
            rest_client.delete("/files", json={"ids": created_ids})
