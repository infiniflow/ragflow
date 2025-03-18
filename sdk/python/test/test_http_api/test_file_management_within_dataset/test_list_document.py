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
    batch_upload_documents,
    create_datasets,
    list_documnet,
)
from libs.auth import RAGFlowHttpApiAuth


def is_sorted(data, field, descending=True):
    timestamps = [ds[field] for ds in data]
    return (
        all(a >= b for a, b in zip(timestamps, timestamps[1:]))
        if descending
        else all(a <= b for a, b in zip(timestamps, timestamps[1:]))
    )


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
        res = list_documnet(auth, ids[0])
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestDocumentList:
    def test_default(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)
        batch_upload_documents(get_http_api_auth, ids[0], 31, tmp_path)
        res = list_documnet(get_http_api_auth, ids[0])
        assert res["code"] == 0
        assert len(res["data"]["docs"]) == 30
        assert res["data"]["total"] == 31

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
    def test_invalid_dataset_id(
        self, get_http_api_auth, dataset_id, expected_code, expected_message
    ):
        create_datasets(get_http_api_auth, 1)
        res = list_documnet(get_http_api_auth, dataset_id)
        assert res["code"] == expected_code
        assert res["message"] == expected_message

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
                marks=pytest.mark.xfail(reason="issues/5851"),
            ),
            pytest.param(
                {"page": "a", "page_size": 2},
                100,
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.xfail(reason="issues/5851"),
            ),
        ],
    )
    def test_page(
        self,
        get_http_api_auth,
        tmp_path,
        params,
        expected_code,
        expected_page_size,
        expected_message,
    ):
        ids = create_datasets(get_http_api_auth, 1)
        batch_upload_documents(get_http_api_auth, ids[0], 5, tmp_path)
        res = list_documnet(get_http_api_auth, ids[0], params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["docs"]) == expected_page_size
            assert res["data"]["total"] == 5
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "params, expected_code, expected_page_size, expected_message",
        [
            ({"page_size": None}, 0, 30, ""),
            ({"page_size": 0}, 0, 0, ""),
            ({"page_size": 1}, 0, 1, ""),
            ({"page_size": 32}, 0, 31, ""),
            ({"page_size": "1"}, 0, 1, ""),
            pytest.param(
                {"page_size": -1},
                100,
                0,
                "1064",
                marks=pytest.mark.xfail(reason="issues/5851"),
            ),
            pytest.param(
                {"page_size": "a"},
                100,
                0,
                """ValueError("invalid literal for int() with base 10: \'a\'")""",
                marks=pytest.mark.xfail(reason="issues/5851"),
            ),
        ],
    )
    def test_page_size(
        self,
        get_http_api_auth,
        tmp_path,
        params,
        expected_code,
        expected_page_size,
        expected_message,
    ):
        ids = create_datasets(get_http_api_auth, 1)
        batch_upload_documents(get_http_api_auth, ids[0], 31, tmp_path)
        res = list_documnet(get_http_api_auth, ids[0], params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            assert len(res["data"]["docs"]) == expected_page_size
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "params, expected_code, assertions, expected_message",
        [
            (
                {"orderby": None},
                0,
                lambda r: (is_sorted(r["data"]["docs"], "create_time", True)),
                "",
            ),
            (
                {"orderby": "create_time"},
                0,
                lambda r: (is_sorted(r["data"]["docs"], "create_time", True)),
                "",
            ),
            (
                {"orderby": "update_time"},
                0,
                lambda r: (is_sorted(r["data"]["docs"], "update_time", True)),
                "",
            ),
            pytest.param(
                {"orderby": "name", "desc": "False"},
                0,
                lambda r: (is_sorted(r["data"]["docs"], "name", False)),
                "",
                marks=pytest.mark.xfail(reason="issues/5851"),
            ),
            pytest.param(
                {"orderby": "unknown"},
                102,
                0,
                "orderby should be create_time or update_time",
                marks=pytest.mark.xfail(reason="issues/5851"),
            ),
        ],
    )
    def test_orderby(
        self,
        get_http_api_auth,
        tmp_path,
        params,
        expected_code,
        assertions,
        expected_message,
    ):
        ids = create_datasets(get_http_api_auth, 1)
        batch_upload_documents(get_http_api_auth, ids[0], 3, tmp_path)
        res = list_documnet(get_http_api_auth, ids[0], params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            if callable(assertions):
                assert assertions(res)
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "params, expected_code, assertions, expected_message",
        [
            (
                {"desc": None},
                0,
                lambda r: (is_sorted(r["data"]["docs"], "create_time", True)),
                "",
            ),
            (
                {"desc": "true"},
                0,
                lambda r: (is_sorted(r["data"]["docs"], "create_time", True)),
                "",
            ),
            (
                {"desc": "True"},
                0,
                lambda r: (is_sorted(r["data"]["docs"], "create_time", True)),
                "",
            ),
            (
                {"desc": True},
                0,
                lambda r: (is_sorted(r["data"]["docs"], "create_time", True)),
                "",
            ),
            pytest.param(
                {"desc": "false"},
                0,
                lambda r: (is_sorted(r["data"]["docs"], "create_time", False)),
                "",
                marks=pytest.mark.xfail(reason="issues/5851"),
            ),
            (
                {"desc": "False"},
                0,
                lambda r: (is_sorted(r["data"]["docs"], "create_time", False)),
                "",
            ),
            (
                {"desc": False},
                0,
                lambda r: (is_sorted(r["data"]["docs"], "create_time", False)),
                "",
            ),
            (
                {"desc": "False", "orderby": "update_time"},
                0,
                lambda r: (is_sorted(r["data"]["docs"], "update_time", False)),
                "",
            ),
            pytest.param(
                {"desc": "unknown"},
                102,
                0,
                "desc should be true or false",
                marks=pytest.mark.xfail(reason="issues/5851"),
            ),
        ],
    )
    def test_desc(
        self,
        get_http_api_auth,
        tmp_path,
        params,
        expected_code,
        assertions,
        expected_message,
    ):
        ids = create_datasets(get_http_api_auth, 1)
        batch_upload_documents(get_http_api_auth, ids[0], 3, tmp_path)
        res = list_documnet(get_http_api_auth, ids[0], params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            if callable(assertions):
                assert assertions(res)
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "params, expected_num",
        [
            ({"keywords": None}, 3),
            ({"keywords": ""}, 3),
            ({"keywords": "0"}, 1),
            ({"keywords": "ragflow_test_upload"}, 3),
            ({"keywords": "unknown"}, 0),
        ],
    )
    def test_keywords(self, get_http_api_auth, tmp_path, params, expected_num):
        ids = create_datasets(get_http_api_auth, 1)
        batch_upload_documents(get_http_api_auth, ids[0], 3, tmp_path)
        res = list_documnet(get_http_api_auth, ids[0], params=params)
        assert res["code"] == 0
        assert len(res["data"]["docs"]) == expected_num
        assert res["data"]["total"] == expected_num

    @pytest.mark.parametrize(
        "params, expected_code, expected_num, expected_message",
        [
            ({"name": None}, 0, 3, ""),
            ({"name": ""}, 0, 3, ""),
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
        get_http_api_auth,
        tmp_path,
        params,
        expected_code,
        expected_num,
        expected_message,
    ):
        ids = create_datasets(get_http_api_auth, 1)
        batch_upload_documents(get_http_api_auth, ids[0], 3, tmp_path)
        res = list_documnet(get_http_api_auth, ids[0], params=params)
        assert res["code"] == expected_code
        if expected_code == 0:
            if params["name"] in [None, ""]:
                assert len(res["data"]["docs"]) == expected_num
            else:
                assert res["data"]["docs"][0]["name"] == params["name"]
        else:
            assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "document_id, expected_code, expected_num, expected_message",
        [
            (None, 0, 3, ""),
            ("", 0, 3, ""),
            (lambda r: r[0], 0, 1, ""),
            ("unknown.txt", 102, 0, "You don't own the document unknown.txt."),
        ],
    )
    def test_id(
        self,
        get_http_api_auth,
        tmp_path,
        document_id,
        expected_code,
        expected_num,
        expected_message,
    ):
        ids = create_datasets(get_http_api_auth, 1)
        document_ids = batch_upload_documents(get_http_api_auth, ids[0], 3, tmp_path)
        if callable(document_id):
            params = {"id": document_id(document_ids)}
        else:
            params = {"id": document_id}
        res = list_documnet(get_http_api_auth, ids[0], params=params)

        assert res["code"] == expected_code
        if expected_code == 0:
            if params["id"] in [None, ""]:
                assert len(res["data"]["docs"]) == expected_num
            else:
                assert res["data"]["docs"][0]["id"] == params["id"]
        else:
            assert res["message"] == expected_message

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
        get_http_api_auth,
        tmp_path,
        document_id,
        name,
        expected_code,
        expected_num,
        expected_message,
    ):
        ids = create_datasets(get_http_api_auth, 1)
        document_ids = batch_upload_documents(get_http_api_auth, ids[0], 3, tmp_path)
        if callable(document_id):
            params = {"id": document_id(document_ids), "name": name}
        else:
            params = {"id": document_id, "name": name}

        res = list_documnet(get_http_api_auth, ids[0], params=params)
        if expected_code == 0:
            assert len(res["data"]["docs"]) == expected_num
        else:
            assert res["message"] == expected_message

    def test_concurrent_list(self, get_http_api_auth, tmp_path):
        ids = create_datasets(get_http_api_auth, 1)
        batch_upload_documents(get_http_api_auth, ids[0], 3, tmp_path)

        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = [
                executor.submit(list_documnet, get_http_api_auth, ids[0])
                for i in range(100)
            ]
        responses = [f.result() for f in futures]
        assert all(r["code"] == 0 for r in responses)

    def test_invalid_params(self, get_http_api_auth):
        ids = create_datasets(get_http_api_auth, 1)
        params = {"a": "b"}
        res = list_documnet(get_http_api_auth, ids[0], params=params)
        assert res["code"] == 0
        assert len(res["data"]["docs"]) == 0
