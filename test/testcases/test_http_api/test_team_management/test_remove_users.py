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
    add_users_to_team,
    create_team,
    create_user,
    encrypt_password,
    login_as_user,
    remove_user_from_team,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when removing users from a team."""

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
        """Test removing users with invalid or missing authentication."""
        # Create a team and add a user first
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if team_res["code"] != 0:
            pytest.skip("Team creation failed, skipping auth test")
        
        tenant_id: str = team_res["data"]["id"]
        
        # Create and add a user
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
        add_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, tenant_id, add_payload)

        # Try to remove user with invalid auth
        remove_payload: dict[str, str] = {"user_id": user_id}
        res: dict[str, Any] = remove_user_from_team(invalid_auth, tenant_id, remove_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestRemoveUser:
    """Comprehensive tests for removing a user from a team."""

    @pytest.fixture
    def test_team(self, web_api_auth: RAGFlowWebApiAuth) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def test_users(self, web_api_auth: RAGFlowWebApiAuth) -> list[dict[str, Any]]:
        """Create test users for use in tests."""
        users = []
        for i in range(5):
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
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> dict[str, Any]:
        """Create a team with users already added."""
        if not test_users:
            return {"team": test_team, "users": []}
        
        tenant_id: str = test_team["id"]
        user_emails: list[str] = [user["email"] for user in test_users[:3]]

        add_payload: dict[str, list[str]] = {"users": user_emails}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert add_res["code"] == 0

        return {
            "team": test_team,
            "users": test_users[:3],
        }

    @pytest.mark.p1
    def test_remove_single_user(
        self, web_api_auth: RAGFlowWebApiAuth, team_with_users: dict[str, Any]
    ) -> None:
        """Test removing a single user from a team."""
        if not team_with_users["users"]:
            pytest.skip("No users in team")
        
        tenant_id: str = team_with_users["team"]["id"]
        user_id: str = team_with_users["users"][0]["id"]

        remove_payload: dict[str, str] = {"user_id": user_id}
        res: dict[str, Any] = remove_user_from_team(web_api_auth, tenant_id, remove_payload)

        assert res["code"] == 0, res
        assert "data" in res
        assert res["data"]["user_id"] == user_id
        assert "email" in res["data"]

    @pytest.mark.p1
    def test_remove_multiple_users(
        self, web_api_auth: RAGFlowWebApiAuth, team_with_users: dict[str, Any]
    ) -> None:
        """Test removing multiple users one by one."""
        if len(team_with_users["users"]) < 2:
            pytest.skip("Need at least 2 users in team")
        
        tenant_id: str = team_with_users["team"]["id"]
        user_ids: list[str] = [user["id"] for user in team_with_users["users"][:2]]

        # Remove first user
        remove_payload1: dict[str, str] = {"user_id": user_ids[0]}
        res1: dict[str, Any] = remove_user_from_team(web_api_auth, tenant_id, remove_payload1)
        assert res1["code"] == 0, res1
        assert res1["data"]["user_id"] == user_ids[0]

        # Remove second user
        remove_payload2: dict[str, str] = {"user_id": user_ids[1]}
        res2: dict[str, Any] = remove_user_from_team(web_api_auth, tenant_id, remove_payload2)
        assert res2["code"] == 0, res2
        assert res2["data"]["user_id"] == user_ids[1]

    @pytest.mark.p1
    def test_remove_user_not_in_team(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test removing a user who is not a member of the team."""
        if len(test_users) < 4:
            pytest.skip("Need at least 4 test users")
        
        tenant_id: str = test_team["id"]
        # Use a user that was not added to the team
        user_id: str = test_users[3]["id"]

        remove_payload: dict[str, str] = {"user_id": user_id}
        res: dict[str, Any] = remove_user_from_team(web_api_auth, tenant_id, remove_payload)

        # API returns error code when removal fails
        assert res["code"] == 102  # DATA_ERROR
        assert "not a member" in res["message"].lower()

    @pytest.mark.p1
    def test_remove_owner(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test that owner cannot be removed."""
        tenant_id: str = test_team["id"]
        owner_id: str = test_team["owner_id"]

        remove_payload: dict[str, str] = {"user_id": owner_id}
        res: dict[str, Any] = remove_user_from_team(web_api_auth, tenant_id, remove_payload)

        # API returns error code when removal fails
        assert res["code"] == 102  # DATA_ERROR
        assert "owner" in res["message"].lower()

    @pytest.mark.p1
    def test_remove_users_partial_success(
        self, web_api_auth: RAGFlowWebApiAuth, team_with_users: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test removing users one by one where some succeed and some fail."""
        if not team_with_users["users"] or len(test_users) < 4:
            pytest.skip("Need users in team and at least 4 test users")
        
        tenant_id: str = team_with_users["team"]["id"]
        # Mix of valid and invalid user IDs
        valid_user_id: str = team_with_users["users"][0]["id"]
        invalid_user_id: str = test_users[3]["id"]  # Not in team

        # First remove valid user - should succeed
        remove_payload1: dict[str, str] = {"user_id": valid_user_id}
        res1: dict[str, Any] = remove_user_from_team(web_api_auth, tenant_id, remove_payload1)
        assert res1["code"] == 0, res1
        assert res1["data"]["user_id"] == valid_user_id

        # Then try to remove invalid user - should fail
        remove_payload2: dict[str, str] = {"user_id": invalid_user_id}
        res2: dict[str, Any] = remove_user_from_team(web_api_auth, tenant_id, remove_payload2)
        assert res2["code"] == 102  # DATA_ERROR
        assert "not a member" in res2["message"].lower()

    @pytest.mark.p1
    def test_remove_users_empty_user_id(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test removing user with empty user_id."""
        tenant_id: str = test_team["id"]
        remove_payload: dict[str, str] = {"user_id": ""}

        res: dict[str, Any] = remove_user_from_team(web_api_auth, tenant_id, remove_payload)
        assert res["code"] == 101  # ARGUMENT_ERROR
        assert "non-empty" in res["message"].lower() or "empty" in res["message"].lower()

    @pytest.mark.p1
    def test_remove_users_missing_user_id_field(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test removing user without 'user_id' field."""
        tenant_id: str = test_team["id"]
        remove_payload: dict[str, Any] = {}

        res: dict[str, Any] = remove_user_from_team(web_api_auth, tenant_id, remove_payload)
        assert res["code"] == 101  # ARGUMENT_ERROR

    @pytest.mark.p1
    def test_remove_users_invalid_user_id_format(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test removing user with invalid user ID format."""
        tenant_id: str = test_team["id"]
        remove_payload: dict[str, Any] = {"user_id": 12345}  # Not a string

        res: dict[str, Any] = remove_user_from_team(web_api_auth, tenant_id, remove_payload)
        # API returns error code when removal fails
        assert res["code"] == 101  # ARGUMENT_ERROR (validation should catch non-string)

    @pytest.mark.p1
    def test_remove_users_not_owner_or_admin(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test that non-admin/non-owner users cannot remove users."""
        if len(test_users) < 2:
            pytest.skip("Need at least 2 test users")
        
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]
        other_user_email: str = test_users[1]["email"]

        # Add two users to the team
        add_payload: dict[str, list[str]] = {"users": [user_email, other_user_email]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert add_res["code"] == 0

        # Small delay to ensure users are fully added
        time.sleep(0.5)

        # Login as the first normal user
        normal_user_auth: RAGFlowWebApiAuth = login_as_user(user_email, test_users[0]["password"])

        # Try to remove the other user (normal user should not be able to)
        other_user_id: str = test_users[1]["id"]
        remove_payload: dict[str, str] = {"user_id": other_user_id}
        res: dict[str, Any] = remove_user_from_team(normal_user_auth, tenant_id, remove_payload)
        assert res["code"] == 108  # PERMISSION_ERROR
        assert "owner" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p2
    def test_remove_last_admin(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test that the last admin cannot remove themselves."""
        if not test_users:
            pytest.skip("No test users created")
        
        from common import accept_team_invitation
        
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]

        # Add user as admin
        add_payload: dict[str, list[dict[str, str]]] = {
            "users": [{"email": user_email, "role": "admin"}]
        }
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert add_res["code"] == 0
        
        # Small delay
        time.sleep(0.5)
        
        # Login as the admin
        admin_auth: RAGFlowWebApiAuth = login_as_user(user_email, test_users[0]["password"])
        
        # Accept the invitation to become admin
        accept_res: dict[str, Any] = accept_team_invitation(admin_auth, tenant_id, role="admin")
        assert accept_res["code"] == 0
        
        # Small delay to ensure role is updated
        time.sleep(0.5)
        
        admin_user_id: str = test_users[0]["id"]

        # Try to remove the admin (should fail - last admin cannot remove themselves)
        remove_payload: dict[str, str] = {"user_id": admin_user_id}
        res: dict[str, Any] = remove_user_from_team(admin_auth, tenant_id, remove_payload)
        # API may return error code when removal fails, or permission error if role not updated
        assert res["code"] in [102, 108]  # DATA_ERROR or PERMISSION_ERROR
        if res["code"] == 102:
            # If we get DATA_ERROR, check the message
            assert "cannot remove yourself" in res["message"].lower() or "at least one" in res["message"].lower()
        else:
            # If we get PERMISSION_ERROR, the user might not have admin role yet
            assert "owner" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p2
    def test_remove_users_invalid_tenant_id(
        self, web_api_auth: RAGFlowWebApiAuth, test_users: list[dict[str, Any]]
    ) -> None:
        """Test removing user from a non-existent team."""
        if not test_users:
            pytest.skip("No test users created")
        
        invalid_tenant_id: str = f"invalid_{uuid.uuid4().hex[:8]}"
        remove_payload: dict[str, str] = {"user_id": test_users[0]["id"]}

        res: dict[str, Any] = remove_user_from_team(web_api_auth, invalid_tenant_id, remove_payload)
        assert res["code"] != 0
        assert "not found" in res["message"].lower() or res["code"] in [100, 102, 108]

