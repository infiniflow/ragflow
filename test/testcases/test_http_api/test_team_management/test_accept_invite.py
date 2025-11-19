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
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when accepting team invitations."""

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
        """Test accepting invitation with invalid or missing authentication."""
        # Create a team and send invitation first
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if team_res["code"] != 0:
            pytest.skip("Team creation failed, skipping auth test")
        
        tenant_id: str = team_res["data"]["id"]
        
        # Create and invite a user
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
        
        add_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, tenant_id, add_payload)

        # Try to accept invitation with invalid auth
        res: dict[str, Any] = accept_team_invitation(invalid_auth, tenant_id)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestAcceptInvite:
    """Comprehensive tests for accepting team invitations."""

    @pytest.fixture
    def test_team(self, web_api_auth: RAGFlowWebApiAuth) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def invited_user(self, web_api_auth: RAGFlowWebApiAuth) -> dict[str, Any]:
        """Create a test user who will be invited."""
        email = f"inviteduser_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Invited User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        assert user_res["code"] == 0
        return {
            "email": email,
            "id": user_res["data"]["id"],
            "password": password,
        }

    @pytest.fixture
    def team_with_invitation(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        invited_user: dict[str, Any],
    ) -> dict[str, Any]:
        """Create a team and send invitation to a user."""
        tenant_id: str = test_team["id"]
        add_payload: dict[str, list[str]] = {"users": [invited_user["email"]]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        assert add_res["code"] == 0
        return {
            "team": test_team,
            "invited_user": invited_user,
        }

    @pytest.mark.p1
    def test_accept_invitation_default_role(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_invitation: dict[str, Any],
    ) -> None:
        """Test accepting an invitation with default role (normal)."""
        tenant_id: str = team_with_invitation["team"]["id"]
        invited_user: dict[str, Any] = team_with_invitation["invited_user"]

        # Login as the invited user
        user_auth: RAGFlowWebApiAuth = login_as_user(invited_user["email"], invited_user["password"])

        # Accept the invitation
        res: dict[str, Any] = accept_team_invitation(user_auth, tenant_id)
        assert res["code"] == 0, res
        assert res["data"] is True
        assert "joined" in res["message"].lower() or "successfully" in res["message"].lower()
        assert "normal" in res["message"].lower() or "role" in res["message"].lower()

    @pytest.mark.p1
    def test_accept_invitation_with_admin_role(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_invitation: dict[str, Any],
    ) -> None:
        """Test accepting an invitation with admin role."""
        tenant_id: str = team_with_invitation["team"]["id"]
        invited_user: dict[str, Any] = team_with_invitation["invited_user"]

        # Login as the invited user
        user_auth: RAGFlowWebApiAuth = login_as_user(invited_user["email"], invited_user["password"])

        # Accept the invitation with admin role
        res: dict[str, Any] = accept_team_invitation(user_auth, tenant_id, role="admin")
        assert res["code"] == 0, res
        assert res["data"] is True
        assert "joined" in res["message"].lower() or "successfully" in res["message"].lower()
        assert "admin" in res["message"].lower() or "role" in res["message"].lower()

    @pytest.mark.p1
    def test_accept_invitation_with_normal_role(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_invitation: dict[str, Any],
    ) -> None:
        """Test accepting an invitation with normal role explicitly."""
        tenant_id: str = team_with_invitation["team"]["id"]
        invited_user: dict[str, Any] = team_with_invitation["invited_user"]

        # Login as the invited user
        user_auth: RAGFlowWebApiAuth = login_as_user(invited_user["email"], invited_user["password"])

        # Accept the invitation with normal role
        res: dict[str, Any] = accept_team_invitation(user_auth, tenant_id, role="normal")
        assert res["code"] == 0, res
        assert res["data"] is True
        assert "joined" in res["message"].lower() or "successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_accept_invitation_no_invitation(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test accepting an invitation when no invitation exists."""
        # Create a user who is not invited
        email = f"notinvited_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Not Invited User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        assert user_res["code"] == 0

        # Login as the user
        user_auth: RAGFlowWebApiAuth = login_as_user(email, password)

        # Try to accept invitation for a team they're not invited to
        tenant_id: str = test_team["id"]
        res: dict[str, Any] = accept_team_invitation(user_auth, tenant_id)
        assert res["code"] != 0
        assert "not found" in res["message"].lower() or "invitation" in res["message"].lower()

    @pytest.mark.p1
    def test_accept_invitation_already_accepted(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_invitation: dict[str, Any],
    ) -> None:
        """Test accepting an invitation that has already been accepted."""
        tenant_id: str = team_with_invitation["team"]["id"]
        invited_user: dict[str, Any] = team_with_invitation["invited_user"]

        # Login as the invited user
        user_auth: RAGFlowWebApiAuth = login_as_user(invited_user["email"], invited_user["password"])

        # Accept the invitation first time
        res1: dict[str, Any] = accept_team_invitation(user_auth, tenant_id)
        assert res1["code"] == 0

        # Try to accept again
        res2: dict[str, Any] = accept_team_invitation(user_auth, tenant_id)
        assert res2["code"] != 0
        assert "invite" in res2["message"].lower() or "role" in res2["message"].lower() or "not found" in res2["message"].lower()

    @pytest.mark.p1
    def test_accept_invitation_invalid_tenant_id(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_invitation: dict[str, Any],
    ) -> None:
        """Test accepting an invitation with invalid team ID."""
        invited_user: dict[str, Any] = team_with_invitation["invited_user"]

        # Login as the invited user
        user_auth: RAGFlowWebApiAuth = login_as_user(invited_user["email"], invited_user["password"])

        # Try to accept invitation for non-existent team
        invalid_tenant_id: str = f"invalid_{uuid.uuid4().hex[:8]}"
        res: dict[str, Any] = accept_team_invitation(user_auth, invalid_tenant_id)
        assert res["code"] != 0
        assert "not found" in res["message"].lower() or "invitation" in res["message"].lower()

    @pytest.mark.p1
    def test_accept_invitation_response_structure(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_invitation: dict[str, Any],
    ) -> None:
        """Test that accepting invitation returns the expected response structure."""
        tenant_id: str = team_with_invitation["team"]["id"]
        invited_user: dict[str, Any] = team_with_invitation["invited_user"]

        # Login as the invited user
        user_auth: RAGFlowWebApiAuth = login_as_user(invited_user["email"], invited_user["password"])

        # Accept the invitation
        res: dict[str, Any] = accept_team_invitation(user_auth, tenant_id)
        assert res["code"] == 0
        assert "data" in res
        assert res["data"] is True
        assert "message" in res
        assert isinstance(res["message"], str)
        assert "successfully" in res["message"].lower() or "joined" in res["message"].lower()

    @pytest.mark.p2
    def test_accept_invitation_wrong_user(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_invitation: dict[str, Any],
    ) -> None:
        """Test that a user cannot accept another user's invitation."""
        # Create another user who is not invited
        email = f"otheruser_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Other User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        assert user_res["code"] == 0

        # Login as the other user
        other_user_auth: RAGFlowWebApiAuth = login_as_user(email, password)

        # Try to accept invitation meant for another user
        tenant_id: str = team_with_invitation["team"]["id"]
        res: dict[str, Any] = accept_team_invitation(other_user_auth, tenant_id)
        assert res["code"] != 0
        assert "not found" in res["message"].lower() or "invitation" in res["message"].lower()

    @pytest.mark.p2
    def test_accept_invitation_multiple_invitations(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test accepting invitations to multiple teams."""
        # Create two teams
        team1_payload: dict[str, str] = {"name": f"Team 1 {uuid.uuid4().hex[:8]}"}
        team1_res: dict[str, Any] = create_team(web_api_auth, team1_payload)
        assert team1_res["code"] == 0
        tenant_id_1: str = team1_res["data"]["id"]

        team2_payload: dict[str, str] = {"name": f"Team 2 {uuid.uuid4().hex[:8]}"}
        team2_res: dict[str, Any] = create_team(web_api_auth, team2_payload)
        assert team2_res["code"] == 0
        tenant_id_2: str = team2_res["data"]["id"]

        # Create and invite a user to both teams
        email = f"multiuser_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Multi User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        assert user_res["code"] == 0

        # Invite to both teams
        add_payload1: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, tenant_id_1, add_payload1)
        add_payload2: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, tenant_id_2, add_payload2)

        # Login as the user
        user_auth: RAGFlowWebApiAuth = login_as_user(email, password)

        # Accept both invitations
        res1: dict[str, Any] = accept_team_invitation(user_auth, tenant_id_1)
        assert res1["code"] == 0

        res2: dict[str, Any] = accept_team_invitation(user_auth, tenant_id_2)
        assert res2["code"] == 0

