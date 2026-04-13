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

"""
The example is about CRUD operations (Create, Read, Update, Delete) on a dataset.
"""

from ragflow_sdk import RAGFlow
import sys

HOST_ADDRESS = "http://127.0.0.1"
API_KEY = "ragflow-IzZmY1MGVhYTBhMjExZWZiYTdjMDI0Mm"

try:
    # create a ragflow instance
    ragflow_instance = RAGFlow(api_key=API_KEY, base_url=HOST_ADDRESS)

    # crate a dataset instance
    dataset_instance = ragflow_instance.create_dataset(name="dataset_instance")

    # update the dataset instance
    updated_message = {"name":"updated_dataset"}
    updated_dataset = dataset_instance.update(updated_message)

    # get the dataset (list datasets)
    print(dataset_instance)
    print(updated_dataset)

    # delete the dataset (delete datasets)
    to_be_deleted_datasets = [dataset_instance.id]
    ragflow_instance.delete_datasets(ids=to_be_deleted_datasets)

    print("test done")
    sys.exit(0)

except Exception as e:
    print(str(e))
    sys.exit(-1)


