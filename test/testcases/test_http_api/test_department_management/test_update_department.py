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
    get_user_info,
    login_as_user,
    update_department,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when updating departments."""

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
        """Test updating department with invalid or missing authentication."""
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

        # Try to update department with invalid auth
        update_payload: dict[str, str] = {"name": "Updated Name"}
        res: dict[str, Any] = update_department(invalid_auth, department_id, update_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestUpdateDepartment:
    """Comprehensive tests for department update API."""

    def _add_creator_as_member(
        self, web_api_auth: RAGFlowWebApiAuth, department_id: str
    ) -> None:
        """Helper to add the current user as a department member."""
        user_info: dict[str, Any] = get_user_info(web_api_auth)
        assert user_info["code"] == 0, user_info
        current_user_id: str = user_info["data"]["id"]
        add_member_payload: dict[str, list[str]] = {"user_ids": [current_user_id]}
        add_member_res: dict[str, Any] = add_department_members(
            web_api_auth, department_id, add_member_payload
        )
        assert add_member_res["code"] == 0, add_member_res

    @pytest.mark.p1
    def test_update_department_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test updating a department's name."""
        # Create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]

        # Create a department
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert dept_res["code"] == 0, dept_res
        department_id: str = dept_res["data"]["id"]

        # Add creator as department member
        self._add_creator_as_member(web_api_auth, department_id)

        # Update the department name
        new_name: str = f"Updated Department {uuid.uuid4().hex[:8]}"
        update_payload: dict[str, str] = {"name": new_name}
        update_res: dict[str, Any] = update_department(
            web_api_auth, department_id, update_payload
        )
        assert update_res["code"] == 0, update_res
        assert "data" in update_res
        assert update_res["data"]["name"] == new_name
        assert update_res["data"]["id"] == department_id
        assert update_res["data"]["tenant_id"] == tenant_id

    @pytest.mark.p1
    def test_update_department_description(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test updating a department's description."""
        # Create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]

        # Create a department
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert dept_res["code"] == 0, dept_res
        department_id: str = dept_res["data"]["id"]

        # Add creator as department member
        self._add_creator_as_member(web_api_auth, department_id)

        # Update the department description
        new_description: str = "This is an updated description"
        update_payload: dict[str, str] = {"description": new_description}
        update_res: dict[str, Any] = update_department(
            web_api_auth, department_id, update_payload
        )
        assert update_res["code"] == 0, update_res
        assert "data" in update_res
        assert update_res["data"]["description"] == new_description

    @pytest.mark.p1
    def test_update_department_name_and_description(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test updating both name and description at once."""
        # Create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]

        # Create a department
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert dept_res["code"] == 0, dept_res
        department_id: str = dept_res["data"]["id"]

        # Add creator as department member
        self._add_creator_as_member(web_api_auth, department_id)

        # Update both name and description
        new_name: str = f"Updated Department {uuid.uuid4().hex[:8]}"
        new_description: str = "Updated description"
        update_payload: dict[str, str] = {
            "name": new_name,
            "description": new_description,
        }
        update_res: dict[str, Any] = update_department(
            web_api_auth, department_id, update_payload
        )
        assert update_res["code"] == 0, update_res
        assert "data" in update_res
        assert update_res["data"]["name"] == new_name
        assert update_res["data"]["description"] == new_description

    @pytest.mark.p1
    def test_update_department_empty_description(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test setting description to empty string (should set to None)."""
        # Create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]

        # Create a department with description
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
            "description": "Original description",
        }
        dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert dept_res["code"] == 0, dept_res
        department_id: str = dept_res["data"]["id"]

        # Add creator as department member
        self._add_creator_as_member(web_api_auth, department_id)

        # Update description to empty string
        update_payload: dict[str, str] = {"description": ""}
        update_res: dict[str, Any] = update_department(
            web_api_auth, department_id, update_payload
        )
        assert update_res["code"] == 0, update_res
        assert "data" in update_res
        # Empty description should be converted to None
        assert update_res["data"]["description"] is None or update_res["data"]["description"] == ""

    @pytest.mark.p2
    def test_update_department_invalid_department_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test updating a non-existent department."""
        invalid_id: str = "invalid_department_id_12345"
        update_payload: dict[str, str] = {"name": "Updated Name"}
        res: dict[str, Any] = update_department(
            web_api_auth, invalid_id, update_payload
        )
        assert res["code"] != 0, "Should fail for invalid department ID"
        assert "not found" in res["message"].lower() or "Department not found" in res["message"]

    @pytest.mark.p2
    def test_update_department_empty_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test updating department with empty name (should fail)."""
        # Create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]

        # Create a department
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert dept_res["code"] == 0, dept_res
        department_id: str = dept_res["data"]["id"]

        # Add creator as department member
        self._add_creator_as_member(web_api_auth, department_id)

        # Try to update with empty name
        update_payload: dict[str, str] = {"name": ""}
        res: dict[str, Any] = update_department(
            web_api_auth, department_id, update_payload
        )
        assert res["code"] != 0, "Should fail for empty name"
        assert "empty" in res["message"].lower() or "cannot be empty" in res["message"]

    @pytest.mark.p2
    def test_update_department_name_too_long(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test updating department with name exceeding 128 characters."""
        # Create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]

        # Create a department
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert dept_res["code"] == 0, dept_res
        department_id: str = dept_res["data"]["id"]

        # Add creator as department member
        self._add_creator_as_member(web_api_auth, department_id)

        # Try to update with name > 128 characters
        long_name: str = "a" * 129
        update_payload: dict[str, str] = {"name": long_name}
        res: dict[str, Any] = update_department(
            web_api_auth, department_id, update_payload
        )
        assert res["code"] != 0, "Should fail for name > 128 characters"
        assert "128" in res["message"] or "too long" in res["message"].lower()

    @pytest.mark.p2
    def test_update_department_no_fields(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test updating department without providing any fields."""
        # Create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]

        # Create a department
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert dept_res["code"] == 0, dept_res
        department_id: str = dept_res["data"]["id"]

        # Add creator as department member
        self._add_creator_as_member(web_api_auth, department_id)

        # Try to update without any fields
        update_payload: dict[str, Any] = {}
        res: dict[str, Any] = update_department(
            web_api_auth, department_id, update_payload
        )
        assert res["code"] != 0, "Should fail when no fields provided"
        assert "no fields" in res["message"].lower() or "provide" in res["message"].lower()

    @pytest.mark.p2
    def test_update_department_not_member(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test updating a department when user is not a member."""
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

        # Create another user
        another_user_email: str = f"anotheruser_{uuid.uuid4().hex[:8]}@example.com"
        another_user_password: str = "TestPassword123!"
        another_user_payload: dict[str, str] = {
            "email": another_user_email,
            "password": another_user_password,
            "nickname": "Another User",
        }
        another_user_res: dict[str, Any] = create_user(web_api_auth, another_user_payload)
        assert another_user_res["code"] == 0, "Failed to create another user"

        # Add another user to the team (but not to the department)
        add_team_payload: dict[str, list[str]] = {"users": [another_user_email]}
        add_team_res: dict[str, Any] = add_users_to_team(
            web_api_auth, tenant_id, add_team_payload
        )
        assert add_team_res["code"] == 0, "Failed to add another user to team"

        # Login as the other user
        another_user_auth: RAGFlowWebApiAuth = login_as_user(
            another_user_email, another_user_password
        )

        # Try to update the department as a non-member (should fail)
        update_payload: dict[str, str] = {"name": "Updated Name"}
        res: dict[str, Any] = update_department(
            another_user_auth, department_id, update_payload
        )
        assert res["code"] != 0, "Should fail when user is not a department member"
        assert "member" in res["message"].lower() or "permission" in res["message"].lower()

    @pytest.mark.p2
    def test_update_department_response_structure(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test that update response has the correct structure."""
        # Create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]

        # Create a department
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert dept_res["code"] == 0, dept_res
        department_id: str = dept_res["data"]["id"]

        # Add creator as department member
        self._add_creator_as_member(web_api_auth, department_id)

        # Update the department
        new_name: str = f"Updated Department {uuid.uuid4().hex[:8]}"
        update_payload: dict[str, str] = {"name": new_name}
        update_res: dict[str, Any] = update_department(
            web_api_auth, department_id, update_payload
        )
        assert update_res["code"] == 0, update_res
        assert "data" in update_res
        assert "message" in update_res
        assert isinstance(update_res["data"], dict)
        assert "id" in update_res["data"]
        assert "name" in update_res["data"]
        assert "tenant_id" in update_res["data"]
        assert update_res["data"]["id"] == department_id
        assert update_res["data"]["name"] == new_name

    @pytest.mark.p2
    def test_update_department_multiple_times(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test updating a department multiple times in sequence."""
        # Create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]

        # Create a department
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert dept_res["code"] == 0, dept_res
        department_id: str = dept_res["data"]["id"]

        # Add creator as department member
        self._add_creator_as_member(web_api_auth, department_id)

        # First update
        first_name: str = f"First Update {uuid.uuid4().hex[:8]}"
        first_payload: dict[str, str] = {"name": first_name}
        first_res: dict[str, Any] = update_department(
            web_api_auth, department_id, first_payload
        )
        assert first_res["code"] == 0, first_res
        assert first_res["data"]["name"] == first_name

        # Second update
        second_name: str = f"Second Update {uuid.uuid4().hex[:8]}"
        second_payload: dict[str, str] = {"name": second_name}
        second_res: dict[str, Any] = update_department(
            web_api_auth, department_id, second_payload
        )
        assert second_res["code"] == 0, second_res
        assert second_res["data"]["name"] == second_name

        # Third update
        third_name: str = f"Third Update {uuid.uuid4().hex[:8]}"
        third_payload: dict[str, str] = {"name": third_name}
        third_res: dict[str, Any] = update_department(
            web_api_auth, department_id, third_payload
        )
        assert third_res["code"] == 0, third_res
        assert third_res["data"]["name"] == third_name

    @pytest.mark.p2
    def test_update_department_only_description(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test updating only description without name."""
        # Create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]

        # Create a department
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        dept_res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert dept_res["code"] == 0, dept_res
        department_id: str = dept_res["data"]["id"]
        original_name: str = dept_res["data"]["name"]

        # Add creator as department member
        self._add_creator_as_member(web_api_auth, department_id)

        # Update only description
        new_description: str = "Only description updated"
        update_payload: dict[str, str] = {"description": new_description}
        update_res: dict[str, Any] = update_department(
            web_api_auth, department_id, update_payload
        )
        assert update_res["code"] == 0, update_res
        assert update_res["data"]["description"] == new_description
        # Name should remain unchanged
        assert update_res["data"]["name"] == original_name

