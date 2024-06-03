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


# abstract class
class RAGFlow(ABC):
    def __init__(self, user_key, base_url):
        self.user_key = user_key
        self.base_url = base_url

    def create_dataset(self, name):
        return name

    def delete_dataset(self, id):
        pass

    def list_datasets(self, id):
        pass

    def get_dataset(self, name):
        pass

    def update_dataset(self, name, dataset):
        pass

