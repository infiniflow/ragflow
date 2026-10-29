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
"""Chat-only fixtures: explicit isolation, bounded requests and owned cleanup."""

import os
from uuid import uuid4

import pytest
import requests
from ragflow_sdk import RAGFlow


@pytest.fixture(autouse=True)
def request_boundary(request, monkeypatch):
    send = requests.sessions.Session.request

    def bounded_request(session, *args, **kwargs):
        if request.node.get_closest_marker("integration") is None:
            pytest.fail("Service-free chat tests must not open a network connection")
        kwargs.setdefault("timeout", 30)
        return send(session, *args, **kwargs)

    monkeypatch.setattr(requests.sessions.Session, "request", bounded_request)


@pytest.fixture(scope="session")
def integration_settings():
    required = ("RAGFLOW_SDK_BASE_URL", "RAGFLOW_SDK_API_KEY")
    if os.getenv("RAGFLOW_SDK_ISOLATED") != "1" or any(not os.getenv(key) for key in required):
        pytest.fail(
            "Integration environment required: set RAGFLOW_SDK_ISOLATED=1, "
            "RAGFLOW_SDK_BASE_URL and RAGFLOW_SDK_API_KEY for a dedicated, empty Go test tenant. "
            "Run service-free tests with -m 'not integration'."
        )
    if os.getenv("PYTEST_XDIST_WORKER"):
        pytest.fail("Chat integrations require a dedicated tenant and serial execution")
    return {"base_url": os.environ[required[0]], "api_key": os.environ[required[1]]}


@pytest.fixture(scope="session")
def isolated_tenant(integration_settings):
    # The dedicated-tenant declaration is mandatory; emptiness alone is not isolation.
    probe = RAGFlow(integration_settings["api_key"], integration_settings["base_url"])
    response = requests.get(f"{probe.api_url}/chats", headers=probe.authorization_header, params={"page_size": 1}, timeout=30)
    response.raise_for_status()
    payload = response.json()
    assert payload["code"] == 0, payload
    assert payload["data"]["total"] == 0, "Refusing integrations: tenant has accessible chats"
    response = requests.get(f"{probe.api_url}/datasets", headers=probe.authorization_header, params={"page_size": 1}, timeout=30)
    response.raise_for_status()
    payload = response.json()
    assert payload["code"] == 0, payload
    assert payload["data"] == [], "Refusing integrations: tenant has accessible datasets"
    return integration_settings


@pytest.fixture
def client(isolated_tenant, monkeypatch):
    rag = RAGFlow(isolated_tenant["api_key"], isolated_tenant["base_url"])
    chats, datasets = [], []
    create_chat, create_dataset = rag.create_chat, rag.create_dataset

    def tracked_chat(*args, **kwargs):
        chat = create_chat(*args, **kwargs)
        chats.append(chat.id)
        return chat

    def tracked_dataset(*args, **kwargs):
        dataset = create_dataset(*args, **kwargs)
        datasets.append(dataset.id)
        return dataset

    monkeypatch.setattr(rag, "create_chat", tracked_chat)
    monkeypatch.setattr(rag, "create_dataset", tracked_dataset)
    yield rag
    # Continue cleaning other owned resources if one cleanup operation fails.
    errors = []
    for chat_id in chats:
        try:
            if rag.list_chats(id=chat_id):
                rag.delete_chats(ids=[chat_id])
            assert rag.list_chats(id=chat_id) == [], f"Chat cleanup failed: {chat_id}"
        except Exception as exc:  # noqa: BLE001 - SDK raises plain Exception; report every cleanup failure.
            errors.append(str(exc))
    for dataset_id in datasets:
        try:
            if rag.list_datasets(id=dataset_id):
                rag.delete_datasets(ids=[dataset_id])
            assert rag.list_datasets(id=dataset_id) == [], f"Dataset cleanup failed: {dataset_id}"
        except Exception as exc:  # noqa: BLE001 - SDK raises plain Exception; report every cleanup failure.
            errors.append(str(exc))
    assert not errors, "Owned resource cleanup failed: " + "; ".join(errors)


@pytest.fixture
def chat_factory(client):
    def create(name=None, **kwargs):
        return client.create_chat(name=name if name is not None else f"sdk-chat-{uuid4().hex}", **kwargs)

    return create


@pytest.fixture
def dataset_factory(client):
    def create(name=None, **kwargs):
        return client.create_dataset(name=name if name is not None else f"sdk-chat-dataset-{uuid4().hex}", **kwargs)

    return create
