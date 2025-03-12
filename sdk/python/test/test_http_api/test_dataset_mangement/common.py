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
API_URL = f"{HOST_ADDRESS}/api/v1/datasets"
HEADERS = {"Content-Type": "application/json"}


INVALID_API_TOKEN = "invalid_key_123"
DATASET_NAME_LIMIT = 128


def create_dataset(auth, payload):
    res = requests.post(url=API_URL, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def list_dataset(auth, params=None):
    res = requests.get(url=API_URL, headers=HEADERS, auth=auth, params=params)
    return res.json()


def update_dataset(auth, dataset_id, payload):
    res = requests.put(
        url=f"{API_URL}/{dataset_id}", headers=HEADERS, auth=auth, json=payload 
    )
    return res.json()


def delete_dataset(auth, payload=None):
    res = requests.delete(url=API_URL, headers=HEADERS, auth=auth, json=payload)
    return res.json()


def create_datasets(auth, num):
    ids = []
    for i in range(num):
        res = create_dataset(auth, {"name": f"dataset_{i}"})
        ids.append(res["data"]["id"])
    return ids
