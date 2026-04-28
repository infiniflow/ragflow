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

import requests
from configs import HOST_ADDRESS, VERSION
from requests_toolbelt import MultipartEncoder
from utils.file_utils import create_txt_file

HEADERS = {"Content-Type": "application/json"}
DATASETS_API_URL = f"/api/{VERSION}/datasets"
FILE_API_URL = f"/api/{VERSION}/datasets/{{dataset_id}}/documents"
FILE_PARSE_API_URL = f"/api/{VERSION}/datasets/{{dataset_id}}/documents/parse"
FILE_STOP_PARSE_API_URL = f"/api/{VERSION}/datasets/{{dataset_id}}/documents/stop"
CHUNK_API_URL = f"/api/{VERSION}/datasets/{{dataset_id}}/documents/{{document_id}}/chunks"
CHAT_ASSISTANT_API_URL = f"/api/{VERSION}/chats"
SESSION_WITH_CHAT_ASSISTANT_API_URL = f"/api/{VERSION}/chats/{{chat_id}}/sessions"
SESSION_WITH_AGENT_API_URL = f"/api/{VERSION}/agents/{{agent_id}}/sessions"
AGENT_API_URL = f"/api/{VERSION}/agents"
RETRIEVAL_API_URL = f"/api/{VERSION}/retrieval"


