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
from common import batch_create_datasets, list_datasets, delete_datasets
from libs.auth import RAGFlowWebApiAuth
from pytest import FixtureRequest
from ragflow_sdk import RAGFlow


@pytest.fixture(scope="class")
def add_datasets(request: FixtureRequest, client: RAGFlow, WebApiAuth: RAGFlowWebApiAuth) -> list[str]:
    dataset_ids = batch_create_datasets(WebApiAuth, 5)

    def cleanup():
        # Web KB cleanup cannot call SDK dataset bulk delete with empty ids; deletion must stay explicit.
        res = list_datasets(WebApiAuth, params={"page_size": 1000})
        existing_ids = {kb["id"] for kb in res["data"]}
        ids_to_delete = list({dataset_id for dataset_id in dataset_ids if dataset_id in existing_ids})
        delete_datasets(WebApiAuth, {"ids": ids_to_delete})

    request.addfinalizer(cleanup)
    return dataset_ids


@pytest.fixture(scope="function")
def add_datasets_func(request: FixtureRequest, client: RAGFlow, WebApiAuth: RAGFlowWebApiAuth) -> list[str]:
    dataset_ids = batch_create_datasets(WebApiAuth, 3)

    def cleanup():
        # Web KB cleanup cannot call SDK dataset bulk delete with empty ids; deletion must stay explicit.
        res = list_datasets(WebApiAuth, params={"page_size": 1000})
        existing_ids = {kb["id"] for kb in res["data"]}
        ids_to_delete = list({dataset_id for dataset_id in dataset_ids if dataset_id in existing_ids})
        delete_datasets(WebApiAuth, {"ids": ids_to_delete})

    request.addfinalizer(cleanup)
    return dataset_ids
