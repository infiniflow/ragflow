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

from http_client import HttpClient


class AuthException(Exception):
    def __init__(self, message, code=401):
        super().__init__(message)
        self.code = code
        self.message = message


def encrypt_password(password_plain: str) -> str:
    try:
        from api.utils.crypt import crypt
    except Exception as exc:
        raise AuthException(
            "Password encryption unavailable; install pycryptodomex (uv sync --python 3.12 --group test)."
        ) from exc
    return crypt(password_plain)


def register_user(client: HttpClient, email: str, nickname: str, password: str) -> None:
    password_enc = encrypt_password(password)
    payload = {"email": email, "nickname": nickname, "password": password_enc}
    res = client.request_json("POST", "/user/register", use_api_base=False, auth_kind=None, json_body=payload)
    if res.get("code") == 0:
        return
    msg = res.get("message", "")
    if "has already registered" in msg:
        return
    raise AuthException(f"Register failed: {msg}")


def login_user(client: HttpClient, server_type: str, email: str, password: str) -> str:
    password_enc = encrypt_password(password)
    payload = {"email": email, "password": password_enc}
    if server_type == "admin":
        response = client.request("POST", "/admin/login", use_api_base=True, auth_kind=None, json_body=payload)
    else:
        response = client.request("POST", "/user/login", use_api_base=False, auth_kind=None, json_body=payload)
    try:
        res = response.json()
    except Exception as exc:
        raise AuthException(f"Login failed: invalid JSON response ({exc})") from exc
    if res.get("code") != 0:
        raise AuthException(f"Login failed: {res.get('message')}")
    token = response.headers.get("Authorization")
    if not token:
        raise AuthException("Login failed: missing Authorization header")
    return token
