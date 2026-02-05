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

import uuid

import pytest
import requests

from common import settings
from configs import EMAIL, HOST_ADDRESS, VERSION
from conftest import ADMIN_HOST_ADDRESS, ENCRYPTED_ADMIN_PASSWORD, change_user_activation, create_user, delete_user, generate_user_api_key
from utils.file_utils import create_txt_file


class TestUserDeletion:

    @pytest.mark.p2
    @pytest.mark.usefixtures("init_storage")
    def test_delete_user_removes_storage_files(self, admin_session, tmp_path):
        """Verify that deleting a user also removes datasets from storage."""
        # create user
        username: str = f"test_user_{uuid.uuid4().hex}@ragflow.io"
        create_user_response = create_user(admin_session, username, ENCRYPTED_ADMIN_PASSWORD)
        assert create_user_response["code"] == 0, create_user_response

        # generate API key for user
        generate_token_response = generate_user_api_key(admin_session, username)
        assert generate_token_response["code"] == 0, generate_token_response
        assert isinstance(generate_token_response["data"], dict), generate_token_response
        assert generate_token_response["data"].get("token"), generate_token_response
        auth_header = {"Authorization": f"Bearer {generate_token_response["data"]["token"]}"}

        # create a dataset
        create_dataset_response = requests.post(
            url=f"{HOST_ADDRESS}/api/{VERSION}/datasets",
            headers=auth_header,
            json={"name": f"test_dataset_{uuid.uuid4().hex}"},
        ).json()
        assert create_dataset_response["code"] == 0, create_user_response
        assert isinstance(create_user_response["data"], dict), create_dataset_response
        assert create_dataset_response["data"].get("id"), create_dataset_response
        dataset_id: str = create_dataset_response["data"]["id"]

        # upload file to dataset
        test_file = create_txt_file(tmp_path / "ragflow_test.txt")
        with open(test_file, "rb") as f:
            upload_response = requests.post(
                url=f"{HOST_ADDRESS}/api/{VERSION}/datasets/{dataset_id}/documents",
                headers=auth_header,
                files={"file": (test_file.name, f)},
            ).json()
        assert upload_response["code"] == 0, upload_response
        assert len(upload_response["data"]) == 1, upload_response
        filename: str = upload_response["data"][0]["location"]

        # check storage contains bucket and uploaded document
        assert settings.STORAGE_IMPL.bucket_exists(dataset_id), f"Storage should have bucket for dataset {dataset_id}"
        assert settings.STORAGE_IMPL.obj_exist(dataset_id, filename), f"Storage should have file {filename} in bucket of dataset {dataset_id}"

        # deactivate user (required before deletion)
        deactivate_response = change_user_activation(admin_session, username, False)
        assert deactivate_response["code"] == 0, deactivate_response

        delete_response = delete_user(admin_session, username)
        assert delete_response["code"] == 0, delete_response

        # verify bucket of user's dataset got deleted
        assert not settings.STORAGE_IMPL.bucket_exists(dataset_id), f"Storage should not contain bucket for dataset {dataset_id} after deletion"
