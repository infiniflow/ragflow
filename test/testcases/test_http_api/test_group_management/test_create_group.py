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

from common import create_group, create_team, create_user, encrypt_password, login_as_user
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior during group creation."""

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
        """Test group creation with invalid or missing authentication."""
        # First create a team to use as tenant_id
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if team_res["code"] != 0:
            pytest.skip("Team creation failed, skipping auth test")
        
        tenant_id: str = team_res["data"]["id"]
        
        # Try to create group with invalid auth
        group_payload: dict[str, str] = {
            "name": "Test Group Auth",
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_group(invalid_auth, group_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestGroupCreate:
    """Comprehensive tests for group creation API."""

    @pytest.mark.p1
    def test_create_group_with_name_and_tenant_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group with name and tenant_id."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create group
        group_name: str = f"Test Group {uuid.uuid4().hex[:8]}"
        group_payload: dict[str, str] = {
            "name": group_name,
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert res["code"] == 0, res
        assert "data" in res
        assert res["data"]["name"] == group_name
        assert res["data"]["tenant_id"] == tenant_id
        assert "id" in res["data"]
        assert "created successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_create_group_with_description(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group with description."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create group with description
        group_name: str = f"Test Group {uuid.uuid4().hex[:8]}"
        description: str = "This is a test group description"
        group_payload: dict[str, str] = {
            "name": group_name,
            "tenant_id": tenant_id,
            "description": description,
        }
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert res["code"] == 0, res
        assert res["data"]["name"] == group_name
        assert res["data"]["description"] == description

    @pytest.mark.p1
    def test_create_group_missing_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group without name."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Try to create group without name
        group_payload: dict[str, str] = {"tenant_id": tenant_id}
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert res["code"] == 101
        assert "name" in res["message"].lower() or "required" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_create_group_empty_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group with empty name."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Try to create group with empty name
        group_payload: dict[str, str] = {"name": "", "tenant_id": tenant_id}
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert res["code"] == 101
        assert "name" in res["message"].lower() or "empty" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_create_group_missing_tenant_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group without tenant_id."""
        # Try to create group without tenant_id
        group_payload: dict[str, str] = {"name": "Test Group"}
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert res["code"] == 101
        assert "tenant_id" in res["message"].lower() or "required" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_create_group_invalid_tenant_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group with non-existent tenant_id."""
        group_payload: dict[str, str] = {
            "name": "Test Group Invalid Tenant",
            "tenant_id": "non_existent_tenant_id_12345",
        }
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        # Permission check happens before tenant existence check,
        # so invalid tenant_id results in permission error (108) not data error (102)
        assert res["code"] == 108
        assert "only team owners or admins" in res["message"].lower() or "permission" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_create_group_name_too_long(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group with name exceeding 128 characters."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Try to create group with name too long
        # Note: The API validates name length (max 128), so it should return an error
        long_name: str = "A" * 129
        group_payload: dict[str, str] = {
            "name": long_name,
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        # API validates length, so it should return an argument error
        assert res["code"] == 101
        assert "128" in res["message"] or "characters" in res["message"].lower()

    @pytest.mark.p1
    def test_create_group_not_team_owner_or_admin(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group when user is not team owner or admin."""
        # Create a team with the main user (owner)
        team_name: str = f"Owner Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create a second user with encrypted password (now supported!)
        other_user_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        other_user_password: str = "test123"
        encrypted_password: str = encrypt_password(other_user_password)
        
        user_payload: dict[str, str] = {
            "nickname": "Other User",
            "email": other_user_email,
            "password": encrypted_password,  # Now works with encryption!
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        assert user_res["code"] == 0, user_res
        
        # Small delay to ensure user is fully created
        import time
        time.sleep(0.5)
        
        # Login as the other user
        other_user_auth: RAGFlowWebApiAuth = login_as_user(
            other_user_email, other_user_password
        )
        
        # Try to create a group in the owner's team as the other user
        group_name: str = f"Group {uuid.uuid4().hex[:8]}"
        group_payload: dict[str, str] = {
            "name": group_name,
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_group(other_user_auth, group_payload)
        
        # Should fail - user is not the team owner or admin
        assert res["code"] != 0, (
            "Non-owner/non-admin should not be able to create groups in another user's team"
        )
        
        # Verify it's a permission-related error
        # Common permission error codes: 108 (Permission denied), 403 (Forbidden), 104 (Permission Error), 102 (Authentication Error)
        assert res["code"] in [108, 403, 104, 102], (
            f"Expected permission error code (108, 403, 104, or 102), got: {res}"
        )
        
        # Verify the error message indicates permission issue
        assert "owner" in res["message"].lower() or "admin" in res["message"].lower() or "permission" in res["message"].lower(), (
            f"Error message should indicate permission issue, got: {res['message']}"
        )

    @pytest.mark.p1
    def test_create_group_response_structure(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test that group creation returns the expected response structure."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create group
        group_name: str = f"Test Group Structure {uuid.uuid4().hex[:8]}"
        group_payload: dict[str, str] = {
            "name": group_name,
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert res["code"] == 0
        assert "data" in res
        assert isinstance(res["data"], dict)
        assert "id" in res["data"]
        assert "name" in res["data"]
        assert "tenant_id" in res["data"]
        assert res["data"]["name"] == group_name
        assert res["data"]["tenant_id"] == tenant_id
        assert "message" in res
        assert "created successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_create_multiple_groups_same_team(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating multiple groups for the same team."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create first group
        group_name_1: str = f"Group 1 {uuid.uuid4().hex[:8]}"
        group_payload_1: dict[str, str] = {
            "name": group_name_1,
            "tenant_id": tenant_id,
        }
        res1: dict[str, Any] = create_group(web_api_auth, group_payload_1)
        assert res1["code"] == 0, res1
        group_id_1: str = res1["data"]["id"]

        # Create second group
        group_name_2: str = f"Group 2 {uuid.uuid4().hex[:8]}"
        group_payload_2: dict[str, str] = {
            "name": group_name_2,
            "tenant_id": tenant_id,
        }
        res2: dict[str, Any] = create_group(web_api_auth, group_payload_2)
        assert res2["code"] == 0, res2
        group_id_2: str = res2["data"]["id"]

        # Verify groups are different
        assert group_id_1 != group_id_2
        assert res1["data"]["name"] == group_name_1
        assert res2["data"]["name"] == group_name_2
        assert res1["data"]["tenant_id"] == tenant_id
        assert res2["data"]["tenant_id"] == tenant_id

    @pytest.mark.p1
    def test_create_group_duplicate_name_same_tenant(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group with duplicate name in the same tenant."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create first group
        group_name: str = f"Duplicate Group {uuid.uuid4().hex[:8]}"
        group_payload: dict[str, str] = {
            "name": group_name,
            "tenant_id": tenant_id,
        }
        res1: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert res1["code"] == 0, res1
        
        # Try to create another group with the same name in the same tenant
        res2: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert res2["code"] == 102  # DATA_ERROR
        assert "already exists" in res2["message"].lower()

    @pytest.mark.p2
    def test_create_group_with_whitespace_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group with whitespace-only name."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Try to create group with whitespace-only name
        group_payload: dict[str, str] = {
            "name": "   ",
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        # Should fail validation
        assert res["code"] == 101
        assert "name" in res["message"].lower() or "empty" in res[
            "message"
        ].lower()

    @pytest.mark.p2
    def test_create_group_special_characters_in_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group with special characters in name."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create group with special characters
        group_name: str = f"Group-{uuid.uuid4().hex[:8]}_Test!"
        group_payload: dict[str, str] = {
            "name": group_name,
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        # Should succeed if special chars are allowed
        assert res["code"] in (0, 101)

    @pytest.mark.p2
    def test_create_group_empty_payload(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group with empty payload."""
        group_payload: dict[str, Any] = {}
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert res["code"] == 101
        assert "required" in res["message"].lower() or "name" in res[
            "message"
        ].lower()

    @pytest.mark.p3
    def test_create_group_unicode_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a group with unicode characters in name."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create group with unicode name
        group_name: str = f"组{uuid.uuid4().hex[:8]}"
        group_payload: dict[str, str] = {
            "name": group_name,
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        # Should succeed if unicode is supported
        assert res["code"] in (0, 101)

