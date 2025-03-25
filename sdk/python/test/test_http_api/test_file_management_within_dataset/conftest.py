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
from common import batch_create_datasets, bulk_upload_documents, delete_dataset


@pytest.fixture(scope="class")
def file_management_tmp_dir(tmp_path_factory):
    return tmp_path_factory.mktemp("file_management")


@pytest.fixture(scope="class")
def get_dataset_id_and_document_ids(get_http_api_auth, file_management_tmp_dir, request):
    def cleanup():
        delete_dataset(get_http_api_auth)

    request.addfinalizer(cleanup)

    dataset_ids = batch_create_datasets(get_http_api_auth, 1)
    dataset_id = dataset_ids[0]
    document_ids = bulk_upload_documents(get_http_api_auth, dataset_id, 5, file_management_tmp_dir)
    return dataset_id, document_ids
