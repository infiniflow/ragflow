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


# FILE MANAGEMENT WITHIN DATASET
def bulk_upload_documents(dataset: DataSet, num: int, tmp_path: Path) -> list[Document]:
    document_infos = []
    for i in range(num):
        fp = create_txt_file(tmp_path / f"ragflow_test_upload_{i}.txt")
        with fp.open("rb") as f:
            blob = f.read()
        document_infos.append({"display_name": fp.name, "blob": blob})

    return dataset.upload_documents(document_infos)


# CHUNK MANAGEMENT WITHIN DATASET
def batch_add_chunks(document: Document, num: int) -> list[Chunk]:
    return [document.add_chunk(content=f"chunk test {i}") for i in range(num)]


# CHAT ASSISTANT MANAGEMENT
def batch_create_chat_assistants(client: RAGFlow, num: int) -> list[Chat]:
    return [client.create_chat(name=f"test_chat_assistant_{i}") for i in range(num)]


# SESSION MANAGEMENT
def batch_add_sessions_with_chat_assistant(chat_assistant: Chat, num) -> list[Session]:
    return [chat_assistant.create_session(name=f"session_with_chat_assistant_{i}") for i in range(num)]
