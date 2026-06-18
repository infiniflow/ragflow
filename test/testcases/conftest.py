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
from configs import EMAIL, HOST_ADDRESS, PASSWORD, VERSION, ZHIPU_AI_API_KEY, SILICONFLOW_API_KEY

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


def get_added_models(auth, factory_name):
    url = HOST_ADDRESS + "/api/v1/models"
    authorization = {"Authorization": auth}
    response = requests.get(url=url, headers=authorization)
    res = response.json()
    if res.get("code") != 0:
        raise Exception(res.get("message"))
    # Go server (post-Python port) serializes this field as `model_provider`
    # in the RESTful `/api/v1/models` response. Fall back to the legacy
    # `provider_name` key so this conftest works against both.
    added_factory = {
        model.get("model_provider") or model["provider_name"]
        for model in res.get("data", [])
    }
    if factory_name in added_factory:
        return True
    return False


def add_model_instance(auth):
    add_provider_api = HOST_ADDRESS + "/api/v1/providers"
    authorization = {"Authorization": auth}

    # Tracks providers that already existed in the catalog before this test
    # run. Their user-tenant_llm binding is whatever was last configured for
    # this user; the final assertion is downgraded to a warning in that
    # case to keep the suite runnable in partially-seeded environments.
    provider_already_existed = set()

    providers = [
        ("ZHIPU-AI", ZHIPU_AI_API_KEY),
        ("SILICONFLOW", SILICONFLOW_API_KEY),
    ]

    for provider_name, api_key in providers:
        if not get_added_models(auth, provider_name):
            add_provider_response = requests.put(url=add_provider_api, headers=authorization, json={"provider_name": provider_name})
            add_provider_res = add_provider_response.json()
            if add_provider_res.get("code") != 0:
                msg = add_provider_res.get("message", "")
                # Provider may already exist in the catalog from a prior run
                # or admin setup but not yet appear in this tenant's
                # `/api/v1/models` listing — treat as success and continue
                # to the instance step. The final assertion below will be
                # downgraded to a warning in that case so the test can run.
                if "duplicated" in msg.lower() or "already exist" in msg.lower():
                    print(f"Note: provider {provider_name} already exists, skipping")
                    provider_already_existed.add(provider_name)
                else:
                    pytest.exit(f"Critical error in add model provider: {msg}")

        # Register "CI" (used by glm-4-flash@CI@ZHIPU-AI in configs.py
        # and BAAI/bge-reranker-v2-m3@CI@SILICONFLOW).
        instance_name = "CI"
        add_instance_api = HOST_ADDRESS + f"/api/v1/providers/{provider_name}/instances"
        add_instance_response = requests.post(url=add_instance_api, headers=authorization, json={
            "instance_name": instance_name,
            "api_key": api_key,
            "region": "default",
            "base_url": ""
        })
        add_instance_res = add_instance_response.json()
        if add_instance_res.get("code") != 0:
            msg = add_instance_res.get("message", "")
            # Instance may already exist with a different API key from a
            # prior test run; that's fine — skip instead of failing.
            if "Already exist instance" in msg or "already exist" in msg.lower():
                print(f"Note: {provider_name}/{instance_name} already exists, skipping")
                continue
            # Python API blocks creating instances named "default".
            # The test_retrieval_parity test handles this by inserting
            # "default" directly into the DB for SILICONFLOW.
            if "cannot be 'default'" in msg:
                print(f"Note: {provider_name}/{instance_name} blocked by API (name reserved), skipping")
                continue
            pytest.exit(
                f"Critical error in add model instance {provider_name}/{instance_name}: "
                f"{msg}"
            )

        add_success = get_added_models(auth, provider_name)
        if not add_success:
            if provider_name in provider_already_existed:
                # The provider/instances were already there from a prior run
                # but this user's tenant_llm binding is missing — the Go
                # server (post-Python port) doesn't auto-create the binding
                # on PUT. Downgrade to a warning so tests that don't depend
                # on the model can still run; tests that do will fail with
                # a real error rather than this opaque setup crash.
                print(
                    f"WARNING: {provider_name} already exists in catalog but "
                    f"missing from this tenant's /api/v1/models. Tests that "
                    f"depend on {provider_name} may fail."
                )
                continue
            pytest.exit(f"Critical error in check added model: {provider_name} add model failed")


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info(auth):
    if not get_added_models(auth, "ZHIPU-AI") or not get_added_models(auth, "SILICONFLOW"):
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
        # The Go server (post-Python port) doesn't yet implement
        # PATCH /api/v1/models/default, so the chat/embedding default
        # can't be set via API. Downgrade to a warning so tests that
        # don't rely on a default LLM can still run; tests that do
        # will fail with their own real error.
        print(
            f"WARNING: failed to set default chat LLM via {url}: "
            f"{llm_res.get('message')!r}. Continuing."
        )
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
        print(
            f"WARNING: failed to set default embedding LLM via {url}: "
            f"{embd_res.get('message')!r}. Continuing."
        )
    # set rerank model
    set_default_rerank_response = requests.patch(
        url=url,
        headers=authorization,
        json={
            "model_provider": "SILICONFLOW",
            "model_instance": "CI",
            "model_type": "rerank",
            "model_name": "BAAI/bge-reranker-v2-m3"
        }
    )
    rerank_res = set_default_rerank_response.json()
    if rerank_res.get("code") != 0:
        print(
            f"WARNING: failed to set default rerank LLM via {url}: "
            f"{rerank_res.get('message')!r}. Continuing."
        )
