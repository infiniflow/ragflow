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

from httpx import HTTPError


class RAGFlow:
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

    def delete_dataset(self, dataset_name):
        dataset_id = self.find_dataset_id_by_name(dataset_name)
        if not dataset_id:
            return {"success": False, "message": "Dataset not found."}

        res = requests.delete(f"{self.dataset_url}/{dataset_id}", headers=self.authorization_header)
        if res.status_code == 200:
            return {"success": True, "message": "Dataset deleted successfully!"}
        else:
            return {"success": False, "message": f"Other status code: {res.status_code}"}

    def find_dataset_id_by_name(self, dataset_name):
        res = requests.get(self.dataset_url, headers=self.authorization_header)
        for dataset in res.json()['data']:
            if dataset['name'] == dataset_name:
                return dataset['id']
        return None

    def list_dataset(self, offset=0, count=-1, orderby="create_time", desc=True):
        params = {
            "offset": offset,
            "count": count,
            "orderby": orderby,
            "desc": desc
        }
        try:
            response = requests.get(url=self.dataset_url, params=params, headers=self.authorization_header)
            response.raise_for_status()  # if it is not 200
            original_data = response.json()
            # TODO: format the data
            # print(original_data)
            # # Process the original data into the desired format
            # formatted_data = {
            #     "datasets": [
            #         {
            #             "id": dataset["id"],
            #             "created": dataset["create_time"],  # Adjust the key based on the actual response
            #             "fileCount": dataset["doc_num"],  # Adjust the key based on the actual response
            #             "name": dataset["name"]
            #         }
            #         for dataset in original_data
            #     ]
            # }
            return response.status_code, original_data
        except HTTPError as http_err:
            print(f"HTTP error occurred: {http_err}")
        except Exception as err:
            print(f"An error occurred: {err}")

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
