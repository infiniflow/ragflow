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
        import base64
        from Cryptodome.PublicKey import RSA
        from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
        def crypt(line):
            """
            decrypt(crypt(input_string)) == base64(input_string), which frontend and ragflow_cli use.
            """
            pub = "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArq9XTUSeYr2+N1h3Afl/z8Dse/2yD0ZGrKwx+EEEcdsBLca9Ynmx3nIB5obmLlSfmskLpBo0UACBmB5rEjBp2Q2f3AG3Hjd4B+gNCG6BDaawuDlgANIhGnaTLrIqWrrcm4EMzJOnAOI1fgzJRsOOUEfaS318Eq9OVO3apEyCCt0lOQK6PuksduOjVxtltDav+guVAA068NrPYmRNabVKRNLJpL8w4D44sfth5RvZ3q9t+6RTArpEtc5sh5ChzvqPOzKGMXW83C95TxmXqpbK6olN4RevSfVjEAgCydH6HN6OhtOQEcnrU97r9H0iZOWwbw3pVrZiUkuRD1R56Wzs2wIDAQAB\n-----END PUBLIC KEY-----"
            rsa_key = RSA.importKey(pub)
            cipher = Cipher_pkcs1_v1_5.new(rsa_key)
            password_base64 = base64.b64encode(line.encode('utf-8')).decode("utf-8")
            encrypted_password = cipher.encrypt(password_base64.encode())
            return base64.b64encode(encrypted_password).decode('utf-8')
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
