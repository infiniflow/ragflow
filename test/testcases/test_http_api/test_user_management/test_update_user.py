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
import base64
import os
import uuid
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from Cryptodome.PublicKey import RSA
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from common import create_user, update_user
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth


def encrypt_password(password: str) -> str:
    """
    Encrypt password for API calls without importing from api.utils.crypt
    Avoids ModuleNotFoundError caused by test helper module named `common`.
    """
    # test/testcases/test_http_api/test_user_management/test_update_user.py -> project root
    current_dir: str = os.path.dirname(os.path.abspath(__file__))
    project_base: str = os.path.abspath(os.path.join(current_dir, "..", "..", "..", ".."))
    file_path: str = os.path.join(project_base, "conf", "public.pem")

    rsa_key: RSA.RSAKey = RSA.importKey(open(file_path).read(), "Welcome")
    cipher: Cipher_pkcs1_v1_5.Cipher_pkcs1_v1_5 = Cipher_pkcs1_v1_5.new(rsa_key)
    password_base64: str = base64.b64encode(password.encode("utf-8")).decode("utf-8")
    encrypted_password: str = cipher.encrypt(password_base64.encode())
    return base64.b64encode(encrypted_password).decode("utf-8")


@pytest.fixture
def test_user(HttpApiAuth: RAGFlowHttpApiAuth) -> dict:
    """Create a test user for update tests"""
    unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
    payload: dict = {
        "nickname": "test_user_original",
        "email": unique_email,
        "password": encrypt_password("test123"),
    }
    res: dict = create_user(HttpApiAuth, payload)
    assert res["code"] == 0, f"Failed to create test user: {res}"
    return {
        "user_id": res["data"]["id"],
        "email": unique_email,
        "original_nickname": "test_user_original",
    }


