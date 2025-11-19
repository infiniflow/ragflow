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
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when adding members to a group."""

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
        """Test adding members with invalid or missing authentication."""
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
        
        # Try to add members with invalid auth
        add_payload: dict[str, list[str]] = {"user_ids": ["test_user_id"]}
        res: dict[str, Any] = add_group_members(invalid_auth, group_id, add_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestAddMembers:
    """Comprehensive tests for adding members to a group."""

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
    def test_users(self, web_api_auth: RAGFlowWebApiAuth) -> list[dict[str, Any]]:
        """Create test users for use in tests."""
        users = []
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
            if user_res["code"] == 0:
                users.append({"email": email, "id": user_res["data"]["id"], "password": password})
        return users

    @pytest.fixture
    def team_with_users(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> dict[str, Any]:
        """Add test users to the team."""
        for user in test_users:
            add_payload: dict[str, list[str]] = {"users": [user["email"]]}
            add_users_to_team(web_api_auth, test_team["id"], add_payload)
        return test_team

    @pytest.mark.p1
    def test_add_single_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test adding a single member to a group."""
        if not test_users:
            pytest.skip("No test users created")
        
        user_id: str = test_users[0]["id"]
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        res: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_payload
        )
        
        assert res["code"] == 0, res
        assert "data" in res
        assert "added" in res["data"]
        assert "failed" in res["data"]
        assert len(res["data"]["added"]) == 1
        assert res["data"]["added"][0] == user_id
        assert len(res["data"]["failed"]) == 0

    @pytest.mark.p1
    def test_add_multiple_members(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test adding multiple members to a group."""
        if len(test_users) < 2:
            pytest.skip("Need at least 2 test users")
        
        user_ids: list[str] = [user["id"] for user in test_users[:2]]
        add_payload: dict[str, list[str]] = {"user_ids": user_ids}
        res: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_payload
        )
        
        assert res["code"] == 0, res
        assert len(res["data"]["added"]) == 2
        assert set(res["data"]["added"]) == set(user_ids)
        assert len(res["data"]["failed"]) == 0

    @pytest.mark.p1
    def test_add_member_missing_request_body(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
    ) -> None:
        """Test adding members without request body."""
        res: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], None
        )
        
        assert res["code"] == 101
        assert "required" in res["message"].lower() or "body" in res["message"].lower()

    @pytest.mark.p1
    def test_add_member_missing_user_ids(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
    ) -> None:
        """Test adding members without user_ids."""
        add_payload: dict[str, Any] = {}
        res: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_payload
        )
        
        assert res["code"] == 101
        assert "user_ids" in res["message"].lower() or "non-empty array" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_add_member_empty_user_ids(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
    ) -> None:
        """Test adding members with empty user_ids array."""
        add_payload: dict[str, list[str]] = {"user_ids": []}
        res: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_payload
        )
        
        assert res["code"] == 101
        assert "non-empty array" in res["message"].lower() or "empty" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_add_member_invalid_user_id(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
    ) -> None:
        """Test adding a non-existent user."""
        add_payload: dict[str, list[str]] = {
            "user_ids": ["non_existent_user_id_12345"]
        }
        res: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_payload
        )
        
        assert res["code"] == 0  # API returns success with failed list
        assert len(res["data"]["added"]) == 0
        assert len(res["data"]["failed"]) == 1
        assert "not found" in res["data"]["failed"][0]["error"].lower() or "invalid" in res[
            "data"
        ]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_add_member_user_not_in_team(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
    ) -> None:
        """Test adding a user who is not a member of the team."""
        # Create a user but don't add to team
        email = f"notinteam_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Not In Team User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        assert user_res["code"] == 0
        
        user_id: str = user_res["data"]["id"]
        
        # Try to add to group (should fail - user not in team)
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        res: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_payload
        )
        
        assert res["code"] == 0  # API returns success with failed list
        assert len(res["data"]["added"]) == 0
        assert len(res["data"]["failed"]) == 1
        assert "not a member of the team" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_add_duplicate_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test adding a user who is already in the group."""
        if not test_users:
            pytest.skip("No test users created")
        
        user_id: str = test_users[0]["id"]
        
        # Add user first time
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        res1: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_payload
        )
        assert res1["code"] == 0
        assert len(res1["data"]["added"]) == 1
        
        # Try to add same user again
        res2: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_payload
        )
        assert res2["code"] == 0  # API returns success with failed list
        assert len(res2["data"]["added"]) == 0
        assert len(res2["data"]["failed"]) == 1
        assert "already a member" in res2["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_add_member_invalid_group_id(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test adding members to a non-existent group."""
        if not test_users:
            pytest.skip("No test users created")
        
        user_id: str = test_users[0]["id"]
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        res: dict[str, Any] = add_group_members(
            web_api_auth, "non_existent_group_id_12345", add_payload
        )
        
        assert res["code"] == 102
        assert "group not found" in res["message"].lower() or "not found" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_add_member_invalid_user_id_format(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
    ) -> None:
        """Test adding members with invalid user ID format."""
        add_payload: dict[str, list[Any]] = {"user_ids": [12345]}  # Not a string
        res: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_payload
        )
        
        assert res["code"] == 0  # API returns success with failed list
        assert len(res["data"]["added"]) == 0
        assert len(res["data"]["failed"]) == 1
        assert "invalid" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p2
    def test_add_member_not_team_owner_or_admin(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test that non-admin/non-owner users cannot add members to groups."""
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
        
        # Small delay to ensure user is fully created
        time.sleep(0.5)
        
        # Login as the other user
        other_user_auth: RAGFlowWebApiAuth = login_as_user(
            other_user_email, other_user_password
        )
        
        # Try to add a member to the group as the other user
        # First, we need another user in the team to add
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
        
        # Try to add member as non-admin user
        add_payload: dict[str, list[str]] = {"user_ids": [third_user_id]}
        res: dict[str, Any] = add_group_members(other_user_auth, group_id, add_payload)
        
        # Should fail - user is not the team owner or admin
        assert res["code"] != 0, (
            "Non-owner/non-admin should not be able to add members to groups"
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
    def test_add_and_list_members(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test adding members and then listing them."""
        if not test_users:
            pytest.skip("No test users created")
        
        from common import list_group_members
        
        # List members before adding
        list_res_before: dict[str, Any] = list_group_members(
            web_api_auth, test_group["id"]
        )
        assert list_res_before["code"] == 0
        initial_count: int = len(list_res_before["data"])
        
        # Add a user
        user_id: str = test_users[0]["id"]
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_res: dict[str, Any] = add_group_members(
            web_api_auth, test_group["id"], add_payload
        )
        assert add_res["code"] == 0
        assert len(add_res["data"]["added"]) == 1
        
        # List members after adding
        list_res_after: dict[str, Any] = list_group_members(
            web_api_auth, test_group["id"]
        )
        assert list_res_after["code"] == 0
        assert len(list_res_after["data"]) == initial_count + 1
        
        # Verify the added user is in the list
        member_user_ids: set[str] = {member["user_id"] for member in list_res_after["data"]}
        assert user_id in member_user_ids

