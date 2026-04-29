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
from common import bulk_upload_documents, delete_documents, list_documents
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowHttpApiAuth


@pytest.mark.p1
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (
                RAGFlowHttpApiAuth(INVALID_API_TOKEN),
                401,
                "<Unauthorized '401: Unauthorized'>",
            ),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = delete_documents(invalid_auth, "dataset_id")
        assert res["code"] == expected_code
        assert res["message"] == expected_message


class TestDocumentsDeletion:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message, remaining",
        [
            ({}, 102, "should either provide doc ids or set delete_all(true), dataset", 3),
            ({"ids": []}, 102, "should either provide doc ids or set delete_all(true), dataset", 3),
            ({"ids": ["invalid_id"]}, 101, "Field: <ids> - Message: <Invalid UUID1 format> - Value: <['invalid_id']>", 3),
            (
                {"ids": ["\n!?。；！？\"'"]},
                101,
                "Field: <ids> - Message: <Invalid UUID1 format> - Value:",
                3,
            ),
            (
                "not json",
                101,
                "Invalid request payload: expected object, got str",
                3,
            ),
            (lambda r: {"ids": r[:1]}, 0, "", 2),
            (lambda r: {"ids": r}, 0, "", 0),
        ],
    )
    def test_basic_scenarios(
        self,
        HttpApiAuth,
        add_documents_func,
        payload,
        expected_code,
        expected_message,
        remaining,
    ):
        dataset_id, document_ids = add_documents_func
        if callable(payload):
            payload = payload(document_ids)
        res = delete_documents(HttpApiAuth, dataset_id, payload)
        assert res["code"] == expected_code
        if res["code"] != 0:
            assert expected_message in res["message"]

        res = list_documents(HttpApiAuth, dataset_id)
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
    def test_invalid_dataset_id(self, HttpApiAuth, add_documents_func, dataset_id, expected_code, expected_message):
        _, document_ids = add_documents_func
        res = delete_documents(HttpApiAuth, dataset_id, {"ids": document_ids[:1]})
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
    def test_delete_partial_invalid_id(self, HttpApiAuth, add_documents_func, payload):
        dataset_id, document_ids = add_documents_func
        if callable(payload):
            payload = payload(document_ids)
        res = delete_documents(HttpApiAuth, dataset_id, payload)
        assert res["code"] == 101
        assert "Field: <ids> - Message: <Invalid UUID1 format> - Value" in res["message"]

        res = list_documents(HttpApiAuth, dataset_id)
        assert len(res["data"]["docs"]) == 3
        assert res["data"]["total"] == 3

    @pytest.mark.p2
    def test_repeated_deletion(self, HttpApiAuth, add_documents_func):
        dataset_id, document_ids = add_documents_func
        res = delete_documents(HttpApiAuth, dataset_id, {"ids": document_ids})
        assert res["code"] == 0

        res = delete_documents(HttpApiAuth, dataset_id, {"ids": document_ids})
        assert res["code"] == 102
        assert "Document not found" in res["message"]

    @pytest.mark.p2
    def test_duplicate_deletion(self, HttpApiAuth, add_documents_func):
        dataset_id, document_ids = add_documents_func
        res = delete_documents(HttpApiAuth, dataset_id, {"ids": document_ids + document_ids})
        assert res["code"] == 101, res
        assert "Field: <ids> - Message: <Duplicate ids:" in res["message"]

        res = list_documents(HttpApiAuth, dataset_id)
        assert len(res["data"]["docs"]) == 3
        assert res["data"]["total"] == 3

    @pytest.mark.p2
    def test_cross_dataset_deletion_is_blocked(self, HttpApiAuth, add_dataset, add_documents_func, tmp_path):
        dataset_id, _document_ids = add_documents_func
        other_dataset_id = add_dataset
        other_document_id = bulk_upload_documents(HttpApiAuth, other_dataset_id, 1, tmp_path)[0]

        res = delete_documents(HttpApiAuth, dataset_id, {"ids": [other_document_id]})
        assert res["code"] == 102
        assert f"These documents do not belong to dataset {dataset_id}" in res["message"]

        res = list_documents(HttpApiAuth, dataset_id)
        assert len(res["data"]["docs"]) == 3
        assert res["data"]["total"] == 3

        res = list_documents(HttpApiAuth, other_dataset_id)
        assert len(res["data"]["docs"]) == 1
        assert res["data"]["total"] == 1


@pytest.mark.p3
def test_concurrent_deletion(HttpApiAuth, add_dataset, tmp_path):
    count = 100
    dataset_id = add_dataset
    document_ids = bulk_upload_documents(HttpApiAuth, dataset_id, count, tmp_path)

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [
            executor.submit(
                delete_documents,
                HttpApiAuth,
                dataset_id,
                {"ids": document_ids[i : i + 1]},
            )
            for i in range(count)
        ]
    responses = list(as_completed(futures))
    assert len(responses) == count, responses
    assert all(future.result()["code"] == 0 for future in futures)


@pytest.mark.p3
def test_delete_1k(HttpApiAuth, add_dataset, tmp_path):
    documents_num = 1_000
    dataset_id = add_dataset
    document_ids = bulk_upload_documents(HttpApiAuth, dataset_id, documents_num, tmp_path)
    res = list_documents(HttpApiAuth, dataset_id)
    assert res["data"]["total"] == documents_num

    res = delete_documents(HttpApiAuth, dataset_id, {"ids": document_ids})
    assert res["code"] == 0

    res = list_documents(HttpApiAuth, dataset_id)
    assert res["data"]["total"] == 0
