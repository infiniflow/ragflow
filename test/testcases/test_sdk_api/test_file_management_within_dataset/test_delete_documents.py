#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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


class TestDocumentsDeletion:
    pytestmark = pytest.mark.integration

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload, expected_message, remaining",
        [
            ({"ids": None}, "should either provide doc ids or set delete_all(true)", 3),
            ({"ids": []}, "should either provide doc ids or set delete_all(true)", 3),
            ({"ids": ["invalid_id"]}, "These documents do not belong to dataset", 3),
            ({"ids": ["\n!?。；！？\"'"]}, "These documents do not belong to dataset", 3),
            (lambda r: {"ids": r[:1]}, "", 2),
            (lambda r: {"ids": r}, "", 0),
        ],
    )
    def test_basic_scenarios(
        self,
        add_documents_func,
        payload,
        expected_message,
        remaining,
    ):
        dataset, documents = add_documents_func
        owned_ids = {document.id for document in documents}
        if callable(payload):
            payload = payload([document.id for document in documents])

        if expected_message:
            with pytest.raises(Exception) as exception_info:
                dataset.delete_documents(**payload)
            assert expected_message in str(exception_info.value), str(exception_info.value)
        else:
            dataset.delete_documents(**payload)

        remaining_docs = dataset.list_documents()
        assert len(remaining_docs) == remaining, str(remaining_docs)
        expected_ids = owned_ids if expected_message else owned_ids - set(payload.get("ids") or [])
        assert {document.id for document in remaining_docs} == expected_ids

    @pytest.mark.p3
    @pytest.mark.parametrize(
        "payload",
        [
            lambda r: {"ids": ["invalid_id"] + r},
            lambda r: {"ids": r[:1] + ["invalid_id"] + r[1:3]},
            lambda r: {"ids": r + ["invalid_id"]},
        ],
    )
    def test_delete_partial_invalid_id(self, add_documents_func, payload):
        dataset, documents = add_documents_func
        payload = payload([document.id for document in documents])

        with pytest.raises(Exception) as exception_info:
            dataset.delete_documents(**payload)
        assert "These documents do not belong to dataset" in str(exception_info.value), str(exception_info.value)

        remaining = dataset.list_documents()
        assert {document.id for document in remaining} == {document.id for document in documents}

    @pytest.mark.p3
    def test_repeated_deletion(self, add_documents_func):
        dataset, documents = add_documents_func
        document_ids = [document.id for document in documents]
        dataset.delete_documents(ids=document_ids)
        with pytest.raises(Exception) as exception_info:
            dataset.delete_documents(ids=document_ids)
        assert "Document not found" in str(exception_info.value), str(exception_info.value)
        assert dataset.list_documents() == []

    @pytest.mark.p2
    def test_duplicate_deletion(self, add_documents_func):
        dataset, documents = add_documents_func
        document_ids = [document.id for document in documents]
        with pytest.raises(Exception) as exception_info:
            dataset.delete_documents(ids=document_ids + document_ids)
        assert "Field: <ids> - Message: <Duplicate ids:" in str(exception_info.value), str(exception_info.value)
        assert {doc.id for doc in dataset.list_documents()} == set(document_ids)


@pytest.mark.p3
@pytest.mark.integration
def test_concurrent_deletion(add_dataset_func, upload_documents):
    count = 100
    dataset = add_dataset_func
    documents = upload_documents(dataset, count)

    def delete_doc(doc_id):
        dataset.delete_documents(ids=[doc_id])

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(delete_doc, doc.id) for doc in documents]

    for future in as_completed(futures):
        future.result()

    assert dataset.list_documents() == []


@pytest.mark.p3
@pytest.mark.integration
def test_delete_1k(add_dataset_func, upload_documents):
    count = 1_000
    dataset = add_dataset_func
    documents = upload_documents(dataset, count)

    dataset.delete_documents(ids=[doc.id for doc in documents])
    assert len(dataset.list_documents()) == 0
