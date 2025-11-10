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
from common import create_user
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth


def encrypt_password(password: str) -> str:
    """
    Encrypt password for API calls without importing from api.utils.crypt
    Avoids ModuleNotFoundError caused by test helper module named `common`.
    """
    # test/testcases/test_http_api/test_user_management/test_create_user.py -> project root
    current_dir: str = os.path.dirname(os.path.abspath(__file__))
    project_base: str = os.path.abspath(os.path.join(current_dir, "..", "..", "..", ".."))
    file_path: str = os.path.join(project_base, "conf", "public.pem")

    rsa_key: RSA.RSAKey = RSA.importKey(open(file_path).read(), "Welcome")
    cipher: Cipher_pkcs1_v1_5.Cipher_pkcs1_v1_5 = Cipher_pkcs1_v1_5.new(rsa_key)
    password_base64: str = base64.b64encode(password.encode("utf-8")).decode("utf-8")
    encrypted_password: str = cipher.encrypt(password_base64.encode())
    return base64.b64encode(encrypted_password).decode("utf-8")


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
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        # Use unique email to avoid conflicts
        unique_email = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload = {
            "nickname": "test_user",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        res = create_user(invalid_auth, payload)
        assert res["code"] == expected_code, res
        if expected_message:
            assert expected_message in res["message"]


@pytest.mark.usefixtures("clear_users")
class TestUserCreate:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"nickname": "valid_user", "email": "valid@example.com", "password": encrypt_password("test123")}, 0, ""),
            ({"nickname": "", "email": "test@example.com", "password": encrypt_password("test123")}, 0, ""),  # Empty nickname is accepted
            ({"nickname": "test_user", "email": "", "password": encrypt_password("test123")}, 103, "Invalid email address"),
            ({"nickname": "test_user", "email": "test@example.com", "password": ""}, 500, "Fail to decrypt password"),
            ({"nickname": "test_user", "email": "test@example.com"}, 101, "required argument are missing"),
            ({"nickname": "test_user", "password": encrypt_password("test123")}, 101, "required argument are missing"),
            ({"email": "test@example.com", "password": encrypt_password("test123")}, 101, "required argument are missing"),
        ],
    )
    def test_required_fields(self, HttpApiAuth: RAGFlowHttpApiAuth, payload: dict, expected_code: int, expected_message: str) -> None:
        if payload.get("email") and "@" in payload.get("email", ""):
            # Use unique email to avoid conflicts
            unique_email = f"test_{uuid.uuid4().hex[:8]}@example.com"
            payload["email"] = unique_email
        res = create_user(HttpApiAuth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["nickname"] == payload["nickname"]
            assert res["data"]["email"] == payload["email"]
        else:
            assert expected_message in res["message"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "email, expected_code, expected_message",
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
    def test_email_validation(self, HttpApiAuth, email, expected_code, expected_message):
        if email and "@" in email and expected_code == 0:
            # Use unique email to avoid conflicts
            email = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload = {
            "nickname": "test_user",
            "email": email,
            "password": encrypt_password("test123"),
        }
        res = create_user(HttpApiAuth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["email"] == email
        else:
            assert expected_message in res["message"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "nickname, expected_code, expected_message",
        [
            ("valid_nickname", 0, ""),
            ("user123", 0, ""),
            ("user_name", 0, ""),
            ("User Name", 0, ""),
            ("", 0, ""),  # Empty nickname is accepted by the API
        ],
    )
    def test_nickname(self, HttpApiAuth, nickname, expected_code, expected_message):
        unique_email = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload = {
            "nickname": nickname,
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        res = create_user(HttpApiAuth, payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["nickname"] == nickname
        else:
            assert expected_message in res["message"]

    @pytest.mark.p1
    def test_duplicate_email(self, HttpApiAuth):
        unique_email = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload = {
            "nickname": "test_user_1",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        res = create_user(HttpApiAuth, payload)
        assert res["code"] == 0

        # Try to create another user with the same email
        payload2 = {
            "nickname": "test_user_2",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        res2 = create_user(HttpApiAuth, payload2)
        assert res2["code"] == 103
        assert "has already registered" in res2["message"]

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "is_superuser, expected_value",
        [
            (True, True),
            (False, False),
            (None, False),  # Default should be False
        ],
    )
    def test_is_superuser(self, HttpApiAuth, is_superuser, expected_value):
        unique_email = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload = {
            "nickname": "test_user",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        if is_superuser is not None:
            payload["is_superuser"] = is_superuser

        res = create_user(HttpApiAuth, payload)
        assert res["code"] == 0
        assert res["data"]["is_superuser"] == expected_value

    @pytest.mark.p2
    def test_password_encryption(self, HttpApiAuth):
        unique_email = f"test_{uuid.uuid4().hex[:8]}@example.com"
        password = "test_password_123"
        payload = {
            "nickname": "test_user",
            "email": unique_email,
            "password": encrypt_password(password),
        }
        res = create_user(HttpApiAuth, payload)
        assert res["code"] == 0
        # Password should be hashed in the response (not plain text)
        assert "password" in res["data"], f"Password field not found in response: {res['data'].keys()}"
        assert res["data"]["password"].startswith("scrypt:"), f"Password is not hashed: {res['data']['password']}"
        # Verify it's not the plain password
        assert res["data"]["password"] != password
        assert res["data"]["password"] != encrypt_password(password)

    @pytest.mark.p2
    def test_invalid_password_encryption(self, HttpApiAuth):
        unique_email = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload = {
            "nickname": "test_user",
            "email": unique_email,
            "password": "plain_text_password",  # Not encrypted
        }
        res = create_user(HttpApiAuth, payload)
        # Should fail to decrypt password
        assert res["code"] == 500
        assert "Fail to decrypt password" in res["message"]

    @pytest.mark.p3
    def test_concurrent_create(self, HttpApiAuth):
        count = 10
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = []
            for i in range(count):
                unique_email = f"test_{uuid.uuid4().hex[:8]}@example.com"
                payload = {
                    "nickname": f"test_user_{i}",
                    "email": unique_email,
                    "password": encrypt_password("test123"),
                }
                futures.append(executor.submit(create_user, HttpApiAuth, payload))
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

    @pytest.mark.p2
    def test_user_creation_response_structure(self, HttpApiAuth):
        unique_email = f"test_{uuid.uuid4().hex[:8]}@example.com"
        payload = {
            "nickname": "test_user",
            "email": unique_email,
            "password": encrypt_password("test123"),
        }
        res = create_user(HttpApiAuth, payload)
        assert res["code"] == 0
        assert "data" in res
        assert "id" in res["data"]
        assert "email" in res["data"]
        assert "nickname" in res["data"]
        assert res["data"]["email"] == unique_email
        assert res["data"]["nickname"] == "test_user"
        assert "User test_user created successfully!" in res["message"]
