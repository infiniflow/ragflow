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
from common import INVALID_API_TOKEN, batch_create_chat_assistants, delete_chat_assistants, list_chat_assistants
from libs.auth import RAGFlowHttpApiAuth


@pytest.mark.p1
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
        res = delete_chat_assistants(auth)
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestChatAssistantsDelete:
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message, remaining",
        [
            pytest.param(None, 0, "", 0, marks=pytest.mark.p3),
            pytest.param({"ids": []}, 0, "", 0, marks=pytest.mark.p3),
            pytest.param({"ids": ["invalid_id"]}, 102, "Assistant(invalid_id) not found.", 5, marks=pytest.mark.p3),
            pytest.param({"ids": ["\n!?。；！？\"'"]}, 102, """Assistant(\n!?。；！？"\') not found.""", 5, marks=pytest.mark.p3),
            pytest.param("not json", 100, "AttributeError(\"'str' object has no attribute 'get'\")", 5, marks=pytest.mark.p3),
            pytest.param(lambda r: {"ids": r[:1]}, 0, "", 4, marks=pytest.mark.p3),
            pytest.param(lambda r: {"ids": r}, 0, "", 0, marks=pytest.mark.p1),
        ],
    )
    def test_basic_scenarios(self, get_http_api_auth, add_chat_assistants_func, payload, expected_code, expected_message, remaining):
        _, _, chat_assistant_ids = add_chat_assistants_func
        if callable(payload):
            payload = payload(chat_assistant_ids)
        res = delete_chat_assistants(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if res["code"] != 0:
            assert res["message"] == expected_message

        res = list_chat_assistants(get_http_api_auth)
        assert len(res["data"]) == remaining

    @pytest.mark.parametrize(
        "payload",
        [
            pytest.param(lambda r: {"ids": ["invalid_id"] + r}, marks=pytest.mark.p3),
            pytest.param(lambda r: {"ids": r[:1] + ["invalid_id"] + r[1:5]}, marks=pytest.mark.p1),
            pytest.param(lambda r: {"ids": r + ["invalid_id"]}, marks=pytest.mark.p3),
        ],
    )
    def test_delete_partial_invalid_id(self, get_http_api_auth, add_chat_assistants_func, payload):
        _, _, chat_assistant_ids = add_chat_assistants_func
        if callable(payload):
            payload = payload(chat_assistant_ids)
        res = delete_chat_assistants(get_http_api_auth, payload)
        assert res["code"] == 0
        assert res["data"]["errors"][0] == "Assistant(invalid_id) not found."
        assert res["data"]["success_count"] == 5

        res = list_chat_assistants(get_http_api_auth)
        assert len(res["data"]) == 0

    @pytest.mark.p3
    def test_repeated_deletion(self, get_http_api_auth, add_chat_assistants_func):
        _, _, chat_assistant_ids = add_chat_assistants_func
        res = delete_chat_assistants(get_http_api_auth, {"ids": chat_assistant_ids})
        assert res["code"] == 0

        res = delete_chat_assistants(get_http_api_auth, {"ids": chat_assistant_ids})
        assert res["code"] == 102
        assert "not found" in res["message"]

    @pytest.mark.p3
    def test_duplicate_deletion(self, get_http_api_auth, add_chat_assistants_func):
        _, _, chat_assistant_ids = add_chat_assistants_func
        res = delete_chat_assistants(get_http_api_auth, {"ids": chat_assistant_ids + chat_assistant_ids})
        assert res["code"] == 0
        assert "Duplicate assistant ids" in res["data"]["errors"][0]
        assert res["data"]["success_count"] == 5

        res = list_chat_assistants(get_http_api_auth)
        assert res["code"] == 0

    @pytest.mark.p3
    def test_concurrent_deletion(self, get_http_api_auth):
        ids = batch_create_chat_assistants(get_http_api_auth, 100)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(delete_chat_assistants, get_http_api_auth, {"ids": ids[i : i + 1]}) for i in range(100)]
        responses = [f.result() for f in futures]
        assert all(r["code"] == 0 for r in responses)

    @pytest.mark.p3
    def test_delete_10k(self, get_http_api_auth):
        ids = batch_create_chat_assistants(get_http_api_auth, 10_000)
        res = delete_chat_assistants(get_http_api_auth, {"ids": ids})
        assert res["code"] == 0

        res = list_chat_assistants(get_http_api_auth)
        assert len(res["data"]) == 0
