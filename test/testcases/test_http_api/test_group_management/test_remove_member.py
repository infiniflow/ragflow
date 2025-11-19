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
    encrypt_password,
    login_as_user,
    remove_group_member,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when removing members from a group."""

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
        """Test removing members with invalid or missing authentication."""
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
        
        # Create a user and add to team and group
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
            pytest.skip("User creation failed, skipping auth test")
        
        user_id: str = user_res["data"]["id"]
        
        # Add user to team
        add_team_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, tenant_id, add_team_payload)
        
        # Add user to group
        add_group_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_group_members(web_api_auth, group_id, add_group_payload)
        
        # Try to remove member with invalid auth
        res: dict[str, Any] = remove_group_member(
            invalid_auth, group_id, user_id
        )
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestRemoveMember:
    """Comprehensive tests for removing members from a group."""

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
    def test_user_with_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        test_group: dict[str, Any],
    ) -> dict[str, Any]:
        """Create a test user and add them to team and group."""
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
        add_team_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, test_team["id"], add_team_payload)
        
        # Add user to group
        add_group_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_res: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_group_payload
        )
        assert add_res["code"] == 0
        assert len(add_res["data"]["added"]) == 1
        
        return {"id": user_id, "email": email, "password": password}

    @pytest.mark.p1
    def test_remove_single_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test removing a single member from a group."""
        user_id: str = test_user_with_member["id"]
        res: dict[str, Any] = remove_group_member(
            web_api_auth, test_group["id"], user_id
        )
        
        assert res["code"] == 0, res
        assert "data" in res
        assert res["data"] is True
        assert "removed" in res["message"].lower() or "success" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_remove_member_invalid_group_id(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test removing a member from a non-existent group."""
        user_id: str = test_user_with_member["id"]
        res: dict[str, Any] = remove_group_member(
            web_api_auth, "non_existent_group_id_12345", user_id
        )
        
        assert res["code"] == 102
        assert "group not found" in res["message"].lower() or "not found" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_remove_member_user_not_in_group(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        test_team: dict[str, Any],
    ) -> None:
        """Test removing a user who is not in the group."""
        # Create a user and add to team but not group
        email = f"notingroup_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Not In Group User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        assert user_res["code"] == 0
        
        user_id: str = user_res["data"]["id"]
        
        # Add user to team but not group
        add_team_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, test_team["id"], add_team_payload)
        
        # Try to remove from group
        res: dict[str, Any] = remove_group_member(
            web_api_auth, test_group["id"], user_id
        )
        
        assert res["code"] == 102
        assert "not a member" in res["message"].lower() or "not found" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_remove_member_invalid_user_id(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
    ) -> None:
        """Test removing a non-existent user from a group."""
        res: dict[str, Any] = remove_group_member(
            web_api_auth, test_group["id"], "non_existent_user_id_12345"
        )
        
        assert res["code"] == 102
        assert "not a member" in res["message"].lower() or "not found" in res[
            "message"
        ].lower()

    @pytest.mark.p2
    def test_remove_member_not_team_owner_or_admin(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test that non-admin/non-owner users cannot remove members from groups."""
        # Create a team with the main user (owner)
        team_name: str = f"Owner Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create a group
        group_name: str = f"Test Group {uuid.uuid4().hex[:8]}"
        group_payload: dict[str, str] = {
            "name": group_name,
            "tenant_id": tenant_id,
        }
        group_res: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert group_res["code"] == 0, group_res
        group_id: str = group_res["data"]["id"]
        
        # Create a second user with encrypted password
        other_user_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        other_user_password: str = "test123"
        encrypted_password: str = encrypt_password(other_user_password)
        
        user_payload: dict[str, str] = {
            "nickname": "Other User",
            "email": other_user_email,
            "password": encrypted_password,
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        assert user_res["code"] == 0, user_res
        
        # Add user to team
        add_team_payload: dict[str, list[str]] = {"users": [other_user_email]}
        add_team_res: dict[str, Any] = add_users_to_team(
            web_api_auth, tenant_id, add_team_payload
        )
        assert add_team_res["code"] == 0
        
        # Create a third user to add to group and then try to remove
        third_user_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        third_user_password: str = "test123"
        third_encrypted_password: str = encrypt_password(third_user_password)
        
        third_user_payload: dict[str, str] = {
            "nickname": "Third User",
            "email": third_user_email,
            "password": third_encrypted_password,
        }
        third_user_res: dict[str, Any] = create_user(web_api_auth, third_user_payload)
        assert third_user_res["code"] == 0
        
        # Add third user to team
        add_third_team_payload: dict[str, list[str]] = {"users": [third_user_email]}
        add_third_team_res: dict[str, Any] = add_users_to_team(
            web_api_auth, tenant_id, add_third_team_payload
        )
        assert add_third_team_res["code"] == 0
        
        third_user_id: str = third_user_res["data"]["id"]
        
        # Add third user to group (as owner)
        add_group_payload: dict[str, list[str]] = {"user_ids": [third_user_id]}
        add_group_res: dict[str, Any] = add_group_members(
            web_api_auth, group_id, add_group_payload
        )
        assert add_group_res["code"] == 0
        
        # Small delay to ensure user is fully created
        time.sleep(0.5)
        
        # Login as the other user (non-admin)
        other_user_auth: RAGFlowWebApiAuth = login_as_user(
            other_user_email, other_user_password
        )
        
        # Try to remove member as non-admin user
        res: dict[str, Any] = remove_group_member(
            other_user_auth, group_id, third_user_id
        )
        
        # Should fail - user is not the team owner or admin
        assert res["code"] != 0, (
            "Non-owner/non-admin should not be able to remove members from groups"
        )
        
        # Verify it's a permission-related error
        assert res["code"] in [108, 403, 104, 102], (
            f"Expected permission error code (108, 403, 104, or 102), got: {res}"
        )
        
        # Verify the error message indicates permission issue
        assert "owner" in res["message"].lower() or "admin" in res["message"].lower() or "permission" in res["message"].lower(), (
            f"Error message should indicate permission issue, got: {res['message']}"
        )

    @pytest.mark.p2
    def test_remove_and_re_add_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test removing a member and then adding them back."""
        user_id: str = test_user_with_member["id"]
        
        # Remove member
        remove_res: dict[str, Any] = remove_group_member(
            web_api_auth, test_group["id"], user_id
        )
        assert remove_res["code"] == 0
        
        # Add member back
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_res: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_payload
        )
        assert add_res["code"] == 0
        assert len(add_res["data"]["added"]) == 1
        assert add_res["data"]["added"][0] == user_id

    @pytest.mark.p2
    def test_remove_multiple_members_sequentially(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        test_team: dict[str, Any],
    ) -> None:
        """Test removing multiple members from a group sequentially."""
        # Create and add multiple users
        user_ids = []
        for i in range(3):
            email = f"testuser{i}_{uuid.uuid4().hex[:8]}@example.com"
            password = "TestPassword123!"
            encrypted_password = encrypt_password(password)
            user_payload: dict[str, str] = {
                "email": email,
                "password": encrypted_password,
                "nickname": f"Test User {i}",
            }
            user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
            assert user_res["code"] == 0
            
            user_id: str = user_res["data"]["id"]
            user_ids.append(user_id)
            
            # Add to team
            add_team_payload: dict[str, list[str]] = {"users": [email]}
            add_users_to_team(web_api_auth, test_team["id"], add_team_payload)
        
        # Add all users to group
        add_payload: dict[str, list[str]] = {"user_ids": user_ids}
        add_res: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_payload
        )
        assert add_res["code"] == 0
        assert len(add_res["data"]["added"]) == 3
        
        # Remove each user sequentially
        for user_id in user_ids:
            remove_res: dict[str, Any] = remove_group_member(
                web_api_auth, test_group["id"], user_id
            )
            assert remove_res["code"] == 0
        
        # Verify all removed by listing members
        from common import list_group_members
        list_res: dict[str, Any] = list_group_members(web_api_auth, test_group["id"])
        assert list_res["code"] == 0
        member_user_ids: set[str] = {member["user_id"] for member in list_res["data"]}
        for user_id in user_ids:
            assert user_id not in member_user_ids

    @pytest.mark.p2
    def test_remove_member_and_list(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test removing a member and verifying via list."""
        from common import list_group_members
        
        user_id: str = test_user_with_member["id"]
        
        # List members before removal
        list_res_before: dict[str, Any] = list_group_members(
            web_api_auth, test_group["id"]
        )
        assert list_res_before["code"] == 0
        member_user_ids_before: set[str] = {
            member["user_id"] for member in list_res_before["data"]
        }
        assert user_id in member_user_ids_before
        
        # Remove member
        remove_res: dict[str, Any] = remove_group_member(
            web_api_auth, test_group["id"], user_id
        )
        assert remove_res["code"] == 0
        
        # List members after removal
        list_res_after: dict[str, Any] = list_group_members(
            web_api_auth, test_group["id"]
        )
        assert list_res_after["code"] == 0
        member_user_ids_after: set[str] = {
            member["user_id"] for member in list_res_after["data"]
        }
        assert user_id not in member_user_ids_after
        assert len(member_user_ids_after) == len(member_user_ids_before) - 1

