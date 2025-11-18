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
    delete_department,
    get_user_info,
    login_as_user,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when deleting a department."""

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
        """Test deleting a department with invalid or missing authentication."""
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
        
        # Try to delete department with invalid auth
        res: dict[str, Any] = delete_department(invalid_auth, department_id)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestDeleteDepartment:
    """Comprehensive tests for deleting a department."""

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
    def test_department_with_members(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_team: dict[str, Any],
    ) -> dict[str, Any]:
        """Create a test department and add the current user as a member."""
        # Get current user ID
        user_info_res: dict[str, Any] = get_user_info(web_api_auth)
        if user_info_res["code"] == 0:
            user_id: str = user_info_res["data"]["id"]
            # Add current user to department (they're already team owner/admin)
            add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
            add_department_members(web_api_auth, test_department["id"], add_payload)
        return test_department

    @pytest.mark.p1
    def test_delete_department_success(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department_with_members: dict[str, Any],
    ) -> None:
        """Test successfully deleting a department."""
        department_id: str = test_department_with_members["id"]
        
        # Delete the department
        res: dict[str, Any] = delete_department(web_api_auth, department_id)
        assert res["code"] == 0, res
        assert res["data"] is True
        assert "deleted successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_delete_department_invalid_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test deleting a department with an invalid department ID."""
        invalid_id: str = f"invalid_{uuid.uuid4().hex[:8]}"
        res: dict[str, Any] = delete_department(web_api_auth, invalid_id)
        assert res["code"] == 100  # DATA_ERROR
        assert "not found" in res["message"].lower()

    @pytest.mark.p1
    def test_delete_department_not_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test deleting a department when user is not a member."""
        # Create a new user and add them to the team but not to the department
        email: str = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        user_payload: dict[str, str] = {
            "email": email,
            "password": "TestPassword123!",
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        if user_res["code"] != 0:
            pytest.skip("User creation failed")
        
        user_id: str = user_res["data"]["id"]
        
        # Add user to team
        team_id: str = test_department["tenant_id"]
        add_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, team_id, add_payload)
        
        # Login as the new user
        new_user_auth: RAGFlowWebApiAuth = login_as_user(email, "TestPassword123!")
        
        # Try to delete department (user is not a member)
        res: dict[str, Any] = delete_department(new_user_auth, test_department["id"])
        assert res["code"] == 103  # PERMISSION_ERROR
        assert "member" in res["message"].lower()

    @pytest.mark.p1
    def test_delete_department_not_team_admin_or_owner(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test deleting a department when user is not team admin or owner."""
        # Create a new user
        email: str = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        user_payload: dict[str, str] = {
            "email": email,
            "password": "TestPassword123!",
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        if user_res["code"] != 0:
            pytest.skip("User creation failed")
        
        user_id: str = user_res["data"]["id"]
        
        # Add user to team as normal member
        team_id: str = test_department["tenant_id"]
        add_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, team_id, add_payload)
        
        # Accept invitation (if needed)
        # Note: This depends on the invitation flow implementation
        
        # Add user to department
        dept_add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_department_members(web_api_auth, test_department["id"], dept_add_payload)
        
        # Login as the new user (normal member, not admin/owner)
        new_user_auth: RAGFlowWebApiAuth = login_as_user(email, "TestPassword123!")
        
        # Try to delete department (user is member but not admin/owner)
        res: dict[str, Any] = delete_department(new_user_auth, test_department["id"])
        assert res["code"] == 103  # PERMISSION_ERROR
        assert "owner" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p1
    def test_delete_department_response_structure(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department_with_members: dict[str, Any],
    ) -> None:
        """Test that department deletion returns the expected response structure."""
        department_id: str = test_department_with_members["id"]
        
        res: dict[str, Any] = delete_department(web_api_auth, department_id)
        assert res["code"] == 0
        assert "data" in res
        assert res["data"] is True
        assert "message" in res
        assert isinstance(res["message"], str)
        assert "deleted successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_delete_department_already_deleted(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department_with_members: dict[str, Any],
    ) -> None:
        """Test deleting a department that has already been deleted."""
        department_id: str = test_department_with_members["id"]
        
        # Delete the department first
        res1: dict[str, Any] = delete_department(web_api_auth, department_id)
        assert res1["code"] == 0
        
        # Try to delete again
        res2: dict[str, Any] = delete_department(web_api_auth, department_id)
        # Should return error (department not found or already deleted)
        assert res2["code"] != 0
        assert "not found" in res2["message"].lower() or "deleted" in res2["message"].lower()

    @pytest.mark.p1
    def test_delete_department_with_members(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_team: dict[str, Any],
    ) -> None:
        """Test deleting a department that has members."""
        # Create test users
        users = []
        for i in range(2):
            email = f"testuser{i}_{uuid.uuid4().hex[:8]}@example.com"
            user_payload: dict[str, str] = {
                "email": email,
                "password": "TestPassword123!",
                "nickname": f"Test User {i}",
            }
            user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
            if user_res["code"] == 0:
                users.append({"email": email, "id": user_res["data"]["id"]})
        
        if not users:
            pytest.skip("User creation failed")
        
        # Add users to team
        for user in users:
            add_payload: dict[str, list[str]] = {"users": [user["email"]]}
            add_users_to_team(web_api_auth, test_team["id"], add_payload)
        
        # Add users to department
        user_ids: list[str] = [user["id"] for user in users]
        dept_add_payload: dict[str, list[str]] = {"user_ids": user_ids}
        add_department_members(web_api_auth, test_department["id"], dept_add_payload)
        
        # Ensure current user is also a member (for deletion permission)
        user_info_res: dict[str, Any] = get_user_info(web_api_auth)
        if user_info_res["code"] == 0:
            current_user_id: str = user_info_res["data"]["id"]
            dept_add_payload2: dict[str, list[str]] = {"user_ids": [current_user_id]}
            add_department_members(web_api_auth, test_department["id"], dept_add_payload2)
        
        # Delete the department (should also remove all member relationships)
        res: dict[str, Any] = delete_department(web_api_auth, test_department["id"])
        assert res["code"] == 0, res
        assert res["data"] is True
        assert "member relationships" in res["message"].lower() or "deleted successfully" in res["message"].lower()

    @pytest.mark.p2
    def test_delete_multiple_departments(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
    ) -> None:
        """Test deleting multiple departments from the same team."""
        # Create multiple departments
        departments = []
        for i in range(3):
            dept_name: str = f"Department {i} {uuid.uuid4().hex[:8]}"
            dept_payload: dict[str, str] = {
                "name": dept_name,
                "tenant_id": test_team["id"],
            }
            dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
            if dept_res["code"] == 0:
                departments.append(dept_res["data"])
        
        if not departments:
            pytest.skip("Department creation failed")
        
        # Add current user to all departments
        user_info_res: dict[str, Any] = get_user_info(web_api_auth)
        if user_info_res["code"] == 0:
            current_user_id: str = user_info_res["data"]["id"]
            for dept in departments:
                dept_add_payload: dict[str, list[str]] = {"user_ids": [current_user_id]}
                add_department_members(web_api_auth, dept["id"], dept_add_payload)
        
        # Delete all departments
        for dept in departments:
            res: dict[str, Any] = delete_department(web_api_auth, dept["id"])
            assert res["code"] == 0, f"Failed to delete department {dept['id']}: {res}"

    @pytest.mark.p2
    def test_delete_department_empty_string_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test deleting a department with empty string ID."""
        res: dict[str, Any] = delete_department(web_api_auth, "")
        assert res["code"] != 0
        assert "not found" in res["message"].lower() or res["code"] == 100

    @pytest.mark.p2
    def test_delete_department_special_characters_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test deleting a department with special characters in ID."""
        invalid_id: str = "dept-123_!@#$%"
        res: dict[str, Any] = delete_department(web_api_auth, invalid_id)
        assert res["code"] != 0
        assert "not found" in res["message"].lower() or res["code"] == 100

