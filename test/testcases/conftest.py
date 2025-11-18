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
import time
from typing import Any, Dict

import pytest
import requests
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from Cryptodome.PublicKey import RSA

from configs import EMAIL, HOST_ADDRESS, PASSWORD, VERSION, ZHIPU_AI_API_KEY
from libs.auth import RAGFlowWebApiAuth

MARKER_EXPRESSIONS = {
    "p1": "p1",
    "p2": "p1 or p2",
    "p3": "p1 or p2 or p3",
}


def pytest_addoption(parser: pytest.Parser) -> None:
    parser.addoption(
        "--level",
        action="store",
        default="p2",
        choices=list(MARKER_EXPRESSIONS.keys()),
        help=f"Test level ({'/'.join(MARKER_EXPRESSIONS)}): p1=smoke, p2=core, p3=full",
    )

    parser.addoption(
        "--client-type",
        action="store",
        default="http",
        choices=["python_sdk", "http", "web"],
        help="Test client type: 'python_sdk', 'http', 'web'",
    )


def pytest_configure(config: pytest.Config) -> None:
    level = config.getoption("--level")
    config.option.markexpr = MARKER_EXPRESSIONS[level]
    if config.option.verbose > 0:
        print(f"\n[CONFIG] Active test level: {level}")


def encrypt_password(password: str) -> str:
    """Encrypt password for API calls.
    
    Args:
        password: Plain text password to encrypt.
        
    Returns:
        Encrypted password as a base64-encoded string.
    """
    current_dir: str = os.path.dirname(os.path.abspath(__file__))
    project_base: str = os.path.abspath(os.path.join(current_dir, "..", ".."))
    file_path: str = os.path.join(project_base, "conf", "public.pem")
    
    with open(file_path, encoding="utf-8") as pem_file:
        rsa_key: RSA.RsaKey = RSA.import_key(pem_file.read(), passphrase="Welcome")
    
    cipher: Cipher_pkcs1_v1_5.PKCS115_Cipher = Cipher_pkcs1_v1_5.new(rsa_key)
    password_base64: str = base64.b64encode(password.encode()).decode()
    encrypted_password: bytes = cipher.encrypt(password_base64.encode())
    return base64.b64encode(encrypted_password).decode()


def register():
    url: str = HOST_ADDRESS + f"/{VERSION}/user/register"
    name: str = "qa"
    # Encrypt the plain password "123" before sending
    plain_password: str = "123"
    encrypted_password: str = encrypt_password(plain_password)
    register_data = {"email": EMAIL, "nickname": name, "password": encrypted_password}
    res: requests.Response = requests.post(url=url, json=register_data)
    res: Dict[str, Any] = res.json()
    if res.get("code") != 0 and "has already registered" not in res.get("message"):
        raise Exception(res.get("message"))


def login():
    url: str = HOST_ADDRESS + f"/{VERSION}/user/login"
    # Encrypt the plain password "123" before sending
    plain_password: str = "123"
    encrypted_password: str = encrypt_password(plain_password)
    login_data = {"email": EMAIL, "password": encrypted_password}
    response: requests.Response = requests.post(url=url, json=login_data)
    res: Dict[str, Any] = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    auth: str = response.headers["Authorization"]
    return auth


def login_as_user(email: str, password: str) -> RAGFlowWebApiAuth:
    """Login as a user and return authentication object.
    
    Args:
        email: User email address.
        password: Plain text password.
        
    Returns:
        RAGFlowWebApiAuth object for authenticated requests.
        
    Raises:
        Exception: If login fails.
    """
    # Small delay to ensure user creation is committed to database
    time.sleep(0.1)
    
    url: str = HOST_ADDRESS + f"/{VERSION}/user/login"
    encrypted_password: str = encrypt_password(password)
    login_data: Dict[str, str] = {"email": email, "password": encrypted_password}
    response: requests.Response = requests.post(url=url, json=login_data)
    res: Dict[str, Any] = response.json()
    if res.get("code") != 0:
        message: str = str(res.get("message", "Login failed"))
        raise Exception(f"Login failed: {message}")
    auth_token: str = response.headers.get("Authorization", "")
    if not auth_token:
        raise Exception("No authorization token received from login response")
    return RAGFlowWebApiAuth(auth_token)


@pytest.fixture(scope="session")
def auth():
    try:
        register()
    except Exception as e:
        print(e)
    try:
        auth: str = login()
        return auth
    except Exception as e:
        error_msg = str(e)
        if "Email and password do not match" in error_msg:
            raise Exception(
                f"Login failed: User {EMAIL} exists but password doesn't match. "
                f"Please ensure the user has the correct password or delete the user first."
            ) from e
        raise


@pytest.fixture(scope="session")
def token(auth):
    url: str = HOST_ADDRESS + f"/{VERSION}/system/new_token"
    auth: Dict[str, str] = {"Authorization": auth}
    response: requests.Response = requests.post(url=url, headers=auth)
    res: Dict[str, Any] = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    return res["data"].get("token")


def get_my_llms(auth: str, name: str) -> bool:
    url: str = HOST_ADDRESS + f"/{VERSION}/llm/my_llms"
    authorization: Dict[str, str] = {"Authorization": auth}
    response: requests.Response = requests.get(url=url, headers=authorization)
    res: Dict[str, Any] = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    if name in res.get("data"):
        return True
    return False


def add_models(auth: str):
    url: str = HOST_ADDRESS + f"/{VERSION}/llm/set_api_key"
    authorization: Dict[str, str] = {"Authorization": auth}
    models_info: Dict[str, Dict[str, str]] = {
        "ZHIPU-AI": {"llm_factory": "ZHIPU-AI", "api_key": ZHIPU_AI_API_KEY},
    }

    for name, model_info in models_info.items():
        if not get_my_llms(auth, name):
            response: requests.Response = requests.post(url=url, headers=authorization, json=model_info)
            res: Dict[str, Any] = response.json()
            if res.get("code") != 0:
                pytest.exit(f"Critical error in add_models: {res.get('message')}")


def get_tenant_info(auth: str) -> str:
    url: str = HOST_ADDRESS + f"/{VERSION}/user/tenant_info"
    authorization: Dict[str, str] = {"Authorization": auth}
    response: requests.Response = requests.get(url=url, headers=authorization)
    res: Dict[str, Any] = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    return res["data"].get("tenant_id")


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info(auth: str):
    try:
        add_models(auth)
        tenant_id: str = get_tenant_info(auth)
    except Exception as e:
        pytest.exit(f"Error in set_tenant_info: {str(e)}")
    url: str = HOST_ADDRESS + f"/{VERSION}/user/set_tenant_info"
    authorization: Dict[str, str] = {"Authorization": auth}
    tenant_info: Dict[str, Any] = {
        "tenant_id": tenant_id,
        "llm_id": "glm-4-flash@ZHIPU-AI",
        "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
        "img2txt_id": "",
        "asr_id": "",
        "tts_id": None,
    }
    response: requests.Response = requests.post(url=url, headers=authorization, json=tenant_info)
    res: Dict[str, Any] = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
