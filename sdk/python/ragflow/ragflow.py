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

import os
import requests
import json

class RAGFLow:
    def __init__(self, user_key, base_url, version = 'v1'):
        '''
        api_url: http://<host_address>/api/v1
        dataset_url: http://<host_address>/api/v1/dataset
        '''
        self.user_key = user_key
        self.api_url = f"{base_url}/api/{version}"
        self.dataset_url = f"{self.api_url}/dataset"
        self.authorization_header = {"Authorization": "{}".format(self.user_key)}

    def create_dataset(self, dataset_name):
        """
        name: dataset name
        """
        res = requests.post(url=self.dataset_url, json={"name": dataset_name}, headers=self.authorization_header)
        result_dict = json.loads(res.text)
        return result_dict

    def delete_dataset(self, dataset_name = None, dataset_id = None):
        return dataset_name

    def list_dataset(self):
        response = requests.get(self.dataset_url)
        print(response)
        if response.status_code == 200:
            return response.json()['datasets']
        else:
            return None

    def get_dataset(self, dataset_id):
        endpoint = f"{self.dataset_url}/{dataset_id}"
        response = requests.get(endpoint)
        if response.status_code == 200:
            return response.json()
        else:
            return None

    def update_dataset(self, dataset_id, params):
        endpoint = f"{self.dataset_url}/{dataset_id}"
        response = requests.put(endpoint, json=params)
        if response.status_code == 200:
            return True
        else:
            return False