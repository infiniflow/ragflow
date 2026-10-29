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
from concurrent.futures import ThreadPoolExecutor
from ragflow_sdk import RAGFlow
from ragflow_sdk.modules.session import Message, Session


@pytest.mark.integration
class TestSessionsWithChatAssistantList:
    @pytest.mark.parametrize("page, offset", [(None, 0), (0, 0), (2, 2), (3, 4), ("3", 4), (-1, 0), ("a", 0)])
    def test_page(self, add_sessions_with_chat_assistant, page, offset):
        chat, sessions = add_sessions_with_chat_assistant
        ordered = sorted(sessions, key=lambda item: item.name)
        result = chat.list_sessions(page=page, page_size=2, orderby="name", desc=False)
        assert [item.id for item in result] == [item.id for item in ordered[offset : offset + 2]]

    @pytest.mark.parametrize("page_size, count", [(None, 5), (0, 0), (1, 1), (6, 5), ("1", 1), (-1, 5), ("a", 5)])
    def test_page_size(self, add_sessions_with_chat_assistant, page_size, count):
        chat, sessions = add_sessions_with_chat_assistant
        result = chat.list_sessions(page_size=page_size, orderby="name", desc=False)
        assert [item.id for item in result] == [item.id for item in sessions[:count]]

    @pytest.mark.parametrize("orderby, desc", [(None, True), ("create_time", True), ("update_time", True), ("name", "False"), ("unknown", True)])
    def test_orderby(self, add_sessions_with_chat_assistant, orderby, desc):
        chat, sessions = add_sessions_with_chat_assistant
        if orderby == "unknown":
            with pytest.raises(Exception, match="invalid orderby field: unknown"):
                chat.list_sessions(orderby=orderby)
            return
        result = chat.list_sessions(orderby=orderby, desc=desc)
        assert {item.id for item in result} == {item.id for item in sessions}
        key = orderby or "create_time"
        values = [getattr(item, key) for item in result]
        assert values == sorted(values, reverse=desc != "False")

    @pytest.mark.parametrize(
        "desc, descending, orderby",
        [
            (None, True, "name"),
            ("true", True, "name"),
            ("True", True, "name"),
            (True, True, "name"),
            ("false", False, "name"),
            ("False", False, "name"),
            (False, False, "name"),
            ("False", False, "update_time"),
            ("unknown", True, "name"),
        ],
    )
    def test_desc(self, add_sessions_with_chat_assistant, desc, descending, orderby):
        chat, sessions = add_sessions_with_chat_assistant
        result = chat.list_sessions(orderby=orderby, desc=desc)
        assert {item.id for item in result} == {item.id for item in sessions}
        values = [getattr(item, orderby) for item in result]
        assert values == sorted(values, reverse=descending)

    @pytest.mark.parametrize("name", [None, "", "session_with_chat_assistant_1", "unknown"])
    def test_name(self, add_sessions_with_chat_assistant, name):
        chat, sessions = add_sessions_with_chat_assistant
        result = chat.list_sessions(name=name)
        expected = [item for item in sessions if not name or item.name == name]
        assert {item.id for item in result} == {item.id for item in expected}
        assert all(not name or item.name == name for item in result)

    @pytest.mark.parametrize("session_id", [None, "", "existing", "unknown"])
    def test_id(self, add_sessions_with_chat_assistant, session_id):
        chat, sessions = add_sessions_with_chat_assistant
        if session_id == "existing":
            session_id = sessions[0].id
        result = chat.list_sessions(id=session_id)
        assert {item.id for item in result} == {item.id for item in sessions if not session_id or item.id == session_id}

    @pytest.mark.parametrize("existing_id, name", [(True, "session_with_chat_assistant_0"), (True, "session_with_chat_assistant_100"), (True, "unknown"), (False, "session_with_chat_assistant_0")])
    def test_name_and_id(self, add_sessions_with_chat_assistant, existing_id, name):
        chat, sessions = add_sessions_with_chat_assistant
        session_id = sessions[0].id if existing_id else "unknown"
        result = chat.list_sessions(id=session_id, name=name)
        assert {item.id for item in result} == {item.id for item in sessions if item.id == session_id and item.name == name}

    def test_concurrent_list(self, add_sessions_with_chat_assistant):
        chat, sessions = add_sessions_with_chat_assistant
        expected = {item.id for item in sessions}
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(chat.list_sessions) for _ in range(100)]
            for future in futures:
                assert {item.id for item in future.result()} == expected

    def test_list_chats_after_deleting_associated_chat_assistant(self, client, add_sessions_with_chat_assistant):
        chat, _ = add_sessions_with_chat_assistant
        client.delete_chats(ids=[chat.id])
        with pytest.raises(Exception, match="^Resource not found$"):
            chat.list_sessions()

    @pytest.mark.skip(reason="Go ListChatSessions never reads user_id or filters by it (internal/handler/chat_session.go); remove when documented user_id filtering is implemented")
    @pytest.mark.parametrize("combined", [False, True])
    def test_user_id(self, add_sessions_with_chat_assistant, combined):
        chat, sessions = add_sessions_with_chat_assistant
        filters = {"id": sessions[0].id, "name": sessions[0].name} if combined else {}
        assert chat.list_sessions(user_id="nonexistent-session-user", **filters) == []

    @pytest.mark.skip(
        reason="Go list serializes entity.ChatSession as dialog_id/message, but SDK Session requires chat_id/messages (internal/handler/chat_session.go and sdk/python/ragflow_sdk/modules/session.py); remove when list returns SDK session schema"
    )
    def test_list_response_session_contract(self, chat):
        created = chat.create_session()
        listed = chat.list_sessions(id=created.id)
        assert len(listed) == 1
        assert listed[0].chat_id == chat.id
        assert listed[0].messages == created.messages
        listed[0].update({"name": "updated from listed session"})
        assert chat.list_sessions(id=created.id)[0].name == "updated from listed session"


