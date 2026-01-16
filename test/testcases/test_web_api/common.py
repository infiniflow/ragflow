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

KB_APP_URL = f"/{VERSION}/kb"
DOCUMENT_APP_URL = f"/{VERSION}/document"
CHUNK_API_URL = f"/{VERSION}/chunk"
DIALOG_APP_URL = f"/{VERSION}/dialog"
# SESSION_WITH_CHAT_ASSISTANT_API_URL = "/api/v1/chats/{chat_id}/sessions"
# SESSION_WITH_AGENT_API_URL = "/api/v1/agents/{agent_id}/sessions"
MEMORY_API_URL = f"/api/{VERSION}/memories"
MESSAGE_API_URL = f"/api/{VERSION}/messages"


# KB APP
def create_kb(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/create", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def list_kbs(auth, params=None, payload=None, *, headers=HEADERS, data=None):
    if payload is None:
        payload = {}
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/list", headers=headers, auth=auth, params=params, json=payload, data=data)
    return res.json()


def update_kb(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/update", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def rm_kb(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/rm", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def detail_kb(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{KB_APP_URL}/detail", headers=headers, auth=auth, params=params)
    return res.json()


def list_tags_from_kbs(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{KB_APP_URL}/tags", headers=headers, auth=auth, params=params)
    return res.json()


def list_tags(auth, dataset_id, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{KB_APP_URL}/{dataset_id}/tags", headers=headers, auth=auth, params=params)
    return res.json()


def rm_tags(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/{dataset_id}/rm_tags", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def rename_tags(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/{dataset_id}/rename_tags", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def knowledge_graph(auth, dataset_id, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{KB_APP_URL}/{dataset_id}/knowledge_graph", headers=headers, auth=auth, params=params)
    return res.json()


def delete_knowledge_graph(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    res = requests.delete(url=f"{HOST_ADDRESS}{KB_APP_URL}/{dataset_id}/delete_knowledge_graph", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def batch_create_datasets(auth, num):
    ids = []
    for i in range(num):
        res = create_kb(auth, {"name": f"kb_{i}"})
        ids.append(res["data"]["kb_id"])
    return ids


# DOCUMENT APP
def upload_documents(auth, payload=None, files_path=None, *, filename_override=None):
    url = f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/upload"

    if files_path is None:
        files_path = []

    fields = []
    file_objects = []
    try:
        if payload:
            for k, v in payload.items():
                fields.append((k, str(v)))

        for fp in files_path:
            p = Path(fp)
            f = p.open("rb")
            filename = filename_override if filename_override is not None else p.name
            fields.append(("file", (filename, f)))
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


def create_document(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/create", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def list_documents(auth, params=None, payload=None, *, headers=HEADERS, data=None):
    if payload is None:
        payload = {}
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/list", headers=headers, auth=auth, params=params, json=payload, data=data)
    return res.json()


def delete_document(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/rm", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def parse_documents(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/run", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def bulk_upload_documents(auth, kb_id, num, tmp_path):
    fps = []
    for i in range(num):
        fp = create_txt_file(tmp_path / f"ragflow_test_upload_{i}.txt")
        fps.append(fp)

    res = upload_documents(auth, {"kb_id": kb_id}, fps)
    document_ids = []
    for document in res["data"]:
        document_ids.append(document["id"])
    return document_ids


# CHUNK APP
def add_chunk(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{CHUNK_API_URL}/create", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def list_chunks(auth, payload=None, *, headers=HEADERS):
    res = requests.post(url=f"{HOST_ADDRESS}{CHUNK_API_URL}/list", headers=headers, auth=auth, json=payload)
    return res.json()


def get_chunk(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{CHUNK_API_URL}/get", headers=headers, auth=auth, params=params)
    return res.json()


def update_chunk(auth, payload=None, *, headers=HEADERS):
    res = requests.post(url=f"{HOST_ADDRESS}{CHUNK_API_URL}/set", headers=headers, auth=auth, json=payload)
    return res.json()


def delete_chunks(auth, payload=None, *, headers=HEADERS):
    res = requests.post(url=f"{HOST_ADDRESS}{CHUNK_API_URL}/rm", headers=headers, auth=auth, json=payload)
    return res.json()


def retrieval_chunks(auth, payload=None, *, headers=HEADERS):
    res = requests.post(url=f"{HOST_ADDRESS}{CHUNK_API_URL}/retrieval_test", headers=headers, auth=auth, json=payload)
    return res.json()


def batch_add_chunks(auth, doc_id, num):
    chunk_ids = []
    for i in range(num):
        res = add_chunk(auth, {"doc_id": doc_id, "content_with_weight": f"chunk test {i}"})
        chunk_ids.append(res["data"]["chunk_id"])
    return chunk_ids


# DIALOG APP
def create_dialog(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DIALOG_APP_URL}/set", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def update_dialog(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DIALOG_APP_URL}/set", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def get_dialog(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{DIALOG_APP_URL}/get", headers=headers, auth=auth, params=params)
    return res.json()


def list_dialogs(auth, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{DIALOG_APP_URL}/list", headers=headers, auth=auth)
    return res.json()


def delete_dialog(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DIALOG_APP_URL}/rm", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def batch_create_dialogs(auth, num, kb_ids=None):
    if kb_ids is None:
        kb_ids = []

    dialog_ids = []
    for i in range(num):
        payload = {
            "name": f"dialog_{i}",
            "description": f"Test dialog {i}",
            "kb_ids": kb_ids,
            "prompt_config": {"system": "You are a helpful assistant. Use the following knowledge to answer questions: {knowledge}", "parameters": [{"key": "knowledge", "optional": False}]},
            "top_n": 6,
            "top_k": 1024,
            "similarity_threshold": 0.1,
            "vector_similarity_weight": 0.3,
            "llm_setting": {"model": "gpt-3.5-turbo", "temperature": 0.7},
        }
        res = create_dialog(auth, payload)
        if res["code"] == 0:
            dialog_ids.append(res["data"]["id"])
    return dialog_ids


def delete_dialogs(auth):
    res = list_dialogs(auth)
    if res["code"] == 0 and res["data"]:
        dialog_ids = [dialog["id"] for dialog in res["data"]]
        if dialog_ids:
            delete_dialog(auth, {"dialog_ids": dialog_ids})

# MEMORY APP
def create_memory(auth, payload=None):
    url = f"{HOST_ADDRESS}{MEMORY_API_URL}"
    res = requests.post(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def update_memory(auth, memory_id:str, payload=None):
    url = f"{HOST_ADDRESS}{MEMORY_API_URL}/{memory_id}"
    res = requests.put(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def delete_memory(auth, memory_id:str):
    url = f"{HOST_ADDRESS}{MEMORY_API_URL}/{memory_id}"
    res = requests.delete(url=url, headers=HEADERS, auth=auth)
    return res.json()


def list_memory(auth, params=None):
    url = f"{HOST_ADDRESS}{MEMORY_API_URL}"
    if params:
        query_parts = []
        for key, value in params.items():
            if isinstance(value, list):
                for item in value:
                    query_parts.append(f"{key}={item}")
            else:
                query_parts.append(f"{key}={value}")
        query_string = "&".join(query_parts)
        url = f"{url}?{query_string}"
    res = requests.get(url=url, headers=HEADERS, auth=auth)
    return res.json()


def get_memory_config(auth, memory_id:str):
    url = f"{HOST_ADDRESS}{MEMORY_API_URL}/{memory_id}/config"
    res = requests.get(url=url, headers=HEADERS, auth=auth)
    return res.json()


def list_memory_message(auth, memory_id, params=None):
    url = f"{HOST_ADDRESS}{MEMORY_API_URL}/{memory_id}"
    if params:
        query_parts = []
        for key, value in params.items():
            if isinstance(value, list):
                for item in value:
                    query_parts.append(f"{key}={item}")
            else:
                query_parts.append(f"{key}={value}")
        query_string = "&".join(query_parts)
        url = f"{url}?{query_string}"
    res = requests.get(url=url, headers=HEADERS, auth=auth)
    return res.json()


def add_message(auth, payload=None):
    url = f"{HOST_ADDRESS}{MESSAGE_API_URL}"
    res = requests.post(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def forget_message(auth, memory_id: str, message_id: int):
    url = f"{HOST_ADDRESS}{MESSAGE_API_URL}/{memory_id}:{message_id}"
    res = requests.delete(url=url, headers=HEADERS, auth=auth)
    return res.json()


def update_message_status(auth, memory_id: str, message_id: int, status: bool):
    url = f"{HOST_ADDRESS}{MESSAGE_API_URL}/{memory_id}:{message_id}"
    payload = {"status": status}
    res = requests.put(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def search_message(auth, params=None):
    url = f"{HOST_ADDRESS}{MESSAGE_API_URL}/search"
    if params:
        query_parts = []
        for key, value in params.items():
            if isinstance(value, list):
                for item in value:
                    query_parts.append(f"{key}={item}")
            else:
                query_parts.append(f"{key}={value}")
        query_string = "&".join(query_parts)
        url = f"{url}?{query_string}"
    res = requests.get(url=url, headers=HEADERS, auth=auth)
    return res.json()


def get_recent_message(auth, params=None):
    url = f"{HOST_ADDRESS}{MESSAGE_API_URL}"
    if params:
        query_parts = []
        for key, value in params.items():
            if isinstance(value, list):
                for item in value:
                    query_parts.append(f"{key}={item}")
            else:
                query_parts.append(f"{key}={value}")
        query_string = "&".join(query_parts)
        url = f"{url}?{query_string}"
    res = requests.get(url=url, headers=HEADERS, auth=auth)
    return res.json()


def get_message_content(auth, memory_id: str, message_id: int):
    url = f"{HOST_ADDRESS}{MESSAGE_API_URL}/{memory_id}:{message_id}/content"
    res = requests.get(url=url, headers=HEADERS, auth=auth)
    return res.json()
