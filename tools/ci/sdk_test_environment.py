#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

"""Provision test-owned tenants inside the disposable CI Compose stack."""

import base64
import os
from pathlib import Path
from uuid import uuid4

import requests
from Cryptodome.Cipher import PKCS1_v1_5
from Cryptodome.PublicKey import RSA

ROOT = Path(__file__).resolve().parents[2]
EMBEDDING = "BAAI/bge-small-en-v1.5@Local@Builtin"
CHAT = "glm-4-flash@CI@ZHIPU-AI"
UPDATE_CHAT = "glm-4-flashx@CI@ZHIPU-AI"
RERANK = "BAAI/bge-reranker-v2-m3@CI@SILICONFLOW"


class _API:
    def __init__(self, base_url):
        self.base_url = base_url.rstrip("/") + "/api/v1"
        self.session = requests.Session()

    def call(self, method, path, body=None):
        # Never print response bodies: provider errors may contain credentials.
        try:
            response = self.session.request(method, self.base_url + path, json=body, timeout=120)
            payload = response.json()
            if response.status_code != 200 or payload.get("code") != 0:
                raise RuntimeError(f"SDK environment setup failed: {method} {path} (HTTP {response.status_code}, API {payload.get('code')})")
        except (requests.RequestException, ValueError):
            raise RuntimeError(f"SDK environment setup failed: {method} {path}") from None
        return payload.get("data"), response.headers

    def close(self):
        self.session.close()


def _tenant(base_url):
    api = _API(base_url)
    try:
        cipher = PKCS1_v1_5.new(RSA.import_key((ROOT / "conf/public.pem").read_bytes()))
        password = base64.b64encode(cipher.encrypt(base64.b64encode(uuid4().hex.encode()))).decode()
        account = {"email": f"sdk-{uuid4().hex}@example.org", "nickname": "SDK CI", "password": password}
        api.call("POST", "/users", account)
        profile, headers = api.call("POST", "/auth/login", {"email": account["email"], "password": password})
        token = headers.get("Authorization")
        if not token or not isinstance(profile, dict) or not profile.get("id"):
            raise RuntimeError("SDK environment login did not return an identity and Authorization header")
        api.session.headers["Authorization"] = token
        key, _ = api.call("POST", "/system/tokens")
        if not isinstance(key, dict) or not key.get("token"):
            raise RuntimeError("SDK environment token creation did not return an API key")
        return api, profile["id"], key["token"]
    except Exception:
        api.close()
        raise


def _models(api, credentials):
    providers = (
        ("ZHIPU-AI", credentials["ZHIPU_AI_API_KEY"], "chat", ("glm-4-flash", "glm-4-flashx")),
        ("SILICONFLOW", credentials["SILICONFLOW_API_KEY"], "rerank", ("BAAI/bge-reranker-v2-m3",)),
    )
    for provider, key, kind, names in providers:
        api.call("PUT", "/providers", {"provider_name": provider})
        api.call(
            "POST",
            f"/providers/{provider}/instances",
            {
                "instance_name": "CI",
                "api_key": key,
                "base_url": "",
                "region": "default",
                "model_info": [{"model_type": [kind], "model_name": name, "max_tokens": 8192} for name in names],
            },
        )
    for kind, identifier in (("chat", CHAT), ("embedding", EMBEDDING), ("rerank", RERANK)):
        name, instance, provider = identifier.rsplit("@", 2)
        api.call(
            "PATCH",
            "/models/default",
            {
                "model_provider": provider,
                "model_instance": instance,
                "model_type": kind,
                "model_name": name,
            },
        )


def prepare_environment(folder_name):
    if os.getenv("CI") != "true" or not os.getenv("HOST_ADDRESS") or os.getenv("DOC_ENGINE") not in {"infinity", "elasticsearch"}:
        raise RuntimeError("SDK provisioning requires CI=true, explicit HOST_ADDRESS and a supported DOC_ENGINE in a disposable stack")
    credentials = {name: os.getenv("SDK_" + name) or os.getenv(name) for name in ("ZHIPU_AI_API_KEY", "SILICONFLOW_API_KEY")}
    missing = [name for name, value in credentials.items() if not value]
    if missing:
        raise RuntimeError("Full SDK integration tests require credentials: " + ", ".join(missing))
    base_url = os.environ["HOST_ADDRESS"].rstrip("/")
    engine = os.environ["DOC_ENGINE"]
    api, tenant_id, api_key = _tenant(base_url)
    try:
        _models(api, credentials)
        other, other_id, other_key = _tenant(base_url)
        other.close()
        env = {
            **credentials,
            "RAGFLOW_SDK_ISOLATED": "1",
            "RAGFLOW_SDK_BASE_URL": base_url,
            "RAGFLOW_SDK_API_KEY": api_key,
            "RAGFLOW_SDK_TENANT_ID": tenant_id,
            "RAGFLOW_SDK_OTHER_API_KEY": other_key,
            "RAGFLOW_SDK_OTHER_TENANT_ID": other_id,
            "RAGFLOW_SDK_DOC_ENGINE": engine,
            "RAGFLOW_SDK_EMBEDDING_MODEL": EMBEDDING,
            "RAGFLOW_SDK_MEMORY_EMBD_ID": EMBEDDING,
            "RAGFLOW_SDK_MEMORY_LLM_ID": CHAT,
            "RAGFLOW_SDK_MEMORY_UPDATE_LLM_ID": UPDATE_CHAT,
            "RAGFLOW_SDK_RERANK_ID": RERANK,
            "RAGFLOW_SDK_CHAT_CONFIGURED": "1",
        }
        if folder_name == "test_dataset_mangement":
            template, _ = api.call("GET", "/pipelines/general")
            pipeline, _ = api.call(
                "POST",
                "/agents",
                {
                    "title": "SDK CI ingestion",
                    "canvas_category": "dataflow_canvas",
                    "dsl": template["dsl"],
                },
            )
            version, _ = api.call("POST", f"/agents/{pipeline['id']}/publish", {})
            env.update(RAGFLOW_SDK_PIPELINE_ID=pipeline["id"], RAGFLOW_SDK_PIPELINE_VERSION_ID=version["id"])
        return env
    finally:
        api.close()
