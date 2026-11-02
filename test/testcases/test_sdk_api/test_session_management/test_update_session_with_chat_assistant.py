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

import pytest


@pytest.mark.integration
class TestSessionWithChatAssistantUpdate:
    @pytest.mark.p3
    @pytest.mark.parametrize(
        "name, expected_name, error",
        [
            ("valid_name", "valid_name", None),
            ("a" * 256, "a" * 255, None),
            (1, None, "`name` can not be empty"),
            ("", None, "`name` can not be empty"),
            ("duplicated_name", "duplicated_name", None),
            ("case insensitive", "case insensitive", None),
            ("  trimmed name  ", "trimmed name", None),
            ("   ", None, "`name` can not be empty"),
            ("会" * 256, "会" * 255, None),
        ],
    )
    def test_name(self, add_sessions_with_chat_assistant, name, expected_name, error):
        chat, sessions = add_sessions_with_chat_assistant
        session = sessions[0]
        if name == "duplicated_name":
            sessions[1].update({"name": name})
        elif name == "case insensitive":
            session.update({"name": name.upper()})
        original = chat.list_sessions(id=session.id)[0].name
        if error:
            with pytest.raises(Exception, match=error):
                session.update({"name": name})
        else:
            assert session.update({"name": name}) is None
        persisted = chat.list_sessions(id=session.id)
        assert len(persisted) == 1
        assert persisted[0].name == (original if error else expected_name)
        if name == "duplicated_name":
            assert chat.list_sessions(id=sessions[1].id)[0].name == name

    @pytest.mark.p3
    def test_repeated_update_session(self, add_sessions_with_chat_assistant):
        chat, sessions = add_sessions_with_chat_assistant
        session = sessions[0]
        for name in ("valid_name_1", "valid_name_2", "valid_name_2"):
            assert session.update({"name": name}) is None
            assert chat.list_sessions(id=session.id)[0].name == name

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, error",
        [
            pytest.param({}, None, id="empty-noop"),
            pytest.param(None, None, id="absent-body-noop"),
            pytest.param({"id": "other", "dialog_id": "other", "chat_id": "other", "user_id": "other"}, None, id="protected-identifiers"),
            pytest.param({"messages": []}, "`messages` cannot be changed", id="messages"),
            pytest.param({"message": []}, "`messages` cannot be changed", id="message"),
            pytest.param({"reference": []}, "`reference` cannot be changed", id="reference"),
        ],
    )
    def test_invalid_params(self, add_sessions_with_chat_assistant, payload, error):
        chat, sessions = add_sessions_with_chat_assistant
        session = sessions[0]
        before = chat.list_sessions(id=session.id)[0].to_json()
        if error:
            with pytest.raises(Exception) as exc:
                session.update(payload)
            assert str(exc.value) == error
        else:
            assert session.update(payload) is None
        after = chat.list_sessions(id=session.id)[0].to_json()
        assert after == before

    @pytest.mark.p3
    def test_concurrent_update_session(self, add_sessions_with_chat_assistant):
        chat, sessions = add_sessions_with_chat_assistant
        names = {session.id: {f"update {session.id} {i}" for i in range(10)} for session in sessions}
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(session.update, {"name": name}) for session in sessions for name in names[session.id]]
            for future in as_completed(futures):
                assert future.result() is None
        persisted = chat.list_sessions()
        assert {session.id for session in persisted} == set(names)
        for session in persisted:
            assert session.name in names[session.id]

    @pytest.mark.p3
    def test_update_session_to_deleted_chat_assistant(self, client, add_sessions_with_chat_assistant):
        chat, sessions = add_sessions_with_chat_assistant
        client.delete_chats(ids=[chat.id])
        with pytest.raises(Exception) as exc:
            sessions[0].update({"name": "valid_name"})
        assert str(exc.value) == "Resource not found"
