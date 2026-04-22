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
from common import batch_add_chunks, delete_all_chunks


@pytest.fixture(scope="class")
def add_chunks(HttpApiAuth, add_document):
    dataset_id, document_id = add_document
    chunk_ids = batch_add_chunks(HttpApiAuth, dataset_id, document_id, 4)
    sleep(1)  # issues/6487
    return dataset_id, document_id, chunk_ids


@pytest.fixture(scope="function")
def add_chunks_func(request, HttpApiAuth, add_document):
    def cleanup():
        delete_all_chunks(HttpApiAuth, dataset_id, document_id)

    request.addfinalizer(cleanup)

    dataset_id, document_id = add_document
    chunk_ids = batch_add_chunks(HttpApiAuth, dataset_id, document_id, 4)
    # issues/6487
    sleep(1)
    return dataset_id, document_id, chunk_ids
