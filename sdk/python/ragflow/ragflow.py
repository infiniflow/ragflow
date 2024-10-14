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
    def __init__(self, user_key, base_url, version='v1'):
        """
        api_url: http://<host_address>/api/v1
        """
        self.user_key = user_key
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
                       permission: str = "me",
                       document_count: int = 0, chunk_count: int = 0, parse_method: str = "naive",
                       parser_config: DataSet.ParserConfig = None) -> DataSet:
        if parser_config is None:
            parser_config = DataSet.ParserConfig(self, {"chunk_token_count": 128, "layout_recognize": True,
                                                        "delimiter": "\n!?。；！？", "task_page_size": 12})
        parser_config = parser_config.to_json()
        res = self.post("/dataset",
                        {"name": name, "avatar": avatar, "description": description, "language": language,
                         "permission": permission,
                         "document_count": document_count, "chunk_count": chunk_count, "parse_method": parse_method,
                         "parser_config": parser_config
                         }
                        )
        res = res.json()
        if res.get("code") == 0:
            return DataSet(self, res["data"])
        raise Exception(res["message"])

    def delete_datasets(self, ids: List[str] = None, names: List[str] = None):
        res = self.delete("/dataset",{"ids": ids, "names": names})
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

    def create_chat(self, name: str = "assistant", avatar: str = "path", knowledgebases: List[DataSet] = [],
                         llm: Chat.LLM = None, prompt: Chat.Prompt = None) -> Chat:
        datasets = []
        for dataset in knowledgebases:
            datasets.append(dataset.to_json())

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
                     "knowledgebases": datasets,
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



    def async_parse_documents(self, doc_ids):
        """
        Asynchronously start parsing multiple documents without waiting for completion.

        :param doc_ids: A list containing multiple document IDs.
        """
        try:
            if not doc_ids or not isinstance(doc_ids, list):
                raise ValueError("doc_ids must be a non-empty list of document IDs")

            data = {"document_ids": doc_ids, "run": 1}

            res = self.post(f'/doc/run', data)

            if res.status_code != 200:
                raise Exception(f"Failed to start async parsing for documents: {res.text}")

            print(f"Async parsing started successfully for documents: {doc_ids}")

        except Exception as e:
            print(f"Error occurred during async parsing for documents: {str(e)}")
            raise

    def async_cancel_parse_documents(self, doc_ids):
        """
        Cancel the asynchronous parsing of multiple documents.

        :param doc_ids: A list containing multiple document IDs.
        """
        try:
            if not doc_ids or not isinstance(doc_ids, list):
                raise ValueError("doc_ids must be a non-empty list of document IDs")
            data = {"document_ids": doc_ids, "run": 2}
            res = self.post(f'/doc/run', data)

            if res.status_code != 200:
                raise Exception(f"Failed to cancel async parsing for documents: {res.text}")

            print(f"Async parsing canceled successfully for documents: {doc_ids}")

        except Exception as e:
            print(f"Error occurred during canceling parsing for documents: {str(e)}")
            raise

    def retrieval(self,
                  question,
                  datasets=None,
                  documents=None,
                  offset=0,
                  limit=6,
                  similarity_threshold=0.1,
                  vector_similarity_weight=0.3,
                  top_k=1024):
        """
        Perform document retrieval based on the given parameters.

        :param question: The query question.
        :param datasets: A list of datasets (optional, as documents may be provided directly).
        :param documents: A list of documents (if specific documents are provided).
        :param offset: Offset for the retrieval results.
        :param limit: Maximum number of retrieval results.
        :param similarity_threshold: Similarity threshold.
        :param vector_similarity_weight: Weight of vector similarity.
        :param top_k: Number of top most similar documents to consider (for pre-filtering or ranking).

        Note: This is a hypothetical implementation and may need adjustments based on the actual backend service API.
        """
        try:
            data = {
                "question": question,
                "datasets": datasets if datasets is not None else [],
                "documents": [doc.id if hasattr(doc, 'id') else doc for doc in
                              documents] if documents is not None else [],
                "offset": offset,
                "limit": limit,
                "similarity_threshold": similarity_threshold,
                "vector_similarity_weight": vector_similarity_weight,
                "top_k": top_k,
                "knowledgebase_id": datasets,
            }

            # Send a POST request to the backend service (using requests library as an example, actual implementation may vary)
            res = self.post(f'/doc/retrieval_test', data)

            # Check the response status code
            if res.status_code == 200:
                res_data = res.json()
                if res_data.get("retmsg") == "success":
                    chunks = []
                    for chunk_data in res_data["data"].get("chunks", []):
                        chunk = Chunk(self, chunk_data)
                        chunks.append(chunk)
                    return chunks
                else:
                    raise Exception(f"Error fetching chunks: {res_data.get('retmsg')}")
            else:
                raise Exception(f"API request failed with status code {res.status_code}")

        except Exception as e:
            print(f"An error occurred during retrieval: {e}")
            raise