# DATASET MANAGEMENT
def create_dataset(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DATASETS_API_URL}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def list_datasets(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{DATASETS_API_URL}", headers=headers, auth=auth, params=params)
    return res.json()


def update_dataset(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    res = requests.put(url=f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def delete_datasets(auth, payload=None, *, headers=HEADERS, data=None):
    """
    Delete datasets.
    The endpoint is DELETE /api/{VERSION}/datasets with payload {"ids": [...]}
    This is the standard SDK REST API endpoint for dataset deletion.
    """
    res = requests.delete(url=f"{HOST_ADDRESS}{DATASETS_API_URL}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def delete_all_datasets(auth, *, page_size=1000):
    return delete_datasets(auth, {"ids": None, "delete_all": True})


def batch_create_datasets(auth, num):
    ids = []
    for i in range(num):
        res = create_dataset(auth, {"name": f"dataset_{i}"})
        ids.append(res["data"]["id"])
    return ids


# FILE MANAGEMENT WITHIN DATASET
def upload_documents(auth, dataset_id, files_path=None):
    url = f"{HOST_ADDRESS}{FILE_API_URL}".format(dataset_id=dataset_id)

    if files_path is None:
        files_path = []

    fields = []
    file_objects = []
    try:
        for fp in files_path:
            p = Path(fp)
            f = p.open("rb")
            fields.append(("file", (p.name, f)))
            file_objects.append(f)
        m = MultipartEncoder(fields=fields)

        res = requests.post(
            url=url,
            headers={"Content-Type": m.content_type},
            auth=auth,
            data=m,
        )
        return res.json()
    finally:
        for f in file_objects:
            f.close()


def download_document(auth, dataset_id, document_id, save_path):
    url = f"{HOST_ADDRESS}{FILE_API_URL}/{document_id}".format(dataset_id=dataset_id)
    res = requests.get(url=url, auth=auth, stream=True)
    try:
        # available for unauthed downloads
        if res.status_code in (200, 401):
            with open(save_path, "wb") as f:
                for chunk in res.iter_content(chunk_size=8192):
                    f.write(chunk)
    finally:
        res.close()

    return res


def list_documents(auth, dataset_id, params=None):
    url = f"{HOST_ADDRESS}{FILE_API_URL}".format(dataset_id=dataset_id)
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def update_document(auth, dataset_id, document_id, payload=None):
    url = f"{HOST_ADDRESS}{FILE_API_URL}/{document_id}".format(dataset_id=dataset_id)
    res = requests.patch(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_documents(auth, dataset_id, payload=None):
    url = f"{HOST_ADDRESS}{FILE_API_URL}".format(dataset_id=dataset_id)
    res = requests.delete(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_all_documents(auth, dataset_id, *, page_size=1000):
    return delete_documents(auth, dataset_id, {"ids": None, "delete_all": True})


def parse_documents(auth, dataset_id, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{FILE_PARSE_API_URL}".format(dataset_id=dataset_id)
    res = requests.post(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


def stop_parse_documents(auth, dataset_id, payload=None):
    url = f"{HOST_ADDRESS}{FILE_STOP_PARSE_API_URL}".format(dataset_id=dataset_id)
    res = requests.post(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def bulk_upload_documents(auth, dataset_id, num, tmp_path):
    fps = []
    for i in range(num):
        fp = create_txt_file(tmp_path / f"ragflow_test_upload_{i}.txt")
        fps.append(fp)
    res = upload_documents(auth, dataset_id, fps)
    document_ids = []
    for document in res["data"]:
        document_ids.append(document["id"])
    return document_ids


# CHUNK MANAGEMENT WITHIN DATASET
def add_chunk(auth, dataset_id, document_id, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.post(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


def list_chunks(auth, dataset_id, document_id, params=None):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def get_chunk(auth, dataset_id, document_id, chunk_id):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}/{chunk_id}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.get(url=url, headers=HEADERS, auth=auth)
    return res.json()


def update_chunk(auth, dataset_id, document_id, chunk_id, payload=None):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}/{chunk_id}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.patch(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_chunks(auth, dataset_id, document_id, payload=None):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.delete(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_all_chunks(auth, dataset_id, document_id, *, page_size=1000):
    return delete_chunks(auth, dataset_id, document_id, {"chunk_ids": None, "delete_all": True})


def retrieval_chunks(auth, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{RETRIEVAL_API_URL}"
    res = requests.post(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


def batch_add_chunks(auth, dataset_id, document_id, num):
    chunk_ids = []
    for i in range(num):
        res = add_chunk(auth, dataset_id, document_id, {"content": f"chunk test {i}"})
        chunk_ids.append(res["data"]["chunk"]["id"])
    return chunk_ids


# CHAT ASSISTANT MANAGEMENT
def create_chat_assistant(auth, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{CHAT_ASSISTANT_API_URL}"
    res = requests.post(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


def list_chat_assistants(auth, params=None):
    url = f"{HOST_ADDRESS}{CHAT_ASSISTANT_API_URL}"
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def get_chat_assistant(auth, chat_assistant_id):
    url = f"{HOST_ADDRESS}{CHAT_ASSISTANT_API_URL}/{chat_assistant_id}"
    res = requests.get(url=url, headers=HEADERS, auth=auth)
    return res.json()


def update_chat_assistant(auth, chat_assistant_id, payload=None):
    url = f"{HOST_ADDRESS}{CHAT_ASSISTANT_API_URL}/{chat_assistant_id}"
    res = requests.put(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def patch_chat_assistant(auth, chat_assistant_id, payload=None):
    url = f"{HOST_ADDRESS}{CHAT_ASSISTANT_API_URL}/{chat_assistant_id}"
    res = requests.patch(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_chat_assistants(auth, payload=None):
    url = f"{HOST_ADDRESS}{CHAT_ASSISTANT_API_URL}"
    res = requests.delete(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_all_chat_assistants(auth, *, page_size=1000):
    return delete_chat_assistants(auth, {"ids": None, "delete_all": True})


def batch_create_chat_assistants(auth, num):
    chat_assistant_ids = []
    for i in range(num):
        res = create_chat_assistant(auth, {"name": f"test_chat_assistant_{i}", "dataset_ids": []})
        chat_assistant_ids.append(res["data"]["id"])
    return chat_assistant_ids


# SESSION MANAGEMENT
def create_session_with_chat_assistant(auth, chat_assistant_id, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{SESSION_WITH_CHAT_ASSISTANT_API_URL}".format(chat_id=chat_assistant_id)
    res = requests.post(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


def list_session_with_chat_assistants(auth, chat_assistant_id, params=None):
    url = f"{HOST_ADDRESS}{SESSION_WITH_CHAT_ASSISTANT_API_URL}".format(chat_id=chat_assistant_id)
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def update_session_with_chat_assistant(auth, chat_assistant_id, session_id, payload=None):
    url = f"{HOST_ADDRESS}{SESSION_WITH_CHAT_ASSISTANT_API_URL}/{session_id}".format(chat_id=chat_assistant_id)
    res = requests.patch(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_session_with_chat_assistants(auth, chat_assistant_id, payload=None):
    url = f"{HOST_ADDRESS}{SESSION_WITH_CHAT_ASSISTANT_API_URL}".format(chat_id=chat_assistant_id)
    res = requests.delete(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_all_sessions_with_chat_assistant(auth, chat_assistant_id, *, page_size=1000):
    return delete_session_with_chat_assistants(auth, chat_assistant_id, {"ids": None, "delete_all": True})


def batch_add_sessions_with_chat_assistant(auth, chat_assistant_id, num):
    session_ids = []
    for i in range(num):
        res = create_session_with_chat_assistant(auth, chat_assistant_id, {"name": f"session_with_chat_assistant_{i}"})
        session_ids.append(res["data"]["id"])
    return session_ids


# DATASET GRAPH AND TASKS
def knowledge_graph(auth, dataset_id, params=None):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/graph/search"
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def delete_knowledge_graph(auth, dataset_id, payload=None):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/graph"
    if payload is None:
        res = requests.delete(url=url, headers=HEADERS, auth=auth)
    else:
        res = requests.delete(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def metadata_summary(auth, dataset_id, params=None):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/metadata/summary"
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def metadata_batch_update(auth, dataset_id, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/metadata/update"
    res = requests.post(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


def update_documents_metadata(auth, dataset_id, payload=None):
    """New unified API for updating document metadata.

    Uses PATCH method at /api/v1/datasets/{dataset_id}/documents/metadatas
    """
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/documents/metadatas"
    res = requests.patch(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


# CHAT COMPLETIONS AND RELATED QUESTIONS
def related_questions(auth, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}/api/{VERSION}/searchbots/related_questions"
    res = requests.post(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


# AGENT MANAGEMENT AND SESSIONS
def create_agent(auth, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{AGENT_API_URL}"
    res = requests.post(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


def list_agents(auth, params=None):
    url = f"{HOST_ADDRESS}{AGENT_API_URL}"
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def delete_agent(auth, agent_id):
    url = f"{HOST_ADDRESS}{AGENT_API_URL}/{agent_id}"
    res = requests.delete(url=url, headers=HEADERS, auth=auth)
    return res.json()


def create_agent_session(auth, agent_id, payload=None, params=None):
    url = f"{HOST_ADDRESS}{SESSION_WITH_AGENT_API_URL}".format(agent_id=agent_id)
    res = requests.post(url=url, headers=HEADERS, auth=auth, json=payload, params=params)
    return res.json()


def list_agent_sessions(auth, agent_id, params=None):
    url = f"{HOST_ADDRESS}{SESSION_WITH_AGENT_API_URL}".format(agent_id=agent_id)
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def delete_agent_sessions(auth, agent_id, payload=None):
    url = f"{HOST_ADDRESS}{SESSION_WITH_AGENT_API_URL}".format(agent_id=agent_id)
    res = requests.delete(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_all_agent_sessions(auth, agent_id, *, page_size=1000):
    return delete_agent_sessions(auth, agent_id, {"ids": None, "delete_all": True})


def agent_completions(auth, agent_id, payload=None):
    url = f"{HOST_ADDRESS}{AGENT_API_URL}/chat/completion"
    body = {"agent_id": agent_id}
    if payload:
        body.update(payload)
    res = requests.post(url=url, headers=HEADERS, auth=auth, json=body)
    return res.json()


def chat_completions(auth, chat_id=None, payload=None):
    """
    Send a question/message to a chat assistant and get completion.

    Args:
        auth: Authentication object
        chat_id: Chat assistant ID
        payload: Dictionary containing:
            - messages: list (required) - Conversation messages
            - stream: bool (optional) - Whether to stream responses, default False
            - session_id: str (optional) - Session ID for conversation context

    Returns:
        Response JSON with answer data
    """
    url = f"{HOST_ADDRESS}/api/{VERSION}/chat/completions"
    payload = dict(payload or {})
    if chat_id:
        payload.setdefault("chat_id", chat_id)
    if "question" in payload and "messages" not in payload:
        payload["messages"] = [{"role": "user", "content": payload.pop("question")}]
    res = requests.post(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def chat_completions_openai(auth, chat_id, payload=None, *, headers=HEADERS):
    """
    Send a request to the OpenAI-compatible chat completions endpoint.

    Args:
        auth: Authentication object
        chat_id: Chat assistant ID
        payload: Dictionary in OpenAI chat completions format containing:
            - messages: list (required) - List of message objects with 'role' and 'content'
            - stream: bool (optional) - Whether to stream responses, default False

    Returns:
        Response JSON in OpenAI chat completions format with usage information
    """
    url = f"{HOST_ADDRESS}/api/{VERSION}/openai/{chat_id}/chat/completions"
    payload = dict(payload or {})
    res = requests.post(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


# NEW DATASET ENDPOINTS
def get_dataset(auth, dataset_id, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}"
    res = requests.get(url=url, headers=headers, auth=auth)
    return res.json()


def get_ingestion_summary(auth, dataset_id, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/ingestions/summary"
    res = requests.get(url=url, headers=headers, auth=auth)
    return res.json()


def list_ingestion_logs(auth, dataset_id, params=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/ingestions"
    res = requests.get(url=url, headers=headers, auth=auth, params=params)
    return res.json()


def get_ingestion_log(auth, dataset_id, log_id, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/ingestions/{log_id}"
    res = requests.get(url=url, headers=headers, auth=auth)
    return res.json()


def run_index(auth, dataset_id, index_type, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/index"
    params = {"type": index_type}
    res = requests.post(url=url, headers=headers, auth=auth, json=payload, params=params)
    return res.json()


def trace_index(auth, dataset_id, index_type, params=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/index"
    all_params = {"type": index_type}
    if params:
        all_params.update(params)
    res = requests.get(url=url, headers=headers, auth=auth, params=all_params)
    return res.json()


def delete_index(auth, dataset_id, index_type, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/{index_type}"
    res = requests.delete(url=url, headers=headers, auth=auth)
    return res.json()


def run_embedding(auth, dataset_id, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/embedding"
    res = requests.post(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


def list_tags(auth, dataset_id, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/tags"
    res = requests.get(url=url, headers=headers, auth=auth)
    return res.json()


def aggregate_tags(auth, dataset_ids, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/tags/aggregation"
    res = requests.get(url=url, headers=headers, auth=auth, params={"dataset_ids": ",".join(dataset_ids)})
    return res.json()


def delete_tags(auth, dataset_id, tags, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/tags"
    res = requests.delete(url=url, headers=headers, auth=auth, json={"tags": tags})
    return res.json()


def rename_tag(auth, dataset_id, from_tag, to_tag, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}/tags"
    res = requests.put(url=url, headers=headers, auth=auth, json={"from_tag": from_tag, "to_tag": to_tag})
    return res.json()


def get_flattened_metadata(auth, dataset_ids, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/metadata/flattened"
    res = requests.get(url=url, headers=headers, auth=auth, params={"dataset_ids": ",".join(dataset_ids)})
    return res.json()
