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
from common import bulk_upload_documents, delete_document, list_documents
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


@pytest.mark.p1
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = delete_document(invalid_auth)
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestDocumentsDeletion:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message, remaining",
        [
            (None, 101, "required argument are missing: doc_id; ", 3),
            ({"doc_id": ""}, 109, "No authorization.", 3),
            ({"doc_id": "invalid_id"}, 109, "No authorization.", 3),
            ({"doc_id": "\n!?。；！？\"'"}, 109, "No authorization.", 3),
            ("not json", 101, "required argument are missing: doc_id; ", 3),
            (lambda r: {"doc_id": r[0]}, 0, "", 2),
        ],
    )
    def test_basic_scenarios(self, WebApiAuth, add_documents_func, payload, expected_code, expected_message, remaining):
        kb_id, document_ids = add_documents_func
        if callable(payload):
            payload = payload(document_ids)
        res = delete_document(WebApiAuth, payload)
        assert res["code"] == expected_code, res
        if res["code"] != 0:
            assert res["message"] == expected_message, res

        res = list_documents(WebApiAuth, {"kb_id": kb_id})
        assert len(res["data"]["docs"]) == remaining, res
        assert res["data"]["total"] == remaining, res

    @pytest.mark.p2
    def test_repeated_deletion(self, WebApiAuth, add_documents_func):
        _, document_ids = add_documents_func
        for doc_id in document_ids:
            res = delete_document(WebApiAuth, {"doc_id": doc_id})
            assert res["code"] == 0, res

        for doc_id in document_ids:
            res = delete_document(WebApiAuth, {"doc_id": doc_id})
            assert res["code"] == 109, res
            assert res["message"] == "No authorization.", res


@pytest.mark.p3
def test_concurrent_deletion(WebApiAuth, add_dataset, tmp_path):
    count = 100
    kb_id = add_dataset
    document_ids = bulk_upload_documents(WebApiAuth, kb_id, count, tmp_path)

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(delete_document, WebApiAuth, {"doc_id": document_ids[i]}) for i in range(count)]
    responses = list(as_completed(futures))
    assert len(responses) == count, responses
    assert all(future.result()["code"] == 0 for future in futures), responses


@pytest.mark.p3
def test_delete_100(WebApiAuth, add_dataset, tmp_path):
    documents_num = 100
    kb_id = add_dataset
    document_ids = bulk_upload_documents(WebApiAuth, kb_id, documents_num, tmp_path)
    res = list_documents(WebApiAuth, {"kb_id": kb_id})
    assert res["data"]["total"] == documents_num, res

    for doc_id in document_ids:
        res = delete_document(WebApiAuth, {"doc_id": doc_id})
        assert res["code"] == 0, res

    res = list_documents(WebApiAuth, {"kb_id": kb_id})
    assert res["data"]["total"] == 0, res
