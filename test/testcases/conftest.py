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
from typing import Any, Dict, Optional

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


def register() -> Optional[str]:
    """Register the test user.
    
    Returns:
        str: Authorization token if registration succeeded and user was logged in,
             None if user already exists
    """
    url: str = HOST_ADDRESS + f"/{VERSION}/user/register"
    name: str = "qa"
    # Encrypt the plain password "123" before sending
    plain_password: str = "123"
    encrypted_password: str = encrypt_password(plain_password)
    register_data = {"email": EMAIL, "nickname": name, "password": encrypted_password}
    res: requests.Response = requests.post(url=url, json=register_data)
    res_json: Dict[str, Any] = res.json()
    if res_json.get("code") != 0 and "has already registered" not in res_json.get("message"):
        print(f"Registration failed with code {res_json.get('code')}: {res_json.get('message')}")
        raise Exception(res_json.get("message"))
    elif res_json.get("code") == 0:
        print(f"Registration successful for {EMAIL}")
        # Registration endpoint logs user in and returns auth token
        auth_token: str = res.headers.get("Authorization", "")
        if auth_token:
            print(f"Received auth token from registration")
            return auth_token
        else:
            print(f"Warning: No auth token in registration response")
            return None
    return None


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


def delete_user_from_db(email: str) -> bool:
    """Delete a user directly from the database using SQL.
    
    This is a helper function for cleanup when a user exists with wrong password.
    Uses direct SQL to avoid import conflicts with test helper modules.
    
    Args:
        email: Email of the user to delete
        
    Returns:
        bool: True if successful, False otherwise
    """
    try:
        import subprocess
        import sys
        import os
        
        current_dir = os.path.dirname(os.path.abspath(__file__))
        project_root = os.path.abspath(os.path.join(current_dir, "..", ".."))
        
        # Create a temporary Python script to hard delete the user
        delete_script = f"""
import sys
sys.path.insert(0, '{project_root}')

# Remove test directories from path to avoid conflicts
test_paths = [p for p in sys.path if 'test/testcases' in p or 'testcases' in p]
for p in test_paths:
    if p in sys.path:
        sys.path.remove(p)

try:
    from api.db.db_models import DB, User, Tenant, UserTenant, File
    
    users = list(User.select().where(User.email == '{email}'))
    if users:
        with DB.atomic():
            for user in users:
                user_id = user.id
                # Hard delete related records and user
                try:
                    # Delete user-tenant relationships
                    UserTenant.delete().where(UserTenant.user_id == user_id).execute()
                    UserTenant.delete().where(UserTenant.tenant_id == user_id).execute()
                    
                    # Delete files owned by user
                    File.delete().where(File.created_by == user_id).execute()
                    File.delete().where(File.tenant_id == user_id).execute()
                    
                    # Delete tenant
                    Tenant.delete().where(Tenant.id == user_id).execute()
                    
                    # Finally delete user
                    User.delete().where(User.id == user_id).execute()
                except Exception as e:
                    print(f"Warning during cleanup: {{e}}")
        print(f"DELETED_USER:{email}")
    else:
        print(f"USER_NOT_FOUND:{email}")
except Exception as e:
    print(f"ERROR:{{e}}")
    import traceback
    traceback.print_exc()
    sys.exit(1)
"""
        
        # Run the delete script in a subprocess to avoid import conflicts
        result = subprocess.run(
            [sys.executable, "-c", delete_script],
            capture_output=True,
            text=True,
            timeout=30
        )
        
        output = result.stdout + result.stderr
        
        if "DELETED_USER:" in output:
            print(f"Successfully deleted user {email} from database")
            return True
        elif "USER_NOT_FOUND:" in output:
            print(f"User {email} not found in database")
            return False
        else:
            print(f"Failed to delete user from database")
            if output:
                print(f"Output: {output}")
            return False
            
    except subprocess.TimeoutExpired:
        print(f"Timeout while trying to delete user from database")
        return False
    except Exception as e:
        print(f"Failed to delete user from database: {e}")
        return False


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
    """Session fixture to authenticate test user.
    
    This fixture tries to login with the test user. If login fails because
    the user doesn't exist, it registers the user first. If the user exists
    with a different password, it provides instructions to fix the issue.
    
    Returns:
        str: Authentication token
    
    Raises:
        Exception: If authentication fails
    """
    # First, try to login (user might already exist with correct password)
    try:
        auth: str = login()
        print(f"Successfully logged in as {EMAIL}")
        return auth
    except Exception as login_error:
        login_error_msg = str(login_error)
        
        # If user doesn't exist, try to register
        if "is not registered" in login_error_msg:
            print(f"User {EMAIL} not found, attempting to register...")
            try:
                auth_token: Optional[str] = register()
                if auth_token:
                    print(f"Successfully registered and logged in as {EMAIL}")
                    return auth_token
                else:
                    # Try login if register didn't return auth token
                    auth: str = login()
                    print(f"Successfully registered and logged in as {EMAIL}")
                    return auth
            except Exception as register_error:
                raise Exception(
                    f"Failed to register user {EMAIL}: {register_error}"
                ) from register_error
        
        # If user exists but password doesn't match
        elif "Email and password do not match" in login_error_msg:
            print(f"User {EMAIL} exists but password doesn't match. Attempting to delete and recreate...")
            
            # Try to delete the user from database directly
            if delete_user_from_db(EMAIL):
                # Delay to ensure deletion is committed to database
                time.sleep(1.0)
                
                # Now try to register and login
                try:
                    print(f"Attempting to register user {EMAIL}...")
                    auth_token: Optional[str] = register()
                    if auth_token:
                        print(f"Successfully recreated user {EMAIL} with correct password")
                        return auth_token
                    else:
                        # Try login if register didn't return auth token
                        print(f"Registration completed, now attempting login...")
                        auth: str = login()
                        print(f"Successfully recreated user {EMAIL} with correct password")
                        return auth
                except Exception as recreate_error:
                    recreate_error_msg = str(recreate_error)
                    print(f"Recreation failed: {recreate_error_msg}")
                    raise Exception(
                        f"Failed to recreate user after deletion: {recreate_error_msg}"
                    ) from recreate_error
            else:
                # If database deletion failed, provide instructions
                raise Exception(
                    f"Login failed: User {EMAIL} exists but password doesn't match.\n"
                    f"Automatic cleanup failed. To fix this issue:\n"
                    f"1. Manually delete the user from the database, OR\n"
                    f"2. Reset the password in the database to '123', OR\n"
                    f"3. Update EMAIL in configs.py to use a different test user\n"
                    f"Original error: {login_error_msg}"
                ) from login_error
        
        # Other login errors
        else:
            raise Exception(
                f"Login failed with unexpected error: {login_error_msg}"
            ) from login_error


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
