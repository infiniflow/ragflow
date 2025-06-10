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
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from common import INVALID_API_TOKEN, list_datasets
from libs.auth import RAGFlowHttpApiAuth
from utils import is_sorted


class TestAuthorization:
    @pytest.mark.p1
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
    def test_auth_invalid(self, invalid_auth, expected_code, expected_message):
        res = list_datasets(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestCapability:
    @pytest.mark.p3
    def test_concurrent_list(self, api_key):
        count = 100
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(list_datasets, api_key) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)


@pytest.mark.usefixtures("add_datasets")
class TestDatasetsList:
    @pytest.mark.p1
    def test_params_unset(self, api_key):
        res = list_datasets(api_key, None)
        assert res["code"] == 0, res
        assert len(res["data"]) == 5, res

    @pytest.mark.p2
    def test_params_empty(self, api_key):
        res = list_datasets(api_key, {})
        assert res["code"] == 0, res
        assert len(res["data"]) == 5, res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size",
        [
            ({"page": 2, "page_size": 2}, 2),
            ({"page": 3, "page_size": 2}, 1),
            ({"page": 4, "page_size": 2}, 0),
            ({"page": "2", "page_size": 2}, 2),
            ({"page": 1, "page_size": 10}, 5),
        ],
        ids=["normal_middle_page", "normal_last_partial_page", "beyond_max_page", "string_page_number", "full_data_single_page"],
    )
    def test_page(self, api_key, params, expected_page_size):
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        assert len(res["data"]) == expected_page_size, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_code, expected_message",
        [
            ({"page": 0}, 101, "Input should be greater than or equal to 1"),
            ({"page": "a"}, 101, "Input should be a valid integer, unable to parse string as an integer"),
        ],
        ids=["page_0", "page_a"],
    )
    def test_page_invalid(self, api_key, params, expected_code, expected_message):
        res = list_datasets(api_key, params=params)
        assert res["code"] == expected_code, res
        assert expected_message in res["message"], res

    @pytest.mark.p2
    def test_page_none(self, api_key):
        params = {"page": None}
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        assert len(res["data"]) == 5, res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size",
        [
            ({"page_size": 1}, 1),
            ({"page_size": 3}, 3),
            ({"page_size": 5}, 5),
            ({"page_size": 6}, 5),
            ({"page_size": "1"}, 1),
        ],
        ids=["min_valid_page_size", "medium_page_size", "page_size_equals_total", "page_size_exceeds_total", "string_type_page_size"],
    )
    def test_page_size(self, api_key, params, expected_page_size):
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        assert len(res["data"]) == expected_page_size, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_code, expected_message",
        [
            ({"page_size": 0}, 101, "Input should be greater than or equal to 1"),
            ({"page_size": "a"}, 101, "Input should be a valid integer, unable to parse string as an integer"),
        ],
    )
    def test_page_size_invalid(self, api_key, params, expected_code, expected_message):
        res = list_datasets(api_key, params)
        assert res["code"] == expected_code, res
        assert expected_message in res["message"], res

    @pytest.mark.p2
    def test_page_size_none(self, api_key):
        params = {"page_size": None}
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        assert len(res["data"]) == 5, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, assertions",
        [
            ({"orderby": "create_time"}, lambda r: (is_sorted(r["data"], "create_time", True))),
            ({"orderby": "update_time"}, lambda r: (is_sorted(r["data"], "update_time", True))),
            ({"orderby": "CREATE_TIME"}, lambda r: (is_sorted(r["data"], "create_time", True))),
            ({"orderby": "UPDATE_TIME"}, lambda r: (is_sorted(r["data"], "update_time", True))),
            ({"orderby": " create_time "}, lambda r: (is_sorted(r["data"], "update_time", True))),
        ],
        ids=["orderby_create_time", "orderby_update_time", "orderby_create_time_upper", "orderby_update_time_upper", "whitespace"],
    )
    def test_orderby(self, api_key, params, assertions):
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        if callable(assertions):
            assert assertions(res), res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params",
        [
            {"orderby": ""},
            {"orderby": "unknown"},
        ],
        ids=["empty", "unknown"],
    )
    def test_orderby_invalid(self, api_key, params):
        res = list_datasets(api_key, params)
        assert res["code"] == 101, res
        assert "Input should be 'create_time' or 'update_time'" in res["message"], res

    @pytest.mark.p3
    def test_orderby_none(self, api_key):
        params = {"orderby": None}
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        assert is_sorted(res["data"], "create_time", True), res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, assertions",
        [
            ({"desc": True}, lambda r: (is_sorted(r["data"], "create_time", True))),
            ({"desc": False}, lambda r: (is_sorted(r["data"], "create_time", False))),
            ({"desc": "true"}, lambda r: (is_sorted(r["data"], "create_time", True))),
            ({"desc": "false"}, lambda r: (is_sorted(r["data"], "create_time", False))),
            ({"desc": 1}, lambda r: (is_sorted(r["data"], "create_time", True))),
            ({"desc": 0}, lambda r: (is_sorted(r["data"], "create_time", False))),
            ({"desc": "yes"}, lambda r: (is_sorted(r["data"], "create_time", True))),
            ({"desc": "no"}, lambda r: (is_sorted(r["data"], "create_time", False))),
            ({"desc": "y"}, lambda r: (is_sorted(r["data"], "create_time", True))),
            ({"desc": "n"}, lambda r: (is_sorted(r["data"], "create_time", False))),
        ],
        ids=["desc=True", "desc=False", "desc=true", "desc=false", "desc=1", "desc=0", "desc=yes", "desc=no", "desc=y", "desc=n"],
    )
    def test_desc(self, api_key, params, assertions):
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        if callable(assertions):
            assert assertions(res), res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params",
        [
            {"desc": 3.14},
            {"desc": "unknown"},
        ],
        ids=["empty", "unknown"],
    )
    def test_desc_invalid(self, api_key, params):
        res = list_datasets(api_key, params)
        assert res["code"] == 101, res
        assert "Input should be a valid boolean, unable to interpret input" in res["message"], res

    @pytest.mark.p3
    def test_desc_none(self, api_key):
        params = {"desc": None}
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        assert is_sorted(res["data"], "create_time", True), res

    @pytest.mark.p1
    def test_name(self, api_key):
        params = {"name": "dataset_1"}
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        assert len(res["data"]) == 1, res
        assert res["data"][0]["name"] == "dataset_1", res

    @pytest.mark.p2
    def test_name_wrong(self, api_key):
        params = {"name": "wrong name"}
        res = list_datasets(api_key, params)
        assert res["code"] == 108, res
        assert "lacks permission for dataset" in res["message"], res

    @pytest.mark.p2
    def test_name_empty(self, api_key):
        params = {"name": ""}
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        assert len(res["data"]) == 5, res

    @pytest.mark.p2
    def test_name_none(self, api_key):
        params = {"name": None}
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        assert len(res["data"]) == 5, res

    @pytest.mark.p1
    def test_id(self, api_key, add_datasets):
        dataset_ids = add_datasets
        params = {"id": dataset_ids[0]}
        res = list_datasets(api_key, params)
        assert res["code"] == 0
        assert len(res["data"]) == 1
        assert res["data"][0]["id"] == dataset_ids[0]

    @pytest.mark.p2
    def test_id_not_uuid(self, api_key):
        params = {"id": "not_uuid"}
        res = list_datasets(api_key, params)
        assert res["code"] == 101, res
        assert "Invalid UUID1 format" in res["message"], res

    @pytest.mark.p2
    def test_id_not_uuid1(self, api_key):
        params = {"id": uuid.uuid4().hex}
        res = list_datasets(api_key, params)
        assert res["code"] == 101, res
        assert "Invalid UUID1 format" in res["message"], res

    @pytest.mark.p2
    def test_id_wrong_uuid(self, api_key):
        params = {"id": "d94a8dc02c9711f0930f7fbc369eab6d"}
        res = list_datasets(api_key, params)
        assert res["code"] == 108, res
        assert "lacks permission for dataset" in res["message"], res

    @pytest.mark.p2
    def test_id_empty(self, api_key):
        params = {"id": ""}
        res = list_datasets(api_key, params)
        assert res["code"] == 101, res
        assert "Invalid UUID1 format" in res["message"], res

    @pytest.mark.p2
    def test_id_none(self, api_key):
        params = {"id": None}
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        assert len(res["data"]) == 5, res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "func, name, expected_num",
        [
            (lambda r: r[0], "dataset_0", 1),
            (lambda r: r[0], "dataset_1", 0),
        ],
        ids=["name_and_id_match", "name_and_id_mismatch"],
    )
    def test_name_and_id(self, api_key, add_datasets, func, name, expected_num):
        dataset_ids = add_datasets
        if callable(func):
            params = {"id": func(dataset_ids), "name": name}
        res = list_datasets(api_key, params)
        assert res["code"] == 0, res
        assert len(res["data"]) == expected_num, res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "dataset_id, name",
        [
            (lambda r: r[0], "wrong_name"),
            (uuid.uuid1().hex, "dataset_0"),
        ],
        ids=["name", "id"],
    )
    def test_name_and_id_wrong(self, api_key, add_datasets, dataset_id, name):
        dataset_ids = add_datasets
        if callable(dataset_id):
            params = {"id": dataset_id(dataset_ids), "name": name}
        else:
            params = {"id": dataset_id, "name": name}
        res = list_datasets(api_key, params)
        assert res["code"] == 108, res
        assert "lacks permission for dataset" in res["message"], res

    @pytest.mark.p2
    def test_field_unsupported(self, api_key):
        params = {"unknown_field": "unknown_field"}
        res = list_datasets(api_key, params)
        assert res["code"] == 101, res
        assert "Extra inputs are not permitted" in res["message"], res
