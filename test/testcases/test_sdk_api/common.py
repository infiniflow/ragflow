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

from pathlib import Path

from ragflow_sdk import Chat, Chunk, DataSet, Document, RAGFlow, Session
from utils.file_utils import create_txt_file


REST_API_MAX_PAGE_SIZE = 100


def list_all_documents(dataset: DataSet, *, limit: int | None = None, page_size: int = REST_API_MAX_PAGE_SIZE) -> list[Document]:
    page_size = min(page_size, REST_API_MAX_PAGE_SIZE)
    documents: list[Document] = []
    page = 1
    while True:
        batch = dataset.list_documents(page=page, page_size=page_size)
        documents.extend(batch)
        if limit is not None and len(documents) >= limit:
            return documents[:limit]
        if len(batch) < page_size:
            return documents
        page += 1


def list_all_sessions(chat_assistant: Chat, *, limit: int | None = None, page_size: int = REST_API_MAX_PAGE_SIZE) -> list[Session]:
    page_size = min(page_size, REST_API_MAX_PAGE_SIZE)
    sessions: list[Session] = []
    page = 1
    while True:
        batch = chat_assistant.list_sessions(page=page, page_size=page_size)
        sessions.extend(batch)
        if limit is not None and len(sessions) >= limit:
            return sessions[:limit]
        if len(batch) < page_size:
            return sessions
        page += 1


def valid_chat_llm_id(client: RAGFlow) -> str:
    # SDK tests use the tenant's configured chat model; this helper discovers test fixture state, not SDK behavior.
    res = client.get('/users/me/models')
    data = res.json()
    if data.get('code') == 0:
        llm_id = (data.get('data') or {}).get('llm_id')
        if llm_id:
            return llm_id
    raise Exception('No valid chat llm_id is configured for the current tenant')


# DATASET MANAGEMENT
def batch_create_datasets(client: RAGFlow, num: int) -> list[DataSet]:
    return [client.create_dataset(name=f"dataset_{i}") for i in range(num)]


def delete_all_datasets(client: RAGFlow, *, page_size: int = 100) -> None:
    client.delete_datasets(delete_all=True)


def delete_all_chats(client: RAGFlow, *, page_size: int = 100) -> None:
    client.delete_chats(delete_all=True)


# FILE MANAGEMENT WITHIN DATASET
def bulk_upload_documents(dataset: DataSet, num: int, tmp_path: Path) -> list[Document]:
    document_infos = []
    for i in range(num):
        fp = create_txt_file(tmp_path / f"ragflow_test_upload_{i}.txt")
        with fp.open("rb") as f:
            blob = f.read()
        document_infos.append({"display_name": fp.name, "blob": blob})

    return dataset.upload_documents(document_infos)


def delete_all_documents(dataset: DataSet, *, page_size: int = 100) -> None:
    dataset.delete_documents(delete_all=True)


def delete_all_sessions(chat_assistant: Chat, *, page_size: int = 100) -> None:
    chat_assistant.delete_sessions(delete_all=True)


def delete_all_chunks(document: Document, *, page_size: int = 100) -> None:
    document.delete_chunks(delete_all=True)


# CHUNK MANAGEMENT WITHIN DATASET
def batch_add_chunks(document: Document, num: int) -> list[Chunk]:
    return [document.add_chunk(content=f"chunk test {i}") for i in range(num)]


# CHAT ASSISTANT MANAGEMENT
def batch_create_chat_assistants(client: RAGFlow, num: int) -> list[Chat]:
    return [client.create_chat(name=f"test_chat_assistant_{i}") for i in range(num)]


# SESSION MANAGEMENT
def batch_add_sessions_with_chat_assistant(chat_assistant: Chat, num) -> list[Session]:
    return [chat_assistant.create_session(name=f"session_with_chat_assistant_{i}") for i in range(num)]
