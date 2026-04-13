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
import pytest
from concurrent.futures import ThreadPoolExecutor, as_completed
from ragflow_sdk import RAGFlow
from ragflow_sdk.modules.session import Message, Session


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


class TestSessionsWithChatAssistantList:
    @pytest.mark.p2
    def test_list_sessions_raises_on_nonzero_response(self, add_sessions_with_chat_assistant, monkeypatch):
        chat_assistant, _ = add_sessions_with_chat_assistant

        class _DummyResponse:
            def json(self):
                return {"code": 1, "message": "boom"}

        monkeypatch.setattr(chat_assistant, "get", lambda *_args, **_kwargs: _DummyResponse())

        with pytest.raises(Exception) as exception_info:
            chat_assistant.list_sessions()
        assert "boom" in str(exception_info.value)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size, expected_message",
        [
            ({"page": None, "page_size": 2}, 0, "not instance of"),
            pytest.param({"page": 0, "page_size": 2}, 0, "ValueError('Search does not support negative slicing.')", marks=pytest.mark.skip),
            ({"page": 2, "page_size": 2}, 2, ""),
            ({"page": 3, "page_size": 2}, 1, ""),
            ({"page": "3", "page_size": 2}, 0, "not instance of"),
            pytest.param({"page": -1, "page_size": 2}, 0, "ValueError('Search does not support negative slicing.')", marks=pytest.mark.skip),
            pytest.param({"page": "a", "page_size": 2}, 0, """ValueError("invalid literal for int() with base 10: \'a\'")""", marks=pytest.mark.skip),
        ],
    )
    def test_page(self, add_sessions_with_chat_assistant, params, expected_page_size, expected_message):
        chat_assistant, _ = add_sessions_with_chat_assistant
        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chat_assistant.list_sessions(**params)
            assert expected_message in str(exception_info.value)
        else:
            sessions = chat_assistant.list_sessions(**params)
            assert len(sessions) == expected_page_size

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size, expected_message",
        [
            ({"page_size": None}, 0, "not instance of"),
            ({"page_size": 0}, 0, ""),
            ({"page_size": 1}, 1, ""),
            ({"page_size": 6}, 5, ""),
            ({"page_size": "1"}, 0, "not instance of"),
            pytest.param({"page_size": -1}, 5, "", marks=pytest.mark.skip),
            pytest.param({"page_size": "a"}, 0, """ValueError("invalid literal for int() with base 10: \'a\'")""", marks=pytest.mark.skip),
        ],
    )
    def test_page_size(self, add_sessions_with_chat_assistant, params, expected_page_size, expected_message):
        chat_assistant, _ = add_sessions_with_chat_assistant
        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chat_assistant.list_sessions(**params)
            assert expected_message in str(exception_info.value)
        else:
            sessions = chat_assistant.list_sessions(**params)
            assert len(sessions) == expected_page_size

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, expected_message",
        [
            ({"orderby": None}, "not instance of"),
            ({"orderby": "create_time"}, ""),
            ({"orderby": "update_time"}, ""),
            ({"orderby": "name", "desc": "False"}, "not instance of"),
            pytest.param({"orderby": "unknown"}, "orderby should be create_time or update_time", marks=pytest.mark.skip(reason="issues/")),
        ],
    )
    def test_orderby(self, add_sessions_with_chat_assistant, params, expected_message):
        chat_assistant, _ = add_sessions_with_chat_assistant
        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chat_assistant.list_sessions(**params)
            assert expected_message in str(exception_info.value)
        else:
            chat_assistant.list_sessions(**params)

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, expected_message",
        [
            ({"desc": None}, "not instance of"),
            ({"desc": "true"}, "not instance of"),
            ({"desc": "True"}, "not instance of"),
            ({"desc": True}, ""),
            ({"desc": "false"}, "not instance of"),
            ({"desc": "False"}, "not instance of"),
            ({"desc": False}, ""),
            ({"desc": "False", "orderby": "update_time"}, "not instance of"),
            pytest.param({"desc": "unknown"}, "desc should be true or false", marks=pytest.mark.skip(reason="issues/")),
        ],
    )
    def test_desc(self, add_sessions_with_chat_assistant, params, expected_message):
        chat_assistant, _ = add_sessions_with_chat_assistant
        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chat_assistant.list_sessions(**params)
            assert expected_message in str(exception_info.value)
        else:
            chat_assistant.list_sessions(**params)

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_num, expected_message",
        [
            ({"name": None}, 0, "not instance of"),
            ({"name": ""}, 5, ""),
            ({"name": "session_with_chat_assistant_1"}, 1, ""),
            ({"name": "unknown"}, 0, ""),
        ],
    )
    def test_name(self, add_sessions_with_chat_assistant, params, expected_num, expected_message):
        chat_assistant, _ = add_sessions_with_chat_assistant
        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chat_assistant.list_sessions(**params)
            assert expected_message in str(exception_info.value)
        else:
            sessions = chat_assistant.list_sessions(**params)
            if params["name"] == "session_with_chat_assistant_1":
                assert sessions[0].name == params["name"]
            else:
                assert len(sessions) == expected_num

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "session_id, expected_num, expected_message",
        [
            (None, 0, "not instance of"),
            ("", 5, ""),
            (lambda r: r[0], 1, ""),
            ("unknown", 0, ""),
        ],
    )
    def test_id(self, add_sessions_with_chat_assistant, session_id, expected_num, expected_message):
        chat_assistant, sessions = add_sessions_with_chat_assistant
        if callable(session_id):
            params = {"id": session_id([s.id for s in sessions])}
        else:
            params = {"id": session_id}

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chat_assistant.list_sessions(**params)
            assert expected_message in str(exception_info.value)
        else:
            list_sessions = chat_assistant.list_sessions(**params)
            if "id" in params and params["id"] == sessions[0].id:
                assert list_sessions[0].id == params["id"]
            else:
                assert len(list_sessions) == expected_num

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "session_id, name, expected_num, expected_message",
        [
            (lambda r: r[0], "session_with_chat_assistant_0", 1, ""),
            (lambda r: r[0], "session_with_chat_assistant_100", 0, ""),
            (lambda r: r[0], "unknown", 0, ""),
            ("id", "session_with_chat_assistant_0", 0, ""),
        ],
    )
    def test_name_and_id(self, add_sessions_with_chat_assistant, session_id, name, expected_num, expected_message):
        chat_assistant, sessions = add_sessions_with_chat_assistant
        if callable(session_id):
            params = {"id": session_id([s.id for s in sessions]), "name": name}
        else:
            params = {"id": session_id, "name": name}

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chat_assistant.list_sessions(**params)
            assert expected_message in str(exception_info.value)
        else:
            list_sessions = chat_assistant.list_sessions(**params)
            assert len(list_sessions) == expected_num

    @pytest.mark.p3
    def test_concurrent_list(self, add_sessions_with_chat_assistant):
        count = 100
        chat_assistant, _ = add_sessions_with_chat_assistant
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(chat_assistant.list_sessions) for _ in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses

    @pytest.mark.p3
    def test_list_chats_after_deleting_associated_chat_assistant(self, client, add_sessions_with_chat_assistant):
        chat_assistant, _ = add_sessions_with_chat_assistant
        client.delete_chats(ids=[chat_assistant.id])

        with pytest.raises(Exception) as exception_info:
            chat_assistant.list_sessions()
        assert "You don't own the assistant" in str(exception_info.value)


@pytest.mark.p2
def test_session_module_error_paths_unit(monkeypatch):
    client = RAGFlow("token", "http://localhost:9380")

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
