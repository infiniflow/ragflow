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
from common import INVALID_API_TOKEN, bulk_upload_documents, delete_documnets, list_documnets
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
        res = delete_documnets(auth, "dataset_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestDocumentsDeletion:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message, remaining",
        [
            (None, 0, "", 0),
            ({"ids": []}, 0, "", 0),
            ({"ids": ["invalid_id"]}, 102, "Documents not found: ['invalid_id']", 3),
            (
                {"ids": ["\n!?。；！？\"'"]},
                102,
                """Documents not found: [\'\\n!?。；！？"\\\'\']""",
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
        self,
        get_http_api_auth,
        add_documents_func,
        payload,
        expected_code,
        expected_message,
        remaining,
    ):
        dataset_id, document_ids = add_documents_func
        if callable(payload):
            payload = payload(document_ids)
        res = delete_documnets(get_http_api_auth, dataset_id, payload)
        assert res["code"] == expected_code
        if res["code"] != 0:
            assert res["message"] == expected_message

        res = list_documnets(get_http_api_auth, dataset_id)
        assert len(res["data"]["docs"]) == remaining
        assert res["data"]["total"] == remaining

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
    def test_invalid_dataset_id(self, get_http_api_auth, add_documents_func, dataset_id, expected_code, expected_message):
        _, document_ids = add_documents_func
        res = delete_documnets(get_http_api_auth, dataset_id, {"ids": document_ids[:1]})
        assert res["code"] == expected_code
        assert res["message"] == expected_message

    @pytest.mark.p2
    @pytest.mark.parametrize(
        "payload",
        [
            lambda r: {"ids": ["invalid_id"] + r},
            lambda r: {"ids": r[:1] + ["invalid_id"] + r[1:3]},
            lambda r: {"ids": r + ["invalid_id"]},
        ],
    )
    def test_delete_partial_invalid_id(self, get_http_api_auth, add_documents_func, payload):
        dataset_id, document_ids = add_documents_func
        if callable(payload):
            payload = payload(document_ids)
        res = delete_documnets(get_http_api_auth, dataset_id, payload)
        assert res["code"] == 102
        assert res["message"] == "Documents not found: ['invalid_id']"

        res = list_documnets(get_http_api_auth, dataset_id)
        assert len(res["data"]["docs"]) == 0
        assert res["data"]["total"] == 0

    @pytest.mark.p2
    def test_repeated_deletion(self, get_http_api_auth, add_documents_func):
        dataset_id, document_ids = add_documents_func
        res = delete_documnets(get_http_api_auth, dataset_id, {"ids": document_ids})
        assert res["code"] == 0

        res = delete_documnets(get_http_api_auth, dataset_id, {"ids": document_ids})
        assert res["code"] == 102
        assert "Documents not found" in res["message"]

    @pytest.mark.p2
    def test_duplicate_deletion(self, get_http_api_auth, add_documents_func):
        dataset_id, document_ids = add_documents_func
        res = delete_documnets(get_http_api_auth, dataset_id, {"ids": document_ids + document_ids})
        assert res["code"] == 0
        assert "Duplicate document ids" in res["data"]["errors"][0]
        assert res["data"]["success_count"] == 3

        res = list_documnets(get_http_api_auth, dataset_id)
        assert len(res["data"]["docs"]) == 0
        assert res["data"]["total"] == 0


@pytest.mark.p3
def test_concurrent_deletion(get_http_api_auth, add_dataset, tmp_path):
    documnets_num = 100
    dataset_id = add_dataset
    document_ids = bulk_upload_documents(get_http_api_auth, dataset_id, documnets_num, tmp_path)

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [
            executor.submit(
                delete_documnets,
                get_http_api_auth,
                dataset_id,
                {"ids": document_ids[i : i + 1]},
            )
            for i in range(documnets_num)
        ]
    responses = [f.result() for f in futures]
    assert all(r["code"] == 0 for r in responses)


@pytest.mark.p3
def test_delete_1k(get_http_api_auth, add_dataset, tmp_path):
    documnets_num = 1_000
    dataset_id = add_dataset
    document_ids = bulk_upload_documents(get_http_api_auth, dataset_id, documnets_num, tmp_path)
    res = list_documnets(get_http_api_auth, dataset_id)
    assert res["data"]["total"] == documnets_num

    res = delete_documnets(get_http_api_auth, dataset_id, {"ids": document_ids})
    assert res["code"] == 0

    res = list_documnets(get_http_api_auth, dataset_id)
    assert res["data"]["total"] == 0
