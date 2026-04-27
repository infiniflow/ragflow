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
import json
import os
from pathlib import Path
from uuid import uuid4

import requests
from configs import HOST_ADDRESS, VERSION
from requests_toolbelt import MultipartEncoder
from utils.file_utils import create_txt_file

HEADERS = {"Content-Type": "application/json"}

DATASETS_URL = f"/api/{VERSION}/datasets"
DOCUMENT_APP_URL = f"/{VERSION}/document"
CHUNK_APP_URL = f"/{VERSION}/chunk"
CHUNK_API_URL = f"/api/{VERSION}/datasets/{{dataset_id}}/documents/{{document_id}}/chunks"
# SESSION_WITH_CHAT_ASSISTANT_API_URL = "/api/v1/chats/{chat_id}/sessions"
# SESSION_WITH_AGENT_API_URL = "/api/v1/agents/{agent_id}/sessions"
MEMORY_API_URL = f"/api/{VERSION}/memories"
MESSAGE_API_URL = f"/api/{VERSION}/messages"
API_APP_URL = f"/{VERSION}/api"
SYSTEM_APP_URL = f"/{VERSION}/system"
SYSTEM_API_URL = f"/api/{VERSION}/system"
LLM_APP_URL = f"/{VERSION}/llm"
PLUGIN_APP_URL = f"/api/{VERSION}/plugin"
SEARCHES_URL = f"/api/{VERSION}/searches"
CHATS_URL = f"/api/{VERSION}/chats"


def _http_debug_enabled():
    return os.getenv("TEST_HTTP_DEBUG") == "1"


def _redact_payload(payload):
    if not isinstance(payload, dict):
        return payload
    redacted = {}
    for key, value in payload.items():
        if any(token in key.lower() for token in ("api_key", "password", "token", "secret", "authorization")):
            redacted[key] = "***redacted***"
        else:
            redacted[key] = value
    return redacted


def _log_http_debug(method, url, req_id, payload, status, text, resp_json, elapsed_ms):
    if not _http_debug_enabled():
        return
    payload_summary = _redact_payload(payload)
    print(f"[HTTP DEBUG] {method} {url} req_id={req_id} elapsed_ms={elapsed_ms:.1f}")
    print(f"[HTTP DEBUG] request_payload={json.dumps(payload_summary, default=str)}")
    print(f"[HTTP DEBUG] status={status}")
    print(f"[HTTP DEBUG] response_text={text}")
    print(f"[HTTP DEBUG] response_json={json.dumps(resp_json, default=str) if resp_json is not None else None}")

