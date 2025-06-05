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
from common import bulk_upload_documents, delete_documents


@pytest.fixture(scope="function")
def add_document_func(request, api_key, add_dataset, ragflow_tmp_dir):
    dataset_id = add_dataset
    document_ids = bulk_upload_documents(api_key, dataset_id, 1, ragflow_tmp_dir)

    def cleanup():
        delete_documents(api_key, dataset_id, {"ids": None})

    request.addfinalizer(cleanup)
    return dataset_id, document_ids[0]


@pytest.fixture(scope="class")
def add_documents(request, api_key, add_dataset, ragflow_tmp_dir):
    dataset_id = add_dataset
    document_ids = bulk_upload_documents(api_key, dataset_id, 5, ragflow_tmp_dir)

    def cleanup():
        delete_documents(api_key, dataset_id, {"ids": None})

    request.addfinalizer(cleanup)
    return dataset_id, document_ids


@pytest.fixture(scope="function")
def add_documents_func(request, api_key, add_dataset_func, ragflow_tmp_dir):
    dataset_id = add_dataset_func
    document_ids = bulk_upload_documents(api_key, dataset_id, 3, ragflow_tmp_dir)

    def cleanup():
        delete_documents(api_key, dataset_id, {"ids": None})

    request.addfinalizer(cleanup)
    return dataset_id, document_ids
