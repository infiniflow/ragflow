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

import pytest
from ragflow_sdk import RAGFlow
from ragflow_sdk.modules.chat import Chat
from ragflow_sdk.modules.session import Session


class _DummyResponse:
    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


@pytest.mark.p2
def test_chat_create_session_raises_server_error_message(monkeypatch):
    client = RAGFlow("token", "http://localhost:9380")
    chat = Chat(client, {"id": "chat-1"})

    monkeypatch.setattr(
        chat,
        "post",
        lambda *_args, **_kwargs: _DummyResponse({"code": 102, "message": "`name` can not be empty."}),
    )

    with pytest.raises(Exception) as exception_info:
        chat.create_session(name="")
    assert "`name` can not be empty." in str(exception_info.value), str(exception_info.value)


@pytest.mark.p2
def test_chat_list_sessions_preserves_zero_page_size_semantics(monkeypatch):
    client = RAGFlow("token", "http://localhost:9380")
    chat = Chat(client, {"id": "chat-1"})
    calls = []

    def _ok_get(path, params=None):
        calls.append((path, params))
        return _DummyResponse(
            {
                "code": 0,
                "data": [
                    {"id": "session-1", "chat_id": "chat-1", "name": "one"},
                    {"id": "session-2", "chat_id": "chat-1", "name": "two"},
                ],
            }
        )

    monkeypatch.setattr(chat, "get", _ok_get)

    sessions = chat.list_sessions(page=2, page_size=2, orderby="create_time", desc=False, id="session-1", name="one")
    assert len(sessions) == 2, str(sessions)
    assert all(isinstance(item, Session) for item in sessions), str(sessions)
    assert calls[-1][0] == "/chats/chat-1/sessions"
    assert calls[-1][1]["page_size"] == 2
    assert calls[-1][1]["name"] == "one"

    empty_sessions = chat.list_sessions(page_size=0)
    assert empty_sessions == [], str(empty_sessions)
    assert calls[-1][1]["page_size"] == 0
