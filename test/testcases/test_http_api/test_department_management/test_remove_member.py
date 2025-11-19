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
        web_api_auth: RAGFlowWebApiAuth,
    ) -> None:
        """Test removing members with invalid or missing authentication."""
        # Create a team, department, and add a user
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
        
        # Create and add a user to team and department
        email = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        user_payload: dict[str, str] = {
            "email": email,
            "password": "TestPassword123!",
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        if user_res["code"] != 0:
            pytest.skip("User creation failed, skipping auth test")
        
        user_id: str = user_res["data"]["id"]
        
        # Add user to team
        add_team_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, tenant_id, add_team_payload)
        
        # Add user to department
        add_dept_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_department_members(web_api_auth, department_id, add_dept_payload)
        
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
    def test_user_with_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
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
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        assert user_res["code"] == 0
        
        user_id: str = user_res["data"]["id"]
        
        # Add user to team
        add_team_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, test_team["id"], add_team_payload)
        
        # Add user to department
        add_dept_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_dept_payload
        )
        assert add_res["code"] == 0
        assert len(add_res["data"]["added"]) == 1
        
        return {"id": user_id, "email": email}

    @pytest.mark.p1
    def test_remove_single_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test removing a single member from a department."""
        user_id: str = test_user_with_member["id"]
        res: dict[str, Any] = remove_department_member(
            web_api_auth, test_department["id"], user_id
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
        web_api_auth: RAGFlowWebApiAuth,
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test removing a member from a non-existent department."""
        user_id: str = test_user_with_member["id"]
        res: dict[str, Any] = remove_department_member(
            web_api_auth, "non_existent_department_id_12345", user_id
        )
        
        assert res["code"] == 102
        assert "department not found" in res["message"].lower() or "not found" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_remove_member_user_not_in_department(
        self,
        web_api_auth: RAGFlowWebApiAuth,
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
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        assert user_res["code"] == 0
        
        user_id: str = user_res["data"]["id"]
        
        # Add user to team but not department
        add_team_payload: dict[str, list[str]] = {"users": [email]}
        add_users_to_team(web_api_auth, test_team["id"], add_team_payload)
        
        # Try to remove from department
        res: dict[str, Any] = remove_department_member(
            web_api_auth, test_department["id"], user_id
        )
        
        assert res["code"] == 102
        assert "not a member" in res["message"].lower() or "not found" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_remove_member_invalid_user_id(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test removing a non-existent user from a department."""
        res: dict[str, Any] = remove_department_member(
            web_api_auth, test_department["id"], "non_existent_user_id_12345"
        )
        
        # The API checks if user is in department first, so this should return not found
        assert res["code"] == 102
        assert "not a member" in res["message"].lower() or "not found" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_remove_member_twice(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test removing the same member twice (idempotent operation)."""
        user_id: str = test_user_with_member["id"]
        
        # Remove first time
        res1: dict[str, Any] = remove_department_member(
            web_api_auth, test_department["id"], user_id
        )
        assert res1["code"] == 0
        
        # Try to remove again - API is idempotent, so it succeeds again
        # (the record exists but is soft-deleted, and we update it again)
        res2: dict[str, Any] = remove_department_member(
            web_api_auth, test_department["id"], user_id
        )
        assert res2["code"] == 0  # API allows removing twice (idempotent)
        assert "removed" in res2["message"].lower() or "success" in res2[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_remove_member_response_structure(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test that remove member returns the expected response structure."""
        user_id: str = test_user_with_member["id"]
        res: dict[str, Any] = remove_department_member(
            web_api_auth, test_department["id"], user_id
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
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test removing members when user is not team owner or admin."""
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
        
        # Add another user to the department (so we have someone to remove)
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
        
        # Add another user to the department (as owner/admin)
        add_dept_payload: dict[str, list[str]] = {"user_ids": [another_user_id]}
        add_dept_res: dict[str, Any] = add_department_members(
            web_api_auth, department_id, add_dept_payload
        )
        assert add_dept_res["code"] == 0, "Failed to add user to department"
        
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
        
        # Try to remove a member as the normal user (should fail)
        remove_res: dict[str, Any] = remove_department_member(
            normal_user_auth, department_id, another_user_id
        )
        assert remove_res["code"] == 108, f"Expected permission error (108), got {remove_res}"
        assert "only team owners or admins" in remove_res["message"].lower()

    @pytest.mark.p2
    def test_remove_and_re_add_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_user_with_member: dict[str, Any],
    ) -> None:
        """Test removing a member and then adding them back."""
        user_id: str = test_user_with_member["id"]
        
        # Remove member
        remove_res: dict[str, Any] = remove_department_member(
            web_api_auth, test_department["id"], user_id
        )
        assert remove_res["code"] == 0
        
        # Add member back
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_payload
        )
        assert add_res["code"] == 0
        assert len(add_res["data"]["added"]) == 1
        assert add_res["data"]["added"][0] == user_id

    @pytest.mark.p2
    def test_remove_multiple_members_sequentially(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
        test_team: dict[str, Any],
    ) -> None:
        """Test removing multiple members sequentially."""
        # Create multiple users
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
        
        if len(users) < 2:
            pytest.skip("Need at least 2 test users")
        
        # Add users to team
        for user in users:
            add_team_payload: dict[str, list[str]] = {"users": [user["email"]]}
            add_users_to_team(web_api_auth, test_team["id"], add_team_payload)
        
        # Add users to department
        user_ids: list[str] = [user["id"] for user in users]
        add_dept_payload: dict[str, list[str]] = {"user_ids": user_ids}
        add_res: dict[str, Any] = add_department_members(
            web_api_auth, test_department["id"], add_dept_payload
        )
        assert add_res["code"] == 0
        assert len(add_res["data"]["added"]) == len(users)
        
        # Remove all users sequentially
        for user in users:
            remove_res: dict[str, Any] = remove_department_member(
                web_api_auth, test_department["id"], user["id"]
            )
            assert remove_res["code"] == 0
            assert "removed" in remove_res["message"].lower() or "success" in remove_res["message"].lower()

    @pytest.mark.p2
    def test_remove_member_empty_string_user_id(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test removing a member with empty string user ID."""
        res: dict[str, Any] = remove_department_member(
            web_api_auth, test_department["id"], ""
        )
        
        # Empty string user ID may return 100 (EXCEPTION_ERROR) or 102 (DATA_ERROR)
        assert res["code"] in [100, 102]
        assert "not a member" in res["message"].lower() or "not found" in res["message"].lower() or "error" in res["message"].lower()

    @pytest.mark.p2
    def test_remove_member_special_characters_user_id(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_department: dict[str, Any],
    ) -> None:
        """Test removing a member with special characters in user ID."""
        invalid_user_id: str = "user@123_!@#$%"
        res: dict[str, Any] = remove_department_member(
            web_api_auth, test_department["id"], invalid_user_id
        )
        
        assert res["code"] == 102
        assert "not a member" in res["message"].lower() or "not found" in res["message"].lower()

