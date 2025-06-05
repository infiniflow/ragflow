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
from common import (
    batch_create_datasets,
)
from configs import HOST_ADDRESS, VERSION
from ragflow_sdk import RAGFlow


@pytest.fixture(scope="session")
def client(token):
    return RAGFlow(api_key=token, base_url=HOST_ADDRESS, version=VERSION)


@pytest.fixture(scope="function")
def clear_datasets(request, client):
    def cleanup():
        client.delete_datasets(ids=None)

    request.addfinalizer(cleanup)


@pytest.fixture(scope="function")
def add_dataset_func(request, client):
    def cleanup():
        client.delete_datasets(ids=None)

    request.addfinalizer(cleanup)

    return batch_create_datasets(client, 1)[0]
