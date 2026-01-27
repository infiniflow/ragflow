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

from ragflow_sdk import Chat, Chunk, DataSet, Document, RAGFlow, Session, Agent
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


# AGENT MANAGEMENT
def create_agent(client: RAGFlow, title: str, dsl: dict, description: str | None = None) -> Agent:
    client.create_agent(title=title, dsl=dsl, description=description)
    # Workaround: Get the first one from the list immediately after creation (assuming descending order by update_time)
    # Note: This depends on list_agents defaulting to update_time desc, which is set in ragflow.py
    agents = client.list_agents(page=1, page_size=1)
    if agents:
        return agents[0]
    raise Exception("Agent created but not found.")


def list_agents(client: RAGFlow, **kwargs) -> list[Agent]:
    return client.list_agents(**kwargs)


def delete_agent(client: RAGFlow, agent_id: str) -> None:
    return client.delete_agent(agent_id)


# AGENT SESSION MANAGEMENT
def create_agent_session(agent: Agent, **kwargs) -> Session:
    return agent.create_session(**kwargs)


def list_agent_sessions(agent: Agent, **kwargs) -> list[Session]:
    return agent.list_sessions(**kwargs)


def delete_agent_sessions(agent: Agent, ids: list[str] | None = None) -> None:
    return agent.delete_sessions(ids=ids)


def batch_add_sessions_with_agent(agent: Agent, num: int) -> list[Session]:
    return [agent.create_session() for _ in range(num)]
