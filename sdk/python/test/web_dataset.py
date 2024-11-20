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


def create_dataset(dataset_name: str):
    url = HOST_ADDRESS + "/create"
    request_data = {"name": "dataset1"}
    response=requests.post(url=url,json=request_data)
    res = response.json()
    if res.get("code")!=0:
        raise Exception(res.get("message"))
    auth = response.headers["Authorization"]
    return auth