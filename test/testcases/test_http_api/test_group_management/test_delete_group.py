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

from common import (
    add_group_members,
    add_users_to_team,
    create_group,
    create_team,
    create_user,
    delete_group,
    encrypt_password,
    get_user_info,
    login_as_user,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when deleting a group."""

    @pytest.mark.parametrize(
        ("invalid_auth", "expected_code", "expected_message"),
        [
            (None, 401, "Unauthorized"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "Unauthorized"),
        ],
    )
    def test_invalid_auth(
        self,
        invalid_auth: RAGFlowWebApiAuth | None,
        expected_code: int,
        expected_message: str,
        web_api_auth: RAGFlowWebApiAuth,
    ) -> None:
        """Test deleting a group with invalid or missing authentication."""
        # Create a team and group first
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if team_res["code"] != 0:
            pytest.skip("Team creation failed, skipping auth test")
        
        tenant_id: str = team_res["data"]["id"]
        
        group_name: str = f"Test Group {uuid.uuid4().hex[:8]}"
        group_payload: dict[str, str] = {
            "name": group_name,
            "tenant_id": tenant_id,
        }
        group_res: dict[str, Any] = create_group(web_api_auth, group_payload)
        if group_res["code"] != 0:
            pytest.skip("Group creation failed, skipping auth test")
        
        group_id: str = group_res["data"]["id"]
        
        # Try to delete group with invalid auth
        res: dict[str, Any] = delete_group(invalid_auth, group_id)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestDeleteGroup:
    """Comprehensive tests for deleting a group."""

    @pytest.fixture
    def test_team(self, web_api_auth: RAGFlowWebApiAuth) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def test_group(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> dict[str, Any]:
        """Create a test group for use in tests."""
        group_payload: dict[str, str] = {
            "name": f"Test Group {uuid.uuid4().hex[:8]}",
            "tenant_id": test_team["id"],
        }
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def test_group_with_members(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        test_team: dict[str, Any],
    ) -> dict[str, Any]:
        """Create a test group and add the current user as a member."""
        # Get current user ID
        user_info_res: dict[str, Any] = get_user_info(web_api_auth)
        if user_info_res["code"] == 0:
            user_id: str = user_info_res["data"]["id"]
            # Add current user to group (they're already team owner/admin)
            add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
            add_group_members(web_api_auth, test_group["id"], add_payload)
        return test_group

    @pytest.mark.p1
    def test_delete_group_success(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group_with_members: dict[str, Any],
    ) -> None:
        """Test successfully deleting a group."""
        group_id: str = test_group_with_members["id"]
        
        # Delete the group
        res: dict[str, Any] = delete_group(web_api_auth, group_id)
        assert res["code"] == 0, res
        assert res["data"] is True
        assert "deleted successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_delete_group_invalid_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test deleting a group with an invalid group ID."""
        invalid_id: str = f"invalid_{uuid.uuid4().hex[:8]}"
        res: dict[str, Any] = delete_group(web_api_auth, invalid_id)
        assert res["code"] == 102  # DATA_ERROR
        assert "not found" in res["message"].lower()

    @pytest.mark.p1
    def test_delete_group_not_team_admin_or_owner(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
    ) -> None:
        """Test deleting a group when user is not team admin or owner."""
        # Create a new user with encrypted password
        email: str = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        if user_res["code"] != 0:
            pytest.skip("User creation failed")
        
        # Add user to team as normal member
        team_id: str = test_group["tenant_id"]
        add_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, team_id, add_payload)
        
        # Small delay to ensure user is fully created
        time.sleep(0.5)
        
        # Login as the new user (normal member, not admin/owner)
        new_user_auth: RAGFlowWebApiAuth = login_as_user(email, password)
        
        # Try to delete group (user is member but not admin/owner)
        res: dict[str, Any] = delete_group(new_user_auth, test_group["id"])
        assert res["code"] == 108  # PERMISSION_ERROR
        assert "owner" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p1
    def test_delete_group_response_structure(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group_with_members: dict[str, Any],
    ) -> None:
        """Test that group deletion returns the expected response structure."""
        group_id: str = test_group_with_members["id"]
        
        res: dict[str, Any] = delete_group(web_api_auth, group_id)
        assert res["code"] == 0
        assert "data" in res
        assert res["data"] is True
        assert "message" in res
        assert isinstance(res["message"], str)
        assert "deleted successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_delete_group_already_deleted(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group_with_members: dict[str, Any],
    ) -> None:
        """Test deleting a group that has already been deleted."""
        group_id: str = test_group_with_members["id"]
        
        # Delete the group first
        res1: dict[str, Any] = delete_group(web_api_auth, group_id)
        assert res1["code"] == 0
        
        # Try to delete again
        res2: dict[str, Any] = delete_group(web_api_auth, group_id)
        # Should return error (group not found or already deleted)
        assert res2["code"] != 0
        assert "not found" in res2["message"].lower() or "deleted" in res2["message"].lower()

    @pytest.mark.p1
    def test_delete_group_with_members(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        test_team: dict[str, Any],
    ) -> None:
        """Test deleting a group that has members."""
        # Create test users
        users = []
        for i in range(2):
            email = f"testuser{i}_{uuid.uuid4().hex[:8]}@example.com"
            password = "TestPassword123!"
            encrypted_password = encrypt_password(password)
            user_payload: dict[str, str] = {
                "email": email,
                "password": encrypted_password,
                "nickname": f"Test User {i}",
            }
            user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
            if user_res["code"] == 0:
                users.append({"email": email, "id": user_res["data"]["id"]})
        
        if not users:
            pytest.skip("User creation failed")
        
        # Add users to team
        for user in users:
            add_payload: dict[str, list[str]] = {"users": [user["email"]]}
            add_users_to_team(web_api_auth, test_team["id"], add_payload)
        
        # Add users to group
        user_ids: list[str] = [user["id"] for user in users]
        group_add_payload: dict[str, list[str]] = {"user_ids": user_ids}
        add_group_members(web_api_auth, test_group["id"], group_add_payload)
        
        # Delete the group (should also remove all member relationships)
        res: dict[str, Any] = delete_group(web_api_auth, test_group["id"])
        assert res["code"] == 0, res
        assert res["data"] is True
        assert "member relationships" in res["message"].lower() or "deleted successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_delete_group_removes_member_relationships(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        test_team: dict[str, Any],
    ) -> None:
        """Test that deleting a group removes all member relationships."""
        from common import list_group_members
        
        # Create and add a user to the group
        email = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        if user_res["code"] != 0:
            pytest.skip("User creation failed")
        
        user_id: str = user_res["data"]["id"]
        
        # Add user to team
        add_team_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, test_team["id"], add_team_payload)
        
        # Add user to group
        add_group_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_group_members(web_api_auth, test_group["id"], add_group_payload)
        
        # Verify user is in group
        list_res_before: dict[str, Any] = list_group_members(web_api_auth, test_group["id"])
        assert list_res_before["code"] == 0
        member_user_ids_before: set[str] = {
            member["user_id"] for member in list_res_before["data"]
        }
        assert user_id in member_user_ids_before
        
        # Delete the group
        delete_res: dict[str, Any] = delete_group(web_api_auth, test_group["id"])
        assert delete_res["code"] == 0
        
        # Try to list members (should fail - group deleted)
        list_res_after: dict[str, Any] = list_group_members(web_api_auth, test_group["id"])
        assert list_res_after["code"] != 0
        assert "not found" in list_res_after["message"].lower()

    @pytest.mark.p2
    def test_delete_multiple_groups(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
    ) -> None:
        """Test deleting multiple groups from the same team."""
        # Create multiple groups
        groups = []
        for i in range(3):
            group_name: str = f"Group {i} {uuid.uuid4().hex[:8]}"
            group_payload: dict[str, str] = {
                "name": group_name,
                "tenant_id": test_team["id"],
            }
            group_res: dict[str, Any] = create_group(web_api_auth, group_payload)
            if group_res["code"] == 0:
                groups.append(group_res["data"])
        
        if not groups:
            pytest.skip("Group creation failed")
        
        # Delete all groups
        for group in groups:
            res: dict[str, Any] = delete_group(web_api_auth, group["id"])
            assert res["code"] == 0, f"Failed to delete group {group['id']}: {res}"

    @pytest.mark.p2
    def test_delete_group_empty_string_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test deleting a group with empty string ID."""
        res: dict[str, Any] = delete_group(web_api_auth, "")
        assert res["code"] != 0
        assert "not found" in res["message"].lower() or res["code"] in [100, 102, 405]

    @pytest.mark.p2
    def test_delete_group_special_characters_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test deleting a group with special characters in ID."""
        invalid_id: str = "group-123_!@#$%"
        res: dict[str, Any] = delete_group(web_api_auth, invalid_id)
        assert res["code"] != 0
        assert "not found" in res["message"].lower() or res["code"] in [100, 102, 405]

    @pytest.mark.p2
    def test_delete_group_and_recreate(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
    ) -> None:
        """Test deleting a group and then recreating a group with the same name."""
        group_name: str = f"Test Group {uuid.uuid4().hex[:8]}"
        group_payload: dict[str, str] = {
            "name": group_name,
            "tenant_id": test_team["id"],
        }
        
        # Create group
        create_res: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert create_res["code"] == 0
        group_id: str = create_res["data"]["id"]
        
        # Delete group
        delete_res: dict[str, Any] = delete_group(web_api_auth, group_id)
        assert delete_res["code"] == 0
        
        # Recreate group with same name (should work - soft delete)
        recreate_res: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert recreate_res["code"] == 0
        assert recreate_res["data"]["name"] == group_name

