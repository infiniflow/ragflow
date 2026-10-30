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
"""Run with this folder's pytest.ini and --confcutdir to exclude live ancestor hooks."""

import json
import os
from uuid import uuid4

import pytest
import requests
from ragflow_sdk import RAGFlow

from test.testcases.test_sdk_api.test_message_management.message_helpers import assert_identity, wait_until


@pytest.fixture(autouse=True)
def request_boundary(request, monkeypatch):
    send = requests.sessions.Session.request
    if request.node.get_closest_marker("integration") is None:

        def forbid_network(*args, **kwargs):
            pytest.fail("Service-free message tests must not open a network connection")

        monkeypatch.setattr(requests.sessions.Session, "send", forbid_network)

    def bounded_request(session, *args, **kwargs):
        kwargs.setdefault("timeout", 30)
        return send(session, *args, **kwargs)

    monkeypatch.setattr(requests.sessions.Session, "request", bounded_request)


@pytest.fixture(scope="session")
def integration_settings():
    keys = ("RAGFLOW_SDK_BASE_URL", "RAGFLOW_SDK_API_KEY", "RAGFLOW_SDK_TENANT_ID")
    if os.getenv("RAGFLOW_SDK_ISOLATED") != "1" or any(not os.getenv(key) for key in keys):
        pytest.fail(
            "Message integrations require a verified disposable Go backend and dedicated empty tenant: "
            "RAGFLOW_SDK_ISOLATED=1, " + ", ".join(keys) + ". Run service-free tests with -m 'not integration'."
        )
    if os.getenv("PYTEST_XDIST_WORKER"):
        pytest.fail("Message integrations require serial execution in a dedicated tenant")
    return dict(zip(("base_url", "api_key", "tenant_id"), (os.environ[key] for key in keys)))


@pytest.fixture(scope="session")
def isolated_tenant(integration_settings):
    rag = RAGFlow(integration_settings["api_key"], integration_settings["base_url"])
    response = requests.get(f"{rag.api_url}/memories", headers=rag.authorization_header, params={"page_size": 1}, timeout=30)
    response.raise_for_status()
    payload = response.json()
    assert payload["code"] == 0, payload
    result = payload["data"]
    assert result["total_count"] == 0 and result["memory_list"] == [], "Refusing integration setup: tenant has accessible memories"
    return integration_settings


@pytest.fixture
def client(isolated_tenant, monkeypatch, request):
    rag = RAGFlow(isolated_tenant["api_key"], isolated_tenant["base_url"])
    owned = []
    create = rag.create_memory

    def cleanup():
        errors = []
        for memory_id in reversed(owned):
            try:
                result = rag.get(f"/memories/{memory_id}/config").json()
                if result["code"] == 0:
                    rag.delete_memory(memory_id)
                else:
                    assert result["code"] == 404, result
                result = rag.get(f"/memories/{memory_id}/config").json()
                assert result["code"] == 404, result
            except Exception as exc:
                errors.append(f"{memory_id}: {exc}")
        assert not errors, "Owned memory cleanup failed: " + "; ".join(errors)

    request.addfinalizer(cleanup)

    def tracked_create(*args, **kwargs):
        memory = create(*args, **kwargs)
        owned.append(memory.id)
        assert memory.tenant_id == isolated_tenant["tenant_id"], "Created memory belongs to unexpected tenant"
        return memory

    monkeypatch.setattr(rag, "create_memory", tracked_create)
    return rag


@pytest.fixture
def memory_factory(client):
    def create(memory_type=None):
        keys = {"embd_id": "RAGFLOW_SDK_MEMORY_EMBD_ID", "llm_id": "RAGFLOW_SDK_MEMORY_LLM_ID"}
        if any(not os.getenv(key) for key in keys.values()):
            pytest.fail("Configure memory models explicitly: " + ", ".join(keys.values()))
        return client.create_memory(name=f"sdk-messages-{uuid4().hex}", memory_type=memory_type or ["raw"], **{field: os.environ[key] for field, key in keys.items()})

    return create


@pytest.fixture
def populated_memory(client, memory_factory):
    memory = memory_factory()
    namespace = uuid4().hex
    conversations = [
        ("What is coriander?", "Coriander leaves are called cilantro and used as an herb."),
        ("Which parts of coriander are edible?", "Coriander seeds are a spice; its leaves are cilantro."),
        ("What is a pineapple?", "Pineapple is a sweet tropical fruit with a spiky rind."),
        ("What does a pineapple symbolize?", "Pineapple is a symbol of hospitality and welcome."),
        ("Where is Mount Everest?", "Mount Everest is a mountain in the Himalayas."),
    ]
    payloads = []
    rows = []
    for i, (question, answer) in enumerate(conversations):
        payload = dict(
            memory_id=[memory.id], agent_id=f"{namespace}-agent-{i % 2}", session_id=f"{namespace}-session-{i}", user_id=f"{namespace}-user-{i % 2}", user_input=question, agent_response=answer
        )
        assert client.add_message(**payload) == "All add to task."
        result = wait_until(memory.list_memory_messages, lambda value: value["messages"]["total_count"] == i + 1)
        matches = [row for row in result["messages"]["message_list"] if row["session_id"] == payload["session_id"]]
        assert len(matches) == 1, result
        assert_identity(matches[0], payload, memory.id)
        payloads.append(payload)
        rows.append(matches[0])
    assert len({str(row["message_id"]) for row in rows}) == 5
    return {"memory": memory, "rows": rows, "payloads": payloads}


@pytest.fixture
def mock_api(monkeypatch):
    """Exercise actual SDK HTTP serialization without a server."""
    calls = []
    responses = []

    def respond(payload, status=200):
        responses.append((payload, status))
        return calls

    def send(session, prepared, **kwargs):
        assert kwargs["timeout"] == 30
        calls.append(prepared)
        assert responses, "Unexpected SDK HTTP request"
        payload, status = responses.pop(0)
        response = requests.Response()
        response.status_code = status
        response._content = json.dumps(payload).encode()
        response.headers["Content-Type"] = "application/json"
        response.request = prepared
        return response

    monkeypatch.setattr(requests.sessions.Session, "send", send)
    yield respond
    assert not responses, "Expected SDK requests were not sent"


@pytest.fixture
def doc_engine(client):
    result = client.get("/system/status").json()
    assert result["code"] == 0, result
    engine = result["data"]["doc_engine"]["type"].lower()
    assert engine, result
    return engine
