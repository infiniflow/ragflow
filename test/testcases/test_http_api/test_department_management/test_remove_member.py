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
    add_department_members,
    add_users_to_team,
    create_department,
    create_team,
    create_user,
    remove_department_member,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when removing members from a department."""

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
        WebApiAuth: RAGFlowWebApiAuth,
    ) -> None:
        """Test removing members with invalid or missing authentication."""
        # Create a team, department, and add a user
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(WebApiAuth, team_payload)
        if team_res["code"] != 0:
            pytest.skip("Team creation failed, skipping auth test")
        
        tenant_id: str = team_res["data"]["id"]
        
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        dept_res: dict[str, Any] = create_department(WebApiAuth, dept_payload)
        if dept_res["code"] != 0:
            pytest.skip("Department creation failed, skipping auth test")
        
        department_id: str = dept_res["data"]["id"]
        
        # Create and add a user to team and department
        email = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        user_payload: dict[str, str] = {
            "email": email,
            "password": "TestPassword123!",
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(WebApiAuth, user_payload)
        if user_res["code"] != 0:
            pytest.skip("User creation failed, skipping auth test")
        
        user_id: str = user_res["data"]["id"]
        
        # Add user to team
        add_team_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(WebApiAuth, tenant_id, add_team_payload)
        
        # Add user to department
        add_dept_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_department_members(WebApiAuth, department_id, add_dept_payload)
        
        # Try to remove member with invalid auth
        res: dict[str, Any] = remove_department_member(
            invalid_auth, department_id, user_id
        )
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestRemoveMember:
    """Comprehensive tests for removing members from a department."""

    @pytest.fixture
    def test_team(self, WebApiAuth: RAGFlowWebApiAuth) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(WebApiAuth, team_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def test_department(
        self, WebApiAuth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> dict[str, Any]:
        """Create a test department for use in tests."""
        dept_payload: dict[str, str] = {
            "name": f"Test Department {uuid.uuid4().hex[:8]}",
            "tenant_id": test_team["id"],
        }
        res: dict[str, Any] = create_department(WebApiAuth, dept_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def test_user_with_member(
        self,
        WebApiAuth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        test_department: dict[str, Any],
    ) -> dict[str, Any]:
        """Create a test user and add them to team and department."""
        email = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        user_payload: dict[str, str] = {
            "email": email,
            "password": "TestPassword123!",
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(WebApiAuth, user_payload)
        assert user_res["code"] == 0
        
        user_id: str = user_res["data"]["id"]
        
        # Add user to team
        add_team_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(WebApiAuth, test_team["id"], add_team_payload)
        
        # Add user to department
        add_dept_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_res: dict[str, Any] = add_department_members(
            WebApiAuth, test_department["id"], add_dept_payload
        )
        assert add_res["code"] == 0
        assert len(add_res["data"]["added"]) == 1
        
        return {"id": user_id, "email": email}

    @pytest.mark.p1
    def test_remove_single_member(
        self,
        WebApiAuth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test removing a single member from a department."""
        user_id: str = test_user_with_member["id"]
        res: dict[str, Any] = remove_department_member(
            WebApiAuth, test_department["id"], user_id
        )
        
        assert res["code"] == 0, res
        assert "data" in res
        assert res["data"] is True
        assert "removed" in res["message"].lower() or "success" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_remove_member_invalid_department_id(
        self,
        WebApiAuth: RAGFlowWebApiAuth,
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test removing a member from a non-existent department."""
        user_id: str = test_user_with_member["id"]
        res: dict[str, Any] = remove_department_member(
            WebApiAuth, "non_existent_department_id_12345", user_id
        )
        
        assert res["code"] == 102
        assert "department not found" in res["message"].lower() or "not found" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_remove_member_user_not_in_department(
        self,
        WebApiAuth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_team: dict[str, Any],
    ) -> None:
        """Test removing a user who is not in the department."""
        # Create a user and add to team but not department
        email = f"notindept_{uuid.uuid4().hex[:8]}@example.com"
        user_payload: dict[str, str] = {
            "email": email,
            "password": "TestPassword123!",
            "nickname": "Not In Dept User",
        }
        user_res: dict[str, Any] = create_user(WebApiAuth, user_payload)
        assert user_res["code"] == 0
        
        user_id: str = user_res["data"]["id"]
        
        # Add user to team but not department
        add_team_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(WebApiAuth, test_team["id"], add_team_payload)
        
        # Try to remove from department
        res: dict[str, Any] = remove_department_member(
            WebApiAuth, test_department["id"], user_id
        )
        
        assert res["code"] == 102
        assert "not a member" in res["message"].lower() or "not found" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_remove_member_invalid_user_id(
        self,
        WebApiAuth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test removing a non-existent user from a department."""
        res: dict[str, Any] = remove_department_member(
            WebApiAuth, test_department["id"], "non_existent_user_id_12345"
        )
        
        # The API checks if user is in department first, so this should return not found
        assert res["code"] == 102
        assert "not a member" in res["message"].lower() or "not found" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_remove_member_twice(
        self,
        WebApiAuth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test removing the same member twice (idempotent operation)."""
        user_id: str = test_user_with_member["id"]
        
        # Remove first time
        res1: dict[str, Any] = remove_department_member(
            WebApiAuth, test_department["id"], user_id
        )
        assert res1["code"] == 0
        
        # Try to remove again - API is idempotent, so it succeeds again
        # (the record exists but is soft-deleted, and we update it again)
        res2: dict[str, Any] = remove_department_member(
            WebApiAuth, test_department["id"], user_id
        )
        assert res2["code"] == 0  # API allows removing twice (idempotent)
        assert "removed" in res2["message"].lower() or "success" in res2[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_remove_member_response_structure(
        self,
        WebApiAuth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test that remove member returns the expected response structure."""
        user_id: str = test_user_with_member["id"]
        res: dict[str, Any] = remove_department_member(
            WebApiAuth, test_department["id"], user_id
        )
        
        assert res["code"] == 0
        assert "data" in res
        assert res["data"] is True
        assert "message" in res
        assert "removed" in res["message"].lower() or "success" in res[
            "message"
        ].lower()

    @pytest.mark.p2
    def test_remove_member_not_team_owner_or_admin(
        self, WebApiAuth: RAGFlowWebApiAuth
    ) -> None:
        """Test removing members when user is not team owner or admin."""
        # This test would require creating a team with a different user
        # and then trying to remove members as a non-admin user
        # For now, we'll skip this as it requires multi-user setup
        pytest.skip("Requires multi-user setup to test permission restrictions")

    @pytest.mark.p2
    def test_remove_and_re_add_member(
        self,
        WebApiAuth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test removing a member and then adding them back."""
        user_id: str = test_user_with_member["id"]
        
        # Remove member
        remove_res: dict[str, Any] = remove_department_member(
            WebApiAuth, test_department["id"], user_id
        )
        assert remove_res["code"] == 0
        
        # Add member back
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_res: dict[str, Any] = add_department_members(
            WebApiAuth, test_department["id"], add_payload
        )
        assert add_res["code"] == 0
        assert len(add_res["data"]["added"]) == 1
        assert add_res["data"]["added"][0] == user_id

