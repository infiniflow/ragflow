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
"""Memory-only fixtures; run with this folder's pytest.ini and --confcutdir."""

import os
from uuid import uuid4

import pytest
import requests
from ragflow_sdk import RAGFlow


@pytest.fixture(autouse=True)
def request_boundary(request, monkeypatch):
    send = requests.sessions.Session.request

    if request.node.get_closest_marker("integration") is None:

        def forbid_network(*args, **kwargs):
            pytest.fail("Service-free memory tests must not open a network connection")

        monkeypatch.setattr(requests.sessions.Session, "send", forbid_network)

    def bounded_request(session, *args, **kwargs):
        kwargs.setdefault("timeout", 30)
        return send(session, *args, **kwargs)

    monkeypatch.setattr(requests.sessions.Session, "request", bounded_request)


@pytest.fixture(scope="session")
def integration_settings():
    required = ("RAGFLOW_SDK_BASE_URL", "RAGFLOW_SDK_API_KEY", "RAGFLOW_SDK_TENANT_ID")
    if os.getenv("RAGFLOW_SDK_ISOLATED") != "1" or any(not os.getenv(key) for key in required):
        pytest.fail(
            "Memory integrations require a verified disposable Go backend and dedicated empty tenant: "
            "RAGFLOW_SDK_ISOLATED=1, RAGFLOW_SDK_BASE_URL, RAGFLOW_SDK_API_KEY, RAGFLOW_SDK_TENANT_ID. "
            "Run service-free tests with -m 'not integration'."
        )
    if os.getenv("PYTEST_XDIST_WORKER"):
        pytest.fail("Memory integrations require serial execution in a dedicated tenant")
    return dict(zip(("base_url", "api_key", "tenant_id"), (os.environ[key] for key in required)))


@pytest.fixture(scope="session")
def isolated_tenant(integration_settings):
    rag = RAGFlow(integration_settings["api_key"], integration_settings["base_url"])
    response = requests.get(f"{rag.api_url}/memories", headers=rag.authorization_header, params={"page_size": 1}, timeout=30)
    response.raise_for_status()
    payload = response.json()
    assert payload["code"] == 0, payload
    assert payload["data"]["total_count"] == 0, "Refusing integrations: tenant has accessible memories"
    assert payload["data"]["memory_list"] == [], payload
    return integration_settings


@pytest.fixture
def model_ids():
    keys = {"embd_id": "RAGFLOW_SDK_MEMORY_EMBD_ID", "llm_id": "RAGFLOW_SDK_MEMORY_LLM_ID"}
    if any(not os.getenv(key) for key in keys.values()):
        pytest.fail("Configure memory model identifiers explicitly: " + ", ".join(keys.values()))
    return {field: os.environ[key] for field, key in keys.items()}


@pytest.fixture
def client(isolated_tenant, monkeypatch):
    rag = RAGFlow(isolated_tenant["api_key"], isolated_tenant["base_url"])
    owned = []
    create = rag.create_memory

    def tracked_create(*args, **kwargs):
        memory = create(*args, **kwargs)
        owned.append(memory.id)
        assert memory.tenant_id == isolated_tenant["tenant_id"], "Created memory belongs to unexpected tenant"
        return memory

    monkeypatch.setattr(rag, "create_memory", tracked_create)
    yield rag
    errors = []
    for memory_id in owned:
        try:
            payload = rag.get(f"/memories/{memory_id}/config").json()
            if payload["code"] == 0:
                rag.delete_memory(memory_id)
            else:
                assert payload["code"] == 404, payload
            payload = rag.get(f"/memories/{memory_id}/config").json()
            assert payload["code"] == 404, payload
        except Exception as exc:  # SDK raises plain Exception; report all cleanup failures.
            errors.append(f"{memory_id}: {exc}")
    assert not errors, "Owned memory cleanup failed: " + "; ".join(errors)


@pytest.fixture
def memory_factory(client, model_ids):
    def create(name=None, memory_type=None, **overrides):
        payload = {
            "name": name if name is not None else f"sdk-memory-{uuid4().hex}",
            "memory_type": memory_type if memory_type is not None else ["raw"],
            **model_ids,
            **overrides,
        }
        return client.create_memory(**payload)

    return create


@pytest.fixture
def memory(memory_factory):
    return memory_factory()


@pytest.fixture
def memory_group(memory_factory):
    prefix = f"sdk-memory-{uuid4().hex}"
    types = (["raw"], ["raw", "semantic"], ["raw", "episodic", "procedural"])
    return prefix, [memory_factory(name=f"{prefix}-{i}", memory_type=value) for i, value in enumerate(types)]
