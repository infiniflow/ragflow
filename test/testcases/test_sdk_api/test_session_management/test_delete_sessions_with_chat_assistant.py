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
from common import batch_add_sessions_with_chat_assistant


class TestSessionWithChatAssistantDelete:
    @pytest.mark.parametrize(
        "payload",
        [
            pytest.param(lambda r: {"ids": ["invalid_id"] + r}, marks=pytest.mark.p3),
            pytest.param(lambda r: {"ids": r[:1] + ["invalid_id"] + r[1:5]}, marks=pytest.mark.p1),
            pytest.param(lambda r: {"ids": r + ["invalid_id"]}, marks=pytest.mark.p3),
        ],
    )
    def test_delete_partial_invalid_id(self, add_sessions_with_chat_assistant_func, payload):
        chat_assistant, sessions = add_sessions_with_chat_assistant_func
        if callable(payload):
            payload = payload([session.id for session in sessions])

        chat_assistant.delete_sessions(**payload)

        sessions = chat_assistant.list_sessions()
        assert len(sessions) == 0

    @pytest.mark.p3
    def test_repeated_deletion(self, add_sessions_with_chat_assistant_func):
        chat_assistant, sessions = add_sessions_with_chat_assistant_func
        session_ids = {"ids": [session.id for session in sessions]}

        chat_assistant.delete_sessions(**session_ids)

        with pytest.raises(Exception) as excinfo:
            chat_assistant.delete_sessions(**session_ids)
        assert "The chat doesn't own the session" in str(excinfo.value)

    @pytest.mark.p3
    def test_duplicate_deletion(self, add_sessions_with_chat_assistant_func):
        chat_assistant, sessions = add_sessions_with_chat_assistant_func
        session_ids = {"ids": [session.id for session in sessions] * 2}
        chat_assistant.delete_sessions(**session_ids)
        sessions = chat_assistant.list_sessions()
        assert len(sessions) == 0

    @pytest.mark.p3
    def test_concurrent_deletion(self, add_chat_assistants):
        count = 100
        _, _, chat_assistants = add_chat_assistants
        chat_assistant = chat_assistants[0]
        sessions = batch_add_sessions_with_chat_assistant(chat_assistant, count)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(chat_assistant.delete_sessions, ids=[sessions[i].id]) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses

    @pytest.mark.p3
    def test_delete_1k(self, add_chat_assistants):
        count = 1_000
        _, _, chat_assistants = add_chat_assistants
        chat_assistant = chat_assistants[0]
        ssessions = batch_add_sessions_with_chat_assistant(chat_assistant, count)
        chat_assistant.delete_sessions(ids=[ssession.id for ssession in ssessions])

        sessions = chat_assistant.list_sessions()
        assert len(sessions) == 0

    @pytest.mark.parametrize(
        "payload, expected_message, remaining",
        [
            pytest.param(None, """TypeError("argument of type \'NoneType\' is not iterable")""", 0, marks=pytest.mark.skip),
            pytest.param({"ids": ["invalid_id"]}, "The chat doesn't own the session invalid_id", 5, marks=pytest.mark.p3),
            pytest.param("not json", """AttributeError("\'str\' object has no attribute \'get\'")""", 5, marks=pytest.mark.skip),
            pytest.param(lambda r: {"ids": r[:1]}, "", 4, marks=pytest.mark.p3),
            pytest.param(lambda r: {"ids": r}, "", 0, marks=pytest.mark.p1),
            pytest.param({"ids": []}, "", 0, marks=pytest.mark.p3),
        ],
    )
    def test_basic_scenarios(self, add_sessions_with_chat_assistant_func, payload, expected_message, remaining):
        chat_assistant, sessions = add_sessions_with_chat_assistant_func
        if callable(payload):
            payload = payload([session.id for session in sessions])

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                chat_assistant.delete_sessions(**payload)
            assert expected_message in str(excinfo.value)
        else:
            chat_assistant.delete_sessions(**payload)

        sessions = chat_assistant.list_sessions()
        assert len(sessions) == remaining
