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
    add_department_members,
    add_users_to_team,
    create_department,
    create_team,
    create_user,
    login_as_user,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when adding members to a department."""

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
        # Create a team and department first
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if team_res["code"] != 0:
            pytest.skip("Team creation failed, skipping auth test")
        
        tenant_id: str = team_res["data"]["id"]
        
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        if dept_res["code"] != 0:
            pytest.skip("Department creation failed, skipping auth test")
        
        department_id: str = dept_res["data"]["id"]
        
        # Try to add members with invalid auth
        add_payload: dict[str, list[str]] = {"user_ids": ["test_user_id"]}
        res: dict[str, Any] = add_department_members(invalid_auth, department_id, add_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestAddMembers:
    """Comprehensive tests for adding members to a department."""

    @pytest.fixture
    def test_team(self, web_api_auth: RAGFlowWebApiAuth) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def test_department(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> dict[str, Any]:
        """Create a test department for use in tests."""
        dept_payload: dict[str, str] = {
            "name": f"Test Department {uuid.uuid4().hex[:8]}",
            "tenant_id": test_team["id"],
        }
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def test_users(self, web_api_auth: RAGFlowWebApiAuth) -> list[dict[str, Any]]:
        """Create test users for use in tests."""
        users = []
        for i in range(3):
            email = f"testuser{i}_{uuid.uuid4().hex[:8]}@example.com"
            user_payload: dict[str, str] = {
                "email": email,
                "password": "TestPassword123!",
                "nickname": f"Test User {i}",
            }
            user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
            if user_res["code"] == 0:
                users.append({"email": email, "id": user_res["data"]["id"]})
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
        test_department: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test adding a single member to a department."""
        if not test_users:
            pytest.skip("No test users created")
        
        user_id: str = test_users[0]["id"]
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_payload
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
        test_department: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test adding multiple members to a department."""
        if len(test_users) < 2:
            pytest.skip("Need at least 2 test users")
        
        user_ids: list[str] = [user["id"] for user in test_users[:2]]
        add_payload: dict[str, list[str]] = {"user_ids": user_ids}
        res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_payload
        )
        
        assert res["code"] == 0, res
        assert len(res["data"]["added"]) == 2
        assert set(res["data"]["added"]) == set(user_ids)
        assert len(res["data"]["failed"]) == 0

    @pytest.mark.p1
    def test_add_member_missing_request_body(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test adding members without request body."""
        res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], None
        )
        
        assert res["code"] == 101
        assert "required" in res["message"].lower() or "body" in res["message"].lower()

    @pytest.mark.p1
    def test_add_member_missing_user_ids(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test adding members without user_ids."""
        add_payload: dict[str, Any] = {}
        res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_payload
        )
        
        assert res["code"] == 101
        assert "user_ids" in res["message"].lower() or "non-empty array" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_add_member_empty_user_ids(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test adding members with empty user_ids array."""
        add_payload: dict[str, list[str]] = {"user_ids": []}
        res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_payload
        )
        
        assert res["code"] == 101
        assert "non-empty array" in res["message"].lower() or "empty" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_add_member_invalid_user_id(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test adding a non-existent user."""
        add_payload: dict[str, list[str]] = {
            "user_ids": ["non_existent_user_id_12345"]
        }
        res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_payload
        )
        
        assert res["code"] == 0  # API returns success with failed list
        assert "data" in res
        assert len(res["data"]["added"]) == 0
        assert len(res["data"]["failed"]) == 1
        assert "not found" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_add_member_user_not_in_team(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test adding a user who is not a member of the team."""
        if not test_users:
            pytest.skip("No test users created")
        
        # Create a user but don't add them to the team
        email = f"notinteam_{uuid.uuid4().hex[:8]}@example.com"
        user_payload: dict[str, str] = {
            "email": email,
            "password": "TestPassword123!",
            "nickname": "Not In Team User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        if user_res["code"] != 0:
            pytest.skip("User creation failed")
        
        user_id: str = user_res["data"]["id"]
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_payload
        )
        
        assert res["code"] == 0  # API returns success with failed list
        assert len(res["data"]["added"]) == 0
        assert len(res["data"]["failed"]) == 1
        assert "not a member of the team" in res["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_add_duplicate_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test adding a user who is already in the department."""
        if not test_users:
            pytest.skip("No test users created")
        
        user_id: str = test_users[0]["id"]
        
        # Add user first time
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        res1: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_payload
        )
        assert res1["code"] == 0
        assert len(res1["data"]["added"]) == 1
        
        # Try to add same user again
        res2: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_payload
        )
        assert res2["code"] == 0  # API returns success with failed list
        assert len(res2["data"]["added"]) == 0
        assert len(res2["data"]["failed"]) == 1
        assert "already a member" in res2["data"]["failed"][0]["error"].lower()

    @pytest.mark.p1
    def test_add_member_invalid_department_id(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test adding members to a non-existent department."""
        if not test_users:
            pytest.skip("No test users created")
        
        user_id: str = test_users[0]["id"]
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        res: dict[str, Any] = add_department_members(
            web_api_auth, "non_existent_department_id_12345", add_payload
        )
        
        assert res["code"] == 102
        assert "department not found" in res["message"].lower() or "not found" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_add_member_invalid_user_id_format(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test adding members with invalid user ID formats."""
        add_payload: dict[str, list[Any]] = {"user_ids": ["", "   ", 123, None]}
        res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_payload
        )
        
        assert res["code"] == 0  # API returns success with failed list
        assert len(res["data"]["added"]) == 0
        assert len(res["data"]["failed"]) >= 1
        # All invalid formats should be in failed list
        for failed in res["data"]["failed"]:
            assert "invalid" in failed["error"].lower() or "format" in failed[
                "error"
            ].lower()

    @pytest.mark.p1
    def test_add_member_mixed_valid_invalid(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test adding a mix of valid and invalid user IDs."""
        if not test_users:
            pytest.skip("No test users created")
        
        valid_user_id: str = test_users[0]["id"]
        invalid_user_id: str = "non_existent_user_id_12345"
        add_payload: dict[str, list[str]] = {
            "user_ids": [valid_user_id, invalid_user_id]
        }
        res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_payload
        )
        
        assert res["code"] == 0
        assert len(res["data"]["added"]) == 1
        assert res["data"]["added"][0] == valid_user_id
        assert len(res["data"]["failed"]) == 1
        assert res["data"]["failed"][0]["user_id"] == invalid_user_id

    @pytest.mark.p2
    def test_add_member_not_team_owner_or_admin(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test adding members when user is not team owner or admin."""
        # Create a team (current user is owner)
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, "Failed to create team"
        tenant_id: str = team_res["data"]["id"]
        
        # Create a department
        dept_payload: dict[str, str] = {
            "name": f"Test Department {uuid.uuid4().hex[:8]}",
            "tenant_id": tenant_id,
        }
        dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert dept_res["code"] == 0, "Failed to create department"
        department_id: str = dept_res["data"]["id"]
        
        # Create a normal user (not admin/owner)
        normal_user_email: str = f"normaluser_{uuid.uuid4().hex[:8]}@example.com"
        normal_user_password: str = "TestPassword123!"
        normal_user_payload: dict[str, str] = {
            "email": normal_user_email,
            "password": normal_user_password,
            "nickname": "Normal User",
        }
        normal_user_res: dict[str, Any] = create_user(web_api_auth, normal_user_payload)
        assert normal_user_res["code"] == 0, "Failed to create normal user"
        
        # Add the normal user to the team as a normal member (not admin)
        add_team_payload: dict[str, list[str]] = {"users": [normal_user_email]}
        add_team_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_team_payload)
        assert add_team_res["code"] == 0, "Failed to add user to team"
        
        # Wait a bit for user creation and team addition to be committed
        time.sleep(0.3)
        
        # Create another user to add to the department
        another_user_email: str = f"anotheruser_{uuid.uuid4().hex[:8]}@example.com"
        another_user_password: str = "TestPassword123!"
        another_user_payload: dict[str, str] = {
            "email": another_user_email,
            "password": another_user_password,
            "nickname": "Another User",
        }
        another_user_res: dict[str, Any] = create_user(web_api_auth, another_user_payload)
        assert another_user_res["code"] == 0, "Failed to create another user"
        another_user_id: str = another_user_res["data"]["id"]
        
        # Add another user to the team
        add_another_team_payload: dict[str, list[str]] = {"users": [another_user_email]}
        add_another_team_res: dict[str, Any] = add_users_to_team(
            web_api_auth, tenant_id, add_another_team_payload
        )
        assert add_another_team_res["code"] == 0, "Failed to add another user to team"
        
        # Wait a bit for team addition to be committed
        time.sleep(0.3)
        
        # Login as the normal user (not admin/owner)
        # Users with INVITE role should still be able to login
        try:
            normal_user_auth: RAGFlowWebApiAuth = login_as_user(normal_user_email, normal_user_password)
        except Exception as e:
            pytest.skip(f"Failed to login as normal user: {e}")
        
        # Accept the invitation to join the team
        accept_res: dict[str, Any] = accept_team_invitation(normal_user_auth, tenant_id)
        if accept_res["code"] != 0:
            pytest.skip(f"Failed to accept invitation: {accept_res.get('message', 'Unknown error')}")
        
        # Wait a bit for invitation acceptance to be committed
        time.sleep(0.2)
        
        # Try to add a member as the normal user (should fail)
        add_payload: dict[str, list[str]] = {"user_ids": [another_user_id]}
        add_res: dict[str, Any] = add_department_members(
            normal_user_auth, department_id, add_payload
        )
        assert add_res["code"] == 108, f"Expected permission error (108), got {add_res}"
        assert "only team owners or admins" in add_res["message"].lower()

    @pytest.mark.p2
    def test_add_member_response_structure(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test that add members returns the expected response structure."""
        if not test_users:
            pytest.skip("No test users created")
        
        user_id: str = test_users[0]["id"]
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_payload
        )
        
        assert res["code"] == 0
        assert "data" in res
        assert isinstance(res["data"], dict)
        assert "added" in res["data"]
        assert "failed" in res["data"]
        assert isinstance(res["data"]["added"], list)
        assert isinstance(res["data"]["failed"], list)
        assert "message" in res
        assert "added" in res["message"].lower() or "member" in res["message"].lower()

