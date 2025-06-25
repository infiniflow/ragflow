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
from common import delete_datasets, list_chat_assistants
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth
from utils import is_sorted


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
        res = list_chat_assistants(invalid_auth)
        assert res["code"] == expected_code
        assert res["message"] == expected_message


@pytest.mark.usefixtures("add_chat_assistants")
class TestChatAssistantsList:
    @pytest.mark.p1
    def test_default(self, HttpApiAuth):
        res = list_chat_assistants(HttpApiAuth)
        assert res["code"] == 0
        assert len(res["data"]) == 5

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_code, expected_page_size, expected_message",
        [
            ({"page": None, "page_size": 2}, 0, 2, ""),
            ({"page": 0, "page_size": 2}, 0, 2, ""),
            ({"page": 2, "page_size": 2}, 0, 2, ""),
            ({"page": 3, "page_size": 2}, 0, 1, ""),
            ({"page": "3", "page_size": 2}, 0, 1, ""),
            pytest.param(
                {"page": -1, "page_size": 2},
                100,
                0,
                "1064",
                marks=pytest.mark.skip(reason="issues/5851"),
            ),
            pytest.param(
                {"page": "a", "page_size": 2},
                100,
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.skip(reason="issues/5851"),
            ),
        ],
    )
    def test_page(self, HttpApiAuth, params, expected_code, expected_page_size, expected_message):
        res = list_chat_assistants(HttpApiAuth, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_code, expected_page_size, expected_message",
        [
            ({"page_size": None}, 0, 5, ""),
            ({"page_size": 0}, 0, 0, ""),
            ({"page_size": 1}, 0, 1, ""),
            ({"page_size": 6}, 0, 5, ""),
            ({"page_size": "1"}, 0, 1, ""),
            pytest.param(
                {"page_size": -1},
                100,
                0,
                "1064",
                marks=pytest.mark.skip(reason="issues/5851"),
            ),
            pytest.param(
                {"page_size": "a"},
                100,
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.skip(reason="issues/5851"),
            ),
        ],
    )
    def test_page_size(
        self,
        HttpApiAuth,
        params,
        expected_code,
        expected_page_size,
        expected_message,
    ):
        res = list_chat_assistants(HttpApiAuth, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, expected_code, assertions, expected_message",
        [
            ({"orderby": None}, 0, lambda r: (is_sorted(r["data"], "create_time", True)), ""),
            ({"orderby": "create_time"}, 0, lambda r: (is_sorted(r["data"], "create_time", True)), ""),
            ({"orderby": "update_time"}, 0, lambda r: (is_sorted(r["data"], "update_time", True)), ""),
            pytest.param(
                {"orderby": "name", "desc": "False"},
                0,
                lambda r: (is_sorted(r["data"], "name", False)),
                "",
                marks=pytest.mark.skip(reason="issues/5851"),
            ),
            pytest.param(
                {"orderby": "unknown"},
                102,
                0,
                "orderby should be create_time or update_time",
                marks=pytest.mark.skip(reason="issues/5851"),
            ),
        ],
    )
    def test_orderby(
        self,
        HttpApiAuth,
        params,
        expected_code,
        assertions,
        expected_message,
    ):
        res = list_chat_assistants(HttpApiAuth, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            if callable(assertions):
                assert assertions(res)
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, expected_code, assertions, expected_message",
        [
            ({"desc": None}, 0, lambda r: (is_sorted(r["data"], "create_time", True)), ""),
            ({"desc": "true"}, 0, lambda r: (is_sorted(r["data"], "create_time", True)), ""),
            ({"desc": "True"}, 0, lambda r: (is_sorted(r["data"], "create_time", True)), ""),
            ({"desc": True}, 0, lambda r: (is_sorted(r["data"], "create_time", True)), ""),
            ({"desc": "false"}, 0, lambda r: (is_sorted(r["data"], "create_time", False)), ""),
            ({"desc": "False"}, 0, lambda r: (is_sorted(r["data"], "create_time", False)), ""),
            ({"desc": False}, 0, lambda r: (is_sorted(r["data"], "create_time", False)), ""),
            ({"desc": "False", "orderby": "update_time"}, 0, lambda r: (is_sorted(r["data"], "update_time", False)), ""),
            pytest.param(
                {"desc": "unknown"},
                102,
                0,
                "desc should be true or false",
                marks=pytest.mark.skip(reason="issues/5851"),
            ),
        ],
    )
    def test_desc(
        self,
        HttpApiAuth,
        params,
        expected_code,
        assertions,
        expected_message,
    ):
        res = list_chat_assistants(HttpApiAuth, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            if callable(assertions):
                assert assertions(res)
        else:
            assert res["message"] == expected_message

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_code, expected_num, expected_message",
        [
            ({"name": None}, 0, 5, ""),
            ({"name": ""}, 0, 5, ""),
            ({"name": "test_chat_assistant_1"}, 0, 1, ""),
            ({"name": "unknown"}, 102, 0, "The chat doesn't exist"),
        ],
    )
    def test_name(self, HttpApiAuth, params, expected_code, expected_num, expected_message):
        res = list_chat_assistants(HttpApiAuth, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            if params["name"] in [None, ""]:
                assert len(res["data"]) == expected_num
            else:
                assert res["data"][0]["name"] == params["name"]
        else:
            assert res["message"] == expected_message

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "chat_assistant_id, expected_code, expected_num, expected_message",
        [
            (None, 0, 5, ""),
            ("", 0, 5, ""),
            (lambda r: r[0], 0, 1, ""),
            ("unknown", 102, 0, "The chat doesn't exist"),
        ],
    )
    def test_id(
        self,
        HttpApiAuth,
        add_chat_assistants,
        chat_assistant_id,
        expected_code,
        expected_num,
        expected_message,
    ):
        _, _, chat_assistant_ids = add_chat_assistants
        if callable(chat_assistant_id):
            params = {"id": chat_assistant_id(chat_assistant_ids)}
        else:
            params = {"id": chat_assistant_id}

        res = list_chat_assistants(HttpApiAuth, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            if params["id"] in [None, ""]:
                assert len(res["data"]) == expected_num
            else:
                assert res["data"][0]["id"] == params["id"]
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "chat_assistant_id, name, expected_code, expected_num, expected_message",
        [
            (lambda r: r[0], "test_chat_assistant_0", 0, 1, ""),
            (lambda r: r[0], "test_chat_assistant_1", 102, 0, "The chat doesn't exist"),
            (lambda r: r[0], "unknown", 102, 0, "The chat doesn't exist"),
            ("id", "chat_assistant_0", 102, 0, "The chat doesn't exist"),
        ],
    )
    def test_name_and_id(
        self,
        HttpApiAuth,
        add_chat_assistants,
        chat_assistant_id,
        name,
        expected_code,
        expected_num,
        expected_message,
    ):
        _, _, chat_assistant_ids = add_chat_assistants
        if callable(chat_assistant_id):
            params = {"id": chat_assistant_id(chat_assistant_ids), "name": name}
        else:
            params = {"id": chat_assistant_id, "name": name}

        res = list_chat_assistants(HttpApiAuth, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]) == expected_num
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    def test_concurrent_list(self, HttpApiAuth):
        count = 100
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(list_chat_assistants, HttpApiAuth) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

    @pytest.mark.p3
    def test_invalid_params(self, HttpApiAuth):
        params = {"a": "b"}
        res = list_chat_assistants(HttpApiAuth, params=params)
        assert res["code"] == 0
        assert len(res["data"]) == 5

    @pytest.mark.p2
    def test_list_chats_after_deleting_associated_dataset(self, HttpApiAuth, add_chat_assistants):
        dataset_id, _, _ = add_chat_assistants
        res = delete_datasets(HttpApiAuth, {"ids": [dataset_id]})
        assert res["code"] == 0

        res = list_chat_assistants(HttpApiAuth)
        assert res["code"] == 0
        assert len(res["data"]) == 5
