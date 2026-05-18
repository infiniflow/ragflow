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

import importlib
import sys
import types


def _make_stub_getattr(module_name):
    def __getattr__(attr_name):
        message = f"{module_name}.{attr_name} is stubbed in tests"

        class _Stub:
            def __init__(self, *_args, **_kwargs):
                raise RuntimeError(message)

            def __call__(self, *_args, **_kwargs):
                raise RuntimeError(message)

            def __getattr__(self, _name):
                raise RuntimeError(message)

        setattr(sys.modules[module_name], attr_name, _Stub)
        return _Stub

    return __getattr__


def _install_rag_llm_stubs():
    rag_llm = sys.modules.get("rag.llm")
    if rag_llm is not None and getattr(rag_llm, "_rag_llm_stubbed", False):
        return

    try:
        rag_pkg = importlib.import_module("rag")
    except Exception:
        rag_pkg = types.ModuleType("rag")
        rag_pkg.__path__ = []
        rag_pkg.__package__ = "rag"
        rag_pkg.__file__ = __file__
        sys.modules["rag"] = rag_pkg

    llm_pkg = types.ModuleType("rag.llm")
    llm_pkg.__path__ = []
    llm_pkg.__package__ = "rag.llm"
    llm_pkg.__file__ = __file__
    sys.modules["rag.llm"] = llm_pkg
    rag_pkg.llm = llm_pkg

    llm_pkg.__getattr__ = _make_stub_getattr("rag.llm")

    for submodule in ("cv_model", "chat_model"):
        full_name = f"rag.llm.{submodule}"
        sub_mod = sys.modules.get(full_name)
        if sub_mod is None or not isinstance(sub_mod, types.ModuleType):
            sub_mod = types.ModuleType(full_name)
            sys.modules[full_name] = sub_mod
        sub_mod.__package__ = "rag.llm"
        sub_mod.__file__ = __file__
        sub_mod.__getattr__ = _make_stub_getattr(full_name)
        setattr(llm_pkg, submodule, sub_mod)

    llm_pkg._rag_llm_stubbed = True


def _install_scholarly_stub():
    if "scholarly" in sys.modules:
        return
    stub = types.ModuleType("scholarly")

    def _stub(*_args, **_kwargs):
        raise RuntimeError("scholarly is stubbed in tests")

    stub.scholarly = _stub
    sys.modules["scholarly"] = stub


_install_rag_llm_stubs()
_install_scholarly_stub()

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
    url = HOST_ADDRESS + f"/api/{VERSION}/users"
    name = "qa"
    register_data = {"email": EMAIL, "nickname": name, "password": PASSWORD}
    res = requests.post(url=url, json=register_data)
    res = res.json()
    if res.get("code") != 0 and "has already registered" not in res.get("message"):
        raise Exception(res.get("message"))


def login():
    url = HOST_ADDRESS + f"/api/{VERSION}/auth/login"
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
    url = HOST_ADDRESS + f"/api/{VERSION}/system/tokens"
    auth = {"Authorization": auth}
    response = requests.post(url=url, headers=auth)
    res = response.json()
    if res.get("code") != 0:
        error_msg = f"access: {url}, POST method, error code: {res.get('code')}, message: {res.get('message')}"
        raise Exception(error_msg)
    return res["data"].get("token")


def get_my_llms(auth, name):
    # todo deprecated
    url = HOST_ADDRESS + f"/{VERSION}/llm/my_llms"
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
    url = HOST_ADDRESS + f"/api/{VERSION}/users/me/models"
    authorization = {"Authorization": auth}
    response = requests.get(url=url, headers=authorization)
    res = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    return res["data"].get("tenant_id")


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info(auth):
    if not get_added_models(auth, "ZHIPU-AI"):
        try:
            add_model_instance(auth)
        except Exception as e:
            pytest.exit(f"Error in set_tenant_info: {str(e)}")
    url = HOST_ADDRESS + "/api/v1/models/default"
    authorization = {"Authorization": auth}
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
