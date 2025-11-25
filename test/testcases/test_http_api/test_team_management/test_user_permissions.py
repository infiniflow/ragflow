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
    add_users_to_team,
    create_team,
    create_user,
    encrypt_password,
    get_user_permissions,
    login_as_user,
    update_user_permissions,
)
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior when managing user permissions."""

    @pytest.mark.parametrize(
        ("invalid_auth", "expected_code", "expected_message"),
        [
            (None, 401, "Unauthorized"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "Unauthorized"),
        ],
    )
    def test_get_permissions_invalid_auth(
        self,
        invalid_auth: RAGFlowWebApiAuth | None,
        expected_code: int,
        expected_message: str,
        web_api_auth: RAGFlowWebApiAuth,
        clear_teams: list[str],
        clear_team_users: list[str],
    ) -> None:
        """Test getting permissions with invalid or missing authentication."""
        # Create a team and add a user first
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if team_res.get("code", -1) != 0:
            pytest.skip(
                f"Team creation failed with code {team_res.get('code')}: "
                f"{team_res.get('message', 'Unknown error')}. Full response: {team_res}"
            )
        
        tenant_id: str = team_res["data"]["id"]
        clear_teams.append(tenant_id)
        
        # Create and add a user
        email = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        if user_res["code"] != 0:
            pytest.skip(f"User creation failed, skipping auth test: {user_res}")
        clear_team_users.append(email)
        
        user_id: str = user_res["data"]["id"]
        add_payload: dict[str, list[str]] = {"users": [email]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        if add_res.get("code", -1) != 0:
            pytest.skip(f"Failed to add user to team in setup: {add_res}")

        # Small delay
        time.sleep(0.5)

        # Accept invitation as the user
        user_auth: RAGFlowWebApiAuth = login_as_user(email, password)
        accept_res: dict[str, Any] = accept_team_invitation(user_auth, tenant_id)
        if accept_res.get("code", -1) != 0:
            pytest.skip(f"Failed to accept team invitation in setup: {accept_res}")

        # Try to get permissions with invalid auth
        res: dict[str, Any] = get_user_permissions(invalid_auth, tenant_id, user_id)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]

    @pytest.mark.parametrize(
        ("invalid_auth", "expected_code", "expected_message"),
        [
            (None, 401, "Unauthorized"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "Unauthorized"),
        ],
    )
    def test_update_permissions_invalid_auth(
        self,
        invalid_auth: RAGFlowWebApiAuth | None,
        expected_code: int,
        expected_message: str,
        web_api_auth: RAGFlowWebApiAuth,
        clear_teams: list[str],
        clear_team_users: list[str],
    ) -> None:
        """Test updating permissions with invalid or missing authentication."""
        # Create a team and add a user first
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        team_res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if team_res.get("code", -1) != 0:
            pytest.skip(
                f"Team creation failed with code {team_res.get('code')}: "
                f"{team_res.get('message', 'Unknown error')}. Full response: {team_res}"
            )
        
        tenant_id: str = team_res["data"]["id"]
        clear_teams.append(tenant_id)
        
        # Create and add a user
        email = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        if user_res["code"] != 0:
            pytest.skip(f"User creation failed, skipping auth test: {user_res}")
        clear_team_users.append(email)
        
        user_id: str = user_res["data"]["id"]
        add_payload: dict[str, list[str]] = {"users": [email]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        if add_res.get("code", -1) != 0:
            pytest.skip(f"Failed to add user to team in setup: {add_res}")

        # Small delay
        time.sleep(0.5)

        # Accept invitation as the user
        user_auth: RAGFlowWebApiAuth = login_as_user(email, password)
        accept_res: dict[str, Any] = accept_team_invitation(user_auth, tenant_id)
        if accept_res.get("code", -1) != 0:
            pytest.skip(f"Failed to accept team invitation in setup: {accept_res}")

        # Try to update permissions with invalid auth
        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"create": True, "read": True, "update": False, "delete": False},
                "canvas": {"create": False, "read": True, "update": False, "delete": False}
            }
        }
        res: dict[str, Any] = update_user_permissions(invalid_auth, tenant_id, user_id, update_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.p1
class TestGetUserPermissions:
    """Comprehensive tests for getting user permissions."""

    @pytest.fixture
    def test_team(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        clear_teams: list[str],
    ) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if res["code"] != 0:
            pytest.skip(
                f"Team creation failed with code {res.get('code')}: "
                f"{res.get('message', 'Unknown error')}. Full response: {res}"
            )
        clear_teams.append(res["data"]["id"])
        return res["data"]

    @pytest.fixture
    def team_with_user(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        clear_team_users: list[str],
    ) -> dict[str, Any]:
        """Create a team with a user who has accepted the invitation."""
        tenant_id: str = test_team["id"]
        
        # Create user
        email = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        if user_res["code"] != 0:
            pytest.skip(f"User creation failed in setup: {user_res}")
        clear_team_users.append(email)
        user_id: str = user_res["data"]["id"]

        # Add user to team
        add_payload: dict[str, list[str]] = {"users": [email]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        if add_res["code"] != 0:
            pytest.skip(f"Failed to add user to team in setup: {add_res}")

        # Small delay
        time.sleep(0.5)

        # Accept invitation as the user
        user_auth: RAGFlowWebApiAuth = login_as_user(email, password)
        accept_res: dict[str, Any] = accept_team_invitation(user_auth, tenant_id)
        if accept_res["code"] != 0:
            pytest.skip(f"Failed to accept team invitation in setup: {accept_res}")

        return {
            "team": test_team,
            "user": {"id": user_id, "email": email, "password": password},
        }

    @pytest.mark.p1
    def test_get_default_permissions(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test getting default permissions for a new team member."""
        tenant_id: str = team_with_user["team"]["id"]
        user_id: str = team_with_user["user"]["id"]

        res: dict[str, Any] = get_user_permissions(web_api_auth, tenant_id, user_id)
        assert res["code"] == 0, res
        assert "data" in res
        permissions: dict[str, Any] = res["data"]
        
        # Check structure
        assert "dataset" in permissions
        assert "canvas" in permissions
        
        # Check default permissions (read-only)
        assert permissions["dataset"]["read"] is True
        assert permissions["dataset"]["create"] is False
        assert permissions["dataset"]["update"] is False
        assert permissions["dataset"]["delete"] is False
        
        assert permissions["canvas"]["read"] is True
        assert permissions["canvas"]["create"] is False
        assert permissions["canvas"]["update"] is False
        assert permissions["canvas"]["delete"] is False

    @pytest.mark.p1
    def test_get_owner_permissions(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
    ) -> None:
        """Test getting permissions for team owner (should have full permissions)."""
        tenant_id: str = test_team["id"]
        owner_id: str = test_team["owner_id"]

        res: dict[str, Any] = get_user_permissions(web_api_auth, tenant_id, owner_id)
        assert res["code"] == 0, res
        assert "data" in res
        permissions: dict[str, Any] = res["data"]
        
        # Owner should have full permissions
        assert permissions["dataset"]["create"] is True
        assert permissions["dataset"]["read"] is True
        assert permissions["dataset"]["update"] is True
        assert permissions["dataset"]["delete"] is True
        
        assert permissions["canvas"]["create"] is True
        assert permissions["canvas"]["read"] is True
        assert permissions["canvas"]["update"] is True
        assert permissions["canvas"]["delete"] is True

    @pytest.mark.p1
    def test_get_permissions_user_not_in_team(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
    ) -> None:
        """Test getting permissions for a user not in the team."""
        tenant_id: str = test_team["id"]
        invalid_user_id: str = f"invalid_{uuid.uuid4().hex[:8]}"

        res: dict[str, Any] = get_user_permissions(web_api_auth, tenant_id, invalid_user_id)
        assert res["code"] != 0
        assert "not a member" in res["message"].lower() or res["code"] in [100, 102]

    @pytest.mark.p1
    def test_get_permissions_not_owner_or_admin(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test that normal users cannot view permissions."""
        tenant_id: str = team_with_user["team"]["id"]
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]

        # Login as normal user
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)

        # Try to get permissions (normal user should not be able to)
        res: dict[str, Any] = get_user_permissions(user_auth, tenant_id, user_id)
        assert res["code"] == 108  # PERMISSION_ERROR
        assert "owner" in res["message"].lower() or "admin" in res["message"].lower()


@pytest.mark.p1
class TestUpdateUserPermissions:
    """Comprehensive tests for updating user permissions."""

    @pytest.fixture
    def test_team(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        clear_teams: list[str],
    ) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if res["code"] != 0:
            pytest.skip(
                f"Team creation failed with code {res.get('code')}: "
                f"{res.get('message', 'Unknown error')}. Full response: {res}"
            )
        clear_teams.append(res["data"]["id"])
        return res["data"]

    @pytest.fixture
    def team_with_user(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        clear_team_users: list[str],
    ) -> dict[str, Any]:
        """Create a team with a user who has accepted the invitation."""
        tenant_id: str = test_team["id"]
        
        # Create user
        email = f"testuser_{uuid.uuid4().hex[:8]}@example.com"
        password = "TestPassword123!"
        encrypted_password = encrypt_password(password)
        user_payload: dict[str, str] = {
            "email": email,
            "password": encrypted_password,
            "nickname": "Test User",
        }
        user_res: dict[str, Any] = create_user(web_api_auth, user_payload)
        if user_res["code"] != 0:
            pytest.skip(f"User creation failed in setup: {user_res}")
        clear_team_users.append(email)
        user_id: str = user_res["data"]["id"]

        # Add user to team
        add_payload: dict[str, list[str]] = {"users": [email]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        if add_res["code"] != 0:
            pytest.skip(f"Failed to add user to team in setup: {add_res}")

        # Small delay
        time.sleep(0.5)

        # Accept invitation as the user
        user_auth: RAGFlowWebApiAuth = login_as_user(email, password)
        accept_res: dict[str, Any] = accept_team_invitation(user_auth, tenant_id)
        if accept_res["code"] != 0:
            pytest.skip(f"Failed to accept team invitation in setup: {accept_res}")

        return {
            "team": test_team,
            "user": {"id": user_id, "email": email, "password": password},
        }

    @pytest.mark.p1
    def test_update_permissions_grant_create(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test updating permissions to grant create permission."""
        tenant_id: str = team_with_user["team"]["id"]
        user_id: str = team_with_user["user"]["id"]

        # Update permissions to grant create for dataset
        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"create": True},
            }
        }
        res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert res["code"] == 0, res
        assert "data" in res
        permissions: dict[str, Any] = res["data"]
        
        # Check that create is now True, but other permissions remain default
        assert permissions["dataset"]["create"] is True
        assert permissions["dataset"]["read"] is True  # Default
        assert permissions["dataset"]["update"] is False  # Default
        assert permissions["dataset"]["delete"] is False  # Default

    @pytest.mark.p1
    def test_update_permissions_grant_all(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test updating permissions to grant all CRUD permissions."""
        tenant_id: str = team_with_user["team"]["id"]
        user_id: str = team_with_user["user"]["id"]

        # Update permissions to grant all for both dataset and canvas
        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"create": True, "read": True, "update": True, "delete": True},
                "canvas": {"create": True, "read": True, "update": True, "delete": True}
            }
        }
        res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert res["code"] == 0, res
        assert "data" in res
        permissions: dict[str, Any] = res["data"]
        
        # Verify all permissions are True
        for resource_type in ["dataset", "canvas"]:
            assert permissions[resource_type]["create"] is True
            assert permissions[resource_type]["read"] is True
            assert permissions[resource_type]["update"] is True
            assert permissions[resource_type]["delete"] is True

    @pytest.mark.p1
    def test_update_permissions_revoke_read(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test updating permissions to revoke read permission."""
        tenant_id: str = team_with_user["team"]["id"]
        user_id: str = team_with_user["user"]["id"]

        # First grant all permissions
        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"create": True, "read": True, "update": True, "delete": True},
            }
        }
        res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert res["code"] == 0

        # Then revoke read
        revoke_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"read": False},
            }
        }
        res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, revoke_payload)
        assert res["code"] == 0, res
        permissions: dict[str, Any] = res["data"]
        
        # Read should be False, but other permissions should remain
        assert permissions["dataset"]["read"] is False
        assert permissions["dataset"]["create"] is True
        assert permissions["dataset"]["update"] is True
        assert permissions["dataset"]["delete"] is True

    @pytest.mark.p1
    def test_update_permissions_partial_update(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test that partial permission updates only change specified fields."""
        tenant_id: str = team_with_user["team"]["id"]
        user_id: str = team_with_user["user"]["id"]

        # Update only canvas create permission
        update_payload: dict[str, Any] = {
            "permissions": {
                "canvas": {"create": True},
            }
        }
        res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert res["code"] == 0, res
        permissions: dict[str, Any] = res["data"]
        
        # Canvas create should be True, but dataset should remain default
        assert permissions["canvas"]["create"] is True
        assert permissions["dataset"]["create"] is False  # Still default

    @pytest.mark.p1
    def test_update_permissions_owner_or_admin(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
    ) -> None:
        """Test that owner/admin permissions cannot be updated."""
        tenant_id: str = test_team["id"]
        owner_id: str = test_team["owner_id"]

        # Try to update owner permissions
        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"create": False},
            }
        }
        res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, owner_id, update_payload)
        assert res["code"] != 0
        assert "owner" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p1
    def test_update_permissions_user_not_in_team(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
    ) -> None:
        """Test updating permissions for a user not in the team."""
        tenant_id: str = test_team["id"]
        invalid_user_id: str = f"invalid_{uuid.uuid4().hex[:8]}"

        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"create": True},
            }
        }
        res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, invalid_user_id, update_payload)
        assert res["code"] != 0
        assert "not a member" in res["message"].lower() or res["code"] in [100, 102]

    @pytest.mark.p1
    def test_update_permissions_not_owner_or_admin(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test that normal users cannot update permissions."""
        tenant_id: str = team_with_user["team"]["id"]
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]

        # Login as normal user
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)

        # Try to update permissions (normal user should not be able to)
        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"create": True},
            }
        }
        res: dict[str, Any] = update_user_permissions(user_auth, tenant_id, user_id, update_payload)
        assert res["code"] == 108  # PERMISSION_ERROR
        assert "owner" in res["message"].lower() or "admin" in res["message"].lower()

    @pytest.mark.p1
    def test_update_permissions_invalid_resource_type(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test updating permissions with invalid resource type."""
        tenant_id: str = team_with_user["team"]["id"]
        user_id: str = team_with_user["user"]["id"]

        update_payload: dict[str, Any] = {
            "permissions": {
                "invalid_resource": {"create": True},
            }
        }
        res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert res["code"] != 0
        assert "invalid" in res["message"].lower() or "resource" in res["message"].lower()

    @pytest.mark.p1
    def test_update_permissions_invalid_permission_name(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test updating permissions with invalid permission name."""
        tenant_id: str = team_with_user["team"]["id"]
        user_id: str = team_with_user["user"]["id"]

        update_payload: dict[str, Any] = {
            "permissions": {
                "dataset": {"invalid_permission": True},
            }
        }
        res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert res["code"] != 0
        assert "invalid" in res["message"].lower() or "permission" in res["message"].lower()

    @pytest.mark.p1
    def test_update_permissions_missing_permissions_field(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test updating permissions without permissions field."""
        tenant_id: str = team_with_user["team"]["id"]
        user_id: str = team_with_user["user"]["id"]

        update_payload: dict[str, Any] = {}
        res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert res["code"] != 0
        assert "required" in res["message"].lower() or "permissions" in res["message"].lower()

