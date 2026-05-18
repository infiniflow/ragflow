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

import os

import pytest
import requests

HOST_ADDRESS = os.getenv("HOST_ADDRESS", "http://127.0.0.1:9380")
ZHIPU_AI_API_KEY = os.getenv("ZHIPU_AI_API_KEY")
if ZHIPU_AI_API_KEY is None:
    pytest.exit("Error: Environment variable ZHIPU_AI_API_KEY must be set")

# def generate_random_email():
#     return 'user_' + ''.join(random.choices(string.ascii_lowercase + string.digits, k=8))+'@1.com'


def generate_email():
    return "user_123@1.com"


EMAIL = generate_email()
# password is "123"
PASSWORD = """ctAseGvejiaSWWZ88T/m4FQVOpQyUvP+x7sXtdv3feqZACiQleuewkUi35E16wSd5C5QcnkkcV9cYc8TKPTRZlxappDuirxghxoOvFcJxFU4ixLsD
fN33jCHRoDUW81IH9zjij/vaw8IbVyb6vuwg6MX6inOEBRRzVbRYxXOu1wkWY6SsI8X70oF9aeLFp/PzQpjoe/YbSqpTq8qqrmHzn9vO+yvyYyvmDsphXe
X8f7fp9c7vUsfOCkM+gHY3PadG+QHa7KI7mzTKgUTZImK6BZtfRBATDTthEUbbaTewY4H0MnWiCeeDhcbeQao6cFy1To8pE3RpmxnGnS8BsBn8w=="""


def register():
    url = HOST_ADDRESS + "/api/v1/users"
    name = "user"
    register_data = {"email": EMAIL, "nickname": name, "password": PASSWORD}
    res = requests.post(url=url, json=register_data)
    res = res.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))


def login():
    url = HOST_ADDRESS + "/api/v1/auth/login"
    login_data = {"email": EMAIL, "password": PASSWORD}
    response = requests.post(url=url, json=login_data)
    res = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    auth = response.headers["Authorization"]
    return auth


@pytest.fixture(scope="session")
def get_api_key_fixture():
    try:
        register()
    except Exception as e:
        print(e)
    auth = login()
    url = HOST_ADDRESS + "/v1/system/tokens"
    auth = {"Authorization": auth}
    response = requests.post(url=url, headers=auth)
    res = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    return res["data"].get("token")


@pytest.fixture(scope="session")
def get_auth():
    try:
        register()
    except Exception as e:
        print(e)
    auth = login()
    return auth


@pytest.fixture(scope="session")
def get_email():
    return EMAIL


def get_my_llms(auth, name):
    # todo deprecated
    url = HOST_ADDRESS + "/v1/llm/my_llms"
    authorization = {"Authorization": auth}
    response = requests.get(url=url, headers=authorization)
    res = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    if name in res.get("data"):
        return True
    return False


def get_added_models(auth, factory_name):
    url = HOST_ADDRESS + "/api/v1/models"
    authorization = {"Authorization": auth}
    response = requests.get(url=url, headers=authorization)
    res = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    added_factory = {model["provider_name"] for model in res.get("data", [])}
    if factory_name in added_factory:
        return True
    return False

def add_models(auth):
    # todo deprecated
    url = HOST_ADDRESS + "/v1/llm/set_api_key"
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


def add_model_instance(auth):
    add_provider_api = HOST_ADDRESS + "/api/v1/providers"
    authorization = {"Authorization": auth}
    add_provider_response = requests.put(url=add_provider_api, headers=authorization, json={"provider_name": "ZHIPU-AI"})
    add_provider_res = add_provider_response.json()
    if add_provider_res.get("code") != 0:
        pytest.exit(f"Critical error in add model provider: {add_provider_res.get('message')}")

    add_instance_api = HOST_ADDRESS + "/api/v1/providers/ZHIPU-AI/instances"
    add_instance_response = requests.post(url=add_instance_api, headers=authorization, json={
        "instance_name": "CI",
        "api_key": ZHIPU_AI_API_KEY,
        "region": "default",
        "base_url": ""
    })
    add_instance_res = add_instance_response.json()
    if add_instance_res.get("code") != 0:
        pytest.exit(f"Critical error in add model instance: {add_instance_res.get('message')}")

    add_success = get_added_models(auth, "ZHIPU-AI")
    if not add_success:
        pytest.exit("Critical error in check added model: add model failed")


def get_tenant_info(auth):
    # todo deprecated
    url = HOST_ADDRESS + "/api/v1/users/me/models"
    authorization = {"Authorization": auth}
    response = requests.get(url=url, headers=authorization)
    res = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    return res["data"].get("tenant_id")


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info(get_auth):
    auth = get_auth
    if not get_added_models(auth, "ZHIPU-AI"):
        try:
            add_model_instance(auth)
        except Exception as e:
            pytest.exit(f"Error in set_tenant_info: {str(e)}")
    url = HOST_ADDRESS + "/api/v1/models/default"
    authorization = {"Authorization": get_auth}
    # set chat model
    set_default_llm_response = requests.patch(
        url=url,
        headers=authorization,
        json={
            "model_provider": "ZHIPU-AI",
            "model_instance": "CI",
            "model_type": "chat",
            "model_name": "glm-4-flash"
        })
    llm_res = set_default_llm_response.json()
    if llm_res.get("code") != 0:
        raise Exception(llm_res.get("message"))
    # set embedding model
    set_default_embedding_response = requests.patch(
        url=url,
        headers=authorization,
        json={
            "model_provider": "Builtin",
            "model_instance": "Local",
            "model_type": "embedding",
            "model_name": "BAAI/bge-small-en-v1.5"
        })
    embd_res = set_default_embedding_response.json()
    if embd_res.get("code") != 0:
        raise Exception(embd_res.get("message"))
    # set image to text model
    set_default_img2txt_response = requests.patch(
        url=url,
        headers=authorization,
        json={
            "model_provider": "ZHIPU-AI",
            "model_instance": "CI",
            "model_type": "vision",
            "model_name": "glm-4v"
        })
    img2txt_res = set_default_img2txt_response.json()
    if img2txt_res.get("code") != 0:
        raise Exception(img2txt_res.get("message"))
