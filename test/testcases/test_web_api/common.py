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
import time
import uuid
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
API_APP_URL = f"/{VERSION}/api"
SYSTEM_APP_URL = f"/{VERSION}/system"
LLM_APP_URL = f"/{VERSION}/llm"
PLUGIN_APP_URL = f"/{VERSION}/plugin"
SEARCH_APP_URL = f"/{VERSION}/search"


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


# API APP
def api_new_token(auth, payload=None, *, headers=HEADERS, data=None):
    if payload is None:
        payload = {}
    res = requests.post(url=f"{HOST_ADDRESS}{API_APP_URL}/new_token", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def api_token_list(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{API_APP_URL}/token_list", headers=headers, auth=auth, params=params)
    return res.json()


def api_rm_token(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{API_APP_URL}/rm", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def api_stats(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{API_APP_URL}/stats", headers=headers, auth=auth, params=params)
    return res.json()


# SYSTEM APP
def system_new_token(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{SYSTEM_APP_URL}/new_token", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def system_token_list(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{SYSTEM_APP_URL}/token_list", headers=headers, auth=auth, params=params)
    return res.json()


def system_delete_token(auth, token, *, headers=HEADERS):
    res = requests.delete(url=f"{HOST_ADDRESS}{SYSTEM_APP_URL}/token/{token}", headers=headers, auth=auth)
    return res.json()


def system_status(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{SYSTEM_APP_URL}/status", headers=headers, auth=auth, params=params)
    return res.json()


def system_version(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{SYSTEM_APP_URL}/version", headers=headers, auth=auth, params=params)
    return res.json()


def system_config(auth=None, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{SYSTEM_APP_URL}/config", headers=headers, auth=auth, params=params)
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
    res = requests.get(url=f"{HOST_ADDRESS}{PLUGIN_APP_URL}/llm_tools", headers=headers, auth=auth, params=params)
    return res.json()


# SEARCH APP
def search_create(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{SEARCH_APP_URL}/create", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def search_update(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{SEARCH_APP_URL}/update", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def search_detail(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{SEARCH_APP_URL}/detail", headers=headers, auth=auth, params=params)
    return res.json()


def search_list(auth, params=None, payload=None, *, headers=HEADERS, data=None):
    if payload is None:
        payload = {}
    res = requests.post(url=f"{HOST_ADDRESS}{SEARCH_APP_URL}/list", headers=headers, auth=auth, params=params, json=payload, data=data)
    return res.json()


def search_rm(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{SEARCH_APP_URL}/rm", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


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


def kb_get_meta(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{KB_APP_URL}/get_meta", headers=headers, auth=auth, params=params)
    return res.json()


def kb_basic_info(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{KB_APP_URL}/basic_info", headers=headers, auth=auth, params=params)
    return res.json()


def kb_update_metadata_setting(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/update_metadata_setting", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def kb_list_pipeline_logs(auth, params=None, payload=None, *, headers=HEADERS, data=None):
    if payload is None:
        payload = {}
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/list_pipeline_logs", headers=headers, auth=auth, params=params, json=payload, data=data)
    return res.json()


def kb_list_pipeline_dataset_logs(auth, params=None, payload=None, *, headers=HEADERS, data=None):
    if payload is None:
        payload = {}
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/list_pipeline_dataset_logs", headers=headers, auth=auth, params=params, json=payload, data=data)
    return res.json()


def kb_delete_pipeline_logs(auth, params=None, payload=None, *, headers=HEADERS, data=None):
    if payload is None:
        payload = {}
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/delete_pipeline_logs", headers=headers, auth=auth, params=params, json=payload, data=data)
    return res.json()


def kb_pipeline_log_detail(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{KB_APP_URL}/pipeline_log_detail", headers=headers, auth=auth, params=params)
    return res.json()


def kb_run_graphrag(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/run_graphrag", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def kb_trace_graphrag(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{KB_APP_URL}/trace_graphrag", headers=headers, auth=auth, params=params)
    return res.json()


def kb_run_raptor(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/run_raptor", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def kb_trace_raptor(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{KB_APP_URL}/trace_raptor", headers=headers, auth=auth, params=params)
    return res.json()


def kb_run_mindmap(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/run_mindmap", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def kb_trace_mindmap(auth, params=None, *, headers=HEADERS):
    res = requests.get(url=f"{HOST_ADDRESS}{KB_APP_URL}/trace_mindmap", headers=headers, auth=auth, params=params)
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
    res = requests.post(url=f"{HOST_ADDRESS}{KB_APP_URL}/{dataset_id}/rename_tag", headers=headers, auth=auth, json=payload, data=data)
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


def document_filter(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/filter", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def document_infos(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/infos", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def document_metadata_summary(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/metadata/summary", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def document_metadata_update(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/metadata/update", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def document_update_metadata_setting(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/update_metadata_setting", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def document_change_status(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/change_status", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def document_rename(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/rename", headers=headers, auth=auth, json=payload, data=data)
    return res.json()


def document_set_meta(auth, payload=None, *, headers=HEADERS, data=None):
    res = requests.post(url=f"{HOST_ADDRESS}{DOCUMENT_APP_URL}/set_meta", headers=headers, auth=auth, json=payload, data=data)
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
    if payload is None:
        payload = {}
    url = f"{HOST_ADDRESS}{DIALOG_APP_URL}/set"
    req_id = str(uuid.uuid4())
    req_headers = dict(headers)
    req_headers["X-Request-ID"] = req_id
    start = time.monotonic()
    res = requests.post(url=url, headers=req_headers, auth=auth, json=payload, data=data)
    elapsed_ms = (time.monotonic() - start) * 1000
    resp_json = None
    json_error = None
    try:
        resp_json = res.json()
    except ValueError as exc:
        json_error = exc
    _log_http_debug("POST", url, req_id, payload, res.status_code, res.text, resp_json, elapsed_ms)
    if _http_debug_enabled():
        if not res.ok or (resp_json is not None and resp_json.get("code") != 0):
            payload_summary = _redact_payload(payload)
            raise AssertionError(
                "HTTP helper failure: "
                f"req_id={req_id} url={url} status={res.status_code} "
                f"payload={payload_summary} response={res.text}"
            )
    if json_error:
        raise json_error
    return resp_json


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
        if kb_ids:
            prompt_config = {
                "system": "You are a helpful assistant. Use the following knowledge to answer questions: {knowledge}",
                "parameters": [{"key": "knowledge", "optional": False}],
            }
        else:
            prompt_config = {
                "system": "You are a helpful assistant.",
                "parameters": [],
            }
        payload = {
            "name": f"dialog_{i}",
            "description": f"Test dialog {i}",
            "kb_ids": kb_ids,
            "prompt_config": prompt_config,
            "top_n": 6,
            "top_k": 1024,
            "similarity_threshold": 0.1,
            "vector_similarity_weight": 0.3,
            "llm_setting": {"model": "gpt-3.5-turbo", "temperature": 0.7},
        }
        res = create_dialog(auth, payload)
        if res is None or res.get("code") != 0:
            uses_knowledge = "{knowledge}" in payload["prompt_config"]["system"]
            raise AssertionError(
                "batch_create_dialogs failed: "
                f"res={res} kb_ids_len={len(kb_ids)} uses_knowledge={uses_knowledge}"
            )
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
