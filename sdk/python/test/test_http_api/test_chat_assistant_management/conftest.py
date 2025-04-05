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
from common import create_chat_assistant, delete_chat_assistants


@pytest.fixture(scope="class")
def add_chat_assistants(request, get_http_api_auth, add_chunks):
    def cleanup():
        delete_chat_assistants(get_http_api_auth)

    request.addfinalizer(cleanup)

    dataset_id, document_id, chunk_ids = add_chunks
    chat_assistant_ids = []
    for i in range(5):
        res = create_chat_assistant(get_http_api_auth, {"name": f"test_chat_assistant_{i}", "dataset_ids": [dataset_id]})
        chat_assistant_ids.append(res["data"]["id"])

    return dataset_id, document_id, chunk_ids, chat_assistant_ids
