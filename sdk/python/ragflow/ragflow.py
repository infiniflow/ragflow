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
import json
import os

import requests

from api.db.services.document_service import DocumentService
from api.settings import RetCode


class RAGFlow:
    def __init__(self, user_key, base_url, version='v1'):
        """
        api_url: http://<host_address>/api/v1
        dataset_url: http://<host_address>/api/v1/dataset
        document_url: http://<host_address>/api/v1/dataset/{dataset_id}/documents
        """
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

        endpoint = f"{self.dataset_url}/{dataset_id}"
        res = requests.delete(endpoint, headers=self.authorization_header)
        return res.json()

    def find_dataset_id_by_name(self, dataset_name):
        res = requests.get(self.dataset_url, headers=self.authorization_header)
        for dataset in res.json()["data"]:
            if dataset["name"] == dataset_name:
                return dataset["id"]
        return None

    def list_dataset(self, offset=0, count=-1, orderby="create_time", desc=True):
        params = {
            "offset": offset,
            "count": count,
            "orderby": orderby,
            "desc": desc
        }
        response = requests.get(url=self.dataset_url, params=params, headers=self.authorization_header)
        return response.json()

    def get_dataset(self, dataset_name):
        dataset_id = self.find_dataset_id_by_name(dataset_name)
        endpoint = f"{self.dataset_url}/{dataset_id}"
        response = requests.get(endpoint, headers=self.authorization_header)
        return response.json()

    def update_dataset(self, dataset_name, **params):
        dataset_id = self.find_dataset_id_by_name(dataset_name)

        endpoint = f"{self.dataset_url}/{dataset_id}"
        response = requests.put(endpoint, json=params, headers=self.authorization_header)
        return response.json()

# ------------------------------- CONTENT MANAGEMENT -----------------------------------------------------

    # ----------------------------upload local files-----------------------------------------------------
    def upload_local_file(self, dataset_id, file_paths):
        files = []

        for file_path in file_paths:
            if not isinstance(file_path, str):
                return {"code": RetCode.ARGUMENT_ERROR, "message": f"{file_path} is not string."}
            if "http" in file_path:
                return {"code": RetCode.ARGUMENT_ERROR, "message": "Remote files have not unsupported."}
            if os.path.isfile(file_path):
                files.append(("file", open(file_path, "rb")))
            else:
                return {"code": RetCode.DATA_ERROR, "message": f"The file {file_path} does not exist"}

        res = requests.request("POST", url=f"{self.dataset_url}/{dataset_id}/documents", files=files,
                               headers=self.authorization_header)

        result_dict = json.loads(res.text)
        return result_dict

    # ----------------------------delete a file-----------------------------------------------------
    def delete_files(self, document_id, dataset_id):
        endpoint = f"{self.dataset_url}/{dataset_id}/documents/{document_id}"
        res = requests.delete(endpoint, headers=self.authorization_header)
        return res.json()

    # ----------------------------list files-----------------------------------------------------
    def list_files(self, dataset_id, offset=0, count=-1, order_by="create_time", descend=True, keywords=""):
        params = {
            "offset": offset,
            "count": count,
            "order_by": order_by,
            "descend": descend,
            "keywords": keywords
        }
        endpoint = f"{self.dataset_url}/{dataset_id}/documents/"
        res = requests.get(endpoint, params=params, headers=self.authorization_header)
        return res.json()

    # ----------------------------update files: enable, rename, template_type-------------------------------------------
    def update_file(self, dataset_id, document_id, **params):
        endpoint = f"{self.dataset_url}/{dataset_id}/documents/{document_id}"
        response = requests.put(endpoint, json=params, headers=self.authorization_header)
        return response.json()

    # ----------------------------download a file-----------------------------------------------------
    def download_file(self, dataset_id, document_id):
        endpoint = f"{self.dataset_url}/{dataset_id}/documents/{document_id}"
        res = requests.get(endpoint, headers=self.authorization_header)

        content = res.content  # binary data
        # decode the binary data
        try:
            decoded_content = content.decode("utf-8")
            json_data = json.loads(decoded_content)
            return json_data  # message
        except json.JSONDecodeError:  # binary data
            _, document = DocumentService.get_by_id(document_id)
            file_path = os.path.join(os.getcwd(), document.name)
            with open(file_path, "wb") as file:
                file.write(content)
            return {"code": RetCode.SUCCESS, "data": content}

    # ----------------------------start parsing-----------------------------------------------------
    def start_parsing_document(self, dataset_id, document_id):
        endpoint = f"{self.dataset_url}/{dataset_id}/documents/{document_id}/status"
        res = requests.post(endpoint, headers=self.authorization_header)

        return res.json()

    def start_parsing_documents(self, dataset_id, doc_ids=None):
        endpoint = f"{self.dataset_url}/{dataset_id}/documents/status"
        res = requests.post(endpoint, headers=self.authorization_header, json={"doc_ids": doc_ids})

        return res.json()

    # ----------------------------stop parsing-----------------------------------------------------
    def stop_parsing_document(self, dataset_id, document_id):
        endpoint = f"{self.dataset_url}/{dataset_id}/documents/{document_id}/status"
        res = requests.delete(endpoint, headers=self.authorization_header)

        return res.json()

    def stop_parsing_documents(self, dataset_id, doc_ids=None):
        endpoint = f"{self.dataset_url}/{dataset_id}/documents/status"
        res = requests.delete(endpoint, headers=self.authorization_header, json={"doc_ids": doc_ids})

        return res.json()

    # ----------------------------show the status of the file-----------------------------------------------------
    def show_parsing_status(self, dataset_id, document_id):
        endpoint = f"{self.dataset_url}/{dataset_id}/documents/{document_id}/status"
        res = requests.get(endpoint, headers=self.authorization_header)

        return res.json()
    # ----------------------------list the chunks of the file-----------------------------------------------------

    # ----------------------------delete the chunk-----------------------------------------------------

    # ----------------------------edit the status of the chunk-----------------------------------------------------

    # ----------------------------insert a new chunk-----------------------------------------------------

    # ----------------------------get a specific chunk-----------------------------------------------------

    # ----------------------------retrieval test-----------------------------------------------------

