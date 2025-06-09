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
from common import batch_create_chat_assistants
from pytest import FixtureRequest
from ragflow_sdk import Chat, DataSet, Document, RAGFlow
from utils import wait_for


@wait_for(30, 1, "Document parsing timeout")
def condition(_dataset: DataSet):
    documents = _dataset.list_documents(page_size=1000)
    for document in documents:
        if document.run != "DONE":
            return False
    return True


@pytest.fixture(scope="function")
def add_chat_assistants_func(request: FixtureRequest, client: RAGFlow, add_document: tuple[DataSet, Document]) -> tuple[DataSet, Document, list[Chat]]:
    def cleanup():
        client.delete_chats(ids=None)

    request.addfinalizer(cleanup)

    dataset, document = add_document
    dataset.async_parse_documents([document.id])
    condition(dataset)
    return dataset, document, batch_create_chat_assistants(client, 5)
