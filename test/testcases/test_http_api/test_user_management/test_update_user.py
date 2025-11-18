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
from concurrent.futures import Future, ThreadPoolExecutor, as_completed
from typing import Any

import pytest

from ..common import create_user, update_user
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
# Tests
# ---------------------------------------------------------------------------

@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior during user updates."""

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
        test_user: dict[str, Any],
    ) -> None:
        payload: dict[str, Any] = {
            "user_id": test_user["user_id"],
            "nickname": "updated_nickname",
        }
        res: dict[str, Any] = update_user(invalid_auth, payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]

    @pytest.mark.p1
    def test_user_can_only_update_themselves(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_user: dict[str, Any],
    ) -> None:
        """Test that users can only update their own account."""
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

        # Try to update another user's account (should fail)
        payload: dict[str, Any] = {
            "user_id": other_user_id,
            "nickname": "hacked_nickname",
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 403, f"Expected 403 FORBIDDEN, got {res}"
        assert "only update your own account" in res["message"].lower()

        # Verify the other user's nickname wasn't changed
        # (We can't easily verify this without a get_user endpoint, but the error is sufficient)


@pytest.mark.usefixtures("clear_users")
class TestUserUpdate:
    """Comprehensive tests for user update API."""

    @pytest.mark.p1
    def test_update_with_user_id(
        self, web_api_auth: RAGFlowWebApiAuth, test_user: dict[str, Any]
    ) -> None:
        payload: dict[str, Any] = {
            "user_id": test_user["user_id"],
            "nickname": "updated_nickname",
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["nickname"] == "updated_nickname"
        assert res["data"]["email"] == test_user["email"]
        assert "updated successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_update_with_email(
        self, web_api_auth: RAGFlowWebApiAuth, test_user: dict[str, Any]
    ) -> None:
        payload: dict[str, Any] = {
            "email": test_user["email"],
            "nickname": "updated_nickname_email",
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["nickname"] == "updated_nickname_email"
        assert res["data"]["email"] == test_user["email"]

    @pytest.mark.p1
    def test_update_missing_identifier(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test update without user_id or email."""
        payload: dict[str, str] = {"nickname": "updated_nickname"}
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 101
        assert "Either user_id or email must be provided" in res["message"]

    @pytest.mark.p1
    def test_update_user_not_found_by_id(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        payload: dict[str, str] = {
            "user_id": "non_existent_user_id_12345",
            "nickname": "updated_nickname",
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 102
        assert "User not found" in res["message"]

    @pytest.mark.p1
    def test_update_user_not_found_by_email(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        payload: dict[str, str] = {
            "email": "nonexistent@example.com",
            "nickname": "updated_nickname",
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 102
        assert "not found" in res["message"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        ("nickname", "expected_code", "expected_message"),
        [
            ("valid_nickname", 0, ""),
            ("user123", 0, ""),
            ("user_name", 0, ""),
            ("User Name", 0, ""),
            ("", 0, ""),  # Empty nickname accepted
        ],
    )
    def test_update_nickname(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_user: dict[str, Any],
        nickname: str,
        expected_code: int,
        expected_message: str,
    ) -> None:
        payload: dict[str, str] = {
            "user_id": test_user["user_id"],
            "nickname": nickname,
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["nickname"] == nickname
        else:
            assert expected_message in res["message"]

    @pytest.mark.p1
    def test_update_password(
        self, web_api_auth: RAGFlowWebApiAuth, test_user: dict[str, Any]
    ) -> None:
        new_password: str = "new_password_456"
        payload: dict[str, str] = {
            "user_id": test_user["user_id"],
            "password": encrypt_password(new_password),
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 0, res
        assert "updated successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_update_password_invalid_encryption(
        self, web_api_auth: RAGFlowWebApiAuth, test_user: dict[str, Any]
    ) -> None:
        payload: dict[str, str] = {
            "user_id": test_user["user_id"],
            "password": "plain_text_password",
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 500
        assert "Fail to decrypt password" in res["message"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        ("new_email", "expected_code", "expected_message"),
        [
            ("valid@example.com", 0, ""),
            ("user.name@example.com", 0, ""),
            ("user+tag@example.co.uk", 0, ""),
            ("invalid_email", 103, "Invalid email address"),
            ("@example.com", 103, "Invalid email address"),
            ("user@", 103, "Invalid email address"),
            ("user@example", 103, "Invalid email address"),
            ("user@.com", 103, "Invalid email address"),
        ],
    )
    def test_update_email(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_user: dict[str, Any],
        new_email: str,
        expected_code: int,
        expected_message: str,
    ) -> None:
        if "@" in new_email and expected_code == 0:
            new_email = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "user_id": test_user["user_id"],
            "new_email": new_email,
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["email"] == new_email
        else:
            assert expected_message in res["message"]

    @pytest.mark.p1
    def test_update_email_duplicate(
        self, web_api_auth: RAGFlowWebApiAuth, test_user: dict[str, Any]
    ) -> None:
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "another_user",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(web_api_auth, create_payload)
        assert create_res["code"] == 0

        update_payload: dict[str, str] = {
            "user_id": test_user["user_id"],
            "new_email": unique_email,
        }
        res: dict[str, Any] = update_user(web_api_auth, update_payload)
        assert res["code"] == 103
        assert "already in use" in res["message"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        ("is_superuser", "expected_value"),
        [(True, True), (False, False)],
    )
    def test_update_is_superuser(
        self,
        web_api_auth: RAGFlowWebApiAuth,
        test_user: dict[str, Any],
        is_superuser: bool,
        expected_value: bool,
    ) -> None:
        payload: dict[str, Any] = {
            "user_id": test_user["user_id"],
            "is_superuser": is_superuser,
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["is_superuser"] is expected_value

    @pytest.mark.p1
    def test_update_multiple_fields(
        self, web_api_auth: RAGFlowWebApiAuth, test_user: dict[str, Any]
    ) -> None:
        new_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, Any] = {
            "user_id": test_user["user_id"],
            "nickname": "updated_multiple",
            "new_email": new_email,
            "is_superuser": True,
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["nickname"] == "updated_multiple"
        assert res["data"]["email"] == new_email
        assert res["data"]["is_superuser"] is True

    @pytest.mark.p1
    def test_update_no_fields(
        self, web_api_auth: RAGFlowWebApiAuth, test_user: dict[str, Any]
    ) -> None:
        payload: dict[str, str] = {"user_id": test_user["user_id"]}
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 101
        assert "No valid fields to update" in res["message"]

    @pytest.mark.p1
    def test_update_email_using_email_field_when_user_id_provided(
        self, web_api_auth: RAGFlowWebApiAuth, test_user: dict[str, Any]
    ) -> None:
        new_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "user_id": test_user["user_id"],
            "email": new_email,
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 0, res
        assert res["data"]["email"] == new_email

    @pytest.mark.p2
    def test_update_response_structure(
        self, web_api_auth: RAGFlowWebApiAuth, test_user: dict[str, Any]
    ) -> None:
        payload: dict[str, Any] = {
            "user_id": test_user["user_id"],
            "nickname": "response_test",
        }
        res: dict[str, Any] = update_user(web_api_auth, payload)
        assert res["code"] == 0
        assert set(("id", "email", "nickname")) <= res["data"].keys()
        assert res["data"]["nickname"] == "response_test"
        assert "updated successfully" in res["message"].lower()

    @pytest.mark.p2
    def test_concurrent_updates(
        self, web_api_auth: RAGFlowWebApiAuth, test_user: dict[str, Any]
    ) -> None:
        """Test concurrent updates to the same user (users can only update themselves)."""
        # Test concurrent updates to the authenticated user's own account
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures: list[Future[dict[str, Any]]] = [
                executor.submit(
                    update_user,
                    web_api_auth,
                    {
                        "user_id": test_user["user_id"],
                        "nickname": f"updated_user_{i}",
                    },
                )
                for i in range(5)
            ]

            for future in as_completed(futures):
                res: dict[str, Any] = future.result()
                assert res["code"] == 0

    @pytest.mark.p3
    def test_update_same_user_multiple_times(
        self, web_api_auth: RAGFlowWebApiAuth, test_user: dict[str, Any]
    ) -> None:
        """Test repeated updates on the same user."""
        for nickname in (
            "first_update",
            "second_update",
            "third_update",
        ):
            payload: dict[str, str] = {
                "user_id": test_user["user_id"],
                "nickname": nickname,
            }
            res: dict[str, Any] = update_user(web_api_auth, payload)
            assert res["code"] == 0
            assert res["data"]["nickname"] == nickname
