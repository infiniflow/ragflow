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


# DATASET MANAGEMENT
def batch_create_datasets(client: RAGFlow, num: int) -> list[DataSet]:
    return [client.create_dataset(name=f"dataset_{i}") for i in range(num)]


def delete_all_datasets(client: RAGFlow, *, page_size: int = 1000) -> None:
    # Dataset DELETE now treats null/empty ids as a no-op, so cleanup must enumerate explicit ids.
    page = 1
    dataset_ids: list[str] = []
    while True:
        datasets = client.list_datasets(page=page, page_size=page_size)
        dataset_ids.extend(dataset.id for dataset in datasets)
        if len(datasets) < page_size:
            break
        page += 1

    if dataset_ids:
        client.delete_datasets(ids=dataset_ids)


def delete_all_chats(client: RAGFlow, *, page_size: int = 1000) -> None:
    # Chat DELETE now treats null/empty ids as a no-op, so cleanup must enumerate explicit ids.
    page = 1
    chat_ids: list[str] = []
    while True:
        chats = client.list_chats(page=page, page_size=page_size)
        chat_ids.extend(chat.id for chat in chats)
        if len(chats) < page_size:
            break
        page += 1

    if chat_ids:
        client.delete_chats(ids=chat_ids)


# FILE MANAGEMENT WITHIN DATASET
def bulk_upload_documents(dataset: DataSet, num: int, tmp_path: Path) -> list[Document]:
    document_infos = []
    for i in range(num):
        fp = create_txt_file(tmp_path / f"ragflow_test_upload_{i}.txt")
        with fp.open("rb") as f:
            blob = f.read()
        document_infos.append({"display_name": fp.name, "blob": blob})

    return dataset.upload_documents(document_infos)


def delete_all_documents(dataset: DataSet, *, page_size: int = 1000) -> None:
    # Document DELETE now treats missing/null/empty ids as a no-op, so cleanup must enumerate explicit ids.
    page = 1
    document_ids: list[str] = []
    while True:
        documents = dataset.list_documents(page=page, page_size=page_size)
        document_ids.extend(document.id for document in documents)
        if len(documents) < page_size:
            break
        page += 1

    if document_ids:
        dataset.delete_documents(ids=document_ids)


def delete_all_sessions(chat_assistant: Chat, *, page_size: int = 1000) -> None:
    # Session DELETE now treats missing/null/empty ids as a no-op, so cleanup must enumerate explicit ids.
    page = 1
    session_ids: list[str] = []
    while True:
        sessions = chat_assistant.list_sessions(page=page, page_size=page_size)
        session_ids.extend(session.id for session in sessions)
        if len(sessions) < page_size:
            break
        page += 1

    if session_ids:
        chat_assistant.delete_sessions(ids=session_ids)


def delete_all_chunks(document: Document, *, page_size: int = 1000) -> None:
    # Chunk DELETE now treats missing/null/empty ids as a no-op, so cleanup must enumerate explicit ids.
    page = 1
    chunk_ids: list[str] = []
    while True:
        chunks = document.list_chunks(page=page, page_size=page_size)
        chunk_ids.extend(chunk.id for chunk in chunks)
        if len(chunks) < page_size:
            break
        page += 1

    if chunk_ids:
        document.delete_chunks(ids=chunk_ids)


# CHUNK MANAGEMENT WITHIN DATASET
def batch_add_chunks(document: Document, num: int) -> list[Chunk]:
    return [document.add_chunk(content=f"chunk test {i}") for i in range(num)]


# CHAT ASSISTANT MANAGEMENT
def batch_create_chat_assistants(client: RAGFlow, num: int) -> list[Chat]:
    return [client.create_chat(name=f"test_chat_assistant_{i}") for i in range(num)]


# SESSION MANAGEMENT
def batch_add_sessions_with_chat_assistant(chat_assistant: Chat, num) -> list[Session]:
    return [chat_assistant.create_session(name=f"session_with_chat_assistant_{i}") for i in range(num)]
