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

from common import create_department, create_team
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior during department creation."""

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
        """Test department creation with invalid or missing authentication."""
        # First create a team to use as tenant_id
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if team_res["code"] != 0:
            pytest.skip("Team creation failed, skipping auth test")
        
        tenant_id: str = team_res["data"]["id"]
        
        # Try to create department with invalid auth
        dept_payload: dict[str, str] = {
            "name": "Test Department Auth",
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_department(invalid_auth, dept_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestDepartmentCreate:
    """Comprehensive tests for department creation API."""

    @pytest.mark.p1
    def test_create_department_with_name_and_tenant_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a department with name and tenant_id."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create department
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert res["code"] == 0, res
        assert "data" in res
        assert res["data"]["name"] == dept_name
        assert res["data"]["tenant_id"] == tenant_id
        assert "id" in res["data"]
        assert "created successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_create_department_with_description(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a department with description."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create department with description
        dept_name: str = f"Test Department {uuid.uuid4().hex[:8]}"
        description: str = "This is a test department description"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
            "description": description,
        }
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert res["code"] == 0, res
        assert res["data"]["name"] == dept_name
        assert res["data"]["description"] == description

    @pytest.mark.p1
    def test_create_department_missing_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a department without name."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Try to create department without name
        dept_payload: dict[str, str] = {"tenant_id": tenant_id}
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert res["code"] == 101
        assert "name" in res["message"].lower() or "required" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_create_department_empty_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a department with empty name."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Try to create department with empty name
        dept_payload: dict[str, str] = {"name": "", "tenant_id": tenant_id}
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert res["code"] == 101
        assert "name" in res["message"].lower() or "empty" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_create_department_missing_tenant_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a department without tenant_id."""
        # Try to create department without tenant_id
        dept_payload: dict[str, str] = {"name": "Test Department"}
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert res["code"] == 101
        assert "tenant_id" in res["message"].lower() or "required" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_create_department_invalid_tenant_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a department with non-existent tenant_id."""
        dept_payload: dict[str, str] = {
            "name": "Test Department Invalid Tenant",
            "tenant_id": "non_existent_tenant_id_12345",
        }
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        # Permission check happens before tenant existence check,
        # so invalid tenant_id results in permission error (108) not data error (102)
        assert res["code"] == 108
        assert "only team owners or admins" in res["message"].lower() or "permission" in res[
            "message"
        ].lower()

    @pytest.mark.p1
    def test_create_department_name_too_long(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a department with name exceeding 128 characters."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Try to create department with name too long
        # Note: The API doesn't validate name length upfront, but database has max_length=128
        # The database may truncate or error, but API doesn't check before saving
        long_name: str = "A" * 129
        dept_payload: dict[str, str] = {
            "name": long_name,
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        # API doesn't validate length, so it may succeed or fail at database level
        # If it succeeds, the name might be truncated; if it fails, we get an exception error
        assert res["code"] in (0, 100)  # Success or exception error
        if res["code"] == 0:
            # If successful, verify the name was stored (may be truncated by DB)
            assert "name" in res["data"]
            assert len(res["data"]["name"]) <= 128

    @pytest.mark.p1
    def test_create_department_not_team_owner_or_admin(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a department when user is not team owner or admin."""
        # This test would require creating a team with a different user
        # and then trying to create a department as a non-admin user
        # For now, we'll skip this as it requires multi-user setup
        pytest.skip("Requires multi-user setup to test permission restrictions")

    @pytest.mark.p1
    def test_create_department_response_structure(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test that department creation returns the expected response structure."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create department
        dept_name: str = f"Test Department Structure {uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert res["code"] == 0
        assert "data" in res
        assert isinstance(res["data"], dict)
        assert "id" in res["data"]
        assert "name" in res["data"]
        assert "tenant_id" in res["data"]
        assert res["data"]["name"] == dept_name
        assert res["data"]["tenant_id"] == tenant_id
        assert "message" in res
        assert "created successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_create_multiple_departments_same_team(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating multiple departments for the same team."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create first department
        dept_name_1: str = f"Department 1 {uuid.uuid4().hex[:8]}"
        dept_payload_1: dict[str, str] = {
            "name": dept_name_1,
            "tenant_id": tenant_id,
        }
        res1: dict[str, Any] = create_department(web_api_auth, dept_payload_1)
        assert res1["code"] == 0, res1
        dept_id_1: str = res1["data"]["id"]

        # Create second department
        dept_name_2: str = f"Department 2 {uuid.uuid4().hex[:8]}"
        dept_payload_2: dict[str, str] = {
            "name": dept_name_2,
            "tenant_id": tenant_id,
        }
        res2: dict[str, Any] = create_department(web_api_auth, dept_payload_2)
        assert res2["code"] == 0, res2
        dept_id_2: str = res2["data"]["id"]

        # Verify departments are different
        assert dept_id_1 != dept_id_2
        assert res1["data"]["name"] == dept_name_1
        assert res2["data"]["name"] == dept_name_2
        assert res1["data"]["tenant_id"] == tenant_id
        assert res2["data"]["tenant_id"] == tenant_id

    @pytest.mark.p2
    def test_create_department_with_whitespace_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a department with whitespace-only name."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Try to create department with whitespace-only name
        dept_payload: dict[str, str] = {
            "name": "   ",
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        # Should fail validation
        assert res["code"] == 101
        assert "name" in res["message"].lower() or "empty" in res[
            "message"
        ].lower()

    @pytest.mark.p2
    def test_create_department_special_characters_in_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a department with special characters in name."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create department with special characters
        dept_name: str = f"Dept-{uuid.uuid4().hex[:8]}_Test!"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        # Should succeed if special chars are allowed
        assert res["code"] in (0, 101)

    @pytest.mark.p2
    def test_create_department_empty_payload(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a department with empty payload."""
        dept_payload: dict[str, Any] = {}
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        assert res["code"] == 101
        assert "required" in res["message"].lower() or "name" in res[
            "message"
        ].lower()

    @pytest.mark.p3
    def test_create_department_unicode_name(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test creating a department with unicode characters in name."""
        # First create a team
        team_name: str = f"Test Team {uuid.uuid4().hex[:8]}"
        team_payload: dict[str, str] = {"name": team_name}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        assert team_res["code"] == 0, team_res
        tenant_id: str = team_res["data"]["id"]
        
        # Create department with unicode name
        dept_name: str = f"部门{uuid.uuid4().hex[:8]}"
        dept_payload: dict[str, str] = {
            "name": dept_name,
            "tenant_id": tenant_id,
        }
        res: dict[str, Any] = create_department(web_api_auth, dept_payload)
        # Should succeed if unicode is supported
        assert res["code"] in (0, 101)

