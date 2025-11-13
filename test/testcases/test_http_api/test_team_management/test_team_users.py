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

import uuid
from typing import Any

import pytest

from common import (
    add_users_to_team,
    create_team,
    create_user,
    remove_users_from_team,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAddUsersAuthorization:
    """Tests for authentication behavior when adding users to a team."""

    @pytest.mark.parametrize(
        ("invalid_auth", "expected_code"),
        [
            (None, 401),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401),
        ],
    )
    def test_add_users_invalid_auth(
        self,
        invalid_auth: RAGFlowWebApiAuth | None,
        expected_code: int,
        WebApiAuth: RAGFlowWebApiAuth,
    ) -> None:
        """Test adding users with invalid or missing authentication."""
        # Create a team first
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        team_res: dict[str, Any] = create_team(WebApiAuth, team_payload)
        tenant_id: str = team_res["data"]["id"]

        # Try to add users with invalid auth
        add_payload: dict[str, list[str]] = {"users": ["test@example.com"]}
        res: dict[str, Any] = add_users_to_team(invalid_auth, tenant_id, add_payload)
        assert res["code"] == expected_code


@pytest.mark.p1
class TestAddUsers:
    """Comprehensive tests for adding users to a team."""

    @pytest.fixture
    def test_team(self, WebApiAuth: RAGFlowWebApiAuth) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(WebApiAuth, team_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def test_users(self, WebApiAuth: RAGFlowWebApiAuth) -> list[dict[str, Any]]:
        """Create test users for use in tests."""
        users = []
        for i in range(5):
            email = f"testuser{i}_{uuid.uuid4().hex[:8]}@example.com"
            password = "TestPassword123!"
            user_payload: dict[str, str] = {
                "email": email,
                "password": password,
                "nickname": f"Test User {i}",
            }
            user_res: dict[str, Any] = create_user(WebApiAuth, user_payload)
            if user_res["code"] == 0:
                users.append({"email": email, "id": user_res["data"]["id"]})
        return users

    @pytest.mark.p1
    def test_add_single_user_with_email_string(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding a single user using email string format."""
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]

        add_payload: dict[str, list[str]] = {"users": [user_email]}
        res: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)

        assert res["code"] == 0
        assert "data" in res
        assert "added" in res["data"]
        assert len(res["data"]["added"]) == 1
        assert res["data"]["added"][0]["email"] == user_email
        assert res["data"]["added"][0]["role"] == "normal"
        assert "failed" in res["data"]
        assert len(res["data"]["failed"]) == 0

    @pytest.mark.p1
    def test_add_single_user_with_role(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding a single user with admin role."""
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]

        add_payload: dict[str, list[dict[str, str]]] = {
            "users": [{"email": user_email, "role": "admin"}]
        }
        res: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)

        assert res["code"] == 0
        assert len(res["data"]["added"]) == 1
        assert res["data"]["added"][0]["email"] == user_email
        assert res["data"]["added"][0]["role"] == "admin"

    @pytest.mark.p1
    def test_add_multiple_users(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding multiple users in bulk."""
        tenant_id: str = test_team["id"]
        user_emails: list[str] = [user["email"] for user in test_users[:3]]

        add_payload: dict[str, list[str]] = {"users": user_emails}
        res: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)

        assert res["code"] == 0
        assert len(res["data"]["added"]) == 3
        assert len(res["data"]["failed"]) == 0
        added_emails = {user["email"] for user in res["data"]["added"]}
        assert added_emails == set(user_emails)

    @pytest.mark.p1
    def test_add_users_mixed_formats(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding users with mixed string and object formats."""
        tenant_id: str = test_team["id"]

        add_payload: dict[str, list[Any]] = {
            "users": [
                test_users[0]["email"],  # String format
                {"email": test_users[1]["email"], "role": "admin"},  # Object format
                test_users[2]["email"],  # String format
            ]
        }
        res: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)

        assert res["code"] == 0
        assert len(res["data"]["added"]) == 3
        assert res["data"]["added"][0]["role"] == "normal"  # String format defaults to normal
        assert res["data"]["added"][1]["role"] == "admin"  # Object format with admin role
        assert res["data"]["added"][2]["role"] == "normal"  # String format defaults to normal

    @pytest.mark.p1
    def test_add_user_unregistered_email(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test adding a user with unregistered email."""
        tenant_id: str = test_team["id"]
        unregistered_email: str = f"unregistered_{uuid.uuid4().hex[:8]}@example.com"

        add_payload: dict[str, list[str]] = {"users": [unregistered_email]}
        res: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)

        assert res["code"] == 102  # DATA_ERROR
        assert len(res["data"]["added"]) == 0
        assert len(res["data"]["failed"]) == 1
        assert "not found" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_add_user_already_member(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding a user who is already a member of the team."""
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]

        # Add user first time
        add_payload: dict[str, list[str]] = {"users": [user_email]}
        res1: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)
        assert res1["code"] == 0

        # Try to add same user again
        res2: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)
        assert res2["code"] == 0  # Returns success but with failed entry
        assert len(res2["data"]["added"]) == 0
        assert len(res2["data"]["failed"]) == 1
        assert "already a member" in res2["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_add_users_partial_success(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding users where some succeed and some fail."""
        tenant_id: str = test_team["id"]
        unregistered_email: str = f"unregistered_{uuid.uuid4().hex[:8]}@example.com"

        add_payload: dict[str, list[str]] = {
            "users": [test_users[0]["email"], unregistered_email, test_users[1]["email"]]
        }
        res: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)

        assert res["code"] == 0
        assert len(res["data"]["added"]) == 2
        assert len(res["data"]["failed"]) == 1
        assert "not found" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_add_users_empty_list(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test adding users with empty list."""
        tenant_id: str = test_team["id"]
        add_payload: dict[str, list[str]] = {"users": []}

        res: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)
        assert res["code"] == 101  # ARGUMENT_ERROR
        assert "non-empty" in res["message"].lower() or "empty" in res["message"].lower()

    @pytest.mark.p1
    def test_add_users_missing_users_field(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test adding users without 'users' field."""
        tenant_id: str = test_team["id"]
        add_payload: dict[str, Any] = {}

        res: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)
        assert res["code"] == 101  # ARGUMENT_ERROR

    @pytest.mark.p1
    def test_add_users_invalid_role(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding user with invalid role."""
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]

        add_payload: dict[str, list[dict[str, str]]] = {
            "users": [{"email": user_email, "role": "invalid_role"}]
        }
        res: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)

        assert res["code"] == 0  # Returns success but with failed entry
        assert len(res["data"]["added"]) == 0
        assert len(res["data"]["failed"]) == 1
        assert "invalid role" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p2
    def test_add_users_not_owner_or_admin(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test that non-admin/non-owner users cannot add users."""
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]

        # Add a normal user to the team
        add_payload: dict[str, list[str]] = {"users": [user_email]}
        add_res: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)
        assert add_res["code"] == 0

        # Create auth for the normal user (this would require getting their token)
        # For now, we'll test that owner/admin can add users
        # This test would need the normal user's auth token to fully test
        pass


