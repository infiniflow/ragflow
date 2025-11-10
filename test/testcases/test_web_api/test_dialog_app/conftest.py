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
from common import batch_create_dialogs, delete_dialogs


@pytest.fixture(scope="function")
def add_dialog_func(request, WebApiAuth, add_dataset_func):
    def cleanup():
        delete_dialogs(WebApiAuth)

    request.addfinalizer(cleanup)

    dataset_id = add_dataset_func
    return dataset_id, batch_create_dialogs(WebApiAuth, 1, [dataset_id])[0]


@pytest.fixture(scope="class")
def add_dialogs(request, WebApiAuth, add_dataset):
    def cleanup():
        delete_dialogs(WebApiAuth)

    request.addfinalizer(cleanup)

    dataset_id = add_dataset
    return dataset_id, batch_create_dialogs(WebApiAuth, 5, [dataset_id])


@pytest.fixture(scope="function")
def add_dialogs_func(request, WebApiAuth, add_dataset_func):
    def cleanup():
        delete_dialogs(WebApiAuth)

    request.addfinalizer(cleanup)

    dataset_id = add_dataset_func
    return dataset_id, batch_create_dialogs(WebApiAuth, 5, [dataset_id])
