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

import random

import pytest
from common import HOST_ADDRESS
from ragflow_sdk import RAGFlow


def test_create_dataset_with_name(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    rag.create_dataset("test_create_dataset_with_name")


def test_create_dataset_with_duplicated_name(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    rag.create_dataset("test_create_dataset_with_duplicated_name")
    with pytest.raises(Exception) as exc_info:
        rag.create_dataset("test_create_dataset_with_duplicated_name")
    assert str(exc_info.value) == "Dataset name 'test_create_dataset_with_duplicated_name' already exists"


def test_create_dataset_with_random_chunk_method(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    valid_chunk_methods = ["naive", "manual", "qa", "table", "paper", "book", "laws", "presentation", "picture", "one", "email"]
    random_chunk_method = random.choice(valid_chunk_methods)
    rag.create_dataset("test_create_dataset_with_random_chunk_method", chunk_method=random_chunk_method)


def test_create_dataset_with_invalid_parameter(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    chunk_method = "invalid_chunk_method"
    with pytest.raises(Exception) as exc_info:
        rag.create_dataset("test_create_dataset_with_invalid_chunk_method", chunk_method=chunk_method)
    assert (
        str(exc_info.value)
        == f"Field: <chunk_method> - Message: <Input should be 'naive', 'book', 'email', 'laws', 'manual', 'one', 'paper', 'picture', 'presentation', 'qa', 'table' or 'tag'> - Value: <{chunk_method}>"
    )


def test_update_dataset_with_name(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    ds = rag.create_dataset("test_update_dataset")
    ds.update({"name": "updated_dataset"})


def test_delete_datasets_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    ds = rag.create_dataset("test_delete_dataset")
    rag.delete_datasets(ids=[ds.id])


def test_list_datasets_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    rag.create_dataset("test_list_datasets")
    rag.list_datasets()
