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
    accept_team_invitation,
    add_users_to_team,
    create_team,
    create_user,
    encrypt_password,
    login_as_user,
    promote_admin,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when promoting users to admin."""

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
        """Test promoting user with invalid or missing authentication."""
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

        # Small delay
        time.sleep(0.5)

        # Accept invitation as the user
        user_auth: RAGFlowWebApiAuth = login_as_user(email, password)
        accept_team_invitation(user_auth, tenant_id)

        # Try to promote user with invalid auth
        res: dict[str, Any] = promote_admin(invalid_auth, tenant_id, user_id)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestPromoteAdmin:
    """Comprehensive tests for promoting users to admin."""

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
    def team_with_normal_user(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> dict[str, Any]:
        """Create a team with a normal user who has accepted the invitation."""
        if not test_users:
            return {"team": test_team, "user": None}
        
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]
        user_password: str = test_users[0]["password"]

        # Add user to team
        add_payload: dict[str, list[str]] = {"users": [user_email]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert add_res["code"] == 0

        # Small delay
        time.sleep(0.5)

        # Accept invitation as the user
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        accept_res: dict[str, Any] = accept_team_invitation(user_auth, tenant_id)
        assert accept_res["code"] == 0

        return {
            "team": test_team,
            "user": test_users[0],
        }

    @pytest.mark.p1
    def test_promote_normal_user_to_admin(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_normal_user: dict[str, Any],
    ) -> None:
        """Test promoting a normal user to admin."""
        if not team_with_normal_user["user"]:
            pytest.skip("No user in team")
        
        tenant_id: str = team_with_normal_user["team"]["id"]
        user_id: str = team_with_normal_user["user"]["id"]

        # Promote user to admin
        res: dict[str, Any] = promote_admin(web_api_auth, tenant_id, user_id)
        assert res["code"] == 0, res
        assert res["data"] is True
        assert "promoted" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p1
    def test_promote_user_already_admin(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test promoting a user who is already an admin."""
        if not test_users:
            pytest.skip("No test users created")
        
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]
        user_password: str = test_users[0]["password"]

        # Add user to team
        add_payload: dict[str, list[str]] = {"users": [user_email]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert add_res["code"] == 0

        # Small delay
        time.sleep(0.5)

        # Accept invitation
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        accept_res: dict[str, Any] = accept_team_invitation(user_auth, tenant_id)
        assert accept_res["code"] == 0

        user_id: str = test_users[0]["id"]

        # Promote user to admin first
        promote_res: dict[str, Any] = promote_admin(web_api_auth, tenant_id, user_id)
        assert promote_res["code"] == 0

        # Try to promote again (should return success but indicate already admin)
        res: dict[str, Any] = promote_admin(web_api_auth, tenant_id, user_id)
        assert res["code"] == 0, res
        assert res["data"] is True
        assert "already" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p1
    def test_promote_user_not_in_team(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any], test_users: list[dict[str, Any]]
    ) -> None:
        """Test promoting a user who is not a member of the team."""
        if not test_users:
            pytest.skip("No test users created")
        
        tenant_id: str = test_team["id"]
        # Use a user that was not added to the team
        user_id: str = test_users[1]["id"] if len(test_users) > 1 else test_users[0]["id"]

        res: dict[str, Any] = promote_admin(web_api_auth, tenant_id, user_id)
        assert res["code"] != 0
        assert "not a member" in res["message"].lower() or res["code"] in [100, 102]

    @pytest.mark.p1
    def test_promote_owner(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test that owner cannot be promoted (owner role is permanent)."""
        tenant_id: str = test_team["id"]
        owner_id: str = test_team["owner_id"]

        res: dict[str, Any] = promote_admin(web_api_auth, tenant_id, owner_id)
        assert res["code"] != 0
        assert "owner" in res["message"].lower() or "permanent" in res["message"].lower()

    @pytest.mark.p1
    def test_promote_user_not_owner_or_admin(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_normal_user: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test that non-admin/non-owner users cannot promote others."""
        if not team_with_normal_user["user"] or len(test_users) < 2:
            pytest.skip("Need user in team and at least 2 test users")
        
        tenant_id: str = team_with_normal_user["team"]["id"]
        normal_user_email: str = team_with_normal_user["user"]["email"]
        normal_user_password: str = team_with_normal_user["user"]["password"]
        other_user_email: str = test_users[1]["email"]
        other_user_password: str = test_users[1]["password"]

        # Add another user to the team
        add_payload: dict[str, list[str]] = {"users": [other_user_email]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert add_res["code"] == 0

        # Small delay
        time.sleep(0.5)

        # Accept invitation as the other user
        other_user_auth: RAGFlowWebApiAuth = login_as_user(other_user_email, other_user_password)
        accept_team_invitation(other_user_auth, tenant_id)

        # Login as the normal user (not admin/owner)
        normal_user_auth: RAGFlowWebApiAuth = login_as_user(normal_user_email, normal_user_password)
        other_user_id: str = test_users[1]["id"]

        # Try to promote the other user (normal user should not be able to)
        res: dict[str, Any] = promote_admin(normal_user_auth, tenant_id, other_user_id)
        assert res["code"] == 108  # PERMISSION_ERROR
        assert "owner" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p1
    def test_promote_user_invalid_tenant_id(
        self, web_api_auth: RAGFlowWebApiAuth, test_users: list[dict[str, Any]]
    ) -> None:
        """Test promoting a user with invalid team ID."""
        if not test_users:
            pytest.skip("No test users created")
        
        invalid_tenant_id: str = f"invalid_{uuid.uuid4().hex[:8]}"
        user_id: str = test_users[0]["id"]

        res: dict[str, Any] = promote_admin(web_api_auth, invalid_tenant_id, user_id)
        assert res["code"] != 0
        assert res["code"] in [100, 102, 108]

    @pytest.mark.p1
    def test_promote_user_invalid_user_id(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test promoting a user with invalid user ID."""
        tenant_id: str = test_team["id"]
        invalid_user_id: str = f"invalid_{uuid.uuid4().hex[:8]}"

        res: dict[str, Any] = promote_admin(web_api_auth, tenant_id, invalid_user_id)
        assert res["code"] != 0
        assert "not a member" in res["message"].lower() or res["code"] in [100, 102]

    @pytest.mark.p1
    def test_promote_user_response_structure(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_normal_user: dict[str, Any],
    ) -> None:
        """Test that promoting user returns the expected response structure."""
        if not team_with_normal_user["user"]:
            pytest.skip("No user in team")
        
        tenant_id: str = team_with_normal_user["team"]["id"]
        user_id: str = team_with_normal_user["user"]["id"]

        res: dict[str, Any] = promote_admin(web_api_auth, tenant_id, user_id)
        assert res["code"] == 0
        assert "data" in res
        assert res["data"] is True
        assert "message" in res
        assert isinstance(res["message"], str)
        assert "promoted" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p2
    def test_promote_user_from_invite_role(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test promoting a user who has invite role (hasn't accepted yet)."""
        if not test_users:
            pytest.skip("No test users created")
        
        tenant_id: str = test_team["id"]
        user_email: str = test_users[0]["email"]

        # Add user to team (they'll have invite role)
        add_payload: dict[str, list[str]] = {"users": [user_email]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert add_res["code"] == 0

        user_id: str = test_users[0]["id"]

        # Promote user before they accept invitation
        res: dict[str, Any] = promote_admin(web_api_auth, tenant_id, user_id)
        assert res["code"] == 0, res
        assert res["data"] is True
        assert "promoted" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p2
    def test_promote_multiple_users_sequentially(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test promoting multiple users to admin sequentially."""
        if len(test_users) < 3:
            pytest.skip("Need at least 3 test users")
        
        tenant_id: str = test_team["id"]
        user_emails: list[str] = [user["email"] for user in test_users[:3]]
        user_passwords: list[str] = [user["password"] for user in test_users[:3]]

        # Add users to team
        add_payload: dict[str, list[str]] = {"users": user_emails}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert add_res["code"] == 0

        # Small delay
        time.sleep(0.5)

        # Accept invitations
        for email, password in zip(user_emails, user_passwords):
            user_auth: RAGFlowWebApiAuth = login_as_user(email, password)
            accept_team_invitation(user_auth, tenant_id)

        # Promote all users to admin
        for user in test_users[:3]:
            user_id: str = user["id"]
            res: dict[str, Any] = promote_admin(web_api_auth, tenant_id, user_id)
            assert res["code"] == 0, res
            assert res["data"] is True

