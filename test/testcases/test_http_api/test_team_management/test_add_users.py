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
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when adding users to a team."""

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
        """Test adding users with invalid or missing authentication."""
        # Create a team first
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if team_res["code"] != 0:
            pytest.skip("Team creation failed, skipping auth test")
        
        tenant_id: str = team_res["data"]["id"]

        # Try to add users with invalid auth
        add_payload: dict[str, list[str]] = {"users": ["test@example.com"]}
        res: dict[str, Any] = add_users_to_team(invalid_auth, tenant_id, add_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestAddUsers:
    """Comprehensive tests for adding users to a team."""

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

    @pytest.mark.p1
    def test_add_single_user_with_email_string(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding a single user using email string format."""
        if not test_users:
            pytest.skip("No test users created")
        
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]

        add_payload: dict[str, list[str]] = {"users": [user_email]}
        res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)

        assert res["code"] == 0, res
        assert "data" in res
        assert "added" in res["data"]
        assert len(res["data"]["added"]) == 1
        assert res["data"]["added"][0]["email"] == user_email
        assert res["data"]["added"][0]["role"] == "invite"  # Users are added with invite role initially
        assert "failed" in res["data"]
        assert len(res["data"]["failed"]) == 0

    @pytest.mark.p1
    def test_add_single_user_with_role(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding a single user with admin role."""
        if not test_users:
            pytest.skip("No test users created")
        
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]

        add_payload: dict[str, list[dict[str, str]]] = {
            "users": [{"email": user_email, "role": "admin"}]
        }
        res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)

        assert res["code"] == 0, res
        assert len(res["data"]["added"]) == 1
        assert res["data"]["added"][0]["email"] == user_email
        assert res["data"]["added"][0]["role"] == "invite"  # Users are added with invite role initially
        assert res["data"]["added"][0]["intended_role"] == "admin"  # Intended role after acceptance

    @pytest.mark.p1
    def test_add_multiple_users(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding multiple users in bulk."""
        if len(test_users) < 3:
            pytest.skip("Need at least 3 test users")
        
        tenant_id: str = test_team["id"]
        user_emails: list[str] = [user["email"] for user in test_users[:3]]

        add_payload: dict[str, list[str]] = {"users": user_emails}
        res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)

        assert res["code"] == 0, res
        assert len(res["data"]["added"]) == 3
        assert len(res["data"]["failed"]) == 0
        added_emails = {user["email"] for user in res["data"]["added"]}
        assert added_emails == set(user_emails)

    @pytest.mark.p1
    def test_add_users_mixed_formats(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding users with mixed string and object formats."""
        if len(test_users) < 3:
            pytest.skip("Need at least 3 test users")
        
        tenant_id: str = test_team["id"]

        add_payload: dict[str, list[Any]] = {
            "users": [
                test_users[0]["email"],  # String format
                {"email": test_users[1]["email"], "role": "admin"},  # Object format
                test_users[2]["email"],  # String format
            ]
        }
        res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)

        assert res["code"] == 0, res
        assert len(res["data"]["added"]) == 3
        assert res["data"]["added"][0]["role"] == "invite"  # String format defaults to invite role initially
        assert res["data"]["added"][0].get("intended_role") == "normal"  # Intended role after acceptance
        assert res["data"]["added"][1]["role"] == "invite"  # Object format - still invite initially
        assert res["data"]["added"][1]["intended_role"] == "admin"  # Intended role after acceptance
        assert res["data"]["added"][2]["role"] == "invite"  # String format defaults to invite role initially
        assert res["data"]["added"][2].get("intended_role") == "normal"  # Intended role after acceptance

    @pytest.mark.p1
    def test_add_user_unregistered_email(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test adding a user with unregistered email."""
        tenant_id: str = test_team["id"]
        unregistered_email: str = f"unregistered_{uuid.uuid4().hex[:8]}@example.com"

        add_payload: dict[str, list[str]] = {"users": [unregistered_email]}
        res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)

        assert res["code"] == 102  # DATA_ERROR
        assert len(res["data"]["added"]) == 0
        assert len(res["data"]["failed"]) == 1
        assert "not found" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_add_user_already_member(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding a user who is already a member of the team."""
        if not test_users:
            pytest.skip("No test users created")
        
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]

        # Add user first time
        add_payload: dict[str, list[str]] = {"users": [user_email]}
        res1: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert res1["code"] == 0

        # Try to add same user again
        res2: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert res2["code"] == 0  # Returns success - invitation is resent
        # API resends invitation instead of failing
        assert len(res2["data"]["added"]) == 1
        assert res2["data"]["added"][0]["email"] == user_email
        assert res2["data"]["added"][0].get("status") == "invitation_resent" or "intended_role" in res2["data"]["added"][0]

    @pytest.mark.p1
    def test_add_users_partial_success(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding users where some succeed and some fail."""
        if len(test_users) < 2:
            pytest.skip("Need at least 2 test users")
        
        tenant_id: str = test_team["id"]
        unregistered_email: str = f"unregistered_{uuid.uuid4().hex[:8]}@example.com"

        add_payload: dict[str, list[str]] = {
            "users": [test_users[0]["email"], unregistered_email, test_users[1]["email"]]
        }
        res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)

        assert res["code"] == 0, res
        assert len(res["data"]["added"]) == 2
        assert len(res["data"]["failed"]) == 1
        assert "not found" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_add_users_empty_list(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test adding users with empty list."""
        tenant_id: str = test_team["id"]
        add_payload: dict[str, list[str]] = {"users": []}

        res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert res["code"] == 101  # ARGUMENT_ERROR
        assert "non-empty" in res["message"].lower() or "empty" in res["message"].lower()

    @pytest.mark.p1
    def test_add_users_missing_users_field(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test adding users without 'users' field."""
        tenant_id: str = test_team["id"]
        add_payload: dict[str, Any] = {}

        res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert res["code"] == 101  # ARGUMENT_ERROR

    @pytest.mark.p1
    def test_add_users_invalid_role(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding user with invalid role."""
        if not test_users:
            pytest.skip("No test users created")
        
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]

        add_payload: dict[str, list[dict[str, str]]] = {
            "users": [{"email": user_email, "role": "invalid_role"}]
        }
        res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)

        # API returns error code when all users fail
        assert res["code"] == 102  # DATA_ERROR
        assert len(res["data"]["added"]) == 0
        assert len(res["data"]["failed"]) == 1
        assert "invalid role" in res["data"]["failed"][0]["error"].lower() or "invalid" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_add_users_not_owner_or_admin(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test that non-admin/non-owner users cannot add users."""
        if not test_users:
            pytest.skip("No test users created")
        
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]
        other_user_email: str = test_users[1]["email"] if len(test_users) > 1 else None

        if not other_user_email:
            pytest.skip("Need at least 2 test users")

        # Add a normal user to the team
        add_payload: dict[str, list[str]] = {"users": [user_email]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert add_res["code"] == 0

        # Small delay to ensure user is fully added
        time.sleep(0.5)

        # Login as the normal user
        normal_user_auth: RAGFlowWebApiAuth = login_as_user(user_email, test_users[0]["password"])

        # Try to add another user (normal user should not be able to)
        add_payload2: dict[str, list[str]] = {"users": [other_user_email]}
        res: dict[str, Any] = add_users_to_team(normal_user_auth, tenant_id, add_payload2)
        assert res["code"] == 108  # PERMISSION_ERROR
        assert "owner" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p2
    def test_add_users_invalid_tenant_id(
        self, web_api_auth: RAGFlowWebApiAuth, test_users: list[dict[str, Any]]
    ) -> None:
        """Test adding users to a non-existent team."""
        if not test_users:
            pytest.skip("No test users created")
        
        invalid_tenant_id: str = f"invalid_{uuid.uuid4().hex[:8]}"
        add_payload: dict[str, list[str]] = {"users": [test_users[0]["email"]]}

        res: dict[str, Any] = add_users_to_team(web_api_auth, invalid_tenant_id, add_payload)
        assert res["code"] != 0
        assert "not found" in res["message"].lower() or res["code"] in [100, 102, 108]

    @pytest.mark.p2
    def test_add_users_invalid_email_format(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test adding users with invalid email format."""
        tenant_id: str = test_team["id"]
        invalid_email: str = "not_an_email"

        add_payload: dict[str, list[str]] = {"users": [invalid_email]}
        res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)

        # Should fail - either validation error or user not found
        assert res["code"] != 0
        assert len(res["data"]["added"]) == 0

