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
from common import batch_add_sessions_with_chat_assistant
from pytest import FixtureRequest
from ragflow_sdk import Chat, DataSet, Document, Session


@pytest.fixture(scope="class")
def add_sessions_with_chat_assistant(request: FixtureRequest, add_chat_assistants: tuple[DataSet, Document, list[Chat]]) -> tuple[Chat, list[Session]]:
    def cleanup():
        for chat_assistant in chat_assistants:
            try:
                chat_assistant.delete_sessions(ids=None)
            except Exception :
                pass

    request.addfinalizer(cleanup)

    _, _, chat_assistants = add_chat_assistants
    return chat_assistants[0], batch_add_sessions_with_chat_assistant(chat_assistants[0], 5)


@pytest.fixture(scope="function")
def add_sessions_with_chat_assistant_func(request: FixtureRequest, add_chat_assistants: tuple[DataSet, Document, list[Chat]]) -> tuple[Chat, list[Session]]:
    def cleanup():
        for chat_assistant in chat_assistants:
            try:
                chat_assistant.delete_sessions(ids=None)
            except Exception :
                pass

    request.addfinalizer(cleanup)

    _, _, chat_assistants = add_chat_assistants
    return chat_assistants[0], batch_add_sessions_with_chat_assistant(chat_assistants[0], 5)
