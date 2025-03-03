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

from ragflow_sdk import RAGFlow
from common import HOST_ADDRESS
import pytest


def test_create_session_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_create_session")
    display_name = "ragflow.txt"
    with open("test_data/ragflow.txt", "rb") as file:
        blob = file.read()
    document = {"display_name": display_name, "blob": blob}
    documents = []
    documents.append(document)
    docs = kb.upload_documents(documents)
    for doc in docs:
        doc.add_chunk("This is a test to add chunk")
    assistant = rag.create_chat("test_create_session", dataset_ids=[kb.id])
    assistant.create_session()


def test_create_conversation_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_create_conversation")
    display_name = "ragflow.txt"
    with open("test_data/ragflow.txt", "rb") as file:
        blob = file.read()
    document = {"display_name": display_name, "blob": blob}
    documents = []
    documents.append(document)
    docs = kb.upload_documents(documents)
    for doc in docs:
        doc.add_chunk("This is a test to add chunk")
    assistant = rag.create_chat("test_create_conversation", dataset_ids=[kb.id])
    session = assistant.create_session()
    question = "What is AI"
    for ans in session.ask(question):
        pass

    # assert not ans.content.startswith("**ERROR**"), "Please check this error."


def test_delete_sessions_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_delete_session")
    display_name = "ragflow.txt"
    with open("test_data/ragflow.txt", "rb") as file:
        blob = file.read()
    document = {"display_name": display_name, "blob": blob}
    documents = []
    documents.append(document)
    docs = kb.upload_documents(documents)
    for doc in docs:
        doc.add_chunk("This is a test to add chunk")
    assistant = rag.create_chat("test_delete_session", dataset_ids=[kb.id])
    session = assistant.create_session()
    assistant.delete_sessions(ids=[session.id])


def test_update_session_with_name(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_update_session")
    display_name = "ragflow.txt"
    with open("test_data/ragflow.txt", "rb") as file:
        blob = file.read()
    document = {"display_name": display_name, "blob": blob}
    documents = []
    documents.append(document)
    docs = kb.upload_documents(documents)
    for doc in docs:
        doc.add_chunk("This is a test to add chunk")
    assistant = rag.create_chat("test_update_session", dataset_ids=[kb.id])
    session = assistant.create_session(name="old session")
    session.update({"name": "new session"})


def test_list_sessions_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_list_session")
    display_name = "ragflow.txt"
    with open("test_data/ragflow.txt", "rb") as file:
        blob = file.read()
    document = {"display_name": display_name, "blob": blob}
    documents = []
    documents.append(document)
    docs = kb.upload_documents(documents)
    for doc in docs:
        doc.add_chunk("This is a test to add chunk")
    assistant = rag.create_chat("test_list_session", dataset_ids=[kb.id])
    assistant.create_session("test_1")
    assistant.create_session("test_2")
    assistant.list_sessions()


@pytest.mark.skip(reason="")
def test_create_agent_session_with_success(get_api_key_fixture):
    API_KEY = "ragflow-BkOGNhYjIyN2JiODExZWY5MzVhMDI0Mm"
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    agent = rag.list_agents(id="2e45b5209c1011efa3e90242ac120006")[0]
    agent.create_session()


@pytest.mark.skip(reason="")
def test_create_agent_conversation_with_success(get_api_key_fixture):
    API_KEY = "ragflow-BkOGNhYjIyN2JiODExZWY5MzVhMDI0Mm"
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    agent = rag.list_agents(id="2e45b5209c1011efa3e90242ac120006")[0]
    session = agent.create_session()
    session.ask("What is this job")


@pytest.mark.skip(reason="")
def test_list_agent_sessions_with_success(get_api_key_fixture):
    API_KEY = "ragflow-BkOGNhYjIyN2JiODExZWY5MzVhMDI0Mm"
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    agent = rag.list_agents(id="2e45b5209c1011efa3e90242ac120006")[0]
    agent.list_sessions()

@pytest.mark.skip(reason="")
def test_delete_session_of_agent_with_success(get_api_key_fixture):
    API_KEY = "ragflow-BkOGNhYjIyN2JiODExZWY5MzVhMDI0Mm"
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    agent = rag.list_agents(id="2e45b5209c1011efa3e90242ac120006")[0]
    agent.delete_sessions(ids=["test_1"])
