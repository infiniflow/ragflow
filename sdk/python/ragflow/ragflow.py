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

from typing import List

import requests

from .modules.assistant import Assistant
from .modules.dataset import DataSet


class RAGFlow:
    def __init__(self, user_key, base_url, version='v1'):
        """
        api_url: http://<host_address>/api/v1
        """
        self.user_key = user_key
        self.api_url = f"{base_url}/api/{version}"
        self.authorization_header = {"Authorization": "{} {}".format("Bearer", self.user_key)}

    def post(self, path, param, stream=False):
        res = requests.post(url=self.api_url + path, json=param, headers=self.authorization_header, stream=stream)
        return res

    def get(self, path, params=None):
        res = requests.get(url=self.api_url + path, params=params, headers=self.authorization_header)
        return res

    def delete(self, path, params):
        res = requests.delete(url=self.api_url + path, params=params, headers=self.authorization_header)
        return res

    def create_dataset(self, name: str, avatar: str = "", description: str = "", language: str = "English",
                       permission: str = "me",
                       document_count: int = 0, chunk_count: int = 0, parse_method: str = "naive",
                       parser_config: DataSet.ParserConfig = None) -> DataSet:
        if parser_config is None:
            parser_config = DataSet.ParserConfig(self, {"chunk_token_count": 128, "layout_recognize": True,
                                                        "delimiter": "\n!?。；！？", "task_page_size": 12})
        parser_config = parser_config.to_json()
        res = self.post("/dataset/save",
                        {"name": name, "avatar": avatar, "description": description, "language": language,
                         "permission": permission,
                         "document_count": document_count, "chunk_count": chunk_count, "parse_method": parse_method,
                         "parser_config": parser_config
                         }
                        )
        res = res.json()
        if res.get("retmsg") == "success":
            return DataSet(self, res["data"])
        raise Exception(res["retmsg"])

    def list_datasets(self, page: int = 1, page_size: int = 1024, orderby: str = "create_time", desc: bool = True) -> \
            List[DataSet]:
        res = self.get("/dataset/list", {"page": page, "page_size": page_size, "orderby": orderby, "desc": desc})
        res = res.json()
        result_list = []
        if res.get("retmsg") == "success":
            for data in res['data']:
                result_list.append(DataSet(self, data))
            return result_list
        raise Exception(res["retmsg"])

    def get_dataset(self, id: str = None, name: str = None) -> DataSet:
        res = self.get("/dataset/detail", {"id": id, "name": name})
        res = res.json()
        if res.get("retmsg") == "success":
            return DataSet(self, res['data'])
        raise Exception(res["retmsg"])

    def create_assistant(self, name: str = "assistant", avatar: str = "path", knowledgebases: List[DataSet] = [],
                         llm: Assistant.LLM = None, prompt: Assistant.Prompt = None) -> Assistant:
        datasets = []
        for dataset in knowledgebases:
            datasets.append(dataset.to_json())

        if llm is None:
            llm = Assistant.LLM(self, {"model_name": None,
                                       "temperature": 0.1,
                                       "top_p": 0.3,
                                       "presence_penalty": 0.4,
                                       "frequency_penalty": 0.7,
                                       "max_tokens": 512, })
        if prompt is None:
            prompt = Assistant.Prompt(self, {"similarity_threshold": 0.2,
                                             "keywords_similarity_weight": 0.7,
                                             "top_n": 8,
                                             "variables": [{
                                                 "key": "knowledge",
                                                 "optional": True
                                             }], "rerank_model": "",
                                             "empty_response": None,
                                             "opener": None,
                                             "show_quote": True,
                                             "prompt": None})
            if prompt.opener is None:
                prompt.opener = "Hi! I'm your assistant, what can I do for you?"
            if prompt.prompt is None:
                prompt.prompt = (
                    "You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. "
                    "Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, "
                    "your answer must include the sentence 'The answer you are looking for is not found in the knowledge base!' "
                    "Answers need to consider chat history.\nHere is the knowledge base:\n{knowledge}\nThe above is the knowledge base."
                )

        temp_dict = {"name": name,
                     "avatar": avatar,
                     "knowledgebases": datasets,
                     "llm": llm.to_json(),
                     "prompt": prompt.to_json()}
        res = self.post("/assistant/save", temp_dict)
        res = res.json()
        if res.get("retmsg") == "success":
            return Assistant(self, res["data"])
        raise Exception(res["retmsg"])

    def get_assistant(self, id: str = None, name: str = None) -> Assistant:
        res = self.get("/assistant/get", {"id": id, "name": name})
        res = res.json()
        if res.get("retmsg") == "success":
            return Assistant(self, res['data'])
        raise Exception(res["retmsg"])

    def list_assistants(self) -> List[Assistant]:
        res = self.get("/assistant/list")
        res = res.json()
        result_list = []
        if res.get("retmsg") == "success":
            for data in res['data']:
                result_list.append(Assistant(self, data))
            return result_list
        raise Exception(res["retmsg"])
