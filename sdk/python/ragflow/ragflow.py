#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
from abc import ABC
import requests


class RAGFLow(ABC):
    def __init__(self, user_key, base_url):
        self.user_key = user_key
        self.base_url = base_url

    def create_dataset(self, name):
        return name

    def delete_dataset(self, name):
        return name

    def list_dataset(self):
        endpoint = f"{self.base_url}/api/v1/dataset"
        response = requests.get(endpoint)
        if response.status_code == 200:
            return response.json()['datasets']
        else:
            return None

    def get_dataset(self, dataset_id):
        endpoint = f"{self.base_url}/api/v1/dataset/{dataset_id}"
        response = requests.get(endpoint)
        if response.status_code == 200:
            return response.json()
        else:
            return None

    def update_dataset(self, dataset_id, params):
        endpoint = f"{self.base_url}/api/v1/dataset/{dataset_id}"
        response = requests.put(endpoint, json=params)
        if response.status_code == 200:
            return True
        else:
            return False