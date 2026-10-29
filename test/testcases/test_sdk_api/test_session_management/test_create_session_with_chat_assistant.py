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
from concurrent.futures import ThreadPoolExecutor

import pytest
from ragflow_sdk import RAGFlow
from ragflow_sdk.modules.session import Session


class _DummyStreamResponse:
    def __init__(self, lines):
        self._lines = lines

    def iter_lines(self, decode_unicode=True):
        del decode_unicode
        for line in self._lines:
            yield line


class _DummyJsonResponse:
    def __init__(self, payload):
        self._payload = payload

    def json(self):
        return self._payload


@pytest.mark.integration
class TestSessionWithChatAssistantCreate:
    @pytest.mark.parametrize(
        "name, expected",
        [
            ("valid_name", "valid_name"),
            ("a" * 256, "a" * 255),
            ("会" * 256, "会" * 255),
            ("", None),
            ("   ", None),
            (1, None),
            (None, None),
            (True, None),
            ([], None),
            ("  trimmed name  ", "trimmed name"),
            ("duplicated_name", "duplicated_name"),
            ("case insensitive", "case insensitive"),
        ],
    )
    def test_name(self, chat, name, expected):
        previous = None
        if name in ("duplicated_name", "case insensitive"):
            previous = chat.create_session(name=name.upper() if name == "case insensitive" else name)
        if expected is None:
            with pytest.raises(Exception, match="`name` can not be empty"):
                chat.create_session(name=name)
            assert chat.list_sessions() == []
        else:
            session = chat.create_session(name=name)
            assert session.id
            assert session.name == expected
            assert session.chat_id == chat.id
            persisted = chat.list_sessions(id=session.id)
            assert len(persisted) == 1
            assert persisted[0].name == expected
            if previous:
                assert session.id != previous.id
                assert {item.id for item in chat.list_sessions()} == {session.id, previous.id}

    def test_default_name(self, chat):
        session = chat.create_session()
        assert session.name == "New session"
        assert session.chat_id == chat.id
        assert session.messages[0]["role"] == "assistant"
        assert session.messages[0]["content"] == "session audit greeting"
        assert [item.id for item in chat.list_sessions()] == [session.id]

    def test_concurrent_create_session(self, chat):
        count = 1000
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(chat.create_session, name=f"session-{i}") for i in range(count)]
            sessions = [future.result() for future in futures]
        assert len({session.id for session in sessions}) == count
        assert {session.name for session in sessions} == {f"session-{i}" for i in range(count)}
        assert all(session.chat_id == chat.id for session in sessions)
        persisted = chat.list_sessions(page_size=count + 1)
        assert {session.id for session in persisted} == {session.id for session in sessions}

    def test_add_session_to_deleted_chat_assistant(self, client, chat):
        client.delete_chats(ids=[chat.id])
        with pytest.raises(Exception, match="^Resource not found$"):
            chat.create_session(name="valid_name")


@pytest.mark.p2
def test_session_module_streaming_and_helper_paths_unit(monkeypatch):
    client = RAGFlow("token", "http://unit.invalid")
    chat_session = Session(client, {"id": "session-chat", "chat_id": "chat-1"})
    chat_done_session = Session(client, {"id": "session-chat-done", "chat_id": "chat-1"})
    agent_session = Session(client, {"id": "session-agent", "agent_id": "agent-1"})
    calls = []

    chat_stream = _DummyStreamResponse(
        [
            "",
            "data: {bad json}",
            'data: {"event":"workflow_started","data":{"content":"skip"}}',
            '{"data":{"answer":"chat-answer","reference":{"chunks":[{"id":"chunk-1"}]}}}',
            'data: {"data": true}',
            "data: [DONE]",
        ]
    )
    agent_stream = _DummyStreamResponse(
        [
            "data: {bad json}",
            'data: {"event":"message","data":{"content":"agent-answer"}}',
            'data: {"event":"message_end","data":{"content":"done"}}',
        ]
    )

    def _chat_post(path, json=None, stream=False, files=None):
        calls.append(("chat", path, json, stream, files))
        return chat_stream

    def _agent_post(path, json=None, stream=False, files=None):
        calls.append(("agent", path, json, stream, files))
        return agent_stream

    monkeypatch.setattr(chat_session, "post", _chat_post)
    monkeypatch.setattr(
        chat_done_session,
        "post",
        lambda *_args, **_kwargs: _DummyStreamResponse(['{"data":{"answer":"chat-done","reference":{"chunks":[]}}}', "data: [DONE]"]),
    )
    monkeypatch.setattr(agent_session, "post", _agent_post)

    chat_messages = list(chat_session.ask("hello chat", stream=True, temperature=0.2))
    assert len(chat_messages) == 1
    assert chat_messages[0].content == "chat-answer"
    assert chat_messages[0].reference == [{"id": "chunk-1"}]

    chat_done_messages = list(chat_done_session.ask("hello done", stream=True))
    assert len(chat_done_messages) == 1
    assert chat_done_messages[0].content == "chat-done"

    agent_messages = list(agent_session.ask("hello agent", stream=True, top_p=0.8))
    assert len(agent_messages) == 1
    assert agent_messages[0].content == "agent-answer"

    assert calls[0][1] == "/chats/chat-1/completions"
    assert calls[0][2]["question"] == "hello chat"
    assert calls[0][2]["session_id"] == "session-chat"
    assert calls[0][2]["temperature"] == 0.2
    assert calls[0][3] is True
    assert calls[1][1] == "/agents/chat/completions"
    assert calls[1][2]["agent_id"] == "agent-1"
    assert calls[1][2]["query"] == "hello agent"
    assert calls[1][2]["session_id"] == "session-agent"
    assert calls[1][2]["openai-compatible"] is False
    assert calls[1][2]["top_p"] == 0.8
    assert calls[1][3] is True


