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
import sys
from pathlib import Path
import types

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from session_stub import load_session_module
from common import batch_add_sessions_with_chat_assistant, delete_session_with_chat_assistants, list_session_with_chat_assistants
from configs import INVALID_API_TOKEN
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
        res = delete_session_with_chat_assistants(invalid_auth, "chat_assistant_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestSessionWithChatAssistantDelete:
    @pytest.mark.p3
    @pytest.mark.parametrize(
        "chat_assistant_id, expected_code, expected_message",
        [
            ("", 100, "<MethodNotAllowed '405: Method Not Allowed'>"),
            (
                "invalid_chat_assistant_id",
                102,
                "You don't own the chat",
            ),
        ],
    )
    def test_invalid_chat_assistant_id(self, HttpApiAuth, add_sessions_with_chat_assistant_func, chat_assistant_id, expected_code, expected_message):
        _, session_ids = add_sessions_with_chat_assistant_func
        res = delete_session_with_chat_assistants(HttpApiAuth, chat_assistant_id, {"ids": session_ids})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "payload",
        [
            pytest.param(lambda r: {"ids": ["invalid_id"] + r}, marks=pytest.mark.p3),
            pytest.param(lambda r: {"ids": r[:1] + ["invalid_id"] + r[1:5]}, marks=pytest.mark.p1),
            pytest.param(lambda r: {"ids": r + ["invalid_id"]}, marks=pytest.mark.p3),
        ],
    )
    def test_delete_partial_invalid_id(self, HttpApiAuth, add_sessions_with_chat_assistant_func, payload):
        chat_assistant_id, session_ids = add_sessions_with_chat_assistant_func
        if callable(payload):
            payload = payload(session_ids)
        res = delete_session_with_chat_assistants(HttpApiAuth, chat_assistant_id, payload)
        assert res["code"] == 0
        assert res["data"]["errors"][0] == "The chat doesn't own the session invalid_id"

        res = list_session_with_chat_assistants(HttpApiAuth, chat_assistant_id)
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]) == 0

    @pytest.mark.p3
    def test_repeated_deletion(self, HttpApiAuth, add_sessions_with_chat_assistant_func):
        chat_assistant_id, session_ids = add_sessions_with_chat_assistant_func
        payload = {"ids": session_ids}
        res = delete_session_with_chat_assistants(HttpApiAuth, chat_assistant_id, payload)
        assert res["code"] == 0

        res = delete_session_with_chat_assistants(HttpApiAuth, chat_assistant_id, payload)
        assert res["code"] == 102
        assert "The chat doesn't own the session" in res["message"]

    @pytest.mark.p3
    def test_duplicate_deletion(self, HttpApiAuth, add_sessions_with_chat_assistant_func):
        chat_assistant_id, session_ids = add_sessions_with_chat_assistant_func
        res = delete_session_with_chat_assistants(HttpApiAuth, chat_assistant_id, {"ids": session_ids * 2})
        assert res["code"] == 0
        assert "Duplicate session ids" in res["data"]["errors"][0]
        assert res["data"]["success_count"] == 5

        res = list_session_with_chat_assistants(HttpApiAuth, chat_assistant_id)
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]) == 0

    @pytest.mark.p3
    def test_concurrent_deletion(self, HttpApiAuth, add_chat_assistants):
        count = 100
        _, _, chat_assistant_ids = add_chat_assistants
        session_ids = batch_add_sessions_with_chat_assistant(HttpApiAuth, chat_assistant_ids[0], count)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    delete_session_with_chat_assistants,
                    HttpApiAuth,
                    chat_assistant_ids[0],
                    {"ids": session_ids[i : i + 1]},
                )
                for i in range(count)
            ]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

    @pytest.mark.p3
    def test_delete_1k(self, HttpApiAuth, add_chat_assistants):
        sessions_num = 1_000
        _, _, chat_assistant_ids = add_chat_assistants
        session_ids = batch_add_sessions_with_chat_assistant(HttpApiAuth, chat_assistant_ids[0], sessions_num)

        res = delete_session_with_chat_assistants(HttpApiAuth, chat_assistant_ids[0], {"ids": session_ids})
        assert res["code"] == 0

        res = list_session_with_chat_assistants(HttpApiAuth, chat_assistant_ids[0])
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]) == 0

    @pytest.mark.parametrize(
        "payload, expected_code, expected_message, remaining",
        [
            pytest.param(None, 0, """TypeError("argument of type \'NoneType\' is not iterable")""", 0, marks=pytest.mark.skip),
            pytest.param({"ids": ["invalid_id"]}, 102, "The chat doesn't own the session invalid_id", 5, marks=pytest.mark.p3),
            pytest.param("not json", 100, """AttributeError("\'str\' object has no attribute \'get\'")""", 5, marks=pytest.mark.skip),
            pytest.param(lambda r: {"ids": r[:1]}, 0, "", 4, marks=pytest.mark.p3),
            pytest.param(lambda r: {"ids": r}, 0, "", 0, marks=pytest.mark.p1),
            pytest.param({"ids": []}, 0, "", 0, marks=pytest.mark.p3),
        ],
    )
    def test_basic_scenarios(
        self,
        HttpApiAuth,
        add_sessions_with_chat_assistant_func,
        payload,
        expected_code,
        expected_message,
        remaining,
    ):
        chat_assistant_id, session_ids = add_sessions_with_chat_assistant_func
        if callable(payload):
            payload = payload(session_ids)
        res = delete_session_with_chat_assistants(HttpApiAuth, chat_assistant_id, payload)
        assert res["code"] == expected_code
        if res["code"] != 0:
            assert res["message"] == expected_message

        res = list_session_with_chat_assistants(HttpApiAuth, chat_assistant_id)
        if res["code"] != 0:
            assert False, res
        assert len(res["data"]) == remaining


@pytest.mark.asyncio
async def test_delete_sessions_all_errors(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"ids": ["s1"]}

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="chat")])
    mod.ConversationService.query = classmethod(lambda cls, **kwargs: [] if "id" in kwargs else [types.SimpleNamespace(id="s1")])

    resp = await mod.delete("tenant", "chat")
    assert resp["code"] != 0
    assert "doesn't own the session" in resp["message"]


@pytest.mark.asyncio
async def test_delete_sessions_duplicates_partial(monkeypatch):
    mod = load_session_module(monkeypatch)

    async def _get_request_json():
        return {"ids": ["s1", "s1"]}

    mod.get_request_json = _get_request_json
    mod.DialogService.query = classmethod(lambda cls, **_kwargs: [types.SimpleNamespace(id="chat")])
    mod.ConversationService.query = classmethod(lambda cls, **kwargs: [types.SimpleNamespace(id="s1")] if "id" in kwargs else [types.SimpleNamespace(id="s1")])

    mod.check_duplicate_ids = lambda ids, _kind="session": (["s1"], ["duplicate s1"])
    resp = await mod.delete("tenant", "chat")
    assert resp["code"] == 0
    assert "Partially deleted" in resp.get("message", "")

    mod.ConversationService.query = classmethod(lambda cls, **kwargs: [] if "id" in kwargs else [types.SimpleNamespace(id="s1")])
    resp = await mod.delete("tenant", "chat")
    assert resp["code"] != 0
    assert "duplicate" in resp.get("message", "")
