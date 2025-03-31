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
from common import add_chunk, batch_create_datasets, bulk_upload_documents, delete_chunks, delete_dataset, list_documnet, parse_documnet
from libs.utils import wait_for


@wait_for(10, 1, "Document parsing timeout")
def condition(_auth, _dataset_id):
    res = list_documnet(_auth, _dataset_id)
    for doc in res["data"]["docs"]:
        if doc["run"] != "DONE":
            return False
    return True


@pytest.fixture(scope="class")
def chunk_management_tmp_dir(tmp_path_factory):
    return tmp_path_factory.mktemp("chunk_management")


@pytest.fixture(scope="class")
def get_dataset_id_and_document_id(get_http_api_auth, chunk_management_tmp_dir, request):
    def cleanup():
        delete_dataset(get_http_api_auth)

    request.addfinalizer(cleanup)

    dataset_ids = batch_create_datasets(get_http_api_auth, 1)
    dataset_id = dataset_ids[0]
    document_ids = bulk_upload_documents(get_http_api_auth, dataset_id, 1, chunk_management_tmp_dir)
    parse_documnet(get_http_api_auth, dataset_id, {"document_ids": document_ids})
    condition(get_http_api_auth, dataset_id)

    return dataset_id, document_ids[0]


@pytest.fixture(scope="class")
def add_chunks(get_http_api_auth, get_dataset_id_and_document_id):
    dataset_id, document_id = get_dataset_id_and_document_id
    chunk_ids = []
    for i in range(4):
        res = add_chunk(get_http_api_auth, dataset_id, document_id, {"content": f"chunk test {i}"})
        chunk_ids.append(res["data"]["chunk"]["id"])

    # issues/6487
    from time import sleep

    sleep(1)
    return dataset_id, document_id, chunk_ids


@pytest.fixture(scope="function")
def add_chunks_func(get_http_api_auth, get_dataset_id_and_document_id, request):
    dataset_id, document_id = get_dataset_id_and_document_id

    chunk_ids = []
    for i in range(4):
        res = add_chunk(get_http_api_auth, dataset_id, document_id, {"content": f"chunk test {i}"})
        chunk_ids.append(res["data"]["chunk"]["id"])

    # issues/6487
    from time import sleep

    sleep(1)

    def cleanup():
        delete_chunks(get_http_api_auth, dataset_id, document_id, {"chunk_ids": chunk_ids})

    request.addfinalizer(cleanup)
    return dataset_id, document_id, chunk_ids
