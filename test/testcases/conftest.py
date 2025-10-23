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

import pytest
import requests
from configs import EMAIL, HOST_ADDRESS, PASSWORD, VERSION, ZHIPU_AI_API_KEY

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


def register():
    url = HOST_ADDRESS + f"/{VERSION}/user/register"
    name = "qa"
    register_data = {"email": EMAIL, "nickname": name, "password": PASSWORD}
    res = requests.post(url=url, json=register_data)
    res = res.json()
    if res.get("code") != 0 and "has already registered" not in res.get("message"):
        raise Exception(res.get("message"))


def login():
    url = HOST_ADDRESS + f"/{VERSION}/user/login"
    login_data = {"email": EMAIL, "password": PASSWORD}
    response = requests.post(url=url, json=login_data)
    res = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    auth = response.headers["Authorization"]
    return auth


@pytest.fixture(scope="session")
def auth():
    try:
        register()
    except Exception as e:
        print(e)
    auth = login()
    return auth


@pytest.fixture(scope="session")
def token(auth):
    url = HOST_ADDRESS + f"/{VERSION}/system/new_token"
    auth = {"Authorization": auth}
    response = requests.post(url=url, headers=auth)
    res = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    return res["data"].get("token")


def get_my_llms(auth, name):
    url = HOST_ADDRESS + f"/{VERSION}/llm/my_llms"
    authorization = {"Authorization": auth}
    response = requests.get(url=url, headers=authorization)
    res = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    if name in res.get("data"):
        return True
    return False


def add_models(auth):
    url = HOST_ADDRESS + f"/{VERSION}/llm/set_api_key"
    authorization = {"Authorization": auth}
    models_info = {
        "ZHIPU-AI": {"llm_factory": "ZHIPU-AI", "api_key": ZHIPU_AI_API_KEY},
    }

    for name, model_info in models_info.items():
        if not get_my_llms(auth, name):
            response = requests.post(url=url, headers=authorization, json=model_info)
            res = response.json()
            if res.get("code") != 0:
                pytest.exit(f"Critical error in add_models: {res.get('message')}")


def get_tenant_info(auth):
    url = HOST_ADDRESS + f"/{VERSION}/user/tenant_info"
    authorization = {"Authorization": auth}
    response = requests.get(url=url, headers=authorization)
    res = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    return res["data"].get("tenant_id")


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info(auth):
    try:
        add_models(auth)
        tenant_id = get_tenant_info(auth)
    except Exception as e:
        pytest.exit(f"Error in set_tenant_info: {str(e)}")
    url = HOST_ADDRESS + f"/{VERSION}/user/set_tenant_info"
    authorization = {"Authorization": auth}
    tenant_info = {
        "tenant_id": tenant_id,
        "llm_id": "glm-4-flash@ZHIPU-AI",
        "embd_id": "BAAI/bge-small-en-v1.5@Builtin",
        "img2txt_id": "",
        "asr_id": "",
        "tts_id": None,
    }
    response = requests.post(url=url, headers=authorization, json=tenant_info)
    res = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
