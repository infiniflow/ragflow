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
from common import list_kbs
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth
from utils import is_sorted


class TestAuthorization:
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_auth_invalid(self, invalid_auth, expected_code, expected_message):
        res = list_kbs(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestCapability:
    @pytest.mark.p3
    def test_concurrent_list(self, WebApiAuth):
        count = 100
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(list_kbs, WebApiAuth) for i in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)


@pytest.mark.usefixtures("add_datasets")
class TestDatasetsList:
    @pytest.mark.p2
    def test_params_unset(self, WebApiAuth):
        res = list_kbs(WebApiAuth, None)
        assert res["code"] == 0, res
        assert len(res["data"]["kbs"]) == 5, res

    @pytest.mark.p2
    def test_params_empty(self, WebApiAuth):
        res = list_kbs(WebApiAuth, {})
        assert res["code"] == 0, res
        assert len(res["data"]["kbs"]) == 5, res

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
    def test_page(self, WebApiAuth, params, expected_page_size):
        res = list_kbs(WebApiAuth, params)
        assert res["code"] == 0, res
        assert len(res["data"]["kbs"]) == expected_page_size, res

    @pytest.mark.skip
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_code, expected_message",
        [
            ({"page": 0}, 101, "Input should be greater than or equal to 1"),
            ({"page": "a"}, 101, "Input should be a valid integer, unable to parse string as an integer"),
        ],
        ids=["page_0", "page_a"],
    )
    def test_page_invalid(self, WebApiAuth, params, expected_code, expected_message):
        res = list_kbs(WebApiAuth, params=params)
        assert res["code"] == expected_code, res
        assert expected_message in res["message"], res

    @pytest.mark.p2
    def test_page_none(self, WebApiAuth):
        params = {"page": None}
        res = list_kbs(WebApiAuth, params)
        assert res["code"] == 0, res
        assert len(res["data"]["kbs"]) == 5, res

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_page_size",
        [
            ({"page": 1, "page_size": 1}, 1),
            ({"page": 1, "page_size": 3}, 3),
            ({"page": 1, "page_size": 5}, 5),
            ({"page": 1, "page_size": 6}, 5),
            ({"page": 1, "page_size": "1"}, 1),
        ],
        ids=["min_valid_page_size", "medium_page_size", "page_size_equals_total", "page_size_exceeds_total", "string_type_page_size"],
    )
    def test_page_size(self, WebApiAuth, params, expected_page_size):
        res = list_kbs(WebApiAuth, params)
        assert res["code"] == 0, res
        assert len(res["data"]["kbs"]) == expected_page_size, res

    @pytest.mark.skip
    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_code, expected_message",
        [
            ({"page_size": 0}, 101, "Input should be greater than or equal to 1"),
            ({"page_size": "a"}, 101, "Input should be a valid integer, unable to parse string as an integer"),
        ],
    )
    def test_page_size_invalid(self, WebApiAuth, params, expected_code, expected_message):
        res = list_kbs(WebApiAuth, params)
        assert res["code"] == expected_code, res
        assert expected_message in res["message"], res

    @pytest.mark.p2
    def test_page_size_none(self, WebApiAuth):
        params = {"page_size": None}
        res = list_kbs(WebApiAuth, params)
        assert res["code"] == 0, res
        assert len(res["data"]["kbs"]) == 5, res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, assertions",
        [
            ({"orderby": "update_time"}, lambda r: (is_sorted(r["data"]["kbs"], "update_time", True))),
        ],
        ids=["orderby_update_time"],
    )
    def test_orderby(self, WebApiAuth, params, assertions):
        res = list_kbs(WebApiAuth, params)
        assert res["code"] == 0, res
        if callable(assertions):
            assert assertions(res), res

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, assertions",
        [
            ({"desc": "True"}, lambda r: (is_sorted(r["data"]["kbs"], "update_time", True))),
            ({"desc": "False"}, lambda r: (is_sorted(r["data"]["kbs"], "update_time", False))),
        ],
        ids=["desc=True", "desc=False"],
    )
    def test_desc(self, WebApiAuth, params, assertions):
        res = list_kbs(WebApiAuth, params)

        assert res["code"] == 0, res
        if callable(assertions):
            assert assertions(res), res

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_page_size",
        [
            ({"parser_id": "naive"}, 5),
            ({"parser_id": "qa"}, 0),
        ],
        ids=["naive", "dqa"],
    )
    def test_parser_id(self, WebApiAuth, params, expected_page_size):
        res = list_kbs(WebApiAuth, params)
        assert res["code"] == 0, res
        assert len(res["data"]["kbs"]) == expected_page_size, res
