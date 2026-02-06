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
