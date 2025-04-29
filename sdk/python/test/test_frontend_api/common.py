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

DATASET_NAME_LIMIT = 128


def create_dataset(auth, dataset_name):
    authorization = {"Authorization": auth}
    url = f"{HOST_ADDRESS}/v1/kb/create"
    json = {"name": dataset_name}
    res = requests.post(url=url, headers=authorization, json=json)
    return res.json()


def list_dataset(auth, page_number, page_size=30):
    authorization = {"Authorization": auth}
    url = f"{HOST_ADDRESS}/v1/kb/list?page={page_number}&page_size={page_size}"
    json = {}
    res = requests.post(url=url, headers=authorization, json=json)
    return res.json()


def rm_dataset(auth, dataset_id):
    authorization = {"Authorization": auth}
    url = f"{HOST_ADDRESS}/v1/kb/rm"
    json = {"kb_id": dataset_id}
    res = requests.post(url=url, headers=authorization, json=json)
    return res.json()


def update_dataset(auth, json_req):
    authorization = {"Authorization": auth}
    url = f"{HOST_ADDRESS}/v1/kb/update"
    res = requests.post(url=url, headers=authorization, json=json_req)
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
    url = f"{HOST_ADDRESS}/v1/document/list?kb_id={dataset_id}"
    json = {}
    res = requests.post(url=url, headers=authorization, json=json)
    return res.json()


def get_docs_info(auth, doc_ids):
    authorization = {"Authorization": auth}
    json_req = {"doc_ids": doc_ids}
    url = f"{HOST_ADDRESS}/v1/document/infos"
    res = requests.post(url=url, headers=authorization, json=json_req)
    return res.json()


def parse_docs(auth, doc_ids):
    authorization = {"Authorization": auth}
    json_req = {"doc_ids": doc_ids, "run": 1}
    url = f"{HOST_ADDRESS}/v1/document/run"
    res = requests.post(url=url, headers=authorization, json=json_req)
    return res.json()


def parse_file(auth, document_id):
    pass

