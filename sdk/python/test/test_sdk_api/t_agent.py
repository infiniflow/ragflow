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

from ragflow_sdk import RAGFlow, Agent
from common import HOST_ADDRESS
import pytest


@pytest.mark.skip(reason="")
def test_list_agents_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    rag.list_agents()


@pytest.mark.skip(reason="")
def test_converse_with_agent_with_success(get_api_key_fixture):
    API_KEY = "ragflow-BkOGNhYjIyN2JiODExZWY5MzVhMDI0Mm"
    agent_id = "ebfada2eb2bc11ef968a0242ac120006"
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    lang = "Chinese"
    file = "How is the weather tomorrow?"
    Agent.ask(agent_id=agent_id, rag=rag, lang=lang, file=file)
