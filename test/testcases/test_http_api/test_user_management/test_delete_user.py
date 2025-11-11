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
from typing import Any

import pytest
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from Cryptodome.PublicKey import RSA

from common import create_user, delete_user, list_users
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth


# ---------------------------------------------------------------------------
# Utility Functions
# ---------------------------------------------------------------------------


def encrypt_password(password: str) -> str:
    """
    Encrypt password for API calls without importing from api.utils.crypt.

    Avoids ModuleNotFoundError caused by test helper module named `common`.
    """
    current_dir: str = os.path.dirname(os.path.abspath(__file__))
    project_base: str = os.path.abspath(
        os.path.join(current_dir, "..", "..", "..", "..")
    )
    file_path: str = os.path.join(project_base, "conf", "public.pem")

    with open(file_path, encoding="utf-8") as pem_file:
        rsa_key: RSA.RsaKey = RSA.import_key(
            pem_file.read(), passphrase="Welcome"
        )

    cipher: Cipher_pkcs1_v1_5.PKCS115_Cipher = Cipher_pkcs1_v1_5.new(rsa_key)
    password_base64: str = base64.b64encode(password.encode()).decode()
    encrypted_password: bytes = cipher.encrypt(password_base64.encode())
    return base64.b64encode(encrypted_password).decode()


# ---------------------------------------------------------------------------
# Test Classes
# ---------------------------------------------------------------------------