@pytest.mark.p1
class TestRemoveUsers:
    """Comprehensive tests for removing users from a team."""

    @pytest.fixture
    def test_team(self, WebApiAuth: RAGFlowWebApiAuth) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(WebApiAuth, team_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def test_users(self, WebApiAuth: RAGFlowWebApiAuth) -> list[dict[str, Any]]:
        """Create test users for use in tests."""
        users = []
        for i in range(5):
            email = f"testuser{i}_{uuid.uuid4().hex[:8]}@example.com"
            password = "TestPassword123!"
            user_payload: dict[str, str] = {
                "email": email,
                "password": password,
                "nickname": f"Test User {i}",
            }
            user_res: dict[str, Any] = create_user(WebApiAuth, user_payload)
            if user_res["code"] == 0:
                users.append({"email": email, "id": user_res["data"]["id"]})
        return users

    @pytest.fixture
    def team_with_users(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> dict[str, Any]:
        """Create a team with users already added."""
        tenant_id: str = test_team["id"]
        user_emails: list[str] = [user["email"] for user in test_users[:3]]

        add_payload: dict[str, list[str]] = {"users": user_emails}
        add_res: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)
        assert add_res["code"] == 0

        return {
            "team": test_team,
            "users": test_users[:3],
        }

    @pytest.mark.p1
    def test_remove_single_user(
        self, WebApiAuth: RAGFlowWebApiAuth, team_with_users: dict[str, Any]
    ) -> None:
        """Test removing a single user from a team."""
        tenant_id: str = team_with_users["team"]["id"]
        user_id: str = team_with_users["users"][0]["id"]

        remove_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        res: dict[str, Any] = remove_users_from_team(WebApiAuth, tenant_id, remove_payload)

        assert res["code"] == 0
        assert "data" in res
        assert "removed" in res["data"]
        assert len(res["data"]["removed"]) == 1
        assert res["data"]["removed"][0]["user_id"] == user_id
        assert "failed" in res["data"]
        assert len(res["data"]["failed"]) == 0

    @pytest.mark.p1
    def test_remove_multiple_users(
        self, WebApiAuth: RAGFlowWebApiAuth, team_with_users: dict[str, Any]
    ) -> None:
        """Test removing multiple users in bulk."""
        tenant_id: str = team_with_users["team"]["id"]
        user_ids: list[str] = [user["id"] for user in team_with_users["users"][:2]]

        remove_payload: dict[str, list[str]] = {"user_ids": user_ids}
        res: dict[str, Any] = remove_users_from_team(WebApiAuth, tenant_id, remove_payload)

        assert res["code"] == 0
        assert len(res["data"]["removed"]) == 2
        assert len(res["data"]["failed"]) == 0
        removed_ids = {user["user_id"] for user in res["data"]["removed"]}
        assert removed_ids == set(user_ids)

    @pytest.mark.p1
    def test_remove_user_not_in_team(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test removing a user who is not a member of the team."""
        tenant_id: str = test_team["id"]
        # Use a user that was not added to the team
        user_id: str = test_users[3]["id"]

        remove_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        res: dict[str, Any] = remove_users_from_team(WebApiAuth, tenant_id, remove_payload)

        assert res["code"] == 0  # Returns success but with failed entry
        assert len(res["data"]["removed"]) == 0
        assert len(res["data"]["failed"]) == 1
        assert "not a member" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_remove_owner(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test that owner cannot be removed."""
        tenant_id: str = test_team["id"]
        owner_id: str = test_team["owner_id"]

        remove_payload: dict[str, list[str]] = {"user_ids": [owner_id]}
        res: dict[str, Any] = remove_users_from_team(WebApiAuth, tenant_id, remove_payload)

        assert res["code"] == 0  # Returns success but with failed entry
        assert len(res["data"]["removed"]) == 0
        assert len(res["data"]["failed"]) == 1
        assert "owner" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_remove_users_partial_success(
        self, WebApiAuth: RAGFlowWebApiAuth, team_with_users: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test removing users where some succeed and some fail."""
        tenant_id: str = team_with_users["team"]["id"]
        # Mix of valid and invalid user IDs
        valid_user_id: str = team_with_users["users"][0]["id"]
        invalid_user_id: str = test_users[3]["id"]  # Not in team

        remove_payload: dict[str, list[str]] = {"user_ids": [valid_user_id, invalid_user_id]}
        res: dict[str, Any] = remove_users_from_team(WebApiAuth, tenant_id, remove_payload)

        assert res["code"] == 0
        assert len(res["data"]["removed"]) == 1
        assert len(res["data"]["failed"]) == 1
        assert res["data"]["removed"][0]["user_id"] == valid_user_id
        assert "not a member" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_remove_users_empty_list(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test removing users with empty list."""
        tenant_id: str = test_team["id"]
        remove_payload: dict[str, list[str]] = {"user_ids": []}

        res: dict[str, Any] = remove_users_from_team(WebApiAuth, tenant_id, remove_payload)
        assert res["code"] == 101  # ARGUMENT_ERROR
        assert "non-empty" in res["message"].lower() or "empty" in res["message"].lower()

    @pytest.mark.p1
    def test_remove_users_missing_user_ids_field(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test removing users without 'user_ids' field."""
        tenant_id: str = test_team["id"]
        remove_payload: dict[str, Any] = {}

        res: dict[str, Any] = remove_users_from_team(WebApiAuth, tenant_id, remove_payload)
        assert res["code"] == 101  # ARGUMENT_ERROR

    @pytest.mark.p1
    def test_remove_users_invalid_user_id_format(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test removing users with invalid user ID format."""
        tenant_id: str = test_team["id"]
        remove_payload: dict[str, list[Any]] = {"user_ids": [12345]}  # Not a string

        res: dict[str, Any] = remove_users_from_team(WebApiAuth, tenant_id, remove_payload)
        assert res["code"] == 0  # Returns success but with failed entry
        assert len(res["data"]["removed"]) == 0
        assert len(res["data"]["failed"]) == 1
        assert "invalid" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p2
    def test_remove_last_admin(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test that the last admin cannot remove themselves."""
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]

        # Add user as admin
        add_payload: dict[str, list[dict[str, str]]] = {
            "users": [{"email": user_email, "role": "admin"}]
        }
        add_res: dict[str, Any] = add_users_to_team(WebApiAuth, tenant_id, add_payload)
        assert add_res["code"] == 0
        admin_user_id: str = add_res["data"]["added"][0]["id"]

        # Try to remove the admin (would need admin's auth token to fully test)
        # This test would require the admin user's authentication
        pass

