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
from common import create_session_with_chat_assistant, delete_session_with_chat_assistants


@pytest.fixture(scope="class")
def add_sessions_with_chat_assistant(request, get_http_api_auth, add_chat_assistants):
    _, _, chat_assistant_ids = add_chat_assistants

    def cleanup():
        for chat_assistant_id in chat_assistant_ids:
            delete_session_with_chat_assistants(get_http_api_auth, chat_assistant_id)

    request.addfinalizer(cleanup)

    session_ids = []
    for i in range(5):
        res = create_session_with_chat_assistant(get_http_api_auth, chat_assistant_ids[0], {"name": f"session_with_chat_assistant_{i}"})
        session_ids.append(res["data"]["id"])

    return chat_assistant_ids[0], session_ids


@pytest.fixture(scope="function")
def add_sessions_with_chat_assistant_func(request, get_http_api_auth, add_chat_assistants):
    _, _, chat_assistant_ids = add_chat_assistants

    def cleanup():
        for chat_assistant_id in chat_assistant_ids:
            delete_session_with_chat_assistants(get_http_api_auth, chat_assistant_id)

    request.addfinalizer(cleanup)

    session_ids = []
    for i in range(5):
        res = create_session_with_chat_assistant(get_http_api_auth, chat_assistant_ids[0], {"name": f"session_with_chat_assistant_{i}"})
        session_ids.append(res["data"]["id"])

    return chat_assistant_ids[0], session_ids
