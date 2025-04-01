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
from common import INVALID_API_TOKEN, bulk_upload_documents, list_documnets, parse_documnets
from libs.auth import RAGFlowHttpApiAuth
from libs.utils import wait_for


def validate_document_details(auth, dataset_id, document_ids):
    for document_id in document_ids:
        res = list_documnets(auth, dataset_id, params={"id": document_id})
        doc = res["data"]["docs"][0]
        assert doc["run"] == "DONE"
        assert len(doc["process_begin_at"]) > 0
        assert doc["process_duation"] > 0
        assert doc["progress"] > 0
        assert "Task done" in doc["progress_msg"]


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
        res = parse_documnets(auth, "dataset_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestDocumentsParse:
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message",
        [
            pytest.param(
                None,
                102,
                """AttributeError("\'NoneType\' object has no attribute \'get\'")""",
                marks=pytest.mark.skip,
            ),
            ({"document_ids": []}, 102, "`document_ids` is required"),
            (
                {"document_ids": ["invalid_id"]},
                102,
                "Documents not found: ['invalid_id']",
            ),
            (
                {"document_ids": ["\n!?。；！？\"'"]},
                102,
                """Documents not found: [\'\\n!?。；！？"\\\'\']""",
            ),
            pytest.param(
                "not json",
                102,
                "AttributeError(\"'str' object has no attribute 'get'\")",
                marks=pytest.mark.skip,
            ),
            (lambda r: {"document_ids": r[:1]}, 0, ""),
            (lambda r: {"document_ids": r}, 0, ""),
        ],
    )
    def test_basic_scenarios(self, get_http_api_auth, add_documents_func, payload, expected_code, expected_message):
        @wait_for(10, 1, "Document parsing timeout")
        def condition(_auth, _dataset_id, _document_ids):
            for _document_id in _document_ids:
                res = list_documnets(_auth, _dataset_id, {"id": _document_id})
                if res["data"]["docs"][0]["run"] != "DONE":
                    return False
            return True

        dataset_id, document_ids = add_documents_func
        if callable(payload):
            payload = payload(document_ids)
        res = parse_documnets(get_http_api_auth, dataset_id, payload)
        assert res["code"] == expected_code
        if expected_code != 0:
            assert res["message"] == expected_message
        if expected_code == 0:
            condition(get_http_api_auth, dataset_id, payload["document_ids"])
            validate_document_details(get_http_api_auth, dataset_id, payload["document_ids"])

    @pytest.mark.parametrize(
        "dataset_id, expected_code, expected_message",
        [
            ("", 100, "<MethodNotAllowed '405: Method Not Allowed'>"),
            (
                "invalid_dataset_id",
                102,
                "You don't own the dataset invalid_dataset_id.",
            ),
        ],
    )
    def test_invalid_dataset_id(
        self,
        get_http_api_auth,
        add_documents_func,
        dataset_id,
        expected_code,
        expected_message,
    ):
        _, document_ids = add_documents_func
        res = parse_documnets(get_http_api_auth, dataset_id, {"document_ids": document_ids})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.parametrize(
        "payload",
        [
            lambda r: {"document_ids": ["invalid_id"] + r},
            lambda r: {"document_ids": r[:1] + ["invalid_id"] + r[1:3]},
            lambda r: {"document_ids": r + ["invalid_id"]},
        ],
    )
    def test_parse_partial_invalid_document_id(self, get_http_api_auth, add_documents_func, payload):
        @wait_for(10, 1, "Document parsing timeout")
        def condition(_auth, _dataset_id):
            res = list_documnets(_auth, _dataset_id)
            for doc in res["data"]["docs"]:
                if doc["run"] != "DONE":
                    return False
            return True

        dataset_id, document_ids = add_documents_func
        if callable(payload):
            payload = payload(document_ids)
        res = parse_documnets(get_http_api_auth, dataset_id, payload)
        assert res["code"] == 102
        assert res["message"] == "Documents not found: ['invalid_id']"

        condition(get_http_api_auth, dataset_id)

        validate_document_details(get_http_api_auth, dataset_id, document_ids)

    def test_repeated_parse(self, get_http_api_auth, add_documents_func):
        @wait_for(10, 1, "Document parsing timeout")
        def condition(_auth, _dataset_id):
            res = list_documnets(_auth, _dataset_id)
            for doc in res["data"]["docs"]:
                if doc["run"] != "DONE":
                    return False
            return True

        dataset_id, document_ids = add_documents_func
        res = parse_documnets(get_http_api_auth, dataset_id, {"document_ids": document_ids})
        assert res["code"] == 0

        condition(get_http_api_auth, dataset_id)

        res = parse_documnets(get_http_api_auth, dataset_id, {"document_ids": document_ids})
        assert res["code"] == 0

    def test_duplicate_parse(self, get_http_api_auth, add_documents_func):
        @wait_for(10, 1, "Document parsing timeout")
        def condition(_auth, _dataset_id):
            res = list_documnets(_auth, _dataset_id)
            for doc in res["data"]["docs"]:
                if doc["run"] != "DONE":
                    return False
            return True

        dataset_id, document_ids = add_documents_func
        res = parse_documnets(get_http_api_auth, dataset_id, {"document_ids": document_ids + document_ids})
        assert res["code"] == 0
        assert "Duplicate document ids" in res["data"]["errors"][0]
        assert res["data"]["success_count"] == 3

        condition(get_http_api_auth, dataset_id)

        validate_document_details(get_http_api_auth, dataset_id, document_ids)


@pytest.mark.slow
def test_parse_100_files(get_http_api_auth, add_datase_func, tmp_path):
    @wait_for(100, 1, "Document parsing timeout")
    def condition(_auth, _dataset_id, _document_num):
        res = list_documnets(_auth, _dataset_id, {"page_size": _document_num})
        for doc in res["data"]["docs"]:
            if doc["run"] != "DONE":
                return False
        return True

    document_num = 100
    dataset_id = add_datase_func
    document_ids = bulk_upload_documents(get_http_api_auth, dataset_id, document_num, tmp_path)
    res = parse_documnets(get_http_api_auth, dataset_id, {"document_ids": document_ids})
    assert res["code"] == 0

    condition(get_http_api_auth, dataset_id, document_num)

    validate_document_details(get_http_api_auth, dataset_id, document_ids)


@pytest.mark.slow
def test_concurrent_parse(get_http_api_auth, add_datase_func, tmp_path):
    @wait_for(120, 1, "Document parsing timeout")
    def condition(_auth, _dataset_id, _document_num):
        res = list_documnets(_auth, _dataset_id, {"page_size": _document_num})
        for doc in res["data"]["docs"]:
            if doc["run"] != "DONE":
                return False
        return True

    document_num = 100
    dataset_id = add_datase_func
    document_ids = bulk_upload_documents(get_http_api_auth, dataset_id, document_num, tmp_path)

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [
            executor.submit(
                parse_documnets,
                get_http_api_auth,
                dataset_id,
                {"document_ids": document_ids[i : i + 1]},
            )
            for i in range(document_num)
        ]
    responses = [f.result() for f in futures]
    assert all(r["code"] == 0 for r in responses)

    condition(get_http_api_auth, dataset_id, document_num)

    validate_document_details(get_http_api_auth, dataset_id, document_ids)
