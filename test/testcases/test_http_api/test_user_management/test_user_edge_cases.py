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
"""Edge case tests for user management APIs."""

from __future__ import annotations

import uuid
from typing import Any

import pytest

from ..common import create_user, list_users
from libs.auth import RAGFlowWebApiAuth


@pytest.mark.edge_cases
@pytest.mark.usefixtures("clear_users")
class TestUserEdgeCases:
    """Edge case tests for user management."""

    @pytest.mark.p2
    def test_extremely_long_nickname(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test nickname with extreme length."""
        long_nickname: str = "A" * 1000
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "nickname": long_nickname,
            "email": unique_email,
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        
        # Should either accept (and possibly truncate) or reject with clear error
        if res["code"] == 0:
            # If accepted, nickname should be truncated to reasonable length
            assert len(res["data"]["nickname"]) <= 255, (
                "Nickname should be truncated to max length"
            )
        else:
            # If rejected, should have clear error message
            assert (
                "length" in res["message"].lower()
                or "long" in res["message"].lower()
                or "characters" in res["message"].lower()
            ), "Error message should mention length issue"

    @pytest.mark.p2
    def test_unicode_in_all_fields(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test unicode characters in various fields."""
        # Use ASCII-safe email local part (unicode in display name)
        local_part: str = f"test_{uuid.uuid4().hex[:8]}"
        unique_email: str = f"{local_part}@example.com"
        payload: dict[str, str] = {
            "nickname": "用户名测试",  # Chinese characters
            "email": unique_email,
            "password": "密码123!",  # Chinese + ASCII
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        
        # Should handle unicode properly
        if res["code"] == 0:
            assert res["data"]["nickname"] == "用户名测试", (
                "Unicode nickname should be preserved"
            )
            assert res["data"]["email"] == unique_email

    @pytest.mark.p2
    def test_emoji_in_nickname(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test emoji characters in nickname."""
        nickname_with_emoji: str = "Test User 😀🎉🔥"
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "nickname": nickname_with_emoji,
            "email": unique_email,
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        
        # Should handle emoji properly (either accept or reject gracefully)
        if res["code"] == 0:
            # Emoji should be preserved
            assert "😀" in res["data"]["nickname"] or "?" in res["data"]["nickname"], (
                "Emoji should be preserved or replaced with placeholder"
            )

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "special_email",
        [
            f"user+tag_{uuid.uuid4().hex[:4]}@example.com",
            f"user.name_{uuid.uuid4().hex[:4]}@example.com",
            f"user_name_{uuid.uuid4().hex[:4]}@example.com",
            f"user-name_{uuid.uuid4().hex[:4]}@example.com",
            f"123_{uuid.uuid4().hex[:4]}@example.com",  # Starts with number
        ],
    )
    def test_special_characters_in_email(
        self, web_api_auth: RAGFlowWebApiAuth, special_email: str
    ) -> None:
        """Test various special characters in email."""
        payload: dict[str, str] = {
            "nickname": "test",
            "email": special_email,
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        assert res["code"] == 0, f"Failed for email: {special_email}, {res}"
        assert res["data"]["email"] == special_email

    @pytest.mark.p2
    def test_whitespace_handling_in_fields(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test whitespace handling in various fields."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "nickname": "  test user  ",  # Leading/trailing spaces
            "email": f"  {unique_email}  ",  # Spaces around email
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        
        # Should handle whitespace (trim or accept)
        if res["code"] == 0:
            # Email should be trimmed
            assert res["data"]["email"] == unique_email, (
                "Email should have whitespace trimmed"
            )
            # Nickname whitespace handling is flexible
            nickname: str = res["data"]["nickname"]
            assert nickname.strip() != "", "Nickname should not be only whitespace"

    @pytest.mark.p2
    def test_null_byte_in_input(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test null byte injection in input fields."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "nickname": "test\x00admin",  # Null byte in nickname
            "email": unique_email,
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        
        # Should handle or reject null bytes
        if res["code"] == 0:
            # Null byte should be removed or replaced
            assert "\x00" not in res["data"]["nickname"], (
                "Null byte should be sanitized"
            )

    @pytest.mark.p2
    def test_very_long_email(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test email exceeding typical length limits."""
        # Create email with very long local part (250 chars)
        long_local: str = "a" * 250
        email: str = f"{long_local}@example.com"
        payload: dict[str, str] = {
            "nickname": "test",
            "email": email,
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        
        # Should reject overly long emails (RFC 5321 limits local part to 64 chars)
        assert res["code"] != 0, "Very long email should be rejected"
        assert (
            "invalid" in res["message"].lower()
            or "long" in res["message"].lower()
        ), "Error should mention invalid/long email"

    @pytest.mark.p2
    def test_email_with_multiple_at_signs(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test email with multiple @ signs."""
        payload: dict[str, str] = {
            "nickname": "test",
            "email": "user@@example.com",
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        
        # Should reject invalid email format
        assert res["code"] != 0, "Email with multiple @ should be rejected"
        assert "invalid" in res["message"].lower()

    @pytest.mark.p2
    def test_email_with_spaces(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test email containing spaces."""
        payload: dict[str, str] = {
            "nickname": "test",
            "email": "user name@example.com",
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        
        # Should reject email with spaces
        assert res["code"] != 0, "Email with spaces should be rejected"
        assert "invalid" in res["message"].lower()

    @pytest.mark.p3
    def test_leading_trailing_dots_in_email(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test email with leading/trailing dots."""
        invalid_emails: list[str] = [
            ".user@example.com",  # Leading dot
            "user.@example.com",  # Trailing dot
            "user..name@example.com",  # Consecutive dots
        ]
        
        for email in invalid_emails:
            payload: dict[str, str] = {
                "nickname": "test",
                "email": email,
                "password": "test123",
            }
            res: dict[str, Any] = create_user(web_api_auth, payload)
            
            # These should be rejected as invalid
            assert res["code"] != 0, f"Invalid email should be rejected: {email}"

    @pytest.mark.p3
    def test_empty_string_vs_none_in_optional_fields(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test difference between empty string and None for optional fields."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        
        # Test with empty string nickname (should be accepted)
        payload_empty: dict[str, str] = {
            "nickname": "",
            "email": unique_email,
            "password": "test123",
        }
        res_empty: dict[str, Any] = create_user(web_api_auth, payload_empty)
        
        # Empty nickname should be accepted per current API behavior
        assert res_empty["code"] == 0, "Empty nickname should be accepted"
        assert res_empty["data"]["nickname"] == ""

    @pytest.mark.p3
    def test_pagination_with_no_results(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test list_users pagination when no users exist."""
        # Assuming clear_users fixture has cleared all test users
        res: dict[str, Any] = list_users(
            web_api_auth, params={"page": 1, "page_size": 10}
        )
        
        # Should return empty list, not error
        assert res["code"] == 0, "Empty result should not cause error"
        # Data might be empty or contain only system users
        assert isinstance(res["data"], list), "Should return list even if empty"

    @pytest.mark.p3
    def test_pagination_beyond_available_pages(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test requesting page beyond available data."""
        # Create one user
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_user(
            web_api_auth,
            {
                "nickname": "test",
                "email": unique_email,
                "password": "test123",
            },
        )
        
        # Request page 100
        res: dict[str, Any] = list_users(
            web_api_auth, params={"page": 100, "page_size": 10}
        )
        
        # Should return empty results, not error
        assert res["code"] == 0, "High page number should not cause error"
        assert isinstance(res["data"], list), "Should return empty list"

    @pytest.mark.p3
    def test_zero_page_size(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test pagination with page_size of 0."""
        res: dict[str, Any] = list_users(
            web_api_auth, params={"page": 1, "page_size": 0}
        )
        
        # Should reject invalid page size
        assert res["code"] != 0, "Zero page size should be rejected"
        assert "page" in res["message"].lower() or "size" in res["message"].lower()

    @pytest.mark.p3
    def test_negative_page_number(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test pagination with negative page number."""
        res: dict[str, Any] = list_users(
            web_api_auth, params={"page": -1, "page_size": 10}
        )
        
        # Should reject negative page number
        assert res["code"] != 0, "Negative page number should be rejected"
        assert "page" in res["message"].lower()

    @pytest.mark.p3
    def test_excessive_page_size(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test pagination with very large page_size."""
        res: dict[str, Any] = list_users(
            web_api_auth, params={"page": 1, "page_size": 10000}
        )
        
        # Should either cap page size or reject
        # Most APIs cap at 100 as per spec
        if res["code"] == 0:
            # If accepted, should return limited results
            assert len(res["data"]) <= 100, (
                "Page size should be capped at reasonable limit"
            )

    @pytest.mark.p3
    def test_special_characters_in_password(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test password with various special characters."""
        special_passwords: list[str] = [
            "Test@123!",
            "Pass#$%^&*()",
            "Pwd[]{}\\|;:'\",<>?/",
            "测试密码123",  # Unicode
            "😀🎉🔥123",  # Emoji
        ]
        
        for password in special_passwords:
            unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
            payload: dict[str, str] = {
                "nickname": "test",
                "email": unique_email,
                "password": password,
            }
            res: dict[str, Any] = create_user(web_api_auth, payload)
            
            # Should accept special characters in password
            assert res["code"] == 0, (
                f"Password with special chars should be accepted: {password}"
            )

    @pytest.mark.p3
    def test_json_injection_in_fields(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test JSON injection attempts."""
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict[str, str] = {
            "nickname": '{"admin": true}',
            "email": unique_email,
            "password": "test123",
        }
        res: dict[str, Any] = create_user(web_api_auth, payload)
        
        # Should treat as literal string, not parse as JSON
        if res["code"] == 0:
            assert res["data"]["nickname"] == '{"admin": true}', (
                "Should store as literal string, not parse JSON"
            )

    @pytest.mark.p3
    def test_path_traversal_in_nickname(
        self, web_api_auth: RAGFlowWebApiAuth
    ) -> None:
        """Test path traversal attempts in nickname."""
        traversal_attempts: list[str] = [
            "../../../etc/passwd",
            "..\\..\\..\\windows\\system32",
            "....//....//....//etc/passwd",
        ]
        
        for nickname in traversal_attempts:
            unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
            payload: dict[str, str] = {
                "nickname": nickname,
                "email": unique_email,
                "password": "test123",
            }
            res: dict[str, Any] = create_user(web_api_auth, payload)
            
            # Should either reject or sanitize path traversal attempts
            # At minimum, should not allow actual file system access
            if res["code"] == 0:
                # Nickname should be stored safely
                stored_nickname: str = res["data"]["nickname"]
                # Verify it's treated as literal string
                assert isinstance(stored_nickname, str)
