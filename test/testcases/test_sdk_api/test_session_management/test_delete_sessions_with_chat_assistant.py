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

from concurrent.futures import ThreadPoolExecutor, as_completed
from unittest.mock import Mock

import pytest
from ragflow_sdk.modules.chat import Chat


@pytest.mark.integration
class TestSessionWithChatAssistantDelete:
    @pytest.mark.p3
    @pytest.mark.parametrize("invalid_index", [0, 1, 5])
    def test_delete_partial_invalid_id(self, add_sessions_with_chat_assistant, invalid_index):
        chat, sessions = add_sessions_with_chat_assistant
        ids = [session.id for session in sessions]
        ids.insert(invalid_index, "invalid_id")
        assert chat.delete_sessions(ids=ids) is None
        assert chat.list_sessions() == []

    @pytest.mark.p3
    def test_repeated_deletion(self, add_sessions_with_chat_assistant):
        chat, sessions = add_sessions_with_chat_assistant
        ids = [session.id for session in sessions]
        chat.delete_sessions(ids=ids)
        with pytest.raises(Exception, match="The chat doesn't own the session"):
            chat.delete_sessions(ids=ids)
        assert chat.list_sessions() == []

    @pytest.mark.p3
    def test_duplicate_deletion(self, add_sessions_with_chat_assistant):
        chat, sessions = add_sessions_with_chat_assistant
        assert chat.delete_sessions(ids=[session.id for session in sessions] * 2) is None
        assert chat.list_sessions() == []

    @pytest.mark.p3
    def test_concurrent_deletion(self, chat):
        sessions = [chat.create_session(name=f"concurrent-delete-{i}") for i in range(100)]
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(chat.delete_sessions, ids=[session.id]) for session in sessions]
            for future in as_completed(futures):
                assert future.result() is None
        assert chat.list_sessions() == []

    @pytest.mark.p3
    def test_delete_1k(self, chat):
        sessions = [chat.create_session(name=f"bulk-delete-{i}") for i in range(1_000)]
        assert len({session.id for session in sessions}) == 1_000
        chat.delete_sessions(ids=[session.id for session in sessions])
        assert chat.list_sessions() == []

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "mode, remaining",
        [
            ("omitted", 5),
            ("invalid", 5),
            ("one", 4),
            ("all_ids", 0),
            ("empty", 5),
            ("null", 5),
            ("delete_all", 0),
            ("empty_delete_all", 0),
            ("ids_with_delete_all", 4),
        ],
    )
    def test_basic_scenarios(self, add_sessions_with_chat_assistant, mode, remaining):
        chat, sessions = add_sessions_with_chat_assistant
        ids = [session.id for session in sessions]
        payload = {
            "omitted": {},
            "invalid": {"ids": ["invalid_id"]},
            "one": {"ids": ids[:1]},
            "all_ids": {"ids": ids},
            "empty": {"ids": []},
            "null": {"ids": None},
            "delete_all": {"delete_all": True},
            "empty_delete_all": {"ids": [], "delete_all": True},
            "ids_with_delete_all": {"ids": ids[:1], "delete_all": True},
        }[mode]
        if mode == "invalid":
            with pytest.raises(Exception, match="The chat doesn't own the session invalid_id"):
                chat.delete_sessions(**payload)
        else:
            assert chat.delete_sessions(**payload) is None
        expected = set(ids if remaining == 5 else ids[1:] if remaining == 4 else [])
        assert {session.id for session in chat.list_sessions()} == expected

    def test_delete_cannot_target_another_chat(self, client, add_sessions_with_chat_assistant):
        chat, sessions = add_sessions_with_chat_assistant
        other_chat = client.create_chat(name="other-delete-owner")
        other_session = None
        try:
            other_session = other_chat.create_session(name="must survive")
            with pytest.raises(Exception, match="The chat doesn't own the session"):
                chat.delete_sessions(ids=[other_session.id])
            assert {session.id for session in chat.list_sessions()} == {session.id for session in sessions}
            assert [session.id for session in other_chat.list_sessions()] == [other_session.id]
        finally:
            if other_session is not None:
                other_chat.delete_sessions(ids=[other_session.id])
                assert other_chat.list_sessions() == []
            client.delete_chats(ids=[other_chat.id])
            assert client.list_chats(id=other_chat.id) == []


@pytest.mark.parametrize(
    "payload, expected",
    [
        ({}, {"ids": None, "delete_all": False}),
        ({"ids": []}, {"ids": [], "delete_all": False}),
        ({"ids": ["s"]}, {"ids": ["s"], "delete_all": False}),
        ({"delete_all": True}, {"ids": None, "delete_all": True}),
    ],
)
def test_delete_sessions_sdk_payload(payload, expected):
    rag = Mock()
    rag.delete.return_value.json.return_value = {"code": 0}
    chat = Chat(rag, {"id": "chat"})
    assert chat.delete_sessions(**payload) is None
    rag.delete.assert_called_once_with("/chats/chat/sessions", expected)


def test_delete_sessions_invalid_keyword_payload():
    rag = Mock()
    with pytest.raises(TypeError):
        Chat(rag, {"id": "chat"}).delete_sessions(**"not json")
    rag.delete.assert_not_called()