@pytest.mark.p1
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            # Note: @login_required is commented out, so endpoint works without auth
            # Testing with None auth should succeed (code 0) if endpoint doesn't require auth
            (None, 0, ""),
            # Invalid token should also work if auth is not required
            (RAGFlowHttpApiAuth(INVALID_API_TOKEN), 0, ""),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message, test_user):
        payload: dict = {
            "user_id": test_user["user_id"],
            "nickname": "updated_nickname",
        }
        res: dict = update_user(invalid_auth, payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.usefixtures("clear_users")
class TestUserUpdate:
    @pytest.mark.p1
    def test_update_with_user_id(self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict) -> None:
        """Test updating user by user_id"""
        payload: dict = {
            "user_id": test_user["user_id"],
            "nickname": "updated_nickname",
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["nickname"] == "updated_nickname"
        assert res["data"]["email"] == test_user["email"]
        assert "updated successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_update_with_email(self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict) -> None:
        """Test updating user by email"""
        payload: dict = {
            "email": test_user["email"],
            "nickname": "updated_nickname_email",
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["nickname"] == "updated_nickname_email"
        assert res["data"]["email"] == test_user["email"]

    @pytest.mark.p1
    def test_update_missing_identifier(self, HttpApiAuth: RAGFlowHttpApiAuth) -> None:
        """Test update without user_id or email"""
        payload: dict = {
            "nickname": "updated_nickname",
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == 101  # ARGUMENT_ERROR
        assert "Either user_id or email must be provided" in res["message"]

    @pytest.mark.p1
    def test_update_user_not_found_by_id(self, HttpApiAuth: RAGFlowHttpApiAuth) -> None:
        """Test update with non-existent user_id"""
        payload: dict = {
            "user_id": "non_existent_user_id_12345",
            "nickname": "updated_nickname",
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == 102  # DATA_ERROR
        assert "User not found" in res["message"]

    @pytest.mark.p1
    def test_update_user_not_found_by_email(self, HttpApiAuth: RAGFlowHttpApiAuth) -> None:
        """Test update with non-existent email"""
        payload: dict = {
            "email": "nonexistent@example.com",
            "nickname": "updated_nickname",
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == 102  # DATA_ERROR
        assert "not found" in res["message"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "nickname, expected_code, expected_message",
        [
            ("valid_nickname", 0, ""),
            ("user123", 0, ""),
            ("user_name", 0, ""),
            ("User Name", 0, ""),
            ("", 0, ""),  # Empty nickname is accepted
        ],
    )
    def test_update_nickname(
        self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict, nickname: str, expected_code: int, expected_message: str
    ) -> None:
        payload: dict = {
            "user_id": test_user["user_id"],
            "nickname": nickname,
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["nickname"] == nickname
        else:
            assert expected_message in res["message"]

    @pytest.mark.p1
    def test_update_password(self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict) -> None:
        """Test updating user password"""
        new_password: str = "new_password_456"
        payload: dict = {
            "user_id": test_user["user_id"],
            "password": encrypt_password(new_password),
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == 0, res
        assert "updated successfully" in res["message"].lower()

    @pytest.mark.p1
    def test_update_password_invalid_encryption(self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict) -> None:
        """Test updating password with invalid encryption"""
        payload: dict = {
            "user_id": test_user["user_id"],
            "password": "plain_text_password",  # Not encrypted
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == 500
        assert "Fail to decrypt password" in res["message"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "new_email, expected_code, expected_message",
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
        self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict, new_email: str, expected_code: int, expected_message: str
    ) -> None:
        if "@" in new_email and expected_code == 0:
            # Use unique email to avoid conflicts
            new_email = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict = {
            "user_id": test_user["user_id"],
            "new_email": new_email,
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["email"] == new_email
        else:
            assert expected_message in res["message"]

    @pytest.mark.p1
    def test_update_email_duplicate(self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict) -> None:
        """Test updating email to an already used email"""
        # Create another user
        unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        create_payload: dict = {
            "nickname": "another_user",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        create_res: dict = create_user(HttpApiAuth, create_payload)
        assert create_res["code"] == 0

        # Try to update test_user's email to the same email
        update_payload: dict = {
            "user_id": test_user["user_id"],
            "new_email": unique_email,
        }
        res: dict = update_user(HttpApiAuth, update_payload)
        assert res["code"] == 103  # OPERATING_ERROR
        assert "already in use" in res["message"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "is_superuser, expected_value",
        [
            (True, True),
            (False, False),
        ],
    )
    def test_update_is_superuser(
        self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict, is_superuser: bool, expected_value: bool
    ) -> None:
        payload: dict = {
            "user_id": test_user["user_id"],
            "is_superuser": is_superuser,
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["is_superuser"] == expected_value

    @pytest.mark.p1
    def test_update_multiple_fields(self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict) -> None:
        """Test updating multiple fields at once"""
        new_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict = {
            "user_id": test_user["user_id"],
            "nickname": "updated_multiple",
            "new_email": new_email,
            "is_superuser": True,
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["nickname"] == "updated_multiple"
        assert res["data"]["email"] == new_email
        assert res["data"]["is_superuser"] is True

    @pytest.mark.p1
    def test_update_no_fields(self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict) -> None:
        """Test update with no fields to update"""
        payload: dict = {
            "user_id": test_user["user_id"],
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == 101  # ARGUMENT_ERROR
        assert "No valid fields to update" in res["message"]

    @pytest.mark.p1
    def test_update_email_using_email_field_when_user_id_provided(
        self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict
    ) -> None:
        """Test that when user_id is provided, 'email' field can be used as new_email"""
        new_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload: dict = {
            "user_id": test_user["user_id"],
            "email": new_email,  # When user_id is provided, email is treated as new_email
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == 0, res
        assert res["data"]["email"] == new_email

    @pytest.mark.p2
    def test_update_response_structure(self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict) -> None:
        """Test that update response has correct structure"""
        payload: dict = {
            "user_id": test_user["user_id"],
            "nickname": "response_test",
        }
        res: dict = update_user(HttpApiAuth, payload)
        assert res["code"] == 0
        assert "data" in res
        assert "id" in res["data"]
        assert "email" in res["data"]
        assert "nickname" in res["data"]
        assert res["data"]["nickname"] == "response_test"
        assert "updated successfully" in res["message"].lower()

    @pytest.mark.p2
    def test_concurrent_updates(self, HttpApiAuth: RAGFlowHttpApiAuth) -> None:
        """Test concurrent updates to different users"""
        # Create multiple users
        users: list = []
        for i in range(5):
            unique_email: str = f"test_{uuid.uuid4().hex[:8]}@example.com"
            create_payload: dict = {
                "nickname": f"user_{i}",
                "email": unique_email,
                "password": encrypt_password("test123"),
            }
            create_res: dict = create_user(HttpApiAuth, create_payload)
            assert create_res["code"] == 0
            users.append(create_res["data"])

        # Update all users concurrently
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures: list = []
            for i, user in enumerate(users):
                payload: dict = {
                    "user_id": user["id"],
                    "nickname": f"updated_user_{i}",
                }
                futures.append(executor.submit(update_user, HttpApiAuth, payload))

            responses: list = list(as_completed(futures))
            assert len(responses) == 5
            assert all(future.result()["code"] == 0 for future in futures)

    @pytest.mark.p3
    def test_update_same_user_multiple_times(self, HttpApiAuth: RAGFlowHttpApiAuth, test_user: dict) -> None:
        """Test updating the same user multiple times"""
        # First update
        payload1: dict = {
            "user_id": test_user["user_id"],
            "nickname": "first_update",
        }
        res1: dict = update_user(HttpApiAuth, payload1)
        assert res1["code"] == 0
        assert res1["data"]["nickname"] == "first_update"

        # Second update
        payload2: dict = {
            "user_id": test_user["user_id"],
            "nickname": "second_update",
        }
        res2: dict = update_user(HttpApiAuth, payload2)
        assert res2["code"] == 0
        assert res2["data"]["nickname"] == "second_update"

        # Third update
        payload3: dict = {
            "user_id": test_user["user_id"],
            "nickname": "third_update",
        }
        res3: dict = update_user(HttpApiAuth, payload3)
        assert res3["code"] == 0
        assert res3["data"]["nickname"] == "third_update"

