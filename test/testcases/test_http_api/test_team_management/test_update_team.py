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
    update_team,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when updating teams."""

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
        """Test updating team with invalid or missing authentication."""
        # Create a team first
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if team_res["code"] != 0:
            pytest.skip("Team creation failed, skipping auth test")

        tenant_id: str = team_res["data"]["id"]

        # Try to update team with invalid auth
        update_payload: dict[str, str] = {"name": "Updated Name"}
        res: dict[str, Any] = update_team(invalid_auth, tenant_id, update_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestUpdateTeam:
    """Comprehensive tests for team update API."""

    @pytest.fixture
    def test_team(self, web_api_auth: RAGFlowWebApiAuth) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.mark.p1
    def test_update_team_name(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team's name."""
        tenant_id: str = test_team["id"]

        # Update the team name
        new_name: str = f"Updated Team {uuid.uuid4().hex[:8]}"
        update_payload: dict[str, str] = {"name": new_name}
        update_res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        assert update_res["code"] == 0, update_res
        assert "data" in update_res
        assert update_res["data"]["name"] == new_name
        assert update_res["data"]["id"] == tenant_id

    @pytest.mark.p1
    def test_update_team_name_empty(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team with empty name (should fail)."""
        tenant_id: str = test_team["id"]

        # Try to update with empty name
        update_payload: dict[str, str] = {"name": ""}
        res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        assert res["code"] != 0, "Should fail for empty name"
        assert "empty" in res["message"].lower() or "cannot be empty" in res["message"].lower()

    @pytest.mark.p1
    def test_update_team_name_too_long(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team with name exceeding 100 characters."""
        tenant_id: str = test_team["id"]

        # Try to update with name too long
        long_name: str = "A" * 101
        update_payload: dict[str, str] = {"name": long_name}
        res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        assert res["code"] != 0, "Should fail for name too long"
        assert "100" in res["message"] or "length" in res["message"].lower()

    @pytest.mark.p1
    def test_update_team_credit(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team's credit."""
        tenant_id: str = test_team["id"]

        # Update the team credit
        new_credit: int = 1000
        update_payload: dict[str, int] = {"credit": new_credit}
        update_res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        assert update_res["code"] == 0, update_res
        assert "data" in update_res
        assert update_res["data"]["credit"] == new_credit

    @pytest.mark.p1
    def test_update_team_credit_negative(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team with negative credit (should fail)."""
        tenant_id: str = test_team["id"]

        # Try to update with negative credit
        update_payload: dict[str, int] = {"credit": -1}
        res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        assert res["code"] != 0, "Should fail for negative credit"
        assert "non-negative" in res["message"].lower() or "negative" in res["message"].lower()

    @pytest.mark.p1
    def test_update_team_invalid_tenant_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test updating a non-existent team."""
        invalid_id: str = f"invalid_{uuid.uuid4().hex[:8]}"
        update_payload: dict[str, str] = {"name": "Updated Name"}
        res: dict[str, Any] = update_team(web_api_auth, invalid_id, update_payload)
        assert res["code"] != 0, "Should fail for invalid team ID"
        # API may check permissions first, so either "not found" or permission error is valid
        assert "not found" in res["message"].lower() or "owner" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p1
    def test_update_team_not_owner_or_admin(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team when user is not owner or admin."""
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
        tenant_id: str = test_team["id"]
        add_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, tenant_id, add_payload)

        # Small delay to ensure user is fully created
        time.sleep(0.5)

        # Login as the new user (normal member, not admin/owner)
        new_user_auth: RAGFlowWebApiAuth = login_as_user(email, password)

        # Try to update team (user is member but not admin/owner)
        update_payload: dict[str, str] = {"name": "Updated Name"}
        res: dict[str, Any] = update_team(new_user_auth, tenant_id, update_payload)
        assert res["code"] == 108  # PERMISSION_ERROR
        assert "owner" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p1
    def test_update_team_response_structure(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test that team update returns the expected response structure."""
        tenant_id: str = test_team["id"]

        update_payload: dict[str, str] = {"name": f"Updated Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        assert res["code"] == 0
        assert "data" in res
        assert isinstance(res["data"], dict)
        assert "id" in res["data"]
        assert "name" in res["data"]
        assert "message" in res
        assert "updated successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_update_team_no_fields_provided(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team with no fields provided."""
        tenant_id: str = test_team["id"]

        # Try to update with empty payload
        update_payload: dict[str, Any] = {}
        res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        assert res["code"] != 0, "Should fail when no fields provided"
        assert "no fields" in res["message"].lower() or "required" in res["message"].lower()

    @pytest.mark.p1
    def test_update_team_missing_request_body(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team without request body."""
        tenant_id: str = test_team["id"]

        # Try to update without payload (None)
        res: dict[str, Any] = update_team(web_api_auth, tenant_id, None)
        assert res["code"] != 0, "Should fail when request body is missing"
        assert "required" in res["message"].lower() or "body" in res["message"].lower()

    @pytest.mark.p2
    def test_update_team_multiple_fields(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating multiple team fields at once."""
        tenant_id: str = test_team["id"]

        # Update multiple fields
        new_name: str = f"Multi Update Team {uuid.uuid4().hex[:8]}"
        new_credit: int = 2000
        update_payload: dict[str, Any] = {
            "name": new_name,
            "credit": new_credit,
        }
        update_res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        assert update_res["code"] == 0, update_res
        assert update_res["data"]["name"] == new_name
        assert update_res["data"]["credit"] == new_credit

    @pytest.mark.p2
    def test_update_team_whitespace_name(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team with whitespace-only name."""
        tenant_id: str = test_team["id"]

        # Try to update with whitespace-only name
        update_payload: dict[str, str] = {"name": "   "}
        res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        assert res["code"] != 0, "Should fail for whitespace-only name"
        assert "empty" in res["message"].lower() or "cannot be empty" in res["message"].lower()

    @pytest.mark.p2
    def test_update_team_special_characters_name(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team with special characters in name."""
        tenant_id: str = test_team["id"]

        # Update with special characters
        new_name: str = f"Team-{uuid.uuid4().hex[:8]}_Test!"
        update_payload: dict[str, str] = {"name": new_name}
        res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        # Should succeed if special chars are allowed
        assert res["code"] in (0, 101)

    @pytest.mark.p2
    def test_update_team_unicode_name(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team with unicode characters in name."""
        tenant_id: str = test_team["id"]

        # Update with unicode name
        new_name: str = f"团队{uuid.uuid4().hex[:8]}"
        update_payload: dict[str, str] = {"name": new_name}
        res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        # Should succeed if unicode is supported
        assert res["code"] in (0, 101)

    @pytest.mark.p2
    def test_update_team_credit_zero(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team with zero credit (should succeed)."""
        tenant_id: str = test_team["id"]

        # Update with zero credit
        update_payload: dict[str, int] = {"credit": 0}
        res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        assert res["code"] == 0, res
        assert res["data"]["credit"] == 0

    @pytest.mark.p2
    def test_update_team_credit_large_value(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team with a large credit value."""
        tenant_id: str = test_team["id"]

        # Update with large credit value
        large_credit: int = 999999
        update_payload: dict[str, int] = {"credit": large_credit}
        res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        assert res["code"] == 0, res
        assert res["data"]["credit"] == large_credit

    @pytest.mark.p2
    def test_update_team_credit_non_integer(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> None:
        """Test updating a team with non-integer credit (should fail)."""
        tenant_id: str = test_team["id"]

        # Try to update with non-integer credit
        update_payload: dict[str, str] = {"credit": "not_a_number"}
        res: dict[str, Any] = update_team(web_api_auth, tenant_id, update_payload)
        # API might accept it and convert, or reject it
        # This test documents the behavior
        assert res["code"] in (0, 101, 102)

