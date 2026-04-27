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

import requests

HOST_ADDRESS = os.getenv("HOST_ADDRESS", "http://127.0.0.1:9380")
API_VERSION = "v1"
DATASETS_API_URL = f"/api/{API_VERSION}/datasets"

DATASET_NAME_LIMIT = 128


def create_dataset(auth, payload=None):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}"
    res = requests.post(url=url, headers={"Content-Type": "application/json"}, auth=auth, json=payload)
    return res.json()


def list_dataset(auth, params=None):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}"
    res = requests.get(url=url, headers={"Content-Type": "application/json"}, auth=auth, params=params)
    return res.json()


def rm_dataset(auth, dataset_ids):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}"
    res = requests.delete(url=url, headers={"Content-Type": "application/json"}, auth=auth, json={"ids": dataset_ids})
    return res.json()


def update_dataset(auth, dataset_id, payload=None):
    url = f"{HOST_ADDRESS}{DATASETS_API_URL}/{dataset_id}"
    res = requests.put(url=url, headers={"Content-Type": "application/json"}, auth=auth, json=payload)
    return res.json()


def upload_file(auth, dataset_id, path):
    authorization = {"Authorization": auth}
    url = f"{HOST_ADDRESS}/v1/document/upload"
    json_req = {
        "kb_id": dataset_id,
    }

    file = {"file": open(f"{path}", "rb")}

    res = requests.post(url=url, headers=authorization, files=file, data=json_req)
    return res.json()


def list_document(auth, dataset_id):
    authorization = {"Authorization": auth}
    url = f"{HOST_ADDRESS}/v1/document/list?id={dataset_id}"
    json = {}
    res = requests.post(url=url, headers=authorization, json=json)
    return res.json()


def get_docs_info(auth, dataset_id, doc_ids=None, doc_id=None):
    """
    Get document information by IDs.
    
    Args:
        auth: Authorization header
        dataset_id: Dataset ID
        doc_ids: List of document IDs (use for multiple) - exclusive with doc_id
        doc_id: Single document ID (use for one) - exclusive with doc_ids
    
    Raises:
        ValueError: If both doc_id and doc_ids are provided
    """
    # Validate that id and ids are not used together
    if doc_id and doc_ids:
        raise ValueError("Cannot use both 'id' and 'ids' parameters at the same time.")
    
    authorization = {"Authorization": auth}
    params = {}
    if doc_ids:
        # Multiple IDs
        for id in doc_ids:
            params.append(("ids", id))
    elif doc_id:
        # Single ID
        params["id"] = doc_id
    
    # Use /api/v1 prefix for dataset API
    url = f"{HOST_ADDRESS}/api/v1/datasets/{dataset_id}/documents"
    res = requests.get(url=url, headers=authorization, params=params)
    return res.json()


def parse_docs(auth, doc_ids):
    authorization = {"Authorization": auth}
    json_req = {"doc_ids": doc_ids, "run": 1}
    url = f"{HOST_ADDRESS}/v1/document/run"
    res = requests.post(url=url, headers=authorization, json=json_req)
    return res.json()


def parse_file(auth, document_id):
    pass

