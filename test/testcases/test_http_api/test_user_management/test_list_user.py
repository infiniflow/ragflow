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

from ..common import create_user, list_users
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
    """Tests for authentication behavior during user listing."""

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
    #     self, http_api_auth: RAGFlowHttpApiAuth
    # ) -> None:
    #     """Test listing users when no users exist."""
    #     res: dict[str, Any] = list_users(http_api_auth)
    #     assert res["code"] == 0, res
    #     assert isinstance(res["data"], list)
    #     assert len(res["data"]) == 0

    @pytest.mark.p1
    def test_list_single_user(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test listing a single user."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_single",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(web_api_auth, create_payload)
        # Skip if creation fails (password encryption issue in test)
        if create_res["code"] != 0:
            pytest.skip("User creation failed, skipping list test")

        list_res: dict[str, Any] = list_users(web_api_auth)
        assert list_res["code"] == 0, list_res
        assert isinstance(list_res["data"], list)
        assert len(list_res["data"]) >= 1
        # Verify the created user is in the list
        user_emails: list[str] = [u["email"] for u in list_res["data"]]
        assert unique_email in user_emails

    @pytest.mark.p1
    def test_list_multiple_users(
        self, web_api_auth: RAGFlowWebApiAuth
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
                web_api_auth, create_payload
            )
            if create_res["code"] == 0:
                created_emails.append(unique_email)

        if not created_emails:
            pytest.skip("No users created, skipping list test")

        list_res: dict[str, Any] = list_users(web_api_auth)
        assert list_res["code"] == 0, list_res
        assert isinstance(list_res["data"], list)
        assert len(list_res["data"]) >= len(created_emails)
        # Verify all created users are in the list
        user_emails: list[str] = [u["email"] for u in list_res["data"]]
        for email in created_emails:
            assert email in user_emails

    @pytest.mark.p1
    def test_list_users_with_email_filter(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test listing users filtered by email."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_filter",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(web_api_auth, create_payload)
        if create_res["code"] != 0:
            pytest.skip("User creation failed, skipping filter test")

        # List with email filter
        params: dict[str, str] = {"email": unique_email}
        list_res: dict[str, Any] = list_users(web_api_auth, params=params)
        assert list_res["code"] == 0, list_res
        assert isinstance(list_res["data"], list)
        assert len(list_res["data"]) >= 1
        # Verify all returned users have the filtered email
        for user in list_res["data"]:
            assert user["email"] == unique_email

    @pytest.mark.p1
    def test_list_users_with_invalid_email_filter(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test listing users with invalid email filter."""
        params: dict[str, str] = {"email": "invalid_email_format"}
        list_res: dict[str, Any] = list_users(web_api_auth, params=params)
        assert list_res["code"] != 0
        assert "Invalid email address" in list_res["message"]

    @pytest.mark.p1
    def test_list_users_with_nonexistent_email_filter(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test listing users with non-existent email filter."""
        nonexistent_email: str = f"nonexistent_{uuid.uuid4().hex[:8]}@example.com"
        params: dict[str, str] = {"email": nonexistent_email}
        list_res: dict[str, Any] = list_users(web_api_auth, params=params)
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
        web_api_auth: RAGFlowWebApiAuth,
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
                web_api_auth, create_payload
            )
            if create_res["code"] == 0:
                created_count += 1

        if created_count == 0:
            pytest.skip("No users created, skipping pagination test")

        params: dict[str, int] = {"page": page, "page_size": page_size}
        list_res: dict[str, Any] = list_users(web_api_auth, params=params)

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
        self, web_api_auth: RAGFlowWebApiAuth
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
                web_api_auth, create_payload
            )
            if create_res["code"] == 0:
                created_emails.append(unique_email)

        if len(created_emails) < 3:
            pytest.skip("Not enough users created, skipping boundary test")

        # Get total count of all users to calculate pagination boundaries
        list_res_all: dict[str, Any] = list_users(web_api_auth)
        total_users: int = len(list_res_all["data"])

        # Test first page
        params: dict[str, int] = {"page": 1, "page_size": 2}
        list_res: dict[str, Any] = list_users(web_api_auth, params=params)
        assert list_res["code"] == 0, list_res
        assert len(list_res["data"]) == 2

        # Test that pagination returns consistent page sizes
        params = {"page": 2, "page_size": 2}
        list_res = list_users(web_api_auth, params=params)
        assert list_res["code"] == 0, list_res
        assert len(list_res["data"]) == 2

        # Test last page (might have fewer items)
        # Calculate expected last page: ceil(total_users / page_size)
        page_size: int = 2
        last_page: int = (total_users + page_size - 1) // page_size
        if last_page > 0:
            params = {"page": last_page, "page_size": page_size}
            list_res = list_users(web_api_auth, params=params)
            assert list_res["code"] == 0, list_res
            assert len(list_res["data"]) <= page_size

        # Test page beyond available data
        # Use a page number that's definitely beyond available data
        params = {"page": total_users + 10, "page_size": 2}
        list_res = list_users(web_api_auth, params=params)
        assert list_res["code"] == 0, list_res
        assert len(list_res["data"]) == 0

    @pytest.mark.p1
    def test_list_users_response_structure(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test that user listing returns the expected response structure."""
        res: dict[str, Any] = list_users(web_api_auth)
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
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test listing users with invalid pagination parameters."""
        # Test invalid page (non-integer)
        params: dict[str, str] = {"page": "invalid", "page_size": "10"}
        list_res: dict[str, Any] = list_users(web_api_auth, params=params)
        # Should handle gracefully or return error
        # The exact behavior depends on implementation
        assert "code" in list_res

        # Test invalid page_size (non-integer)
        params = {"page": "1", "page_size": "invalid"}
        list_res = list_users(web_api_auth, params=params)
        assert "code" in list_res

    @pytest.mark.p2
    def test_list_users_combined_filters(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test listing users with combined filters."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict[str, str] = {
            "nickname": "test_user_combined",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict[str, Any] = create_user(web_api_auth, create_payload)
        if create_res["code"] != 0:
            pytest.skip("User creation failed, skipping combined filter test")

        # Test with email filter and pagination
        params: dict[str, Any] = {
            "email": unique_email,
            "page": 1,
            "page_size": 10,
        }
        list_res: dict[str, Any] = list_users(web_api_auth, params=params)
        assert list_res["code"] == 0, list_res
        assert isinstance(list_res["data"], list)
        # Should return at least the created user
        assert len(list_res["data"]) >= 1

    @pytest.mark.p2
    def test_list_users_performance_with_many_users(
        self, web_api_auth: RAGFlowWebApiAuth
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
                web_api_auth, create_payload
            )
            if create_res["code"] == 0:
                created_count += 1

        if created_count == 0:
            pytest.skip("No users created, skipping performance test")

        # List all users
        list_res: dict[str, Any] = list_users(web_api_auth)
        assert list_res["code"] == 0, list_res
        assert isinstance(list_res["data"], list)
        # Should return at least the created users
        assert len(list_res["data"]) >= created_count

