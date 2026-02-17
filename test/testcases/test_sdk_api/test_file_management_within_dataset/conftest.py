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
from common import bulk_upload_documents
from pytest import FixtureRequest
from ragflow_sdk import DataSet, Document


@pytest.fixture(scope="function")
def add_document_func(request: FixtureRequest, add_dataset: DataSet, ragflow_tmp_dir) -> tuple[DataSet, Document]:
    dataset = add_dataset
    documents = bulk_upload_documents(dataset, 1, ragflow_tmp_dir)

    def cleanup():
        dataset.delete_documents(ids=None)

    request.addfinalizer(cleanup)
    return dataset, documents[0]


@pytest.fixture(scope="class")
def add_documents(request: FixtureRequest, add_dataset: DataSet, ragflow_tmp_dir) -> tuple[DataSet, list[Document]]:
    dataset = add_dataset
    documents = bulk_upload_documents(dataset, 5, ragflow_tmp_dir)

    def cleanup():
        dataset.delete_documents(ids=None)

    request.addfinalizer(cleanup)
    return dataset, documents


@pytest.fixture(scope="function")
def add_documents_func(request: FixtureRequest, add_dataset_func: DataSet, ragflow_tmp_dir) -> tuple[DataSet, list[Document]]:
    dataset = add_dataset_func
    documents = bulk_upload_documents(dataset, 3, ragflow_tmp_dir)

    def cleanup():
        dataset.delete_documents(ids=None)

    request.addfinalizer(cleanup)
    return dataset, documents
