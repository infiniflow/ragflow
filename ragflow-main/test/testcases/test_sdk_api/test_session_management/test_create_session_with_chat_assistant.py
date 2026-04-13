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
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from configs import SESSION_WITH_CHAT_NAME_LIMIT
from ragflow_sdk import RAGFlow
from ragflow_sdk.modules.session import Session


class _DummyStreamResponse:
    def __init__(self, lines):
        self._lines = lines

    def iter_lines(self, decode_unicode=True):
        del decode_unicode
        for line in self._lines:
            yield line


@pytest.fixture(scope="session")
def auth():
    return "unit-auth"


@pytest.fixture(scope="session", autouse=True)
def set_tenant_info():
    return None


@pytest.mark.usefixtures("clear_session_with_chat_assistants")
class TestSessionWithChatAssistantCreate:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "name, expected_message",
        [
            ("valid_name", ""),
            pytest.param("a" * (SESSION_WITH_CHAT_NAME_LIMIT + 1), "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param(1, "", marks=pytest.mark.skip(reason="issues/")),
            ("", "`name` can not be empty."),
            ("duplicated_name", ""),
            ("case insensitive", ""),
        ],
    )
    def test_name(self, add_chat_assistants, name, expected_message):
        _, _, chat_assistants = add_chat_assistants
        chat_assistant = chat_assistants[0]

        if name == "duplicated_name":
            chat_assistant.create_session(name=name)
        elif name == "case insensitive":
            chat_assistant.create_session(name=name.upper())

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                chat_assistant.create_session(name=name)
            assert expected_message in str(exception_info.value)
        else:
            session = chat_assistant.create_session(name=name)
            assert session.name == name, str(session)
            assert session.chat_id == chat_assistant.id, str(session)

    @pytest.mark.p3
    def test_concurrent_create_session(self, add_chat_assistants):
        count = 1000
        _, _, chat_assistants = add_chat_assistants
        chat_assistant = chat_assistants[0]

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(chat_assistant.create_session, name=f"session with chat assistant test {i}") for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses

        updated_sessions = chat_assistant.list_sessions(page_size=count * 2)
        assert len(updated_sessions) == count

    @pytest.mark.p3
    def test_add_session_to_deleted_chat_assistant(self, client, add_chat_assistants):
        _, _, chat_assistants = add_chat_assistants
        chat_assistant = chat_assistants[0]

        client.delete_chats(ids=[chat_assistant.id])
        with pytest.raises(Exception) as exception_info:
            chat_assistant.create_session(name="valid_name")
        assert "You do not own the assistant" in str(exception_info.value)


@pytest.mark.p2
def test_session_module_streaming_and_helper_paths_unit(monkeypatch):
    client = RAGFlow("token", "http://localhost:9380")
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
        lambda *_args, **_kwargs: _DummyStreamResponse(
            ['{"data":{"answer":"chat-done","reference":{"chunks":[]}}}', "data: [DONE]"]
        ),
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
    assert calls[1][1] == "/agents/agent-1/completions"
    assert calls[1][2]["question"] == "hello agent"
    assert calls[1][2]["session_id"] == "session-agent"
    assert calls[1][2]["top_p"] == 0.8
    assert calls[1][3] is True