@pytest.mark.p2
def test_agent_session_preserves_non_stream_reference_chunks(monkeypatch, caplog):
    first_chunk = {"id": "chunk-1", "content": "first source"}
    second_chunk = {"id": "chunk-2", "content": "second source"}
    session = Session(None, {"id": "session-agent", "agent_id": "agent-1"})
    response = _DummyJsonResponse(
        {
            "data": {
                "event": "message_end",
                "data": {
                    "content": "agent-answer",
                    "reference": {"chunks": {"500": first_chunk, "100": second_chunk}},
                },
            }
        }
    )
    monkeypatch.setattr(session, "post", lambda *_args, **_kwargs: response)

    with caplog.at_level("DEBUG", logger="ragflow_sdk.modules.session"):
        messages = list(session.ask("hello agent", stream=False))

    assert len(messages) == 1
    assert messages[0].content == "agent-answer"
    assert messages[0].reference == [first_chunk, second_chunk]
    assert "session_id=session-agent event=message_end reference_count=2" in caplog.text
    assert "first source" not in caplog.text
    assert "second source" not in caplog.text


@pytest.mark.p2
def test_agent_session_streams_all_message_references_without_duplicate_content(monkeypatch):
    first_chunk = {"id": "chunk-1", "content": "first source"}
    second_chunk = {"id": "chunk-2", "content": "second source"}
    session = Session(None, {"id": "session-agent", "agent_id": "agent-1"})
    response = _DummyStreamResponse(
        [
            'data: {"event":"message","data":{"content":"first answer"}}',
            'data: {"event":"message_end","data":{"content":"first answer","reference":{"chunks":{"123":{"id":"chunk-1","content":"first source"}}}}}',
            'data: {"event":"message","data":{"content":"second answer"}}',
            'data: {"event":"message_end","data":{"content":"second answer","reference":{"chunks":{"456":{"id":"chunk-2","content":"second source"}}}}}',
            "data: [DONE]",
        ]
    )
    monkeypatch.setattr(session, "post", lambda *_args, **_kwargs: response)

    messages = list(session.ask("hello agent", stream=True))

    assert "".join(message.content for message in messages) == "first answersecond answer"
    assert [message.reference for message in messages if message.reference] == [[first_chunk], [second_chunk]]


@pytest.mark.p2
@pytest.mark.parametrize("session_type", ["chat", "agent"])
@pytest.mark.parametrize("stream", [False, True])
def test_session_ask_forwards_request_and_message_identity(monkeypatch, session_type, stream):
    session = Session(None, {"id": "saved-session", f"{session_type}_id": "parent"})
    calls = []
    answer = {"content": "answer", "reference": {"chunks": [{"id": "source"}]}}
    event = {"event": "message", "message_id": "message-id", "data": answer}
    if session_type == "chat":
        event = {"data": {"id": "message-id", "answer": "answer", "reference": answer["reference"]}}
    if stream:
        import json

        response = _DummyStreamResponse(["data: " + json.dumps(event), "data:[DONE]"])
    else:
        response = _DummyJsonResponse({"code": 0, "data": event if session_type == "agent" else event["data"]})

    def post(path, payload, stream=False):
        calls.append((path, payload, stream))
        return response

    monkeypatch.setattr(session, "post", post)
    options = {"files": [{"id": "file-id"}], "user_id": "external-user", "session_id": "override-session"}
    if session_type == "agent":
        options.update(inputs={"topic": {"value": "SDK"}}, release=False, return_trace=True)
    messages = list(session.ask("question", stream=stream, **options))
    assert len(messages) == 1
    assert messages[0].id == "message-id"
    assert messages[0].content == "answer"
    assert messages[0].role == "assistant"
    assert messages[0].reference == [{"id": "source"}]
    path, payload, transport_stream = calls[0]
    expected = {"stream": stream, **options}
    if session_type == "chat":
        expected["question"] = "question"
        assert path == "/chats/parent/completions"
    else:
        expected.update(agent_id="parent", query="question", **{"openai-compatible": False})
        assert path == "/agents/chat/completions"
    assert payload == expected
    assert transport_stream is stream


