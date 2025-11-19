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

from common import create_team
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior during team creation."""

    @pytest.mark.parametrize(
        ("invalid_auth", "expected_code", "expected_message"),
        [
            # Endpoint now requires @login_required (JWT token auth)
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
        """Test team creation with invalid or missing authentication."""
        # Try to create team with invalid auth
        team_payload: dict[str, str] = {
            "name": "Test Team Auth",
        }
        res: dict[str, Any] = create_team(invalid_auth, team_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestTeamCreate:
    """Comprehensive tests for team creation API."""

    @pytest.mark.p1
    def test_create_team_with_name_and_user_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with name and user_id."""
        # Create team (user_id is optional, defaults to current authenticated user)
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {
            "name": team_name,
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 0, res
        assert "data" in res
        assert res["data"]["name"] == team_name
        assert "owner_id" in res["data"]
        assert "id" in res["data"]
        assert "deleted successfully" not in res["message"].lower()
        assert "created successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_create_team_missing_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team without name."""
        # Try to create team without name
        team_payload: dict[str, str] = {}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 101
        assert "name" in res["message"].lower() or "required" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_create_team_empty_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with empty name."""
        # Try to create team with empty name
        team_payload: dict[str, str] = {"name": ""}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 101
        assert "name" in res["message"].lower() or "required" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_create_team_name_too_long(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with name exceeding 100 characters."""
        # Try to create team with name too long
        long_name: str = "A" * 101
        team_payload: dict[str, str] = {"name": long_name}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 101
        assert "100" in res["message"] or "length" in res["message"].lower()

    @pytest.mark.p1
    def test_create_team_invalid_user_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with non-existent user_id."""
        team_payload: dict[str, str] = {
            "name": "Test Team Invalid User",
            "user_id": "non_existent_user_id_12345",
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 102
        assert "not found" in res["message"].lower()

    @pytest.mark.p1
    def test_create_team_missing_user_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team without user_id (should use current authenticated user)."""
        team_payload: dict[str, str] = {"name": "Test Team No User"}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        # Should succeed since user_id defaults to current authenticated user
        assert res["code"] == 0
        assert "data" in res
        assert "owner_id" in res["data"]
        assert "created successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_create_team_response_structure(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test that team creation returns the expected response structure."""
        # Create team
        team_name: str = f"Test Team Structure {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {
            "name": team_name,
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 0
        assert "data" in res
        assert isinstance(res["data"], dict)
        assert "id" in res["data"]
        assert "name" in res["data"]
        assert "owner_id" in res["data"]
        assert res["data"]["name"] == team_name
        assert "message" in res
        assert "created successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_create_multiple_teams_same_user(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating multiple teams for the same user."""
        # Create first team
        team_name_1: str = f"Team 1 {uuid.uuid4().hex[:8]}"
        team_payload_1: dict[str, str] = {
            "name": team_name_1,
        }
        res1: dict[str, Any] = create_team(web_api_auth, team_payload_1)
        assert res1["code"] == 0, res1
        team_id_1: str = res1["data"]["id"]

        # Create second team
        team_name_2: str = f"Team 2 {uuid.uuid4().hex[:8]}"
        team_payload_2: dict[str, str] = {
            "name": team_name_2,
        }
        res2: dict[str, Any] = create_team(web_api_auth, team_payload_2)
        assert res2["code"] == 0, res2
        team_id_2: str = res2["data"]["id"]

        # Verify teams are different
        assert team_id_1 != team_id_2
        assert res1["data"]["name"] == team_name_1
        assert res2["data"]["name"] == team_name_2

    @pytest.mark.p2
    def test_create_team_with_whitespace_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with whitespace-only name."""
        # Try to create team with whitespace-only name
        team_payload: dict[str, str] = {
            "name": "   ",
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        # Should fail validation
        assert res["code"] == 101
        assert "name" in res["message"].lower() or "required" in res[
            "message"
        ].lower()

    @pytest.mark.p2
    def test_create_team_special_characters_in_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with special characters in name."""
        # Create team with special characters
        team_name: str = f"Team-{uuid.uuid4().hex[:8]}_Test!"
        team_payload: dict[str, str] = {
            "name": team_name,
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        # Should succeed if special chars are allowed
        assert res["code"] in (0, 101)

    @pytest.mark.p2
    def test_create_team_empty_payload(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with empty payload."""
        team_payload: dict[str, Any] = {}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 101
        assert "required" in res["message"].lower() or "name" in res[
            "message"
        ].lower()

    @pytest.mark.p3
    def test_create_team_unicode_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a team with unicode characters in name."""
        # Create team with unicode name
        team_name: str = f"团队{uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {
            "name": team_name,
        }
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        # Should succeed if unicode is supported
        assert res["code"] in (0, 101)

