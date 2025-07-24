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
from common import bulk_upload_documents
from ragflow_sdk import DataSet
from utils import wait_for


@wait_for(30, 1, "Document parsing timeout")
def condition(_dataset: DataSet, _document_ids: list[str] = None):
    documents = _dataset.list_documents(page_size=1000)

    if _document_ids is None:
        for document in documents:
            if document.run != "DONE":
                return False
        return True

    target_ids = set(_document_ids)
    for document in documents:
        if document.id in target_ids:
            if document.run != "DONE":
                return False
    return True


def validate_document_details(dataset, document_ids):
    documents = dataset.list_documents(page_size=1000)
    for document in documents:
        if document.id in document_ids:
            assert document.run == "DONE"
            assert len(document.process_begin_at) > 0
            assert document.process_duration > 0
            assert document.progress > 0
            assert "Task done" in document.progress_msg


class TestDocumentsParse:
    @pytest.mark.parametrize(
        "payload, expected_message",
        [
            pytest.param(None, "AttributeError", marks=pytest.mark.skip),
            pytest.param({"document_ids": []}, "`document_ids` is required", marks=pytest.mark.p1),
            pytest.param({"document_ids": ["invalid_id"]}, "Documents not found: ['invalid_id']", marks=pytest.mark.p3),
            pytest.param({"document_ids": ["\n!?。；！？\"'"]}, "Documents not found: ['\\n!?。；！？\"\\'']", marks=pytest.mark.p3),
            pytest.param("not json", "AttributeError", marks=pytest.mark.skip),
            pytest.param(lambda r: {"document_ids": r[:1]}, "", marks=pytest.mark.p1),
            pytest.param(lambda r: {"document_ids": r}, "", marks=pytest.mark.p1),
        ],
    )
    def test_basic_scenarios(self, add_documents_func, payload, expected_message):
        dataset, documents = add_documents_func
        if callable(payload):
            payload = payload([doc.id for doc in documents])

        if expected_message:
            with pytest.raises(Exception) as excinfo:
                dataset.async_parse_documents(**payload)
            assert expected_message in str(excinfo.value), str(excinfo.value)
        else:
            dataset.async_parse_documents(**payload)
            condition(dataset, payload["document_ids"])
            validate_document_details(dataset, payload["document_ids"])

    @pytest.mark.parametrize(
        "payload",
        [
            pytest.param(lambda r: {"document_ids": ["invalid_id"] + r}, marks=pytest.mark.p3),
            pytest.param(lambda r: {"document_ids": r[:1] + ["invalid_id"] + r[1:3]}, marks=pytest.mark.p1),
            pytest.param(lambda r: {"document_ids": r + ["invalid_id"]}, marks=pytest.mark.p3),
        ],
    )
    def test_parse_partial_invalid_document_id(self, add_documents_func, payload):
        dataset, documents = add_documents_func
        document_ids = [doc.id for doc in documents]
        payload = payload(document_ids)

        with pytest.raises(Exception) as excinfo:
            dataset.async_parse_documents(**payload)
        assert "Documents not found: ['invalid_id']" in str(excinfo.value), str(excinfo.value)

        condition(dataset, document_ids)
        validate_document_details(dataset, document_ids)

    @pytest.mark.p3
    def test_repeated_parse(self, add_documents_func):
        dataset, documents = add_documents_func
        document_ids = [doc.id for doc in documents]
        dataset.async_parse_documents(document_ids=document_ids)
        condition(dataset, document_ids)
        dataset.async_parse_documents(document_ids=document_ids)

    @pytest.mark.p3
    def test_duplicate_parse(self, add_documents_func):
        dataset, documents = add_documents_func
        document_ids = [doc.id for doc in documents]
        dataset.async_parse_documents(document_ids=document_ids + document_ids)
        condition(dataset, document_ids)
        validate_document_details(dataset, document_ids)


@pytest.mark.p3
def test_parse_100_files(add_dataset_func, tmp_path):
    @wait_for(200, 1, "Document parsing timeout")
    def condition(_dataset: DataSet, _count: int):
        documents = _dataset.list_documents(page_size=_count * 2)
        for document in documents:
            if document.run != "DONE":
                return False
        return True

    count = 100
    dataset = add_dataset_func
    documents = bulk_upload_documents(dataset, count, tmp_path)
    document_ids = [doc.id for doc in documents]

    dataset.async_parse_documents(document_ids=document_ids)
    condition(dataset, count)
    validate_document_details(dataset, document_ids)


@pytest.mark.p3
def test_concurrent_parse(add_dataset_func, tmp_path):
    @wait_for(200, 1, "Document parsing timeout")
    def condition(_dataset: DataSet, _count: int):
        documents = _dataset.list_documents(page_size=_count * 2)
        for document in documents:
            if document.run != "DONE":
                return False
        return True

    count = 100
    dataset = add_dataset_func
    documents = bulk_upload_documents(dataset, count, tmp_path)
    document_ids = [doc.id for doc in documents]

    def parse_doc(doc_id):
        dataset.async_parse_documents(document_ids=[doc_id])

    with ThreadPoolExecutor(max_workers=5) as executor:
        futures = [executor.submit(parse_doc, doc.id) for doc in documents]

    responses = list(as_completed(futures))
    assert len(responses) == count, responses

    condition(dataset, count)
    validate_document_details(dataset, document_ids)