def test_list_sessions_raises_on_nonzero_response(monkeypatch):
    from ragflow_sdk import Chat

    chat = Chat(None, {"id": "chat-1"})

    class Response:
        def json(self):
            return {"code": 1, "message": "boom"}

    monkeypatch.setattr(chat, "get", lambda *_args, **_kwargs: Response())
    with pytest.raises(Exception, match="boom"):
        chat.list_sessions()


@pytest.mark.p2
def test_session_module_error_paths_unit(monkeypatch):
    client = RAGFlow("token", "http://unit.invalid")

    unknown_session = Session(client, {"id": "session-unknown", "chat_id": "chat-1"})
    unknown_session._Session__session_type = "unknown"  # noqa: SLF001
    with pytest.raises(Exception) as exception_info:
        list(unknown_session.ask("hello", stream=False))
    assert "Unknown session type" in str(exception_info.value)

    bad_json_session = Session(client, {"id": "session-bad-json", "chat_id": "chat-1"})

    class _BadJsonResponse:
        def json(self):
            raise ValueError("json decode failed")

    monkeypatch.setattr(bad_json_session, "post", lambda *_args, **_kwargs: _BadJsonResponse())
    with pytest.raises(Exception) as exception_info:
        list(bad_json_session.ask("hello", stream=False))
    assert "Invalid response" in str(exception_info.value)

    ok_json_session = Session(client, {"id": "session-ok-json", "chat_id": "chat-1"})

    class _OkJsonResponse:
        def json(self):
            return {"data": {"answer": "ok-answer", "reference": {"chunks": [{"id": "chunk-ok"}]}}}

    monkeypatch.setattr(ok_json_session, "post", lambda *_args, **_kwargs: _OkJsonResponse())
    ok_messages = list(ok_json_session.ask("hello", stream=False))
    assert len(ok_messages) == 1
    assert ok_messages[0].content == "ok-answer"
    assert ok_messages[0].reference == [{"id": "chunk-ok"}]

    transport_session = Session(client, {"id": "session-transport", "chat_id": "chat-1"})
    monkeypatch.setattr(
        transport_session,
        "post",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("transport boom")),
    )
    with pytest.raises(RuntimeError) as exception_info:
        list(transport_session.ask("hello", stream=False))
    assert "transport boom" in str(exception_info.value)

    message = Message(client, {})
    assert message.content == "Hi! I am your assistant, can I help you?"
    assert message.reference is None
    assert message.role == "assistant"
    assert message.prompt is None
    assert message.id is None


@pytest.mark.parametrize("operation", ["create", "list", "update", "delete"])
def test_sdk_crud_http_contract(monkeypatch, operation):
    import requests
    from ragflow_sdk import Chat

    client = RAGFlow("unit-token", "http://unit.invalid")
    chat = Chat(client, {"id": "chat-1"})
    row = {"id": "session-1", "chat_id": "chat-1", "name": "session name", "messages": []}
    calls = []

    def request(_session, method, url, **kwargs):
        calls.append((method, url, kwargs))
        response = requests.Response()
        response.status_code = 200
        import json

        response._content = json.dumps({"code": 0, "data": [row] if operation == "list" else row}).encode()
        return response

    monkeypatch.setattr(requests.sessions.Session, "request", request)
    if operation == "create":
        result = chat.create_session()
        method, path, payload_key, payload = "post", "/chats/chat-1/sessions", "json", {"name": "New session"}
        assert isinstance(result, Session)
        assert result.chat_id == chat.id
    elif operation == "list":
        result = chat.list_sessions(page=2, page_size=7, orderby="name", desc=False, id="session-1", name="session name", user_id="external-user")
        method, path, payload_key = "get", "/chats/chat-1/sessions", "params"
        payload = {"page": 2, "page_size": 7, "orderby": "name", "desc": False, "id": "session-1", "name": "session name", "user_id": "external-user"}
        assert len(result) == 1
        assert result[0].id == row["id"]
        assert result[0].messages == []
    elif operation == "update":
        session = Session(client, row)
        assert session.update({"name": "updated"}) is None
        method, path, payload_key, payload = "patch", "/chats/chat-1/sessions/session-1", "json", {"name": "updated"}
    else:
        assert chat.delete_sessions(ids=["session-1"]) is None
        method, path, payload_key, payload = "delete", "/chats/chat-1/sessions", "json", {"ids": ["session-1"], "delete_all": False}
    assert len(calls) == 1
    actual_method, url, kwargs = calls[0]
    assert actual_method.lower() == method
    assert url == "http://unit.invalid/api/v1" + path
    assert kwargs[payload_key] == payload
    assert kwargs["headers"] == {"Authorization": "Bearer unit-token"}


@pytest.mark.parametrize("operation", ["create", "update", "delete"])
def test_sdk_crud_api_errors(monkeypatch, operation):
    from ragflow_sdk import Chat

    class Response:
        def json(self):
            return {"code": 102, "message": "session contract error"}

    chat = Chat(None, {"id": "chat-1"})
    session = Session(None, {"id": "session-1", "chat_id": "chat-1"})
    monkeypatch.setattr(chat, "post", lambda *_args, **_kwargs: Response())
    monkeypatch.setattr(chat, "rm", lambda *_args, **_kwargs: Response())
    monkeypatch.setattr(session, "patch", lambda *_args, **_kwargs: Response())
    with pytest.raises(Exception, match="session contract error"):
        if operation == "create":
            chat.create_session()
        elif operation == "update":
            session.update({"name": "new"})
        else:
            chat.delete_sessions(ids=[session.id])
