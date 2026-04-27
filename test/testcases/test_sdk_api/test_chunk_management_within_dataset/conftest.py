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



import pytest
from common import batch_add_chunks, delete_all_chunks
from pytest import FixtureRequest
from ragflow_sdk import Chunk, DataSet, Document
from utils import wait_for


@wait_for(30, 1, "Document parsing timeout")
def condition(_dataset: DataSet):
    documents = _dataset.list_documents(page_size=1000)
    for document in documents:
        if document.run != "DONE":
            return False
    return True

@wait_for(30, 1, "Chunk indexing timeout")
def chunks_visible(_document: Document, _chunk_ids: list[str]):
    visible_ids = {chunk.id for chunk in _document.list_chunks(page_size=1000)}
    return set(_chunk_ids).issubset(visible_ids)

@pytest.fixture(scope="function")
def add_chunks_func(request: FixtureRequest, add_document: tuple[DataSet, Document]) -> tuple[DataSet, Document, list[Chunk]]:
    def cleanup():
        try:
            delete_all_chunks(document)
        except Exception:
            pass

    request.addfinalizer(cleanup)

    dataset, document = add_document
    dataset.async_parse_documents([document.id])
    condition(dataset)
    chunks = batch_add_chunks(document, 4)
    chunks_visible(document, [chunk.id for chunk in chunks])
    return dataset, document, chunks
