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
from common import (
    INVALID_API_TOKEN,
    create_datasets,
    delete_dataset,
    list_dataset,
)
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
    def test_invalid_auth(
        self, get_http_api_auth, auth, expected_code, expected_message
    ):
        ids = create_datasets(get_http_api_auth, 1)
        res = delete_dataset(auth, {"ids": ids})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

        res = list_dataset(get_http_api_auth)
        assert len(res["data"]) == 1


class TestDatasetDeletion:
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message, remaining",
        [
            (None, 0, "", 0),
            ({"ids": []}, 0, "", 0),
            ({"ids": ["invalid_id"]}, 102, "You don't own the dataset invalid_id", 3),
            (
                {"ids": ["\n!?。；！？\"'"]},
                102,
                "You don't own the dataset \n!?。；！？\"'",
                3,
            ),
            (
                "not json",
                100,
                "AttributeError(\"'str' object has no attribute 'get'\")",
                3,
            ),
            (lambda r: {"ids": r[:1]}, 0, "", 2),
            (lambda r: {"ids": r}, 0, "", 0),
        ],
    )
    def test_basic_scenarios(
        self, get_http_api_auth, payload, expected_code, expected_message, remaining
    ):
        ids = create_datasets(get_http_api_auth, 3)
        if callable(payload):
            payload = payload(ids)
        res = delete_dataset(get_http_api_auth, payload)
        assert res["code"] == expected_code
        if res["code"] != 0:
            assert res["message"] == expected_message

        res = list_dataset(get_http_api_auth)
        assert len(res["data"]) == remaining

    @pytest.mark.parametrize(
        "payload",
        [
            lambda r: {"ids": ["invalid_id"] + r},
            lambda r: {"ids": r[:1] + ["invalid_id"] + r[1:3]},
            lambda r: {"ids": r + ["invalid_id"]},
        ],
    )
    def test_delete_partial_invalid_id(self, get_http_api_auth, payload):
        ids = create_datasets(get_http_api_auth, 3)
        if callable(payload):
            payload = payload(ids)
        res = delete_dataset(get_http_api_auth, payload)
        assert res["code"] == 0
        assert res["data"]["errors"][0] == "You don't own the dataset invalid_id"
        assert res["data"]["success_count"] == 3

        res = list_dataset(get_http_api_auth)
        assert len(res["data"]) == 0

    def test_repeated_deletion(self, get_http_api_auth):
        ids = create_datasets(get_http_api_auth, 1)
        res = delete_dataset(get_http_api_auth, {"ids": ids})
        assert res["code"] == 0

        res = delete_dataset(get_http_api_auth, {"ids": ids})
        assert res["code"] == 102
        assert res["message"] == f"You don't own the dataset {ids[0]}"

    def test_concurrent_deletion(self, get_http_api_auth):
        ids = create_datasets(get_http_api_auth, 100)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(
                    delete_dataset, get_http_api_auth, {"ids": ids[i : i + 1]}
                )
                for i in range(100)
            ]
        responses = [f.result() for f in futures]
        assert all(r["code"] == 0 for r in responses)

    @pytest.mark.slow
    def test_delete_10k(self, get_http_api_auth):
        ids = create_datasets(get_http_api_auth, 10_000)
        res = delete_dataset(get_http_api_auth, {"ids": ids})
        assert res["code"] == 0

        res = list_dataset(get_http_api_auth)
        assert len(res["data"]) == 0
