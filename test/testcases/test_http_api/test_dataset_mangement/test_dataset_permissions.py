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
from __future__ import annotations

import time
import uuid
from typing import Any

import pytest
import requests

from common import (
    accept_team_invitation,
    add_users_to_team,
    create_dataset,
    create_team,
    create_user,
    delete_datasets,
    encrypt_password,
    get_user_permissions,
    list_datasets,
    login_as_user,
    update_dataset,
    update_user_permissions,
)
from configs import HOST_ADDRESS, INVALID_API_TOKEN, VERSION
from libs.auth import RAGFlowWebApiAuth


def get_dataset_detail(auth: RAGFlowWebApiAuth, kb_id: str) -> dict[str, Any]:
    """Get dataset details by ID.
    
    Args:
        auth: Authentication object.
        kb_id: Knowledge base (dataset) ID.
        
    Returns:
        JSON response as a dictionary containing the dataset data.
    """
    # Use the web KB detail endpoint, which is JWT-authenticated and
    # enforces dataset/KB read permissions consistently with canvas tests.
    url: str = f"{HOST_ADDRESS}/{VERSION}/kb/detail"
    res: requests.Response = requests.get(
        url=url, headers={"Content-Type": "application/json"}, auth=auth, params={"kb_id": kb_id}
    )
    return res.json()


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestDatasetPermissions:
    """Comprehensive tests for dataset permissions with CRUD operations."""

    @pytest.fixture
    def test_team(self, web_api_auth: RAGFlowWebApiAuth) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def team_with_user(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
    ) -> dict[str, Any]:
        """Create a team with a user who has accepted the invitation."""
        tenant_id: str = test_team["id"]
        
        # Create user
        email = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        assert user_res["code"] == 0
        user_id: str = user_res["data"]["id"]

        # Add user to team
        add_payload: dict[str, list[str]] = {"users": [email]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert add_res["code"] == 0

        # Small delay
        time.sleep(0.5)

        # Accept invitation as the user
        user_auth: RAGFlowWebApiAuth = login_as_user(email, password)
        accept_res: dict[str, Any] = accept_team_invitation(user_auth, tenant_id)
        assert accept_res["code"] == 0

        return {
            "team": test_team,
            "user": {"id": user_id, "email": email, "password": password},
        }

    @pytest.fixture
    def team_dataset(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
    ) -> dict[str, Any]:
        """Create a dataset shared with team."""
        dataset_payload: dict[str, Any] = {
            "name": f"Test Dataset {uuid.uuid4().hex[:8]}",
            "permission": "team",
            "shared_tenant_id": test_team["id"],
        }
        
        res: dict[str, Any] = create_dataset(web_api_auth, dataset_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.mark.p1
    def test_read_permission_allows_get_dataset(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_dataset: dict[str, Any],
    ) -> None:
        """Test that user with read permission can get dataset."""
        dataset_id: str = team_dataset["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]

        # User should have read permission by default
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        
        res: dict[str, Any] = get_dataset_detail(user_auth, dataset_id)
        assert res["code"] == 0, res
        assert "data" in res
        assert res["data"]["id"] == dataset_id

    @pytest.mark.p1
    def test_no_read_permission_denies_get_dataset(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_dataset: dict[str, Any],
    ) -> None:
        """Test that user without read permission cannot get dataset."""
        dataset_id: str = team_dataset["id"]
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Revoke read permission
        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"read": False},
            }
        }
        update_res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert update_res["code"] == 0

        # User should not be able to get dataset
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        res: dict[str, Any] = get_dataset_detail(user_auth, dataset_id)
        assert res["code"] != 0
        assert "permission" in res["message"].lower() or "read" in res["message"].lower() or "authorized" in res["message"].lower()

    @pytest.mark.p1
    def test_update_permission_allows_update_dataset(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_dataset: dict[str, Any],
    ) -> None:
        """Test that user with update permission can update dataset."""
        dataset_id: str = team_dataset["id"]
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Grant update permission
        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"update": True},
            }
        }
        update_res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert update_res["code"] == 0

        # User should be able to update dataset
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        update_dataset_payload: dict[str, Any] = {
            "name": f"Updated Dataset {uuid.uuid4().hex[:8]}",
            "description": "Updated description",
            "parser_id": team_dataset.get("parser_id", ""),
        }
        res: dict[str, Any] = update_dataset(user_auth, dataset_id, update_dataset_payload)
        assert res["code"] == 0, res

    @pytest.mark.p1
    def test_no_update_permission_denies_update_dataset(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_dataset: dict[str, Any],
    ) -> None:
        """Test that user without update permission cannot update dataset."""
        dataset_id: str = team_dataset["id"]
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Ensure update permission is False (default)
        permissions_res: dict[str, Any] = get_user_permissions(web_api_auth, tenant_id, user_id)
        assert permissions_res["code"] == 0
        assert permissions_res["data"]["dataset"]["update"] is False

        # User should not be able to update dataset
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        update_dataset_payload: dict[str, Any] = {
            "name": f"Updated Dataset {uuid.uuid4().hex[:8]}",
            "description": "Updated description",
            "parser_id": team_dataset.get("parser_id", ""),
        }
        res: dict[str, Any] = update_dataset(user_auth, dataset_id, update_dataset_payload)
        assert res["code"] != 0
        assert "permission" in res["message"].lower() or "update" in res["message"].lower()

    @pytest.mark.p1
    def test_delete_permission_allows_delete_dataset(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_dataset: dict[str, Any],
    ) -> None:
        """Test that user with delete permission can delete dataset."""
        # Create a new dataset for deletion
        tenant_id: str = team_with_user["team"]["id"]
        dataset_payload: dict[str, Any] = {
            "name": f"Test Dataset Delete {uuid.uuid4().hex[:8]}",
            "permission": "team",
            "shared_tenant_id": tenant_id,
        }
        create_res: dict[str, Any] = create_dataset(web_api_auth, dataset_payload)
        assert create_res["code"] == 0
        dataset_id: str = create_res["data"]["id"]

        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]

        # Grant delete permission
        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"delete": True},
            }
        }
        update_res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert update_res["code"] == 0

        # User should be able to delete dataset
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        delete_payload: dict[str, Any] = {"ids": [dataset_id]}
        res: dict[str, Any] = delete_datasets(user_auth, delete_payload)
        assert res["code"] == 0, res

    @pytest.mark.p1
    def test_no_delete_permission_denies_delete_dataset(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_dataset: dict[str, Any],
    ) -> None:
        """Test that user without delete permission cannot delete dataset."""
        dataset_id: str = team_dataset["id"]
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Ensure delete permission is False (default)
        permissions_res: dict[str, Any] = get_user_permissions(web_api_auth, tenant_id, user_id)
        assert permissions_res["code"] == 0
        assert permissions_res["data"]["dataset"]["delete"] is False

        # User should not be able to delete dataset
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        delete_payload: dict[str, Any] = {"ids": [dataset_id]}
        res: dict[str, Any] = delete_datasets(user_auth, delete_payload)
        assert res["code"] != 0
        assert "permission" in res["message"].lower() or "delete" in res["message"].lower()

    @pytest.mark.p1
    def test_read_permission_allows_list_datasets(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_dataset: dict[str, Any],
    ) -> None:
        """Test that user with read permission can list datasets."""
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        dataset_id: str = team_dataset["id"]

        # User should have read permission by default (allows listing)
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        
        res: dict[str, Any] = list_datasets(user_auth)
        # Permission check should pass (code != PERMISSION_ERROR)
        if res["code"] == 0:
            # If listing succeeds, check that the dataset is visible
            dataset_ids = [ds["id"] for ds in res.get("data", [])]
            assert dataset_id in dataset_ids, "Team dataset should be visible to user with read permission"
        else:
            # If it fails, it should not be due to permission error
            assert "permission" not in res["message"].lower() or "read" not in res["message"].lower()

    @pytest.mark.p1
    def test_no_read_permission_denies_list_datasets(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_dataset: dict[str, Any],
    ) -> None:
        """Test that user without read permission cannot see team datasets in list."""
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]
        dataset_id: str = team_dataset["id"]

        # Revoke read permission
        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"read": False},
            }
        }
        update_res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert update_res["code"] == 0

        # User should not see team datasets in list
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        res: dict[str, Any] = list_datasets(user_auth)
        # If listing succeeds, the team dataset should not be visible
        if res["code"] == 0:
            dataset_ids = [ds["id"] for ds in res.get("data", [])]
            assert dataset_id not in dataset_ids, "Team dataset should not be visible to user without read permission"

    @pytest.mark.p1
    def test_create_permission_allows_create_team_dataset(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test that user with create permission can create team dataset."""
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Grant create permission
        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"create": True},
            }
        }
        update_res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert update_res["code"] == 0

        # User should be able to create team dataset
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        dataset_payload: dict[str, Any] = {
            "name": f"User Created Dataset {uuid.uuid4().hex[:8]}",
            "permission": "team",
            "shared_tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_dataset(user_auth, dataset_payload)
        assert res["code"] == 0, res

    @pytest.mark.p1
    def test_no_create_permission_denies_create_team_dataset(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test that user without create permission cannot create team dataset."""
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Ensure create permission is False (default)
        permissions_res: dict[str, Any] = get_user_permissions(web_api_auth, tenant_id, user_id)
        assert permissions_res["code"] == 0
        assert permissions_res["data"]["dataset"]["create"] is False

        # User should not be able to create team dataset
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        dataset_payload: dict[str, Any] = {
            "name": f"User Created Dataset {uuid.uuid4().hex[:8]}",
            "permission": "team",
            "shared_tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_dataset(user_auth, dataset_payload)
        assert res["code"] != 0
        assert "permission" in res["message"].lower() or "create" in res["message"].lower()

    @pytest.mark.p1
    def test_owner_always_has_full_permissions(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        team_dataset: dict[str, Any],
    ) -> None:
        """Test that team owner always has full permissions regardless of settings."""
        dataset_id: str = team_dataset["id"]
        owner_id: str = test_team["owner_id"]
        tenant_id: str = test_team["id"]

        # Owner should have full permissions
        permissions_res: dict[str, Any] = get_user_permissions(web_api_auth, tenant_id, owner_id)
        assert permissions_res["code"] == 0
        permissions: dict[str, Any] = permissions_res["data"]
        
        assert permissions["dataset"]["create"] is True
        assert permissions["dataset"]["read"] is True
        assert permissions["dataset"]["update"] is True
        assert permissions["dataset"]["delete"] is True

        # Owner should be able to perform all operations
        update_payload: dict[str, Any] = {
            "name": f"Owner Updated Dataset {uuid.uuid4().hex[:8]}",
            "description": "Owner updated description",
            "parser_id": team_dataset.get("parser_id", ""),
        }
        res: dict[str, Any] = update_dataset(web_api_auth, dataset_id, update_payload)
        assert res["code"] == 0, res


