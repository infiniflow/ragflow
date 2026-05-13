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
import asyncio
from concurrent.futures import ThreadPoolExecutor, as_completed

import pytest
from test_common import bulk_upload_documents, delete_document, list_documents
from configs import INVALID_API_TOKEN
from libs.auth import RAGFlowWebApiAuth


def _run(coro):
    return asyncio.run(coro)


@pytest.mark.p2
class TestAuthorization:
    @pytest.mark.parametrize(
        "invalid_auth, expected_code, expected_message",
        [
            (None, 401, "<Unauthorized '401: Unauthorized'>"),
            (RAGFlowWebApiAuth(INVALID_API_TOKEN), 401, "<Unauthorized '401: Unauthorized'>"),
        ],
    )
    def test_invalid_auth(self, invalid_auth, expected_code, expected_message):
        res = delete_document(invalid_auth, "kb_id")
        assert res["code"] == expected_code, res
        assert res["message"] == expected_message, res


class TestDocumentsDeletion:
    @pytest.mark.p1
    @pytest.mark.parametrize(
        "payload, expected_code, expected_message, remaining",
        [
            ({}, 102, "should either provide doc ids or set delete_all(true), dataset:", 3),
            ({"invalid_key":[]}, 101, "Field: <invalid_key> - Message: <Extra inputs are not permitted> - Value: <[]>", 3),
            ({"ids": ""}, 101, "Field: <ids> - Message: <Input should be a valid list> - Value: <>", 3),
            ({"ids": ["invalid_id"]}, 102, "These documents do not belong to dataset", 3),
            ("not json", 101, "Invalid request payload: expected object, got str", 3),
            (lambda r: {"ids": r[0]}, 101, "Field: <ids> - Message: <Input should be a valid list> - Value", 3),
            (lambda r: {"ids": r}, 0, "", 0),
        ],
    )
    def test_basic_scenarios(self, WebApiAuth, add_documents_func, payload, expected_code, expected_message, remaining):
        kb_id, document_ids = add_documents_func
        if callable(payload):
            payload = payload(document_ids)
        res = delete_document(WebApiAuth, kb_id, payload)
        assert res["code"] == expected_code, res
        if res["code"] != 0:
            assert expected_message in res["message"], res

        res = list_documents(WebApiAuth, {"kb_id": kb_id})
        assert len(res["data"]["docs"]) == remaining, res
        assert res["data"]["total"] == remaining, res

    @pytest.mark.p2
    def test_repeated_deletion(self, WebApiAuth, add_documents_func):
        kb_id, document_ids = add_documents_func
        for doc_id in document_ids:
            res = delete_document(WebApiAuth, kb_id, {"ids": [doc_id]})
            assert res["code"] == 0, res

        for doc_id in document_ids:
            res = delete_document(WebApiAuth, kb_id, {"ids": [doc_id]})
            assert res["code"] == 102, res
            assert "Document not found" in res["message"], res

    @pytest.mark.p2
    def test_delete_all(self, WebApiAuth, add_documents_func):
        kb_id, document_ids = add_documents_func

        res = delete_document(WebApiAuth, kb_id, {"delete_all": True})
        assert res["code"] == 0, res

        res = list_documents(WebApiAuth, {"kb_id": kb_id})
        assert len(res["data"]["docs"]) == 0, res
        assert res["data"]["total"] == 0, res


@pytest.mark.p2
def test_concurrent_deletion(WebApiAuth, add_dataset, tmp_path):
    count = 100
    kb_id = add_dataset
    document_ids = bulk_upload_documents(WebApiAuth, kb_id, count, tmp_path)

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(delete_document, WebApiAuth, kb_id, {"ids": [document_ids[i]]}) for i in range(count)]
    responses = list(as_completed(futures))
    assert len(responses) == count, responses
    assert all(future.result()["code"] == 0 for future in futures), responses

    res = list_documents(WebApiAuth, {"kb_id": kb_id})
    assert len(res["data"]["docs"]) == 0, res
    assert res["data"]["total"] == 0, res


@pytest.mark.p2
def test_delete_100(WebApiAuth, add_dataset, tmp_path):
    documents_num = 100
    kb_id = add_dataset
    document_ids = bulk_upload_documents(WebApiAuth, kb_id, documents_num, tmp_path)
    res = list_documents(WebApiAuth, {"kb_id": kb_id})
    assert res["data"]["total"] == documents_num, res

    for doc_id in document_ids:
        res = delete_document(WebApiAuth, kb_id, {"ids": [doc_id]})
        assert res["code"] == 0, res

    res = list_documents(WebApiAuth, {"kb_id": kb_id})
    assert res["data"]["total"] == 0, res
