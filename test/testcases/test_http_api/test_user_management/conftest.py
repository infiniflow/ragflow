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
from pathlib import Path
from typing import Any

import importlib.util
import pytest
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from Cryptodome.PublicKey import RSA

from common import get_user_info
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Import helpers from root test configuration
# ---------------------------------------------------------------------------

_root_conftest_path = (
    Path(__file__).parent.parent.parent / "conftest.py"
)
_root_spec = importlib.util.spec_from_file_location(
    "root_test_conftest", _root_conftest_path
)
_root_conftest_module = importlib.util.module_from_spec(_root_spec)
assert _root_spec.loader is not None
_root_spec.loader.exec_module(_root_conftest_module)
delete_user_from_db = _root_conftest_module.delete_user_from_db


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
def clear_users(request: pytest.FixtureRequest) -> list[str]:
    """Fixture to clean up users created during tests.

    Tests should append test user *emails* to the returned list. After each
    test, we hard-delete those users directly from the database via the
    shared `delete_user_from_db` helper in the root test ``conftest.py``.
    """
    created_user_emails: list[str] = []

    def cleanup() -> None:
        for email in created_user_emails:
            try:
                delete_user_from_db(email)
            except Exception as exc:  # pragma: no cover - best-effort cleanup
                print(
                    f"[clear_users] Failed to delete test user {email}: {exc}"
                )

    request.addfinalizer(cleanup)
    return created_user_emails


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
