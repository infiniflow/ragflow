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
from ragflow_sdk import Chat
from ragflow_sdk.modules.session import Session


class _StubResponse:
    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


class _StubRag:
    def __init__(self, post_payload=None, get_payload=None, delete_payload=None, put_payload=None):
        self._post_payload = post_payload or {}
        self._get_payload = get_payload or {}
        self._delete_payload = delete_payload or {}
        self._put_payload = put_payload or {}

    def post(self, _path, _json=None, stream=False, files=None):
        return _StubResponse(self._post_payload)

    def get(self, _path, _params=None):
        return _StubResponse(self._get_payload)

    def delete(self, _path, _json=None):
        return _StubResponse(self._delete_payload)

    def put(self, _path, _json=None):
        return _StubResponse(self._put_payload)


@pytest.mark.p2
def test_chat_update_validation_errors():
    chat = Chat(_StubRag(), {"id": "chat_id"})

    with pytest.raises(Exception) as excinfo:
        chat.update("invalid")
    assert "update_message" in str(excinfo.value).lower()

    with pytest.raises(Exception) as excinfo:
        chat.update({"llm": {}})
    assert "llm" in str(excinfo.value).lower()

    with pytest.raises(Exception) as excinfo:
        chat.update({"prompt": {}})
    assert "prompt" in str(excinfo.value).lower()


@pytest.mark.p2
def test_chat_session_methods_success_and_error():
    rag = _StubRag(
        post_payload={"code": 0, "data": {"id": "sess_id", "chat_id": "chat_id"}},
        get_payload={"code": 0, "data": [{"id": "sess_id", "chat_id": "chat_id"}]},
        delete_payload={"code": 1, "message": "boom"},
    )
    chat = Chat(rag, {"id": "chat_id"})

    session = chat.create_session()
    assert isinstance(session, Session)

    sessions = chat.list_sessions()
    assert isinstance(sessions, list)
    assert sessions and isinstance(sessions[0], Session)

    with pytest.raises(Exception) as excinfo:
        chat.delete_sessions(ids=["sess_id"])
    assert "boom" in str(excinfo.value)
