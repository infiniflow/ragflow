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
from common import create_chat_assistant, delete_chat_assistants, list_documnets, parse_documnets
from utils import wait_for


@wait_for(30, 1, "Document parsing timeout")
def condition(_auth, _dataset_id):
    res = list_documnets(_auth, _dataset_id)
    for doc in res["data"]["docs"]:
        if doc["run"] != "DONE":
            return False
    return True


@pytest.fixture(scope="function")
def add_chat_assistants_func(request, api_key, add_document):
    def cleanup():
        delete_chat_assistants(api_key)

    request.addfinalizer(cleanup)

    dataset_id, document_id = add_document
    parse_documnets(api_key, dataset_id, {"document_ids": [document_id]})
    condition(api_key, dataset_id)

    chat_assistant_ids = []
    for i in range(5):
        res = create_chat_assistant(api_key, {"name": f"test_chat_assistant_{i}", "dataset_ids": [dataset_id]})
        chat_assistant_ids.append(res["data"]["id"])

    return dataset_id, document_id, chat_assistant_ids