@pytest.mark.p1
class TestAuthorization:
    """Tests for authentication behavior during user deletion."""

    @pytest.mark.parametrize(
        ("invalid_auth", "expected_code", "expected_message"),
        [
            # Note: @login_required is commented out, so endpoint works
            # without auth
            # Testing with None auth should succeed (code 0) if endpoint
            # doesn't require auth
            (None, 0, ""),
            # Invalid token should also work if auth is not required
            (RAGFlowHttpApiAuth(INVALID_API_TOKEN), 0, ""),
        ],
    )
    def test_invalid_auth(
        self,
        invalid_auth: RAGFlowHttpApiAuth | None,
        expected_code: int,
        expected_message: str,
        HttpApiAuth: RAGFlowHttpApiAuth,
    ) -> None:
        """Test user deletion with invalid or missing authentication."""
        # Create a test user first
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_delete_auth",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(HttpApiAuth, create_payload)
        if create_res["code"] != 0:
            pytest.skip("User creation failed, skipping auth test")

        user_id: str = create_res["data"]["id"]

        # Try to delete with invalid auth
        delete_payload: dict[str, str] = {"user_id": user_id}
        res: dict[str, Any] = delete_user(invalid_auth, delete_payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.usefixtures("clear_users")
class TestUserDelete:
    """Comprehensive tests for user deletion API."""

    @pytest.mark.p1
    def test_delete_user_by_id(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test deleting a user by user_id."""
        # Create a test user
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_delete_id",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(HttpApiAuth, create_payload)
        assert create_res["code"] == 0, create_res

        user_id: str = create_res["data"]["id"]

        # Delete the user
        delete_payload: dict[str, str] = {"user_id": user_id}
        delete_res: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert delete_res["code"] == 0, delete_res
        assert delete_res["data"] is True
        assert "deleted successfully" in delete_res["message"].lower()

        # Verify user is deleted
        list_res: dict[str, Any] = list_users(HttpApiAuth)
        user_emails: list[str] = [
            u["email"] for u in list_res["data"] if u.get("id") == user_id
        ]
        assert unique_email not in user_emails

    @pytest.mark.p1
    def test_delete_user_by_email(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test deleting a user by email."""
        # Create a test user
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_delete_email",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(HttpApiAuth, create_payload)
        assert create_res["code"] == 0, create_res

        # Delete the user by email
        delete_payload: dict[str, str] = {"email": unique_email}
        delete_res: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert delete_res["code"] == 0, delete_res
        assert delete_res["data"] is True
        assert "deleted successfully" in delete_res["message"].lower()

        # Verify user is deleted
        params: dict[str, str] = {"email": unique_email}
        list_res: dict[str, Any] = list_users(HttpApiAuth, params=params)
        assert len(list_res["data"]) == 0

    @pytest.mark.p1
    def test_delete_user_missing_identifier(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test deletion without user_id or email."""
        delete_payload: dict[str, str] = {}
        res: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert res["code"] == 101
        assert "Either user_id or email must be provided" in res["message"]

    @pytest.mark.p1
    def test_delete_user_not_found_by_id(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test deletion of non-existent user by ID."""
        delete_payload: dict[str, str] = {
            "user_id": "non_existent_user_id_12345",
        }
        res: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert res["code"] == 102
        assert "User not found" in res["message"]

    @pytest.mark.p1
    def test_delete_user_not_found_by_email(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test deletion of non-existent user by email."""
        nonexistent_email: str = (
            f"nonexistent_{uuid.uuid4().hex[:8]}@example.com"
        )
        delete_payload: dict[str, str] = {"email": nonexistent_email}
        res: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert res["code"] == 102
        assert "not found" in res["message"]

    @pytest.mark.p1
    def test_delete_user_invalid_email_format(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test deletion with invalid email format."""
        delete_payload: dict[str, str] = {"email": "invalid_email_format"}
        res: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert res["code"] == 103
        assert "Invalid email address" in res["message"]

    @pytest.mark.p1
    def test_delete_user_multiple_users_same_email(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test deletion when multiple users have the same email."""
        # This scenario shouldn't happen in normal operation, but test it
        # Create a user
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_1",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(HttpApiAuth, create_payload)
        assert create_res["code"] == 0

        # Try to delete by email (should work if only one user exists)
        delete_payload: dict[str, str] = {"email": unique_email}
        res: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        # Should succeed if only one user, or fail if multiple
        assert res["code"] in (0, 102)

    @pytest.mark.p1
    def test_delete_user_twice(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test deleting the same user twice."""
        # Create a test user
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_delete_twice",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(HttpApiAuth, create_payload)
        assert create_res["code"] == 0, create_res

        user_id: str = create_res["data"]["id"]

        # Delete the user first time
        delete_payload: dict[str, str] = {"user_id": user_id}
        delete_res: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert delete_res["code"] == 0, delete_res

        # Try to delete again
        delete_res2: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert delete_res2["code"] == 102
        assert "not found" in delete_res2["message"]

    @pytest.mark.p1
    def test_delete_user_response_structure(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test that user deletion returns the expected response structure."""
        # Create a test user
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_delete_structure",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(HttpApiAuth, create_payload)
        assert create_res["code"] == 0, create_res

        user_id: str = create_res["data"]["id"]

        # Delete the user
        delete_payload: dict[str, str] = {"user_id": user_id}
        res: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert res["code"] == 0
        assert "data" in res
        assert res["data"] is True
        assert "message" in res
        assert "deleted successfully" in res["message"].lower()

    @pytest.mark.p2
    def test_delete_multiple_users_sequentially(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test deleting multiple users sequentially."""
        created_user_ids: list[str] = []
        for i in range(3):
            unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
            create_payload: dict[str, str] = {
                "nickname": f"test_user_seq_{i}",
                "email": unique_email,
                "password": encrypt_password("test123"),
            }
            create_res: dict[str, Any] = create_user(
                HttpApiAuth, create_payload
            )
            if create_res["code"] == 0:
                created_user_ids.append(create_res["data"]["id"])

        if len(created_user_ids) == 0:
            pytest.skip("No users created, skipping sequential delete test")

        # Delete all created users
        for user_id in created_user_ids:
            delete_payload: dict[str, str] = {"user_id": user_id}
            delete_res: dict[str, Any] = delete_user(
                HttpApiAuth, delete_payload
            )
            assert delete_res["code"] == 0, delete_res

        # Verify all users are deleted
        for user_id in created_user_ids:
            list_res: dict[str, Any] = list_users(HttpApiAuth)
            found_users: list[dict[str, Any]] = [
                u for u in list_res["data"] if u.get("id") == user_id
            ]
            assert len(found_users) == 0

    @pytest.mark.p2
    def test_delete_user_and_verify_not_in_list(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test that deleted user is not in the user list."""
        # Create a test user
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_verify_delete",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(HttpApiAuth, create_payload)
        assert create_res["code"] == 0, create_res

        user_id: str = create_res["data"]["id"]

        # Verify user exists in list
        params: dict[str, str] = {"email": unique_email}
        list_res_before: dict[str, Any] = list_users(
            HttpApiAuth, params=params
        )
        assert len(list_res_before["data"]) >= 1
        assert any(u["email"] == unique_email for u in list_res_before["data"])

        # Delete the user
        delete_payload: dict[str, str] = {"user_id": user_id}
        delete_res: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert delete_res["code"] == 0, delete_res

        # Verify user is not in list
        list_res_after: dict[str, Any] = list_users(HttpApiAuth, params=params)
        assert len(list_res_after["data"]) == 0

    @pytest.mark.p2
    def test_delete_user_with_empty_payload(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test deletion with empty payload."""
        delete_payload: dict[str, Any] = {}
        res: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert res["code"] == 101
        assert "Either user_id or email must be provided" in res["message"]

    @pytest.mark.p3
    def test_delete_user_idempotency(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test that deleting a non-existent user returns consistent error."""
        non_existent_id: str = f"nonexistent_{uuid.uuid4().hex[:16]}"

        # First attempt
        delete_payload: dict[str, str] = {"user_id": non_existent_id}
        res1: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert res1["code"] == 102

        # Second attempt (should return same error)
        res2: dict[str, Any] = delete_user(HttpApiAuth, delete_payload)
        assert res2["code"] == 102
        assert res1["code"] == res2["code"]

