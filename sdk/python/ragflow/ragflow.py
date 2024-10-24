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

from .modules.chat import Chat
from .modules.chunk import Chunk
from .modules.dataset import DataSet
from .modules.document import Document


class RAGFlow:
    def __init__(self, api_key, base_url, version='v1'):
        """
        api_url: http://<host_address>/api/v1
        """
        self.user_key = api_key
        self.api_url = f"{base_url}/api/{version}"
        self.authorization_header = {"Authorization": "{} {}".format("Bearer", self.user_key)}

    def post(self, path, json=None, stream=False, files=None):
        res = requests.post(url=self.api_url + path, json=json, headers=self.authorization_header, stream=stream,files=files)
        return res

    def get(self, path, params=None, json=None):
        res = requests.get(url=self.api_url + path, params=params, headers=self.authorization_header,json=json)
        return res

    def delete(self, path, json):
        res = requests.delete(url=self.api_url + path, json=json, headers=self.authorization_header)
        return res

    def put(self, path, json):
        res = requests.put(url=self.api_url + path, json= json,headers=self.authorization_header)
        return res

    def create_dataset(self, name: str, avatar: str = "", description: str = "", language: str = "English",
                       permission: str = "me",chunk_method: str = "naive",
                       parser_config: DataSet.ParserConfig = None) -> DataSet:
        if parser_config:
            parser_config = parser_config.to_json()
        res = self.post("/dataset",
                        {"name": name, "avatar": avatar, "description": description, "language": language,
                         "permission": permission, "chunk_method": chunk_method,
                         "parser_config": parser_config
                         }
                        )
        res = res.json()
        if res.get("code") == 0:
            return DataSet(self, res["data"])
        raise Exception(res["message"])

    def delete_datasets(self, ids: List[str]):
        res = self.delete("/dataset",{"ids": ids})
        res=res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])

    def get_dataset(self,name: str):
        _list = self.list_datasets(name=name)
        if len(_list) > 0:
            return _list[0]
        raise Exception("Dataset %s not found" % name)

    def list_datasets(self, page: int = 1, page_size: int = 1024, orderby: str = "create_time", desc: bool = True,
                      id: str = None, name: str = None) -> \
            List[DataSet]:
        res = self.get("/dataset",
                       {"page": page, "page_size": page_size, "orderby": orderby, "desc": desc, "id": id, "name": name})
        res = res.json()
        result_list = []
        if res.get("code") == 0:
            for data in res['data']:
                result_list.append(DataSet(self, data))
            return result_list
        raise Exception(res["message"])

    def create_chat(self, name: str, avatar: str = "", dataset_ids: List[str] = [],
                         llm: Chat.LLM = None, prompt: Chat.Prompt = None) -> Chat:
        dataset_list = []
        for id in dataset_ids:
            dataset_list.append(id)

        if llm is None:
            llm = Chat.LLM(self, {"model_name": None,
                                       "temperature": 0.1,
                                       "top_p": 0.3,
                                       "presence_penalty": 0.4,
                                       "frequency_penalty": 0.7,
                                       "max_tokens": 512, })
        if prompt is None:
            prompt = Chat.Prompt(self, {"similarity_threshold": 0.2,
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
                     "dataset_ids": dataset_list,
                     "llm": llm.to_json(),
                     "prompt": prompt.to_json()}
        res = self.post("/chat", temp_dict)
        res = res.json()
        if res.get("code") == 0:
            return Chat(self, res["data"])
        raise Exception(res["message"])

    def delete_chats(self,ids: List[str] = None,names: List[str] = None ) -> bool:
        res = self.delete('/chat',
                      {"ids":ids, "names":names})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])

    def list_chats(self, page: int = 1, page_size: int = 1024, orderby: str = "create_time", desc: bool = True,
                      id: str = None, name: str = None) -> List[Chat]:
        res = self.get("/chat",{"page": page, "page_size": page_size, "orderby": orderby, "desc": desc, "id": id, "name": name})
        res = res.json()
        result_list = []
        if res.get("code") == 0:
            for data in res['data']:
                result_list.append(Chat(self, data))
            return result_list
        raise Exception(res["message"])


    def retrieve(self, dataset_ids, document_ids=None, question="", offset=1, limit=1024, similarity_threshold=0.2, vector_similarity_weight=0.3, top_k=1024, rerank_id:str=None, keyword:bool=False, ):
            if document_ids is None:
                document_ids = []
            data_json ={
                "offset": offset,
                "limit": limit,
                "similarity_threshold": similarity_threshold,
                "vector_similarity_weight": vector_similarity_weight,
                "top_k": top_k,
                "rerank_id": rerank_id,
                "keyword": keyword,
                "question": question,
                "datasets": dataset_ids,
                "documents": document_ids
            }
            # Send a POST request to the backend service (using requests library as an example, actual implementation may vary)
            res = self.post(f'/retrieval',json=data_json)
            res = res.json()
            if res.get("code") ==0:
                chunks=[]
                for chunk_data in res["data"].get("chunks"):
                    chunk=Chunk(self,chunk_data)
                    chunks.append(chunk)
                return chunks
            raise Exception(res.get("message"))
