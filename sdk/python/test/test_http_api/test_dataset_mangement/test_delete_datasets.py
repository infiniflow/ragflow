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
import uuid
from concurrent.futures import ThreadPoolExecutor

import pytest
from common import (
    INVALID_API_TOKEN,
    batch_create_datasets,
    delete_datasets,
    list_datasets,
)
from libs.auth import RAGFlowHttpApiAuth


class TestAuthorization:
    @pytest.mark.p1
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
    def test_auth_invalid(self, auth, expected_code, expected_message):
        res = delete_datasets(auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestRquest:
    @pytest.mark.p3
    def test_content_type_bad(self, get_http_api_auth):
        BAD_CONTENT_TYPE = "text/xml"
        res = delete_datasets(get_http_api_auth, headers={"Content-Type": BAD_CONTENT_TYPE})
        assert res["code"] == 101, res
        assert res["message"] == f"Unsupported content type: Expected application/json, got {BAD_CONTENT_TYPE}", res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            ("a", "Malformed JSON syntax: Missing commas/brackets or invalid encoding"),
            ('"a"', "Invalid request payload: expected object, got str"),
        ],
        ids=["malformed_json_syntax", "invalid_request_payload_type"],
    )
    def test_payload_bad(self, get_http_api_auth, payload, expected_message):
        res = delete_datasets(get_http_api_auth, data=payload)
        assert res["code"] == 101, res
        assert res["message"] == expected_message, res

    @pytest.mark.p3
    def test_payload_unset(self, get_http_api_auth):
        res = delete_datasets(get_http_api_auth, None)
        assert res["code"] == 101, res
        assert res["message"] == "Malformed JSON syntax: Missing commas/brackets or invalid encoding", res


class TestCapability:
    @pytest.mark.p3
    def test_delete_dataset_1k(self, get_http_api_auth):
        ids = batch_create_datasets(get_http_api_auth, 1_000)
        res = delete_datasets(get_http_api_auth, {"ids": ids})
        assert res["code"] == 0, res

        res = list_datasets(get_http_api_auth)
        assert len(res["data"]) == 0, res

    @pytest.mark.p3
    def test_concurrent_deletion(self, get_http_api_auth):
        dataset_num = 1_000
        ids = batch_create_datasets(get_http_api_auth, dataset_num)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(delete_datasets, get_http_api_auth, {"ids": ids[i : i + 1]}) for i in range(dataset_num)]
        responses = [f.result() for f in futures]
        assert all(r["code"] == 0 for r in responses), responses


class TestDatasetsDelete:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "func, expected_code, expected_message, remaining",
        [
            (lambda r: {"ids": r[:1]}, 0, "", 2),
            (lambda r: {"ids": r}, 0, "", 0),
        ],
        ids=["single_dataset", "multiple_datasets"],
    )
    def test_ids(self, get_http_api_auth, add_datasets_func, func, expected_code, expected_message, remaining):
        dataset_ids = add_datasets_func
        if callable(func):
            payload = func(dataset_ids)
        res = delete_datasets(get_http_api_auth, payload)
        assert res["code"] == expected_code, res

        res = list_datasets(get_http_api_auth)
        assert len(res["data"]) == remaining, res

    @pytest.mark.p1
    @pytest.mark.usefixtures("add_dataset_func")
    def test_ids_empty(self, get_http_api_auth):
        payload = {"ids": []}
        res = delete_datasets(get_http_api_auth, payload)
        assert res["code"] == 0, res

        res = list_datasets(get_http_api_auth)
        assert len(res["data"]) == 1, res

    @pytest.mark.p1
    @pytest.mark.usefixtures("add_datasets_func")
    def test_ids_none(self, get_http_api_auth):
        payload = {"ids": None}
        res = delete_datasets(get_http_api_auth, payload)
        assert res["code"] == 0, res

        res = list_datasets(get_http_api_auth)
        assert len(res["data"]) == 0, res

    @pytest.mark.p2
    @pytest.mark.usefixtures("add_dataset_func")
    def test_id_not_uuid(self, get_http_api_auth):
        payload = {"ids": ["not_uuid"]}
        res = delete_datasets(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert "Invalid UUID1 format" in res["message"], res

        res = list_datasets(get_http_api_auth)
        assert len(res["data"]) == 1, res

    @pytest.mark.p3
    @pytest.mark.usefixtures("add_dataset_func")
    def test_id_not_uuid1(self, get_http_api_auth):
        payload = {"ids": [uuid.uuid4().hex]}
        res = delete_datasets(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert "Invalid UUID1 format" in res["message"], res

    @pytest.mark.p2
    @pytest.mark.usefixtures("add_dataset_func")
    def test_id_wrong_uuid(self, get_http_api_auth):
        payload = {"ids": ["d94a8dc02c9711f0930f7fbc369eab6d"]}
        res = delete_datasets(get_http_api_auth, payload)
        assert res["code"] == 108, res
        assert "lacks permission for dataset" in res["message"], res

        res = list_datasets(get_http_api_auth)
        assert len(res["data"]) == 1, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "func",
        [
            lambda r: {"ids": ["d94a8dc02c9711f0930f7fbc369eab6d"] + r},
            lambda r: {"ids": r[:1] + ["d94a8dc02c9711f0930f7fbc369eab6d"] + r[1:3]},
            lambda r: {"ids": r + ["d94a8dc02c9711f0930f7fbc369eab6d"]},
        ],
    )
    def test_ids_partial_invalid(self, get_http_api_auth, add_datasets_func, func):
        dataset_ids = add_datasets_func
        if callable(func):
            payload = func(dataset_ids)
        res = delete_datasets(get_http_api_auth, payload)
        assert res["code"] == 108, res
        assert "lacks permission for dataset" in res["message"], res

        res = list_datasets(get_http_api_auth)
        assert len(res["data"]) == 3, res

    @pytest.mark.p2
    def test_ids_duplicate(self, get_http_api_auth, add_datasets_func):
        dataset_ids = add_datasets_func
        payload = {"ids": dataset_ids + dataset_ids}
        res = delete_datasets(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert "Duplicate ids:" in res["message"], res

        res = list_datasets(get_http_api_auth)
        assert len(res["data"]) == 3, res

    @pytest.mark.p2
    def test_repeated_delete(self, get_http_api_auth, add_datasets_func):
        dataset_ids = add_datasets_func
        payload = {"ids": dataset_ids}
        res = delete_datasets(get_http_api_auth, payload)
        assert res["code"] == 0, res

        res = delete_datasets(get_http_api_auth, payload)
        assert res["code"] == 108, res
        assert "lacks permission for dataset" in res["message"], res

    @pytest.mark.p2
    @pytest.mark.usefixtures("add_dataset_func")
    def test_field_unsupported(self, get_http_api_auth):
        payload = {"unknown_field": "unknown_field"}
        res = delete_datasets(get_http_api_auth, payload)
        assert res["code"] == 101, res
        assert "Extra inputs are not permitted" in res["message"], res

        res = list_datasets(get_http_api_auth)
        assert len(res["data"]) == 1, res
