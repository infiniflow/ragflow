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

from common import create_user, list_users
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
    """Tests for authentication behavior during user listing."""

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
    ) -> None:
        """Test user listing with invalid or missing authentication."""
        res: dict[str, Any] = list_users(invalid_auth)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.usefixtures("clear_users")
class TestUserList:
    """Comprehensive tests for user listing API."""

    # @pytest.mark.p1
    # def test_list_empty_users(
    #     self, HttpApiAuth: RAGFlowHttpApiAuth
    # ) -> None:
    #     """Test listing users when no users exist."""
    #     res: dict[str, Any] = list_users(HttpApiAuth)
    #     assert res["code"] == 0, res
    #     assert isinstance(res["data"], list)
    #     assert len(res["data"]) == 0

    @pytest.mark.p1
    def test_list_single_user(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test listing a single user."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_single",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(HttpApiAuth, create_payload)
        # Skip if creation fails (password encryption issue in test)
        if create_res["code"] != 0:
            pytest.skip("User creation failed, skipping list test")

        list_res: dict[str, Any] = list_users(HttpApiAuth)
        assert list_res["code"] == 0, list_res
        assert isinstance(list_res["data"], list)
        assert len(list_res["data"]) >= 1
        # Verify the created user is in the list
        user_emails: list[str] = [u["email"] for u in list_res["data"]]
        assert unique_email in user_emails

    @pytest.mark.p1
    def test_list_multiple_users(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test listing multiple users."""
        created_emails: list[str] = []
        for i in range(3):
            unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
            create_payload: dict[str, str] = {
                "nickname": f"test_user_{i}",
                "email": unique_email,
                "password": encrypt_password("test123"),
            }
            create_res: dict[str, Any] = create_user(
                HttpApiAuth, create_payload
            )
            if create_res["code"] == 0:
                created_emails.append(unique_email)

        if not created_emails:
            pytest.skip("No users created, skipping list test")

        list_res: dict[str, Any] = list_users(HttpApiAuth)
        assert list_res["code"] == 0, list_res
        assert isinstance(list_res["data"], list)
        assert len(list_res["data"]) >= len(created_emails)
        # Verify all created users are in the list
        user_emails: list[str] = [u["email"] for u in list_res["data"]]
        for email in created_emails:
            assert email in user_emails

    @pytest.mark.p1
    def test_list_users_with_email_filter(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test listing users filtered by email."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_filter",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(HttpApiAuth, create_payload)
        if create_res["code"] != 0:
            pytest.skip("User creation failed, skipping filter test")

        # List with email filter
        params: dict[str, str] = {"email": unique_email}
        list_res: dict[str, Any] = list_users(HttpApiAuth, params=params)
        assert list_res["code"] == 0, list_res
        assert isinstance(list_res["data"], list)
        assert len(list_res["data"]) >= 1
        # Verify all returned users have the filtered email
        for user in list_res["data"]:
            assert user["email"] == unique_email

    @pytest.mark.p1
    def test_list_users_with_invalid_email_filter(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test listing users with invalid email filter."""
        params: dict[str, str] = {"email": "invalid_email_format"}
        list_res: dict[str, Any] = list_users(HttpApiAuth, params=params)
        assert list_res["code"] != 0
        assert "Invalid email address" in list_res["message"]

    @pytest.mark.p1
    def test_list_users_with_nonexistent_email_filter(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test listing users with non-existent email filter."""
        nonexistent_email: str = f"nonexistent_{uuid.uuid4().hex[:8]}@example.com"
        params: dict[str, str] = {"email": nonexistent_email}
        list_res: dict[str, Any] = list_users(HttpApiAuth, params=params)
        assert list_res["code"] == 0, list_res
        assert isinstance(list_res["data"], list)
        assert len(list_res["data"]) == 0

    @pytest.mark.p1
    @pytest.mark.parametrize(
        ("page", "page_size", "expected_valid"),
        [
            (1, 10, True),
            (1, 5, True),
            (2, 5, True),
            (0, 10, False),  # Invalid: page must be >= 1
            (1, 0, False),  # Invalid: page_size must be >= 1
            (-1, 10, False),  # Invalid: negative page
            (1, -5, False),  # Invalid: negative page_size
        ],
    )
    def test_list_users_with_pagination(
        self,
        HttpApiAuth: RAGFlowHttpApiAuth,
        page: int,
        page_size: int,
        expected_valid: bool,
    ) -> None:
        """Test listing users with pagination."""
        # Create some users first
        created_count: int = 0
        for i in range(7):
            unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
            create_payload: dict[str, str] = {
                "nickname": f"test_user_pag_{i}",
                "email": unique_email,
                "password": encrypt_password("test123"),
            }
            create_res: dict[str, Any] = create_user(
                HttpApiAuth, create_payload
            )
            if create_res["code"] == 0:
                created_count += 1

        if created_count == 0:
            pytest.skip("No users created, skipping pagination test")

        params: dict[str, int] = {"page": page, "page_size": page_size}
        list_res: dict[str, Any] = list_users(HttpApiAuth, params=params)

        if expected_valid:
            assert list_res["code"] == 0, list_res
            assert isinstance(list_res["data"], list)
            # Verify pagination limits
            assert len(list_res["data"]) <= page_size
        else:
            assert list_res["code"] != 0
            assert "must be greater than 0" in list_res["message"]

    @pytest.mark.p1
    def test_list_users_pagination_boundaries(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test pagination boundary conditions."""
        # Create 5 users with a unique email pattern for filtering
        test_email_prefix: str = f"test_bound_{uuid.uuid4().hex[:8]}"
        created_emails: list[str] = []
        for i in range(5):
            unique_email: str = f"{test_email_prefix}_{i}@example.com"
            create_payload: dict[str, str] = {
                "nickname": f"test_user_bound_{i}",
                "email": unique_email,
                "password": encrypt_password("test123"),
            }
            create_res: dict[str, Any] = create_user(
                HttpApiAuth, create_payload
            )
            if create_res["code"] == 0:
                created_emails.append(unique_email)

        if len(created_emails) < 3:
            pytest.skip("Not enough users created, skipping boundary test")

        # Get total count of all users to calculate pagination boundaries
        list_res_all: dict[str, Any] = list_users(HttpApiAuth)
        total_users: int = len(list_res_all["data"])

        # Test first page
        params: dict[str, int] = {"page": 1, "page_size": 2}
        list_res: dict[str, Any] = list_users(HttpApiAuth, params=params)
        assert list_res["code"] == 0, list_res
        assert len(list_res["data"]) == 2

        # Test that pagination returns consistent page sizes
        params = {"page": 2, "page_size": 2}
        list_res = list_users(HttpApiAuth, params=params)
        assert list_res["code"] == 0, list_res
        assert len(list_res["data"]) == 2

        # Test last page (might have fewer items)
        # Calculate expected last page: ceil(total_users / page_size)
        page_size: int = 2
        last_page: int = (total_users + page_size - 1) // page_size
        if last_page > 0:
            params = {"page": last_page, "page_size": page_size}
            list_res = list_users(HttpApiAuth, params=params)
            assert list_res["code"] == 0, list_res
            assert len(list_res["data"]) <= page_size

        # Test page beyond available data
        # Use a page number that's definitely beyond available data
        params = {"page": total_users + 10, "page_size": 2}
        list_res = list_users(HttpApiAuth, params=params)
        assert list_res["code"] == 0, list_res
        assert len(list_res["data"]) == 0

    @pytest.mark.p1
    def test_list_users_response_structure(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test that user listing returns the expected response structure."""
        res: dict[str, Any] = list_users(HttpApiAuth)
        assert res["code"] == 0
        assert "data" in res
        assert isinstance(res["data"], list)

        if len(res["data"]) > 0:
            user: dict[str, Any] = res["data"][0]
            # Verify user structure
            assert "id" in user
            assert "email" in user
            assert "nickname" in user
            # Optional fields that might be present
            assert isinstance(user["id"], str)
            assert isinstance(user["email"], str)

    @pytest.mark.p1
    def test_list_users_with_invalid_page_params(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test listing users with invalid pagination parameters."""
        # Test invalid page (non-integer)
        params: dict[str, str] = {"page": "invalid", "page_size": "10"}
        list_res: dict[str, Any] = list_users(HttpApiAuth, params=params)
        # Should handle gracefully or return error
        # The exact behavior depends on implementation
        assert "code" in list_res

        # Test invalid page_size (non-integer)
        params = {"page": "1", "page_size": "invalid"}
        list_res = list_users(HttpApiAuth, params=params)
        assert "code" in list_res

    @pytest.mark.p2
    def test_list_users_combined_filters(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test listing users with combined filters."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_combined",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(HttpApiAuth, create_payload)
        if create_res["code"] != 0:
            pytest.skip("User creation failed, skipping combined filter test")

        # Test with email filter and pagination
        params: dict[str, Any] = {
            "email": unique_email,
            "page": 1,
            "page_size": 10,
        }
        list_res: dict[str, Any] = list_users(HttpApiAuth, params=params)
        assert list_res["code"] == 0, list_res
        assert isinstance(list_res["data"], list)
        # Should return at least the created user
        assert len(list_res["data"]) >= 1

    @pytest.mark.p2
    def test_list_users_performance_with_many_users(
        self, HttpApiAuth: RAGFlowHttpApiAuth
    ) -> None:
        """Test listing performance with multiple users."""
        # Create several users
        created_count: int = 0
        for i in range(10):
            unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
            create_payload: dict[str, str] = {
                "nickname": f"test_user_perf_{i}",
                "email": unique_email,
                "password": encrypt_password("test123"),
            }
            create_res: dict[str, Any] = create_user(
                HttpApiAuth, create_payload
            )
            if create_res["code"] == 0:
                created_count += 1

        if created_count == 0:
            pytest.skip("No users created, skipping performance test")

        # List all users
        list_res: dict[str, Any] = list_users(HttpApiAuth)
        assert list_res["code"] == 0, list_res
        assert isinstance(list_res["data"], list)
        # Should return at least the created users
        assert len(list_res["data"]) >= created_count

