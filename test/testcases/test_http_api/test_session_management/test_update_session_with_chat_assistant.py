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
from common import delete_chat_assistants, list_session_with_chat_assistants, update_session_with_chat_assistant
from configs import INVALID_API_TOKEN, INVALID_ID_32, SESSION_WITH_CHAT_NAME_LIMIT
from libs.auth import RAGFlowHttpApiAuth


@pytest.mark.p1
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 0, "`Authorization` can't be empty"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                109,
                "Authentication error: API key is invalid!",
            ),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = update_session_with_chat_assistant(invalid_auth, "chat_assistant_id", "session_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestSessionWithChatAssistantUpdate:
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            pytest.param({"name": "valid_name"}, 0, "", marks=pytest.mark.p1),
            pytest.param({"name": "a" * (SESSION_WITH_CHAT_NAME_LIMIT + 1)}, 102, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param({"name": 1}, 100, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param({"name": ""}, 102, "`name` can not be empty.", marks=pytest.mark.p3),
            pytest.param({"name": "duplicated_name"}, 0, "", marks=pytest.mark.p3),
            pytest.param({"name": "case insensitive"}, 0, "", marks=pytest.mark.p3),
        ],
    )
    def test_name(self, HttpApiAuth, add_sessions_with_chat_assistant_func, payload, expected_code, expected_message):
        chat_assistant_id, session_ids = add_sessions_with_chat_assistant_func
        if payload["name"] == "duplicated_name":
            update_session_with_chat_assistant(HttpApiAuth, chat_assistant_id, session_ids[0], payload)
        elif payload["name"] == "case insensitive":
            update_session_with_chat_assistant(HttpApiAuth, chat_assistant_id, session_ids[0], {"name": payload["name"].upper()})

        res = update_session_with_chat_assistant(HttpApiAuth, chat_assistant_id, session_ids[0], payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            res = list_session_with_chat_assistants(HttpApiAuth, chat_assistant_id, {"id": session_ids[0]})
            assert res["data"][0]["name"] == payload["name"]
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "chat_assistant_id, expected_code, expected_message",
        [
            (INVALID_ID_32, 102, "Session does not exist"),
        ],
    )
    def test_invalid_chat_assistant_id(self, HttpApiAuth, add_sessions_with_chat_assistant_func, chat_assistant_id, expected_code, expected_message):
        _, session_ids = add_sessions_with_chat_assistant_func
        res = update_session_with_chat_assistant(HttpApiAuth, chat_assistant_id, session_ids[0], {"name": "valid_name"})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "session_id, expected_code, expected_message",
        [
            ("", 100, "<MethodNotAllowed '405: Method Not Allowed'>"),
            ("invalid_session_id", 102, "Session does not exist"),
        ],
    )
    def test_invalid_session_id(self, HttpApiAuth, add_sessions_with_chat_assistant_func, session_id, expected_code, expected_message):
        chat_assistant_id, _ = add_sessions_with_chat_assistant_func
        res = update_session_with_chat_assistant(HttpApiAuth, chat_assistant_id, session_id, {"name": "valid_name"})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.p3
    def test_repeated_update_session(self, HttpApiAuth, add_sessions_with_chat_assistant_func):
        chat_assistant_id, session_ids = add_sessions_with_chat_assistant_func
        res = update_session_with_chat_assistant(HttpApiAuth, chat_assistant_id, session_ids[0], {"name": "valid_name_1"})
        assert res["code"] == 0

        res = update_session_with_chat_assistant(HttpApiAuth, chat_assistant_id, session_ids[0], {"name": "valid_name_2"})
        assert res["code"] == 0

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            pytest.param({"unknown_key": "unknown_value"}, 100, "ValueError", marks=pytest.mark.skip),
            ({}, 0, ""),
            pytest.param(None, 100, "TypeError", marks=pytest.mark.skip),
        ],
    )
    def test_invalid_params(self, HttpApiAuth, add_sessions_with_chat_assistant_func, payload, expected_code, expected_message):
        chat_assistant_id, session_ids = add_sessions_with_chat_assistant_func
        res = update_session_with_chat_assistant(HttpApiAuth, chat_assistant_id, session_ids[0], payload)
        assert res["code"] == expected_code
        if expected_code != 0:
            assert expected_message in res["message"]

    @pytest.mark.p3
    def test_concurrent_update_session(self, HttpApiAuth, add_sessions_with_chat_assistant_func):
        count = 50
        chat_assistant_id, session_ids = add_sessions_with_chat_assistant_func

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    update_session_with_chat_assistant,
                    HttpApiAuth,
                    chat_assistant_id,
                    session_ids[randint(0, 4)],
                    {"name": f"update session test {i}"},
                )
                for i in range(count)
            ]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

    @pytest.mark.p3
    def test_update_session_to_deleted_chat_assistant(self, HttpApiAuth, add_sessions_with_chat_assistant_func):
        chat_assistant_id, session_ids = add_sessions_with_chat_assistant_func
        delete_chat_assistants(HttpApiAuth, {"ids": [chat_assistant_id]})
        res = update_session_with_chat_assistant(HttpApiAuth, chat_assistant_id, session_ids[0], {"name": "valid_name"})
        assert res["code"] == 102
        assert res["message"] == "You do not own the session"
