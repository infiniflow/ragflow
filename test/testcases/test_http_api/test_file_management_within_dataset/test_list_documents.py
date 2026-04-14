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
from common import list_documents
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
        res = list_documents(invalid_auth, "dataset_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestDocumentsList:
    @pytest.mark.p1
    def test_default(self, HttpApiAuth, add_documents):
        dataset_id, _ = add_documents
        res = list_documents(HttpApiAuth, dataset_id)
        assert res["code"] == 0
        assert len(res["data"]["docs"]) == 5
        assert res["data"]["total"] == 5

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "dataset_id, expected_code, expected_message",
        [
            ("", 100, "<MethodNotAllowed '405: Method Not Allowed'>"),
            (
                "invalid_dataset_id",
                102,
                "You don't own the dataset invalid_dataset_id. ",
            ),
        ],
    )
    def test_invalid_dataset_id(self, HttpApiAuth, dataset_id, expected_code, expected_message):
        res = list_documents(HttpApiAuth, dataset_id)
        assert res["code"] == expected_code
        assert res["message"] == expected_message

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
    def test_page(
        self,
        HttpApiAuth,
        add_documents,
        params,
        expected_code,
        expected_page_size,
        expected_message,
    ):
        dataset_id, _ = add_documents
        res = list_documents(HttpApiAuth, dataset_id, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["docs"]) == expected_page_size
            assert res["data"]["total"] == 5
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
        add_documents,
        params,
        expected_code,
        expected_page_size,
        expected_message,
    ):
        dataset_id, _ = add_documents
        res = list_documents(HttpApiAuth, dataset_id, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["docs"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "params, expected_code, assertions, expected_message",
        [
            ({"orderby": None}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"orderby": "create_time"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"orderby": "update_time"}, 0, lambda r: (is_sorted(r["data"]["docs"], "update_time", True)), ""),
            pytest.param({"orderby": "name", "desc": "False"}, 0, lambda r: (is_sorted(r["data"]["docs"], "name", False)), "", marks=pytest.mark.skip(reason="issues/5851")),
            pytest.param({"orderby": "unknown"}, 102, 0, "orderby should be create_time or update_time", marks=pytest.mark.skip(reason="issues/5851")),
        ],
    )
    def test_orderby(
        self,
        HttpApiAuth,
        add_documents,
        params,
        expected_code,
        assertions,
        expected_message,
    ):
        dataset_id, _ = add_documents
        res = list_documents(HttpApiAuth, dataset_id, params=params)
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
            ({"desc": None}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"desc": "true"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"desc": "True"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            ({"desc": True}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", True)), ""),
            pytest.param({"desc": "false"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", False)), "", marks=pytest.mark.skip(reason="issues/5851")),
            ({"desc": "False"}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", False)), ""),
            ({"desc": False}, 0, lambda r: (is_sorted(r["data"]["docs"], "create_time", False)), ""),
            ({"desc": "False", "orderby": "update_time"}, 0, lambda r: (is_sorted(r["data"]["docs"], "update_time", False)), ""),
            pytest.param({"desc": "unknown"}, 102, 0, "desc should be true or false", marks=pytest.mark.skip(reason="issues/5851")),
        ],
    )
    def test_desc(
        self,
        HttpApiAuth,
        add_documents,
        params,
        expected_code,
        assertions,
        expected_message,
    ):
        dataset_id, _ = add_documents
        res = list_documents(HttpApiAuth, dataset_id, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            if callable(assertions):
                assert assertions(res)
        else:
            assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "params, expected_num",
        [
            ({"keywords": None}, 5),
            ({"keywords": ""}, 5),
            ({"keywords": "0"}, 1),
            ({"keywords": "ragflow_test_upload"}, 5),
            ({"keywords": "unknown"}, 0),
        ],
    )
    def test_keywords(self, HttpApiAuth, add_documents, params, expected_num):
        dataset_id, _ = add_documents
        res = list_documents(HttpApiAuth, dataset_id, params=params)
        assert res["code"] == 0
        assert len(res["data"]["docs"]) == expected_num
        assert res["data"]["total"] == expected_num

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "params, expected_code, expected_num, expected_message",
        [
            ({"name": None}, 0, 5, ""),
            ({"name": ""}, 0, 5, ""),
            ({"name": "ragflow_test_upload_0.txt"}, 0, 1, ""),
            (
                {"name": "unknown.txt"},
                102,
                0,
                "You don't own the document unknown.txt.",
            ),
        ],
    )
    def test_name(
        self,
        HttpApiAuth,
        add_documents,
        params,
        expected_code,
        expected_num,
        expected_message,
    ):
        dataset_id, _ = add_documents
        res = list_documents(HttpApiAuth, dataset_id, params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            if params["name"] in [None, ""]:
                assert len(res["data"]["docs"]) == expected_num
            else:
                assert res["data"]["docs"][0]["name"] == params["name"]
        else:
            assert res["message"] == expected_message

    @pytest.mark.p1
    @pytest.mark.parametrize(
        "document_id, expected_code, expected_num, expected_message",
        [
            (None, 0, 5, ""),
            ("", 0, 5, ""),
            (lambda r: r[0], 0, 1, ""),
            ("unknown.txt", 102, 0, "You don't own the document unknown.txt."),
        ],
    )
    def test_id(
        self,
        HttpApiAuth,
        add_documents,
        document_id,
        expected_code,
        expected_num,
        expected_message,
    ):
        dataset_id, document_ids = add_documents
        if callable(document_id):
            params = {"id": document_id(document_ids)}
        else:
            params = {"id": document_id}
        res = list_documents(HttpApiAuth, dataset_id, params=params)

        assert res["code"] == expected_code
        if expected_code == 0:
            if params["id"] in [None, ""]:
                assert len(res["data"]["docs"]) == expected_num
            else:
                assert res["data"]["docs"][0]["id"] == params["id"]
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "document_id, name, expected_code, expected_num, expected_message",
        [
            (lambda r: r[0], "ragflow_test_upload_0.txt", 0, 1, ""),
            (lambda r: r[0], "ragflow_test_upload_1.txt", 0, 0, ""),
            (lambda r: r[0], "unknown", 102, 0, "You don't own the document unknown."),
            (
                "id",
                "ragflow_test_upload_0.txt",
                102,
                0,
                "You don't own the document id.",
            ),
        ],
    )
    def test_name_and_id(
        self,
        HttpApiAuth,
        add_documents,
        document_id,
        name,
        expected_code,
        expected_num,
        expected_message,
    ):
        dataset_id, document_ids = add_documents
        if callable(document_id):
            params = {"id": document_id(document_ids), "name": name}
        else:
            params = {"id": document_id, "name": name}

        res = list_documents(HttpApiAuth, dataset_id, params=params)
        if expected_code == 0:
            assert len(res["data"]["docs"]) == expected_num
        else:
            assert res["message"] == expected_message

    @pytest.mark.p3
    def test_concurrent_list(self, HttpApiAuth, add_documents):
        dataset_id, _ = add_documents
        count = 100

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [executor.submit(list_documents, HttpApiAuth, dataset_id) for _ in range(count)]
        responses = list(as_completed(futures))
        assert len(responses) == count, responses
        assert all(future.result()["code"] == 0 for future in futures)

    @pytest.mark.p3
    def test_invalid_params(self, HttpApiAuth, add_documents):
        dataset_id, _ = add_documents
        params = {"a": "b"}
        res = list_documents(HttpApiAuth, dataset_id, params=params)
        assert res["code"] == 0
        assert len(res["data"]["docs"]) == 5
