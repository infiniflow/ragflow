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
from random import randint

import pytest
from configs import SESSION_WITH_CHAT_NAME_LIMIT


class TestSessionWithChatAssistantUpdate:
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            pytest.param({"name": "valid_name"}, "", marks=pytest.mark.p1),
            pytest.param({"name": "a" * (SESSION_WITH_CHAT_NAME_LIMIT + 1)}, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param({"name": 1}, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param({"name": ""}, "`name` can not be empty.", marks=pytest.mark.p3),
            pytest.param({"name": "duplicated_name"}, "", marks=pytest.mark.p3),
            pytest.param({"name": "case insensitive"}, "", marks=pytest.mark.p3),
        ],
    )
    def test_name(self, add_sessions_with_chat_assistant_func, payload, expected_message):
        chat_assistant, sessions = add_sessions_with_chat_assistant_func
        session = sessions[0]

        if payload["name"] == "duplicated_name":
            session.update(payload)
        elif payload["name"] == "case insensitive":
            session.update({"name": payload["name"].upper()})

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                session.update(payload)
            assert expected_message in str(excinfo.value)
        else:
            session.update(payload)
            updated_session = chat_assistant.list_sessions(id=session.id)[0]
            assert updated_session.name == payload["name"]

    @pytest.mark.p3
    def test_repeated_update_session(self, add_sessions_with_chat_assistant_func):
        _, sessions = add_sessions_with_chat_assistant_func
        session = sessions[0]

        session.update({"name": "valid_name_1"})
        session.update({"name": "valid_name_2"})

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            pytest.param({"unknown_key": "unknown_value"}, "ValueError", marks=pytest.mark.skip),
            ({}, ""),
            pytest.param(None, "TypeError", marks=pytest.mark.skip),
        ],
    )
    def test_invalid_params(self, add_sessions_with_chat_assistant_func, payload, expected_message):
        _, sessions = add_sessions_with_chat_assistant_func
        session = sessions[0]

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                session.update(payload)
            assert expected_message in str(excinfo.value)
        else:
            session.update(payload)

    @pytest.mark.p3
    def test_concurrent_update_session(self, add_sessions_with_chat_assistant_func):
        count = 50
        _, sessions = add_sessions_with_chat_assistant_func

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(sessions[randint(0, 4)].update, {"name": f"update session test {i}"}) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses

    @pytest.mark.p3
    def test_update_session_to_deleted_chat_assistant(self, client, add_sessions_with_chat_assistant_func):
        chat_assistant, sessions = add_sessions_with_chat_assistant_func
        client.delete_chats(ids=[chat_assistant.id])

        with pytest.raises(Exception) as excinfo:
            sessions[0].update({"name": "valid_name"})
        assert "You do not own the session" in str(excinfo.value)
