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

import base64
import os
import uuid
from concurrent.futures import Future, ThreadPoolExecutor, as_completed
from typing import Any

import pytest
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from Cryptodome.PublicKey import RSA

from ..common import create_user
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth, RAGFlowWebApiAuth

# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior during user creation."""

    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "Unauthorized"),
            (
                RAGFlowWebApiAuth(INVALID_API_TOKEN),
                401,
                "Unauthorized",
            ),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        """Test user creation with invalid or missing authentication."""
        # Use unique email to avoid conflicts
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "nickname": "test_user",
            "email": unique_email,
            "password": "test123",
        }
        res: dict[str, Any] = create_user(invalid_auth, payload)
        assert res["code"] == expected_code
        assert expected_message in res["message"]


@pytest.mark.usefixtures("clear_users")
class TestUserCreate:
    """Comprehensive tests for user creation API."""

    @pytest.mark.p1
    @pytest.mark.parametrize(
        ("payload", "expected_code", "expected_message"),
        [
            (
                {
                    "nickname": "valid_user",
                    "email": "valid@example.com",
                    "password": "test123",
                },
                0,
                "",
            ),
            (
                {
                    "nickname": "",
                    "email": "test@example.com",
                    "password": "test123",
                },
                0,
                "",
            ),  # Empty nickname is accepted
            (
                {
                    "nickname": "test_user",
                    "email": "",
                    "password": "test123",
                },
                103,
                "Invalid email address",
            ),
            (
                {
                    "nickname": "test_user",
                    "email": "test@example.com",
                    "password": "",
                },
                101,
                "Password cannot be empty",
            ),
            (
                {"nickname": "test_user", "email": "test@example.com"},
                101,
                "required argument are missing",
            ),
            (
                {
                    "nickname": "test_user",
                    "password": "test123",
                },
                101,
                "required argument are missing",
            ),
            (
                {
                    "email": "test@example.com",
                    "password": "test123",
                },
                101,
                "required argument are missing",
            ),
        ],
    )
    def test_required_fields(
        self,
        web_api_auth,
        payload: dict[str, Any],
        expected_code: int,
        expected_message: str,
    ) -> None:
        """Test user creation with various required field combinations."""
        if payload.get("email") and "@" in payload.get("email", ""):
            # Use unique email to avoid conflicts
            unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
            payload["email"] = unique_email
        res: dict[str, Any] = create_user(web_api_auth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["nickname"] == payload["nickname"]
            assert res["data"]["email"] == payload["email"]
        else:
            assert expected_message in res["message"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        ("email", "expected_code", "expected_message"),
        [
            ("valid@example.com", 0, ""),
            ("user.name@example.com", 0, ""),
            ("user+tag@example.co.uk", 0, ""),
            ("invalid_email", 103, "Invalid email address"),
            ("@example.com", 103, "Invalid email address"),
            ("user@", 103, "Invalid email address"),
            ("user@example", 103, "Invalid email address"),
            ("user@.com", 103, "Invalid email address"),
            ("", 103, "Invalid email address"),
        ],
    )
    def test_email_validation(
        self,
        web_api_auth,
        email: str,
        expected_code: int,
        expected_message: str,
    ) -> None:
        """Test email validation with various email formats."""
        if email and "@" in email and expected_code == 0:
            # Use unique email to avoid conflicts
            email = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "nickname": "test_user",
            "email": email,
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["email"] == email
        else:
            assert expected_message in res["message"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        ("nickname", "expected_code", "expected_message"),
        [
            ("valid_nickname", 0, ""),
            ("user123", 0, ""),
            ("user_name", 0, ""),
            ("User Name", 0, ""),
            ("", 0, ""),  # Empty nickname is accepted by the API
        ],
    )
    def test_nickname(
        self,
        web_api_auth,
        nickname: str,
        expected_code: int,
        expected_message: str,
    ) -> None:
        """Test nickname validation with various nickname formats."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "nickname": nickname,
            "email": unique_email,
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["nickname"] == nickname
        else:
            assert expected_message in res["message"]

    @pytest.mark.p1
    def test_duplicate_email(
        self, web_api_auth
    ) -> None:
        """Test that creating a user with duplicate email fails."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "nickname": "test_user_1",
            "email": unique_email,
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        assert res["code"] == 0

        # Try to create another user with the same email
        payload2: dict[str, str] = {
            "nickname": "test_user_2",
            "email": unique_email,
            "password": "test123",
        }
        res2: dict[str, Any] = create_user(web_api_auth, payload2)
        assert res2["code"] == 103
        assert "has already registered" in res2["message"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        ("is_superuser", "expected_value"),
        [
            (True, True),
            (False, False),
            (None, False),  # Default should be False
        ],
    )
    def test_is_superuser(
        self,
        web_api_auth,
        is_superuser: bool | None,
        expected_value: bool,
    ) -> None:
        """Test is_superuser flag handling during user creation."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, Any] = {
            "nickname": "test_user",
            "email": unique_email,
            "password": "test123",
        }
        if is_superuser is not None:
            payload["is_superuser"] = is_superuser

        res: dict[str, Any] = create_user(web_api_auth, payload)
        assert res["code"] == 0
        assert res["data"]["is_superuser"] == expected_value

    @pytest.mark.p2
    def test_password_hashing(
        self, web_api_auth
    ) -> None:
        """Test that password is properly hashed when stored."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        password: str = "test_password_123"
        payload: dict[str, str] = {
            "nickname": "test_user",
            "email": unique_email,
            "password": password,  # Plain text password
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        assert res["code"] == 0
        # Password should be hashed in the response (not plain text)
        assert "password" in res["data"], (
            f"Password field not found in response: {res['data'].keys()}"
        )
        assert res["data"]["password"].startswith("scrypt:"), (
            f"Password is not hashed: {res['data']['password']}"
        )
        # Verify it's not the plain password
        assert res["data"]["password"] != password

    @pytest.mark.p2
    def test_plain_text_password_accepted(
        self, web_api_auth
    ) -> None:
        """Test that plain text password is accepted."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "nickname": "test_user",
            "email": unique_email,
            "password": "plain_text_password",  # Plain text, no encryption
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        # Should succeed with plain text password
        assert res["code"] == 0
        assert res["data"]["email"] == unique_email

    @pytest.mark.p3
    def test_concurrent_create(
        self, web_api_auth
    ) -> None:
        """Test concurrent user creation with multiple threads."""
        count: int = 10
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures: list[Future[dict[str, Any]]] = []
            for i in range(count):
                unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
                payload: dict[str, str] = {
                    "nickname": f"test_user_{i}",
                    "email": unique_email,
                    "password": "test123",
                }
                futures.append(
                    executor.submit(create_user, web_api_auth, payload)
                )
            responses: list[Future[dict[str, Any]]] = list(
                as_completed(futures)
            )
            assert len(responses) == count, responses
            assert all(
                future.result()["code"] == 0 for future in futures
            )

    @pytest.mark.p2
    def test_user_creation_response_structure(
        self, web_api_auth
    ) -> None:
        """Test that user creation returns the expected response structure."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "nickname": "test_user",
            "email": unique_email,
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        assert res["code"] == 0
        assert "data" in res
        assert "id" in res["data"]
        assert "email" in res["data"]
        assert "nickname" in res["data"]
        assert res["data"]["email"] == unique_email
        assert res["data"]["nickname"] == "test_user"
        assert "User test_user created successfully!" in res["message"]
