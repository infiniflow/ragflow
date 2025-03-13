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

import os
from pathlib import Path

import requests
from libs.utils.file_utils import create_txt_file
from requests_toolbelt import MultipartEncoder

HEADERS = {"Content-Type": "application/json"}
HOST_ADDRESS = os.getenv("HOST_ADDRESS", "http://127.0.0.1:9380")
DATASETS_API_URL = "/api/v1/datasets"
FILE_API_URL = "/api/v1/datasets/{dataset_id}/documents"

INVALID_API_TOKEN = "invalid_key_123"
DATASET_NAME_LIMIT = 128
DOCUMENT_NAME_LIMIT = 128


# DATASET MANAGEMENT
def create_dataset(auth, payload):
    res = requests.post(
        url=f"{HOST_ADDRESS}{DATASETS_API_URL}",
        headers=HEADERS,
        auth=auth,
        json=payload,
    )
    return res.json()


def list_dataset(auth, params=None):
    res = requests.get(
        url=f"{HOST_ADDRESS}{DATASETS_API_URL}",
        headers=HEADERS,
        auth=auth,
        params=params,
    )
    return res.json()


def update_dataset(auth, dataset_id, payload):
    res = requests.put(
        url=f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}",
        headers=HEADERS,
        auth=auth,
        json=payload,
    )
    return res.json()


def delete_dataset(auth, payload=None):
    res = requests.delete(
        url=f"{HOST_ADDRESS}{DATASETS_API_URL}",
        headers=HEADERS,
        auth=auth,
        json=payload,
    )
    return res.json()


def create_datasets(auth, num):
    ids = []
    for i in range(num):
        res = create_dataset(auth, {"name": f"dataset_{i}"})
        ids.append(res["data"]["id"])
    return ids


# FILE MANAGEMENT WITHIN DATASET
def upload_documnets(auth, dataset_id, files_path=None):
    url = f"{HOST_ADDRESS}{FILE_API_URL}".format(dataset_id=dataset_id)

    if files_path is None:
        files_path = []

    fields = []
    for i, fp in enumerate(files_path):
        p = Path(fp)
        fields.append(("file", (p.name, p.open("rb"))))
    m = MultipartEncoder(fields=fields)

    res = requests.post(
        url=url,
        headers={"Content-Type": m.content_type},
        auth=auth,
        data=m,
    )
    return res.json()


def batch_upload_documents(auth, dataset_id, num, tmp_path):
    fps = []
    for i in range(num):
        fp = create_txt_file(tmp_path / f"ragflow_test_upload_{i}.txt")
        fps.append(fp)
    res = upload_documnets(auth, dataset_id, fps)
    document_ids = []
    for document in res["data"]:
        document_ids.append(document["id"])
    return document_ids


def download_document(auth, dataset_id, document_id, save_path):
    url = f"{HOST_ADDRESS}{FILE_API_URL}/{document_id}".format(dataset_id=dataset_id)
    res = requests.get(url=url, auth=auth, stream=True)
    try:
        if res.status_code == 200:
            with open(save_path, "wb") as f:
                for chunk in res.iter_content(chunk_size=8192):
                    f.write(chunk)
    finally:
        res.close()

    return res


def list_documnet(auth, dataset_id, params=None):
    url = f"{HOST_ADDRESS}{FILE_API_URL}".format(dataset_id=dataset_id)
    res = requests.get(
        url=url,
        headers=HEADERS,
        auth=auth,
        params=params,
    )
    return res.json()


def update_documnet(auth, dataset_id, document_id, payload):
    url = f"{HOST_ADDRESS}{FILE_API_URL}/{document_id}".format(dataset_id=dataset_id)
    res = requests.put(url=url, headers=HEADERS, auth=auth, json=payload)
    return res.json()