@pytest.mark.p2
@pytest.mark.parametrize("session_type", ["chat", "agent"])
@pytest.mark.parametrize("terminator", ["data: [DONE]", "data:[DONE]"])
def test_session_stream_stops_before_trailing_messages(monkeypatch, session_type, terminator):
    session = Session(None, {"id": "session", f"{session_type}_id": "parent"})
    if session_type == "chat":
        frame = 'data:{"event":"message","data":{"id":"answer-id","answer":"answer"}}'
    else:
        frame = 'data:{"event":"message","message_id":"answer-id","data":{"content":"answer"}}'
    response = _DummyStreamResponse(["", ": keepalive", "event: message", "data: {invalid json}", 'data:{"event":"node_finished","data":{}}', frame, terminator, frame])
    monkeypatch.setattr(session, "post", lambda *_args, **_kwargs: response)
    messages = list(session.ask("question", stream=True))
    assert [(message.id, message.content) for message in messages] == [("answer-id", "answer")]


@pytest.mark.p2
def test_chat_stream_preserves_runtime_error_answer(monkeypatch):
    session = Session(None, {"id": "session", "chat_id": "parent"})
    response = _DummyStreamResponse(['data:{"code":500,"message":"Session not found!","data":{"answer":"**ERROR**: Session not found!","reference":[]}}'])
    monkeypatch.setattr(session, "post", lambda *_args, **_kwargs: response)
    messages = list(session.ask("question", stream=True))
    assert len(messages) == 1
    assert messages[0].content == "**ERROR**: Session not found!"
    assert messages[0].reference is None


@pytest.mark.p2
@pytest.mark.parametrize("session_type", ["chat", "agent"])
@pytest.mark.parametrize("stream", [False, True])
@pytest.mark.skip(reason="Python SDK Session.ask ignores nonzero response codes: Go validation errors lose their message in KeyError('data') instead of raising the API error")
def test_session_ask_preserves_api_error_message(monkeypatch, session_type, stream):
    session = Session(None, {"id": "session", f"{session_type}_id": "parent"})
    error = {"code": 102, "message": "Session not found!"}
    response = _DummyJsonResponse(error)
    if stream:
        response = _DummyStreamResponse(['{"code":102,"message":"Session not found!"}'])
    monkeypatch.setattr(session, "post", lambda *_args, **_kwargs: response)
    with pytest.raises(Exception, match="Session not found!"):
        list(session.ask("question", stream=stream))


@pytest.mark.p2
@pytest.mark.parametrize("session_type", ["chat", "agent"])
def test_session_non_stream_rejects_malformed_json(monkeypatch, session_type):
    session = Session(None, {"id": "session", f"{session_type}_id": "parent"})

    class InvalidResponse:
        def json(self):
            raise ValueError("invalid JSON")

        def __str__(self):
            return "malformed response"

    monkeypatch.setattr(session, "post", lambda *_args, **_kwargs: InvalidResponse())
    with pytest.raises(Exception, match="Invalid response malformed response"):
        list(session.ask("question", stream=False))


@pytest.mark.p2
def test_chat_stream_true_terminator_ignores_trailing_messages(monkeypatch):
    session = Session(None, {"id": "session", "chat_id": "parent"})
    frame = 'data:{"data":{"answer":"answer"}}'
    monkeypatch.setattr(session, "post", lambda *_args, **_kwargs: _DummyStreamResponse([frame, 'data:{"data":true}', frame]))
    assert [message.content for message in session.ask("question", stream=True)] == ["answer"]


@pytest.mark.p2
@pytest.mark.parametrize("session_type", ["chat", "agent"])
def test_session_stream_eof_returns_received_messages(monkeypatch, session_type):
    session = Session(None, {"id": "session", f"{session_type}_id": "parent"})
    frame = 'data:{"data":{"answer":"answer"}}'
    if session_type == "agent":
        frame = 'data:{"event":"message","data":{"content":"answer"}}'
    monkeypatch.setattr(session, "post", lambda *_args, **_kwargs: _DummyStreamResponse([frame]))
    assert [message.content for message in session.ask("question", stream=True)] == ["answer"]
