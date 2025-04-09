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
from concurrent.futures import ThreadPoolExecutor

import pytest
from common import INVALID_API_TOKEN, SESSION_WITH_CHAT_NAME_LIMIT, create_session_with_chat_assistant, delete_chat_assistants, list_session_with_chat_assistants
from libs.auth import RAGFlowHttpApiAuth


class TestAuthorization:
    @pytest.mark.parametrize(
        "auth, expected_code, expected_message",
        [
            (None, 0, "`Authorization` can't be empty"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                109,
                "Authentication error: API key is invalid!",
            ),
        ],
    )
    def test_invalid_auth(self, auth, expected_code, expected_message):
        res = create_session_with_chat_assistant(auth, "chat_assistant_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


@pytest.mark.usefixtures("clear_session_with_chat_assistants")
class TestSessionWithChatAssistantCreate:
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            ({"name": "valid_name"}, 0, ""),
            pytest.param({"name": "a" * (SESSION_WITH_CHAT_NAME_LIMIT + 1)}, 102, "", marks=pytest.mark.skip(reason="issues/")),
            pytest.param({"name": 1}, 100, "", marks=pytest.mark.skip(reason="issues/")),
            ({"name": ""}, 102, "`name` can not be empty."),
            ({"name": "duplicated_name"}, 0, ""),
            ({"name": "case insensitive"}, 0, ""),
        ],
    )
    def test_name(self, get_http_api_auth, add_chat_assistants, payload, expected_code, expected_message):
        _, _, chat_assistant_ids = add_chat_assistants
        if payload["name"] == "duplicated_name":
            create_session_with_chat_assistant(get_http_api_auth, chat_assistant_ids[0], payload)
        elif payload["name"] == "case insensitive":
            create_session_with_chat_assistant(get_http_api_auth, chat_assistant_ids[0], {"name": payload["name"].upper()})

        res = create_session_with_chat_assistant(get_http_api_auth, chat_assistant_ids[0], payload)
        assert res["code"] == expected_code, res
        if expected_code == 0:
            assert res["data"]["name"] == payload["name"]
            assert res["data"]["chat_id"] == chat_assistant_ids[0]
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "chat_assistant_id, expected_code, expected_message",
        [
            ("", 100, "<MethodNotAllowed '405: Method Not Allowed'>"),
            ("invalid_chat_assistant_id", 102, "You do not own the assistant."),
        ],
    )
    def test_invalid_chat_assistant_id(self, get_http_api_auth, chat_assistant_id, expected_code, expected_message):
        res = create_session_with_chat_assistant(get_http_api_auth, chat_assistant_id, {"name": "valid_name"})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.slow
    def test_concurrent_create_session(self, get_http_api_auth, add_chat_assistants):
        chunk_num = 1000
        _, _, chat_assistant_ids = add_chat_assistants
        res = list_session_with_chat_assistants(get_http_api_auth, chat_assistant_ids[0])
        if res["code"] != 0:
            assert False, res
        chunks_count = len(res["data"])

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    create_session_with_chat_assistant,
                    get_http_api_auth,
                    chat_assistant_ids[0],
                    {"name": f"session with chat assistant test {i}"},
                )
                for i in range(chunk_num)
            ]
        responses = [f.result() for f in futures]
        assert all(r["code"] == 0 for r in responses)
        res = list_session_with_chat_assistants(get_http_api_auth, chat_assistant_ids[0], {"page_size": chunk_num})
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]) == chunks_count + chunk_num

    def test_add_session_to_deleted_chat_assistant(self, get_http_api_auth, add_chat_assistants):
        _, _, chat_assistant_ids = add_chat_assistants
        res = delete_chat_assistants(get_http_api_auth, {"ids": [chat_assistant_ids[0]]})
        assert res["code"] == 0
        res = create_session_with_chat_assistant(get_http_api_auth, chat_assistant_ids[0], {"name": "valid_name"})
        assert res["code"] == 102
        assert res["message"] == "You do not own the assistant."
