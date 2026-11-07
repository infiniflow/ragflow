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
import os
from uuid import uuid4

import pytest
import requests
from ragflow_sdk import RAGFlow


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    # Session CRUD needs no models; override the ancestor's live model configuration.
    return None


@pytest.fixture(autouse=True)
def network_boundary(request, monkeypatch):
    original = requests.sessions.Session.request

    def guarded(session, method, url, **kwargs):
        if request.node.get_closest_marker("integration") is None:
            pytest.fail(f"Service-free test attempted HTTP: {method} {url}")
        kwargs.setdefault("timeout", 30)
        return original(session, method, url, **kwargs)

    monkeypatch.setattr(requests.sessions.Session, "request", guarded)


@pytest.fixture
def client():
    required = ("RAGFLOW_SDK_BASE_URL", "RAGFLOW_SDK_API_KEY")
    if os.environ.get("RAGFLOW_SDK_ISOLATED") != "1" or not all(os.environ.get(key) for key in required):
        pytest.fail("Environment blocked: requires RAGFLOW_SDK_ISOLATED=1, RAGFLOW_SDK_BASE_URL and RAGFLOW_SDK_API_KEY for a disposable Go backend and dedicated empty tenant")
    client = RAGFlow(os.environ["RAGFLOW_SDK_API_KEY"], os.environ["RAGFLOW_SDK_BASE_URL"])
    assert not client.list_chats(page_size=1), "Dedicated tenant must start without chats"
    assert not client.list_datasets(page_size=1), "Dedicated tenant must start without datasets"
    return client


@pytest.fixture
def chat(client):
    chat = client.create_chat(name=f"sdk-session-{uuid4().hex}", prompt_config={"prologue": "session audit greeting"})
    try:
        yield chat
    finally:
        if client.list_chats(id=chat.id):
            sessions = chat.list_sessions(page_size=-1)
            if sessions:
                chat.delete_sessions(ids=[session.id for session in sessions])
            assert chat.list_sessions(page_size=-1) == [], "Test-owned session cleanup did not persist"
            client.delete_chats(ids=[chat.id])
        assert not client.list_chats(id=chat.id), "Test-owned chat cleanup did not persist"


@pytest.fixture
def add_sessions_with_chat_assistant(chat):
    return chat, [chat.create_session(name=f"session_with_chat_assistant_{i}") for i in range(5)]