def api_stats(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{API_APP_URL}/stats", headers=headers, auth=auth, params=params)
    return res.json()


# SYSTEM APP
def system_new_token(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{SYSTEM_API_URL}/tokens", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def system_token_list(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{SYSTEM_API_URL}/tokens", headers=headers, auth=auth, params=params)
    return res.json()


def system_delete_token(auth, token, *, headers=HEADERS):
    res = requests.delete(url=f"{HOST_ADDRESS}{SYSTEM_API_URL}/tokens/{token}", headers=headers, auth=auth)
    return res.json()


def system_status(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{SYSTEM_API_URL}/status", headers=headers, auth=auth, params=params)
    return res.json()


def system_version(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{SYSTEM_API_URL}/version", headers=headers, auth=auth, params=params)
    return res.json()


def system_config(auth=None, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{SYSTEM_API_URL}/config", headers=headers, auth=auth, params=params)
    return res.json()


# LLM APP
def llm_factories(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{LLM_APP_URL}/factories", headers=headers, auth=auth, params=params)
    return res.json()


def llm_list(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{LLM_APP_URL}/list", headers=headers, auth=auth, params=params)
    return res.json()


# PLUGIN APP
def plugin_llm_tools(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{PLUGIN_APP_URL}/tools", headers=headers, auth=auth, params=params)
    return res.json()


# SEARCH APP
def search_create(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{SEARCHES_URL}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def search_update(auth, search_id, payload=None, *, headers=HEADERS, data=None):
    res = requests.put(url=f"{HOST_ADDRESS}{SEARCHES_URL}/{search_id}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def search_detail(auth, search_id, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{SEARCHES_URL}/{search_id}", headers=headers, auth=auth)
    return res.json()


def search_list(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{SEARCHES_URL}", headers=headers, auth=auth, params=params)
    return res.json()


def search_rm(auth, search_id, *, headers=HEADERS):
    res = requests.delete(url=f"{HOST_ADDRESS}{SEARCHES_URL}/{search_id}", headers=headers, auth=auth)
    return res.json()


# CHAT APP
def create_chat(auth, payload=None, *, headers=HEADERS, data=None):
    if payload is None:
        payload = {}
    res = requests.post(url=f"{HOST_ADDRESS}{CHATS_URL}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def list_chats(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{CHATS_URL}", headers=headers, auth=auth, params=params)
    return res.json()


def delete_chat(auth, chat_id, *, headers=HEADERS):
    res = requests.delete(url=f"{HOST_ADDRESS}{CHATS_URL}/{chat_id}", headers=headers, auth=auth)
    return res.json()


def delete_chats(auth, payload=None, *, headers=HEADERS, data=None):
    if payload is None:
        payload = {"delete_all": True}
    res = requests.delete(url=f"{HOST_ADDRESS}{CHATS_URL}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def batch_create_chats(auth, num):
    ids = []
    for i in range(num):
        res = create_chat(auth, {"name": f"chat_{uuid4().hex}_{i}"})
        ids.append(res["data"]["id"])
    return ids


# KB APP
def create_dataset(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DATASETS_URL}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def list_datasets(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{DATASETS_URL}", headers=headers, auth=auth, params=params)
    return res.json()


def update_dataset(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    res = requests.put(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def delete_datasets(auth, payload=None, *, headers=HEADERS, data=None):
    """
    Delete datasets.
    The endpoint is DELETE /api/{VERSION}/datasets with payload {"ids": [...]}
    This is the standard SDK REST API endpoint for dataset deletion.
    """
    res = requests.delete(url=f"{HOST_ADDRESS}{DATASETS_URL}", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def detail_kb(auth, dataset_id, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}", headers=headers, auth=auth)
    return res.json()


def kb_get_meta(auth, dataset_ids, *, headers=HEADERS):
    params = {"dataset_ids": dataset_ids}
    res = requests.get(url=f"{HOST_ADDRESS}{DATASETS_URL}/metadata/flattened", headers=headers, auth=auth, params=params)
    return res.json()


def kb_basic_info(auth, dataset_id, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/ingestions/summary", headers=headers, auth=auth)
    return res.json()


def kb_update_metadata_setting(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    res = requests.put(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/metadata/config", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def kb_list_pipeline_logs(auth, dataset_id, params=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/ingestions"
    res = requests.get(url=url, headers=headers, auth=auth, params=params)
    return res.json()


def kb_list_pipeline_dataset_logs(auth, dataset_id, params=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/ingestions"
    res = requests.get(url=url, headers=headers, auth=auth, params=params)
    return res.json()


def kb_pipeline_log_detail(auth, dataset_id, log_id, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/ingestions/{log_id}", headers=headers, auth=auth)
    return res.json()


# DATASET GRAPH AND TASKS
def knowledge_graph(auth, dataset_id, params=None):
    url = f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/knowledge_graph"
    res = requests.get(url=url, headers=HEADERS, auth=auth, params=params)
    return res.json()


def delete_knowledge_graph(auth, dataset_id, payload=None):
    url = f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/knowledge_graph"
    if payload is None:
        res = requests.delete(url=url, headers=HEADERS, auth=auth)
    else:
        res = requests.delete(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def list_tags_from_kbs(auth, dataset_ids, *, headers=HEADERS):
    params = {"dataset_ids": dataset_ids}
    res = requests.get(url=f"{HOST_ADDRESS}{DATASETS_URL}/tags/aggregation", headers=headers, auth=auth, params=params)
    return res.json()


def list_tags(auth, dataset_id, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/tags", headers=headers, auth=auth)
    return res.json()


def rm_tags(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    res = requests.delete(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/tags", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def rename_tags(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    res = requests.put(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/tags", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def batch_create_datasets(auth, num):
    ids = []
    for i in range(num):
        res = create_dataset(auth, {"name": f"kb_{i}"})
        ids.append(res["data"]["id"])
    return ids


# DOCUMENT APP
def upload_documents(auth, payload=None, files_path=None, *, filename_override=None):
    # New endpoint: /api/v1/datasets/{kb_id}/documents
    kb_id = payload.get("kb_id") if payload else None
    url = f"{HOST_ADDRESS}/api/{VERSION}/datasets/{kb_id}/documents"

    if files_path is None:
        files_path = []

    fields = []
    file_objects = []
    try:
        # Note: kb_id is now in the URL path, not in the form data
        if payload:
            for k, v in payload.items():
                if k != "kb_id":  # Skip kb_id as it's in the URL
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


def upload_info(auth, files_path=None, *, url=None):
    """
    Call the /api/v1/documents/upload endpoint to get upload info.
    This is used to get file metadata before actually uploading to a dataset.

    Args:
        auth: Authentication object
        files_path: List of file paths to upload (optional)
        url: URL to fetch file from (optional, can be used alone or with files_path to test mixed input rejection)

    Returns:
        Response JSON with upload info
    """
    url_endpoint = f"{HOST_ADDRESS}/api/{VERSION}/documents/upload"

    fields = []
    file_objects = []
    try:
        if files_path:
            for fp in files_path:
                p = Path(fp)
                f = p.open("rb")
                fields.append(("file", (p.name, f)))
                file_objects.append(f)

        # Add url as query parameter if provided
        if url:
            url_endpoint = f"{url_endpoint}?url={url}"

        # Handle empty fields (no files) - create empty MultipartEncoder
        if not fields:
            fields = [("empty", ("", ""))]

        m = MultipartEncoder(fields=fields)

        res = requests.post(
            url=url_endpoint,
            headers={"Content-Type": m.content_type},
            auth=auth,
            data=m,
        )
        return res.json()
    finally:
        for f in file_objects:
            f.close()


def create_document(auth, payload=None, *, headers=HEADERS, data=None):
    kb_id = payload.get("kb_id") if payload else None
    request_payload = dict(payload or {})
    request_payload.pop("kb_id", None)
    res = requests.post(
        url=f"{HOST_ADDRESS}{DATASETS_URL}/{kb_id}/documents?type=empty",
        headers=headers,
        auth=auth,
        json=request_payload,
        data=data,
    )
    return res.json()


def list_documents(auth, params=None, payload=None, *, headers=HEADERS, data=None):
    kb_id = params.get("kb_id") if params else None
    url = f"{HOST_ADDRESS}{DATASETS_URL}/{kb_id}/documents"
    if payload is None:
        payload = {}
    res = requests.get(url=url, headers=headers, auth=auth, params=params, json=payload, data=data)
    return res.json()


def delete_document(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    # New API: DELETE /api/v1/datasets/<dataset_id>/documents
    url = f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/documents"
    res = requests.delete(url=url, headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def parse_documents(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}/api/{VERSION}/documents/ingest", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def document_filter(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    res = requests.get(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/documents?type=filter", params=payload, headers=headers, auth=auth, data=data)
    return res.json()


def document_infos(auth, dataset_id, params=None, payload=None, *, headers=HEADERS, data=None):
    res = requests.get(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/documents", params=params, json=payload, headers=headers, auth=auth, data=data)
    return res.json()


def document_metadata_summary(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/metadata/summary", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def document_metadata_update(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    """New unified API for updating document metadata.

    Uses PATCH method at /api/v1/datasets/{dataset_id}/documents/metadatas
    """
    res = requests.patch(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/documents/metadatas", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def document_update_metadata_setting(auth, dataset_id, doc_id, payload=None, *, headers=HEADERS, data=None):
    res = requests.put(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/documents/{doc_id}/metadata/config", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def document_change_status(auth, dataset_id, payload=None, *, headers=HEADERS, data=None):
    """
    Batch update document status within a dataset.
    
    Args:
        auth: Authentication credentials
        dataset_id: ID of the dataset
        payload: Request body containing doc_ids and status
    """
    res = requests.post(url=f"{HOST_ADDRESS}{DATASETS_URL}/{dataset_id}/documents/batch-update-status", headers=headers, auth=auth, json=payload, data=data)
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


# CHUNK MANAGEMENT
def add_chunk(auth, dataset_id, document_id, payload=None, *, headers=HEADERS, data=None):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.post(url=url, headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def list_chunks(auth, dataset_id, document_id, params=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.get(url=url, headers=headers, auth=auth, params=params)
    return res.json()


def get_chunk(auth, dataset_id, document_id, chunk_id, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}/{chunk_id}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.get(url=url, headers=headers, auth=auth)
    return res.json()


def update_chunk(auth, dataset_id, document_id, chunk_id, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}/{chunk_id}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.patch(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


def switch_chunks(auth, dataset_id, document_id, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.patch(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


def delete_chunks(auth, dataset_id, document_id, payload=None, *, headers=HEADERS):
    url = f"{HOST_ADDRESS}{CHUNK_API_URL}".format(dataset_id=dataset_id, document_id=document_id)
    res = requests.delete(url=url, headers=headers, auth=auth, json=payload)
    return res.json()


def retrieval_chunks(auth, payload=None, *, headers=HEADERS):
    res = requests.post(url=f"{HOST_ADDRESS}{CHUNK_APP_URL}/retrieval_test", headers=headers, auth=auth, json=payload)
    return res.json()


def batch_add_chunks(auth, dataset_id, document_id, num):
    chunk_ids = []
    for i in range(num):
        res = add_chunk(auth, dataset_id, document_id, {"content": f"chunk test {i}"})
        chunk_ids.append(res["data"]["chunk"]["id"])
    return chunk_ids


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
