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
    add_group_members,
    add_users_to_team,
    create_group,
    create_team,
    create_user,
    encrypt_password,
    list_group_members,
    login_as_user,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when listing group members."""

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
        # Create a team and group first
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if team_res["code"] != 0:
            pytest.skip("Team creation failed, skipping auth test")
        
        tenant_id: str = team_res["data"]["id"]
        
        group_name: str = f"Test Group {uuid.uuid4().hex[:8]}"
        group_payload: dict[str, str] = {
            "name": group_name,
            "tenant_id": tenant_id,
        }
        group_res: dict[str, Any] = create_group(web_api_auth, group_payload)
        if group_res["code"] != 0:
            pytest.skip("Group creation failed, skipping auth test")
        
        group_id: str = group_res["data"]["id"]
        
        # Try to list members with invalid auth
        res: dict[str, Any] = list_group_members(invalid_auth, group_id)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestListMembers:
    """Comprehensive tests for listing group members."""

    @pytest.fixture
    def test_team(self, web_api_auth: RAGFlowWebApiAuth) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def test_group(
        self, web_api_auth: RAGFlowWebApiAuth, test_team: dict[str, Any]
    ) -> dict[str, Any]:
        """Create a test group for use in tests."""
        group_payload: dict[str, str] = {
            "name": f"Test Group {uuid.uuid4().hex[:8]}",
            "tenant_id": test_team["id"],
        }
        res: dict[str, Any] = create_group(web_api_auth, group_payload)
        assert res["code"] == 0
        return res["data"]

    @pytest.fixture
    def test_users(self, web_api_auth: RAGFlowWebApiAuth) -> list[dict[str, Any]]:
        """Create test users for use in tests."""
        users = []
        for i in range(3):
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
    def group_with_members(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> dict[str, Any]:
        """Add test users to the group."""
        if not test_users:
            return test_group
        
        user_ids: list[str] = [user["id"] for user in test_users]
        add_payload: dict[str, list[str]] = {"user_ids": user_ids}
        add_group_members(web_api_auth, test_group["id"], add_payload)
        return test_group

    @pytest.mark.p1
    def test_list_members_empty_group(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
    ) -> None:
        """Test listing members from an empty group."""
        group_id: str = test_group["id"]
        
        res: dict[str, Any] = list_group_members(web_api_auth, group_id)
        assert res["code"] == 0, res
        assert "data" in res
        assert isinstance(res["data"], list)
        assert len(res["data"]) == 0

    @pytest.mark.p1
    def test_list_members_with_multiple_users(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        group_with_members: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test listing members from a group with multiple users."""
        if not test_users:
            pytest.skip("No test users created")
        
        group_id: str = group_with_members["id"]
        
        res: dict[str, Any] = list_group_members(web_api_auth, group_id)
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
    def test_list_members_invalid_group_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test listing members from a non-existent group."""
        invalid_id: str = f"invalid_{uuid.uuid4().hex[:8]}"
        res: dict[str, Any] = list_group_members(web_api_auth, invalid_id)
        assert res["code"] == 102  # DATA_ERROR
        assert "not found" in res["message"].lower()

    @pytest.mark.p1
    def test_list_members_not_team_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
    ) -> None:
        """Test listing members when user is not a team member."""
        # Create a new user who is not in the team
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
        
        # Login as the new user (not in team)
        new_user_auth: RAGFlowWebApiAuth = login_as_user(email, password)
        
        # Try to list members (user is not a team member)
        res: dict[str, Any] = list_group_members(new_user_auth, test_group["id"])
        assert res["code"] == 108  # PERMISSION_ERROR
        assert "team member" in res["message"].lower() or "member of the team" in res["message"].lower()

    @pytest.mark.p1
    def test_list_members_response_structure(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        group_with_members: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test that listing members returns the expected response structure."""
        if not test_users:
            pytest.skip("No test users created")
        
        group_id: str = group_with_members["id"]
        
        res: dict[str, Any] = list_group_members(web_api_auth, group_id)
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
        test_group: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test listing members after adding users to the group."""
        if not test_users:
            pytest.skip("No test users created")
        
        group_id: str = test_group["id"]
        
        # List members before adding
        res_before: dict[str, Any] = list_group_members(web_api_auth, group_id)
        assert res_before["code"] == 0
        initial_count: int = len(res_before["data"])
        
        # Add a user
        user_id: str = test_users[0]["id"]
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_res: dict[str, Any] = add_group_members(web_api_auth, group_id, add_payload)
        assert add_res["code"] == 0
        
        # List members after adding
        res_after: dict[str, Any] = list_group_members(web_api_auth, group_id)
        assert res_after["code"] == 0
        assert len(res_after["data"]) == initial_count + 1
        
        # Verify the added user is in the list
        member_user_ids: set[str] = {member["user_id"] for member in res_after["data"]}
        assert user_id in member_user_ids

    @pytest.mark.p1
    def test_list_members_only_valid_status(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test that only members with valid status are returned."""
        if not test_users:
            pytest.skip("No test users created")
        
        group_id: str = test_group["id"]
        
        # Add users to group
        user_ids: list[str] = [user["id"] for user in test_users]
        add_payload: dict[str, list[str]] = {"user_ids": user_ids}
        add_group_members(web_api_auth, group_id, add_payload)
        
        # List members - all should have valid status
        res: dict[str, Any] = list_group_members(web_api_auth, group_id)
        assert res["code"] == 0
        
        for member in res["data"]:
            assert member["status"] == "1"  # VALID status

    @pytest.mark.p1
    def test_list_members_team_member_not_group_member(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        team_with_users: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test that a team member (not group member) can list group members."""
        if not test_users or len(test_users) < 2:
            pytest.skip("Need at least 2 test users")
        
        group_id: str = test_group["id"]
        
        # Add one user to group
        user_id: str = test_users[0]["id"]
        add_payload: dict[str, list[str]] = {"user_ids": [user_id]}
        add_group_members(web_api_auth, group_id, add_payload)
        
        # Login as another team member (not in group)
        other_user_email: str = test_users[1]["email"]
        other_user_password: str = test_users[1]["password"]
        other_user_auth: RAGFlowWebApiAuth = login_as_user(other_user_email, other_user_password)
        
        # Team member should be able to list members
        res: dict[str, Any] = list_group_members(other_user_auth, group_id)
        assert res["code"] == 0
        assert len(res["data"]) == 1
        assert res["data"][0]["user_id"] == user_id

    @pytest.mark.p1
    def test_list_members_after_removing(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        group_with_members: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test listing members after removing a user from the group."""
        if not test_users:
            pytest.skip("No test users created")
        
        from common import remove_group_member
        
        group_id: str = group_with_members["id"]
        
        # List members before removing
        res_before: dict[str, Any] = list_group_members(web_api_auth, group_id)
        assert res_before["code"] == 0
        initial_count: int = len(res_before["data"])
        
        if initial_count == 0:
            pytest.skip("No members to remove")
        
        # Remove a user
        user_id: str = test_users[0]["id"]
        remove_res: dict[str, Any] = remove_group_member(web_api_auth, group_id, user_id)
        assert remove_res["code"] == 0
        
        # List members after removing
        res_after: dict[str, Any] = list_group_members(web_api_auth, group_id)
        assert res_after["code"] == 0
        assert len(res_after["data"]) == initial_count - 1
        
        # Verify the removed user is not in the list
        member_user_ids: set[str] = {member["user_id"] for member in res_after["data"]}
        assert user_id not in member_user_ids

    @pytest.mark.p2
    def test_list_members_empty_string_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test listing members with empty string group ID."""
        res: dict[str, Any] = list_group_members(web_api_auth, "")
        assert res["code"] != 0
        # Empty string ID may result in 405 (Method Not Allowed) or 102 (Data Error)
        assert res["code"] in [100, 102, 405] or "not found" in res["message"].lower()

    @pytest.mark.p2
    def test_list_members_special_characters_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test listing members with special characters in group ID."""
        invalid_id: str = "group-123_!@#$%"
        res: dict[str, Any] = list_group_members(web_api_auth, invalid_id)
        assert res["code"] != 0
        # Invalid ID may result in 405 (Method Not Allowed) or 102 (Data Error)
        assert res["code"] in [100, 102, 405] or "not found" in res["message"].lower()

    @pytest.mark.p2
    def test_list_members_multiple_groups(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        test_users: list[dict[str, Any]],
    ) -> None:
        """Test listing members from multiple groups."""
        if not test_users:
            pytest.skip("No test users created")
        
        # Create multiple groups
        groups = []
        for i in range(2):
            group_name: str = f"Group {i} {uuid.uuid4().hex[:8]}"
            group_payload: dict[str, str] = {
                "name": group_name,
                "tenant_id": test_team["id"],
            }
            group_res: dict[str, Any] = create_group(web_api_auth, group_payload)
            if group_res["code"] == 0:
                groups.append(group_res["data"])
        
        if len(groups) < 2:
            pytest.skip("Group creation failed")
        
        # Add users to team first
        for user in test_users:
            add_payload: dict[str, list[str]] = {"users": [user["email"]]}
            add_users_to_team(web_api_auth, test_team["id"], add_payload)
        
        # Add different users to each group
        group1_add_payload: dict[str, list[str]] = {"user_ids": [test_users[0]["id"]]}
        add_group_members(web_api_auth, groups[0]["id"], group1_add_payload)
        
        if len(test_users) > 1:
            group2_add_payload: dict[str, list[str]] = {"user_ids": [test_users[1]["id"]]}
            add_group_members(web_api_auth, groups[1]["id"], group2_add_payload)
        
        # List members from each group
        res1: dict[str, Any] = list_group_members(web_api_auth, groups[0]["id"])
        assert res1["code"] == 0
        assert len(res1["data"]) == 1
        assert res1["data"][0]["user_id"] == test_users[0]["id"]
        
        if len(test_users) > 1:
            res2: dict[str, Any] = list_group_members(web_api_auth, groups[1]["id"])
            assert res2["code"] == 0
            assert len(res2["data"]) == 1
            assert res2["data"][0]["user_id"] == test_users[1]["id"]

    @pytest.mark.p2
    def test_list_members_large_group(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_group: dict[str, Any],
        test_team: dict[str, Any],
    ) -> None:
        """Test listing members from a group with many users."""
        # Create multiple users
        users = []
        for i in range(10):
            email = f"bulkuser{i}_{uuid.uuid4().hex[:8]}@example.com"
            password = "TestPassword123!"
            encrypted_password = encrypt_password(password)
            user_payload: dict[str, str] = {
                "email": email,
                "password": encrypted_password,
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
            add_users_to_team(web_api_auth, test_team["id"], add_payload)
        
        # Add users to group
        user_ids: list[str] = [user["id"] for user in users]
        group_add_payload: dict[str, list[str]] = {"user_ids": user_ids}
        add_group_members(web_api_auth, test_group["id"], group_add_payload)
        
        # List members
        res: dict[str, Any] = list_group_members(web_api_auth, test_group["id"])
        assert res["code"] == 0
        assert len(res["data"]) == len(users)
        
        # Verify all users are in the list
        member_user_ids: set[str] = {member["user_id"] for member in res["data"]}
        test_user_ids: set[str] = {user["id"] for user in users}
        assert member_user_ids == test_user_ids

