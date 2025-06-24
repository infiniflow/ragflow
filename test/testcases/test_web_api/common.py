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
# FILE_API_URL = "/api/v1/datasets/{dataset_id}/documents"
# FILE_CHUNK_API_URL = "/api/v1/datasets/{dataset_id}/chunks"
# CHUNK_API_URL = "/api/v1/datasets/{dataset_id}/documents/{document_id}/chunks"
# CHAT_ASSISTANT_API_URL = "/api/v1/chats"
# SESSION_WITH_CHAT_ASSISTANT_API_URL = "/api/v1/chats/{chat_id}/sessions"
# SESSION_WITH_AGENT_API_URL = "/api/v1/agents/{agent_id}/sessions"


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
def upload_documents(auth, payload=None, files_path=None):
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
