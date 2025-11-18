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
"""Shared fixtures and utilities for user management tests."""

from __future__ import annotations

import base64
import os
import uuid
from typing import Any

import pytest
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from Cryptodome.PublicKey import RSA

from common import get_user_info
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Utility Functions
# ---------------------------------------------------------------------------


def encrypt_password(password: str) -> str:
    """Encrypt password for API calls without importing from api.utils.crypt.

    Avoids ModuleNotFoundError caused by test helper module named `common`.

    Args:
        password: Plain text password to encrypt.

    Returns:
        Encrypted password as a base64-encoded string.
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


def generate_unique_email(prefix: str = "test") -> str:
    """Generate a unique email address for testing.

    Args:
        prefix: Optional prefix for the email local part. Defaults to "test".

    Returns:
        A unique email address string.
    """
    return f"{prefix}_{uuid.uuid4().hex[:8]}@example.com"


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture(scope="function")
def clear_users(request, http_api_auth):
    """Fixture to clean up users created during tests."""
    created_user_ids: list[str] = []

    def cleanup() -> None:
        # Clean up users if delete endpoint exists
        # For now, we'll just track them
        pass

    request.addfinalizer(cleanup)
    return created_user_ids


@pytest.fixture(name="test_user")
def fixture_test_user(web_api_auth: RAGFlowWebApiAuth) -> dict[str, Any]:
    """Get the current authenticated user's information for update tests.

    Since users can only update their own account, we use the authenticated
    user's information instead of creating a new user.

    Args:
        web_api_auth: Authenticated user's auth object.

    Returns:
        Dictionary containing user_id, email, and original_nickname.
    """
    res: dict[str, Any] = get_user_info(web_api_auth)
    assert res["code"] == 0, f"Failed to get user info: {res}"

    user_data: dict[str, Any] = res["data"]

    # Store original nickname for comparison
    original_nickname: str = user_data.get("nickname", "")

    return {
        "user_id": user_data["id"],
        "email": user_data["email"],
        "original_nickname": original_nickname,
    }
