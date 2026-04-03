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


from time import sleep

import pytest
from common import batch_add_chunks, delete_chunks, list_documents, parse_documents
from utils import wait_for


@wait_for(30, 1, "Document parsing timeout")
def condition(_auth, _dataset_id):
    res = list_documents(_auth, _dataset_id)
    for doc in res["data"]["docs"]:
        if doc["run"] != "DONE":
            return False
    return True


@pytest.fixture(scope="function")
def add_chunks_func(request, HttpApiAuth, add_document):
    def cleanup():
        delete_chunks(HttpApiAuth, dataset_id, document_id, {"chunk_ids": []})

    request.addfinalizer(cleanup)

    dataset_id, document_id = add_document
    parse_documents(HttpApiAuth, dataset_id, {"document_ids": [document_id]})
    condition(HttpApiAuth, dataset_id)
    chunk_ids = batch_add_chunks(HttpApiAuth, dataset_id, document_id, 4)
    # issues/6487
    sleep(1)
    return dataset_id, document_id, chunk_ids
