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
    list_department_members,
    login_as_user,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when listing department members."""

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
        """Test listing members with invalid or missing authentication."""
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
        
        # Try to list members with invalid auth
        res: dict[str, Any] = list_department_members(invalid_auth, department_id)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestListMembers:
    """Comprehensive tests for listing department members."""

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

    @pytest.fixture
    def department_with_members(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> dict[str, Any]:
        """Add test users to the department."""
        if not test_users:
            return test_department
        
        user_ids: list[str] = [user["id"] for user in test_users]
        add_payload: dict[str, list[str]] = {"user_ids": user_ids}
        add_department_members(web_api_auth, test_department["id"], add_payload)
        return test_department

    @pytest.mark.p1
    def test_list_members_empty_department(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test listing members from an empty department."""
        department_id: str = test_department["id"]
        
        res: dict[str, Any] = list_department_members(web_api_auth, department_id)
        assert res["code"] == 0, res
        assert "data" in res
        assert isinstance(res["data"], list)
        assert len(res["data"]) == 0
        assert "message" in res
        assert "0 member" in res["message"].lower()

    @pytest.mark.p1
    def test_list_members_with_multiple_users(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        department_with_members: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test listing members from a department with multiple users."""
        if not test_users:
            pytest.skip("No test users created")
        
        department_id: str = department_with_members["id"]
        
        res: dict[str, Any] = list_department_members(web_api_auth, department_id)
        assert res["code"] == 0, res
        assert "data" in res
        assert isinstance(res["data"], list)
        assert len(res["data"]) == len(test_users)
        
        # Verify member structure
        for member in res["data"]:
            assert "user_id" in member
            assert "email" in member
            assert "nickname" in member
            assert "status" in member
            assert member["status"] == "1"  # VALID status
        
        # Verify all test users are in the list
        member_user_ids: set[str] = {member["user_id"] for member in res["data"]}
        test_user_ids: set[str] = {user["id"] for user in test_users}
        assert member_user_ids == test_user_ids

    @pytest.mark.p1
    def test_list_members_invalid_department_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test listing members from a non-existent department."""
        invalid_id: str = f"invalid_{uuid.uuid4().hex[:8]}"
        res: dict[str, Any] = list_department_members(web_api_auth, invalid_id)
        assert res["code"] == 100  # DATA_ERROR
        assert "not found" in res["message"].lower()

    @pytest.mark.p1
    def test_list_members_not_team_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test listing members when user is not a team member."""
        # Create a new user who is not in the team
        email: str = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        user_payload: dict[str, str] = {
            "email": email,
            "password": "TestPassword123!",
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        if user_res["code"] != 0:
            pytest.skip("User creation failed")
        
        # Login as the new user (not in team)
        new_user_auth: RAGFlowWebApiAuth = login_as_user(email, "TestPassword123!")
        
        # Try to list members (user is not a team member)
        res: dict[str, Any] = list_department_members(new_user_auth, test_department["id"])
        assert res["code"] == 103  # PERMISSION_ERROR
        assert "team member" in res["message"].lower() or "member of the team" in res["message"].lower()

    @pytest.mark.p1
    def test_list_members_response_structure(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        department_with_members: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test that listing members returns the expected response structure."""
        if not test_users:
            pytest.skip("No test users created")
        
        department_id: str = department_with_members["id"]
        
        res: dict[str, Any] = list_department_members(web_api_auth, department_id)
        assert res["code"] == 0
        assert "data" in res
        assert isinstance(res["data"], list)
        assert "message" in res
        assert isinstance(res["message"], str)
        
        # Verify member object structure
        if len(res["data"]) > 0:
            member: dict[str, Any] = res["data"][0]
            required_fields: set[str] = {
                "id",
                "user_id",
                "status",
                "nickname",
                "email",
                "avatar",
                "is_active",
            }
            for field in required_fields:
                assert field in member, f"Missing field: {field}"

    @pytest.mark.p1
    def test_list_members_after_adding(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test listing members after adding users to the department."""
        if not test_users:
            pytest.skip("No test users created")
        
        department_id: str = test_department["id"]
        
        # List members before adding
        res_before: dict[str, Any] = list_department_members(web_api_auth, department_id)
        assert res_before["code"] == 0
        initial_count: int = len(res_before["data"])
        
        # Add a user
        user_id: str = test_users[0]["id"]
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_res: dict[str, Any] = add_department_members(web_api_auth, department_id, add_payload)
        assert add_res["code"] == 0
        
        # List members after adding
        res_after: dict[str, Any] = list_department_members(web_api_auth, department_id)
        assert res_after["code"] == 0
        assert len(res_after["data"]) == initial_count + 1
        
        # Verify the added user is in the list
        member_user_ids: set[str] = {member["user_id"] for member in res_after["data"]}
        assert user_id in member_user_ids

    @pytest.mark.p1
    def test_list_members_only_valid_status(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test that only members with valid status are returned."""
        if not test_users:
            pytest.skip("No test users created")
        
        department_id: str = test_department["id"]
        
        # Add users to department
        user_ids: list[str] = [user["id"] for user in test_users]
        add_payload: dict[str, list[str]] = {"user_ids": user_ids}
        add_department_members(web_api_auth, department_id, add_payload)
        
        # List members - all should have valid status
        res: dict[str, Any] = list_department_members(web_api_auth, department_id)
        assert res["code"] == 0
        
        for member in res["data"]:
            assert member["status"] == "1"  # VALID status

    @pytest.mark.p1
    def test_list_members_team_member_not_department_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test that a team member (not department member) can list department members."""
        if not test_users:
            pytest.skip("No test users created")
        
        department_id: str = test_department["id"]
        
        # Add one user to department
        user_id: str = test_users[0]["id"]
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_department_members(web_api_auth, department_id, add_payload)
        
        # Login as another team member (not in department)
        other_user_email: str = test_users[1]["email"]
        other_user_auth: RAGFlowWebApiAuth = login_as_user(other_user_email, "TestPassword123!")
        
        # Team member should be able to list members
        res: dict[str, Any] = list_department_members(other_user_auth, department_id)
        assert res["code"] == 0
        assert len(res["data"]) == 1
        assert res["data"][0]["user_id"] == user_id

    @pytest.mark.p2
    def test_list_members_empty_string_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test listing members with empty string department ID."""
        res: dict[str, Any] = list_department_members(web_api_auth, "")
        assert res["code"] != 0
        assert "not found" in res["message"].lower() or res["code"] == 100

    @pytest.mark.p2
    def test_list_members_special_characters_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test listing members with special characters in department ID."""
        invalid_id: str = "dept-123_!@#$%"
        res: dict[str, Any] = list_department_members(web_api_auth, invalid_id)
        assert res["code"] != 0
        assert "not found" in res["message"].lower() or res["code"] == 100

    @pytest.mark.p2
    def test_list_members_multiple_departments(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test listing members from multiple departments."""
        if not test_users:
            pytest.skip("No test users created")
        
        # Create multiple departments
        departments = []
        for i in range(2):
            dept_name: str = f"Department {i} {uuid.uuid4().hex[:8]}"
            dept_payload: dict[str, str] = {
                "name": dept_name,
                "tenant_id": test_team["id"],
            }
            dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
            if dept_res["code"] == 0:
                departments.append(dept_res["data"])
        
        if len(departments) < 2:
            pytest.skip("Department creation failed")
        
        # Add different users to each department
        dept1_add_payload: dict[str, list[str]] = {"user_ids": [test_users[0]["id"]]}
        add_department_members(web_api_auth, departments[0]["id"], dept1_add_payload)
        
        if len(test_users) > 1:
            dept2_add_payload: dict[str, list[str]] = {"user_ids": [test_users[1]["id"]]}
            add_department_members(web_api_auth, departments[1]["id"], dept2_add_payload)
        
        # List members from each department
        res1: dict[str, Any] = list_department_members(web_api_auth, departments[0]["id"])
        assert res1["code"] == 0
        assert len(res1["data"]) == 1
        assert res1["data"][0]["user_id"] == test_users[0]["id"]
        
        if len(test_users) > 1:
            res2: dict[str, Any] = list_department_members(web_api_auth, departments[1]["id"])
            assert res2["code"] == 0
            assert len(res2["data"]) == 1
            assert res2["data"][0]["user_id"] == test_users[1]["id"]

    @pytest.mark.p2
    def test_list_members_large_department(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        team_with_users: dict[str, Any],
    ) -> None:
        """Test listing members from a department with many users."""
        # Create multiple users
        users = []
        for i in range(10):
            email = f"bulkuser{i}_{uuid.uuid4().hex[:8]}@example.com"
            user_payload: dict[str, str] = {
                "email": email,
                "password": "TestPassword123!",
                "nickname": f"Bulk User {i}",
            }
            user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
            if user_res["code"] == 0:
                users.append({"email": email, "id": user_res["data"]["id"]})
        
        if len(users) < 5:
            pytest.skip("Not enough users created")
        
        # Add users to team
        for user in users:
            add_payload: dict[str, list[str]] = {"users": [user["email"]]}
            add_users_to_team(web_api_auth, test_department["tenant_id"], add_payload)
        
        # Add users to department
        user_ids: list[str] = [user["id"] for user in users]
        dept_add_payload: dict[str, list[str]] = {"user_ids": user_ids}
        add_department_members(web_api_auth, test_department["id"], dept_add_payload)
        
        # List members
        res: dict[str, Any] = list_department_members(web_api_auth, test_department["id"])
        assert res["code"] == 0
        assert len(res["data"]) == len(users)
        
        # Verify all users are in the list
        member_user_ids: set[str] = {member["user_id"] for member in res["data"]}
        test_user_ids: set[str] = {user["id"] for user in users}
        assert member_user_ids == test_user_ids

