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

from ..common import create_user, delete_user
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth, RAGFlowWebApiAuth

# Import from conftest - load it directly to avoid import issues
import importlib.util
from pathlib import Path

_conftest_path = Path(__file__).parent / "conftest.py"
spec = importlib.util.spec_from_file_location("conftest", _conftest_path)
conftest_module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(conftest_module)
encrypt_password = conftest_module.encrypt_password


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior during user deletion."""

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
        """Test user deletion with invalid or missing authentication."""
        # Create a test user first
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_delete_auth",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(web_api_auth, create_payload)
        if create_res["code"] != 0:
            pytest.skip("User creation failed, skipping auth test")

        user_id: str = create_res["data"]["id"]

        # Try to delete with invalid auth
        delete_payload: dict[str, str] = {"user_id": user_id}
        res: dict[str, Any] = delete_user(invalid_auth, delete_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]

    @pytest.mark.p1
    def test_user_can_only_delete_themselves(
        self,
        web_api_auth: RAGFlowWebApiAuth,
    ) -> None:
        """Test that users can only delete their own account."""
        # Create another user
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "another_user",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(web_api_auth, create_payload)
        assert create_res["code"] == 0, "Failed to create second user"
        other_user_id: str = create_res["data"]["id"]

        # Try to delete another user's account (should fail)
        delete_payload: dict[str, Any] = {
            "user_id": other_user_id,
        }
        res: dict[str, Any] = delete_user(web_api_auth, delete_payload)
        assert res["code"] == 403, f"Expected 403 FORBIDDEN, got {res}"
        assert "only delete your own account" in res["message"].lower()


@pytest.mark.usefixtures("clear_users")
class TestUserDelete:
    """Comprehensive tests for user deletion API."""

    @pytest.mark.p1
    def test_delete_user_missing_identifier(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test deletion without user_id or email."""
        delete_payload: dict[str, str] = {}
        res: dict[str, Any] = delete_user(web_api_auth, delete_payload)
        assert res["code"] == 101
        assert "Either user_id or email must be provided" in res["message"]

    @pytest.mark.p1
    def test_delete_user_not_found_by_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test deletion of non-existent user by ID."""
        delete_payload: dict[str, str] = {
            "user_id": "non_existent_user_id_12345",
        }
        res: dict[str, Any] = delete_user(web_api_auth, delete_payload)
        assert res["code"] == 102
        assert "User not found" in res["message"]

    @pytest.mark.p1
    def test_delete_user_invalid_email_format(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test deletion with invalid email format."""
        delete_payload: dict[str, str] = {"email": "invalid_email_format"}
        res: dict[str, Any] = delete_user(web_api_auth, delete_payload)
        assert res["code"] == 103
        assert "Invalid email address" in res["message"]

    @pytest.mark.p3
    def test_delete_user_idempotency(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test that deleting a non-existent user returns consistent error."""
        non_existent_id: str = f"nonexistent_{uuid.uuid4().hex[:16]}"

        # First attempt
        delete_payload: dict[str, str] = {"user_id": non_existent_id}
        res1: dict[str, Any] = delete_user(web_api_auth, delete_payload)
        assert res1["code"] == 102

        # Second attempt (should return same error)
        res2: dict[str, Any] = delete_user(web_api_auth, delete_payload)
        assert res2["code"] == 102
        assert res1["code"] == res2["code"]

