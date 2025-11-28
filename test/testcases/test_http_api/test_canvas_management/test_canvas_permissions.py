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

import json
import uuid
from typing import Any, List

import pytest

from common import (
    add_users_to_team,
    create_canvas,
    create_team,
    create_user,
    debug_canvas,
    delete_canvas,
    encrypt_password,
    get_canvas,
    get_user_permissions,
    login_as_user,
    reset_canvas,
    run_canvas,
    update_canvas_setting,
    update_user_permissions,
)
from libs.auth import RAGFlowWebApiAuth


@pytest.mark.p1
class TestCanvasPermissions:
    """Comprehensive tests for canvas permissions with CRUD operations."""

    @pytest.fixture
    def test_team(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        clear_teams: List[str],
    ) -> dict[str, Any]:
        """Create a test team for use in tests."""
        team_payload: dict[str, str] = {"name": f"Test Team {uuid.uuid4().hex[:8]}"}
        res: dict[str, Any] = create_team(web_api_auth, team_payload)
        if res["code"] != 0:
            pytest.skip(f"Team creation failed in setup: {res}")
        clear_teams.append(res["data"]["id"])
        return res["data"]

    @pytest.fixture
    def team_with_user(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        clear_team_users: List[str],
    ) -> dict[str, Any]:
        """Create a team with a user who has been added to the team."""
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

        # Add user to team (users are now added directly, no invitation needed)
        add_payload: dict[str, list[str]] = {"users": [email]}
        add_res: dict[str, Any] = add_users_to_team(web_api_auth, tenant_id, add_payload)
        if add_res["code"] != 0:
            pytest.skip(f"Failed to add user to team in setup: {add_res}")

        return {
            "team": test_team,
            "user": {"id": user_id, "email": email, "password": password},
        }

    @pytest.fixture
    def team_canvas(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        clear_canvases: List[str],
    ) -> dict[str, Any]:
        """Create a canvas shared with team."""
        # Simple canvas DSL
        canvas_dsl = {
            "nodes": [
                {
                    "id": "start",
                    "type": "start",
                    "position": {"x": 100, "y": 100},
                }
            ],
            "edges": [],
        }
        
        canvas_payload: dict[str, Any] = {
            "title": f"Test Canvas {uuid.uuid4().hex[:8]}",
            "dsl": json.dumps(canvas_dsl),
            "permission": "team",
            "shared_tenant_id": test_team["id"],
            "canvas_category": "Agent",
        }
        
        res: dict[str, Any] = create_canvas(web_api_auth, canvas_payload)
        if res["code"] != 0:
            pytest.skip(f"Canvas creation failed in setup: {res}")
        clear_canvases.append(res["data"]["id"])
        return res["data"]

    @pytest.mark.p1
    def test_read_permission_allows_get_canvas(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_canvas: dict[str, Any],
    ) -> None:
        """Test that user with read permission can get canvas."""
        canvas_id: str = team_canvas["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]

        # User should have read permission by default
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        
        res: dict[str, Any] = get_canvas(user_auth, canvas_id)
        assert res["code"] == 0, res
        assert "data" in res
        assert res["data"]["id"] == canvas_id

    @pytest.mark.p1
    def test_no_read_permission_denies_get_canvas(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_canvas: dict[str, Any],
    ) -> None:
        """Test that user without read permission cannot get canvas."""
        canvas_id: str = team_canvas["id"]
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Revoke read permission
        update_payload: dict[str, Any] = {
            "permissions": {
                "canvas": {"read": False},
            }
        }
        update_res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert update_res["code"] == 0

        # User should not be able to get canvas
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        res: dict[str, Any] = get_canvas(user_auth, canvas_id)
        assert res["code"] != 0
        assert "permission" in res["message"].lower() or "read" in res["message"].lower()

    @pytest.mark.p1
    def test_update_permission_allows_update_canvas(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_canvas: dict[str, Any],
    ) -> None:
        """Test that user with update permission can update canvas."""
        canvas_id: str = team_canvas["id"]
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Grant update permission
        update_payload: dict[str, Any] = {
            "permissions": {
                "canvas": {"update": True},
            }
        }
        update_res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert update_res["code"] == 0

        # User should be able to update canvas settings
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        setting_payload: dict[str, Any] = {
            "id": canvas_id,
            "title": "Updated Title",
            "permission": "team",
        }
        res: dict[str, Any] = update_canvas_setting(user_auth, setting_payload)
        assert res["code"] == 0, res

    @pytest.mark.p1
    def test_no_update_permission_denies_update_canvas(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_canvas: dict[str, Any],
    ) -> None:
        """Test that user without update permission cannot update canvas."""
        canvas_id: str = team_canvas["id"]
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Ensure update permission is False (default)
        permissions_res: dict[str, Any] = get_user_permissions(web_api_auth, tenant_id, user_id)
        assert permissions_res["code"] == 0
        assert permissions_res["data"]["canvas"]["update"] is False

        # User should not be able to update canvas
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        setting_payload: dict[str, Any] = {
            "id": canvas_id,
            "title": "Updated Title",
            "permission": "team",
        }
        res: dict[str, Any] = update_canvas_setting(user_auth, setting_payload)
        assert res["code"] != 0
        assert "permission" in res["message"].lower() or "update" in res["message"].lower()

    @pytest.mark.p1
    def test_delete_permission_allows_delete_canvas(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_canvas: dict[str, Any],
        clear_canvases: List[str],
    ) -> None:
        """Test that user with delete permission can delete canvas."""
        # Create a new canvas for deletion
        canvas_dsl = {
            "nodes": [{"id": "start", "type": "start", "position": {"x": 100, "y": 100}}],
            "edges": [],
        }
        tenant_id: str = team_with_user["team"]["id"]
        canvas_payload: dict[str, Any] = {
            "title": f"Test Canvas Delete {uuid.uuid4().hex[:8]}",
            "dsl": json.dumps(canvas_dsl),
            "permission": "team",
            "shared_tenant_id": tenant_id,
            "canvas_category": "Agent",
        }
        create_res: dict[str, Any] = create_canvas(web_api_auth, canvas_payload)
        if create_res["code"] != 0:
            pytest.skip(f"Canvas creation failed in setup: {create_res}")
        canvas_id: str = create_res["data"]["id"]
        clear_canvases.append(canvas_id)

        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id = team_with_user["team"]["id"]

        # Grant delete permission
        update_payload: dict[str, Any] = {
            "permissions": {
                "canvas": {"delete": True},
            }
        }
        update_res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert update_res["code"] == 0

        # User should be able to delete canvas
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        delete_payload: dict[str, Any] = {"canvas_ids": [canvas_id]}
        res: dict[str, Any] = delete_canvas(user_auth, delete_payload)
        assert res["code"] == 0, res

    @pytest.mark.p1
    def test_no_delete_permission_denies_delete_canvas(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_canvas: dict[str, Any],
    ) -> None:
        """Test that user without delete permission cannot delete canvas."""
        canvas_id: str = team_canvas["id"]
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Ensure delete permission is False (default)
        permissions_res: dict[str, Any] = get_user_permissions(web_api_auth, tenant_id, user_id)
        assert permissions_res["code"] == 0
        assert permissions_res["data"]["canvas"]["delete"] is False

        # User should not be able to delete canvas
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        delete_payload: dict[str, Any] = {"canvas_ids": [canvas_id]}
        res: dict[str, Any] = delete_canvas(user_auth, delete_payload)
        assert res["code"] != 0
        assert "permission" in res["message"].lower() or "delete" in res["message"].lower()

    @pytest.mark.p1
    def test_read_permission_allows_run_canvas(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_canvas: dict[str, Any],
    ) -> None:
        """Test that user with read permission can run canvas."""
        canvas_id: str = team_canvas["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]

        # User should have read permission by default (allows running)
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        
        run_payload: dict[str, Any] = {
            "id": canvas_id,
            "query": "test query",
        }
        # Note: This might fail for other reasons (missing components, etc.)
        # but should not fail due to permission
        res: dict[str, Any] = run_canvas(user_auth, run_payload)
        # Permission check should pass (code != PERMISSION_ERROR)
        if res["code"] != 0:
            assert "permission" not in res["message"].lower() or "read" not in res["message"].lower()

    @pytest.mark.p1
    def test_read_permission_allows_debug_canvas(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_canvas: dict[str, Any],
    ) -> None:
        """Test that user with read permission can debug canvas."""
        canvas_id: str = team_canvas["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]

        # User should have read permission by default (allows debugging)
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        
        debug_payload: dict[str, Any] = {
            "id": canvas_id,
            "component_id": "start",
            "params": {},
        }
        # Note: This might fail for other reasons (missing components, etc.)
        # but should not fail due to permission
        res: dict[str, Any] = debug_canvas(user_auth, debug_payload)
        # Permission check should pass (code != PERMISSION_ERROR)
        if res["code"] != 0:
            assert "permission" not in res["message"].lower() or "read" not in res["message"].lower()

    @pytest.mark.p1
    def test_update_permission_allows_reset_canvas(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        team_canvas: dict[str, Any],
    ) -> None:
        """Test that user with update permission can reset canvas."""
        canvas_id: str = team_canvas["id"]
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Grant update permission
        update_payload: dict[str, Any] = {
            "permissions": {
                "canvas": {"update": True},
            }
        }
        update_res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert update_res["code"] == 0

        # User should be able to reset canvas
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        reset_payload: dict[str, Any] = {"id": canvas_id}
        # Note: This might fail if there are no versions, but should not fail due to permission
        res: dict[str, Any] = reset_canvas(user_auth, reset_payload)
        # Permission check should pass (code != PERMISSION_ERROR)
        if res["code"] != 0:
            assert "permission" not in res["message"].lower() or "update" not in res["message"].lower()

    @pytest.mark.p1
    def test_create_permission_allows_create_team_canvas(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
        clear_canvases: List[str],
    ) -> None:
        """Test that user with create permission can create team canvas."""
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Grant create permission
        update_payload: dict[str, Any] = {
            "permissions": {
                "canvas": {"create": True},
            }
        }
        update_res: dict[str, Any] = update_user_permissions(web_api_auth, tenant_id, user_id, update_payload)
        assert update_res["code"] == 0

        # User should be able to create team canvas
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        canvas_dsl = {
            "nodes": [{"id": "start", "type": "start", "position": {"x": 100, "y": 100}}],
            "edges": [],
        }
        canvas_payload: dict[str, Any] = {
            "title": f"User Created Canvas {uuid.uuid4().hex[:8]}",
            "dsl": json.dumps(canvas_dsl),
            "permission": "team",
            "shared_tenant_id": tenant_id,
            "canvas_category": "Agent",
        }
        res: dict[str, Any] = create_canvas(user_auth, canvas_payload)
        assert res["code"] == 0, res
        clear_canvases.append(res["data"]["id"])

    @pytest.mark.p1
    def test_no_create_permission_denies_create_team_canvas(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        team_with_user: dict[str, Any],
    ) -> None:
        """Test that user without create permission cannot create team canvas."""
        user_id: str = team_with_user["user"]["id"]
        user_email: str = team_with_user["user"]["email"]
        user_password: str = team_with_user["user"]["password"]
        tenant_id: str = team_with_user["team"]["id"]

        # Ensure create permission is False (default)
        permissions_res: dict[str, Any] = get_user_permissions(web_api_auth, tenant_id, user_id)
        assert permissions_res["code"] == 0
        assert permissions_res["data"]["canvas"]["create"] is False

        # User should not be able to create team canvas
        user_auth: RAGFlowWebApiAuth = login_as_user(user_email, user_password)
        canvas_dsl = {
            "nodes": [{"id": "start", "type": "start", "position": {"x": 100, "y": 100}}],
            "edges": [],
        }
        canvas_payload: dict[str, Any] = {
            "title": f"User Created Canvas {uuid.uuid4().hex[:8]}",
            "dsl": json.dumps(canvas_dsl),
            "permission": "team",
            "shared_tenant_id": tenant_id,
            "canvas_category": "Agent",
        }
        res: dict[str, Any] = create_canvas(user_auth, canvas_payload)
        assert res["code"] != 0
        assert "permission" in res["message"].lower() or "create" in res["message"].lower()

    @pytest.mark.p1
    def test_owner_always_has_full_permissions(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_team: dict[str, Any],
        team_canvas: dict[str, Any],
    ) -> None:
        """Test that team owner always has full permissions regardless of settings."""
        canvas_id: str = team_canvas["id"]
        owner_id: str = test_team["owner_id"]
        tenant_id: str = test_team["id"]

        # Owner should have full permissions
        permissions_res: dict[str, Any] = get_user_permissions(web_api_auth, tenant_id, owner_id)
        assert permissions_res["code"] == 0
        permissions: dict[str, Any] = permissions_res["data"]
        
        assert permissions["canvas"]["create"] is True
        assert permissions["canvas"]["read"] is True
        assert permissions["canvas"]["update"] is True
        assert permissions["canvas"]["delete"] is True

        # Owner should be able to perform all operations
        setting_payload: dict[str, Any] = {
            "id": canvas_id,
            "title": "Owner Updated Title",
            "permission": "team",
        }
        res: dict[str, Any] = update_canvas_setting(web_api_auth, setting_payload)
        assert res["code"] == 0, res

