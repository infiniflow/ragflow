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

from typing import Optional

import requests

from .modules.agent import Agent
from .modules.chat import Chat
from .modules.chunk import Chunk
from .modules.dataset import DataSet
from .modules.memory import Memory


class RAGFlow:
    def __init__(self, api_key, base_url, version="v1"):
        """
        api_url: http://<host_address>/api/v1
        """
        self.user_key = api_key
        self.api_url = f"{base_url}/api/{version}"
        self.authorization_header = {"Authorization": "{} {}".format("Bearer", self.user_key)}

    def post(self, path, json=None, stream=False, files=None):
        res = requests.post(url=self.api_url + path, json=json, headers=self.authorization_header, stream=stream, files=files)
        return res

    def get(self, path, params=None, json=None):
        res = requests.get(url=self.api_url + path, params=params, headers=self.authorization_header, json=json)
        return res

    def delete(self, path, json):
        res = requests.delete(url=self.api_url + path, json=json, headers=self.authorization_header)
        return res

    def put(self, path, json):
        res = requests.put(url=self.api_url + path, json=json, headers=self.authorization_header)
        return res

    def create_dataset(
        self,
        name: str,
        avatar: Optional[str] = None,
        description: Optional[str] = None,
        embedding_model: Optional[str] = None,
        permission: str = "me",
        chunk_method: str = "naive",
        parser_config: Optional[DataSet.ParserConfig] = None,
    ) -> DataSet:
        payload = {
            "name": name,
            "avatar": avatar,
            "description": description,
            "embedding_model": embedding_model,
            "permission": permission,
            "chunk_method": chunk_method,
        }
        if parser_config is not None:
            payload["parser_config"] = parser_config.to_json()

        res = self.post("/datasets", payload)
        res = res.json()
        if res.get("code") == 0:
            return DataSet(self, res["data"])
        raise Exception(res["message"])

    def delete_datasets(self, ids: list[str] | None = None):
        res = self.delete("/datasets", {"ids": ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])

    def get_dataset(self, name: str):
        _list = self.list_datasets(name=name)
        if len(_list) > 0:
            return _list[0]
        raise Exception("Dataset %s not found" % name)

    def list_datasets(self, page: int = 1, page_size: int = 30, orderby: str = "create_time", desc: bool = True, id: str | None = None, name: str | None = None) -> list[DataSet]:
        res = self.get(
            "/datasets",
            {
                "page": page,
                "page_size": page_size,
                "orderby": orderby,
                "desc": desc,
                "id": id,
                "name": name,
            },
        )
        res = res.json()
        result_list = []
        if res.get("code") == 0:
            for data in res["data"]:
                result_list.append(DataSet(self, data))
            return result_list
        raise Exception(res["message"])

    def create_chat(self, name: str, avatar: str = "", dataset_ids=None, llm: Chat.LLM | None = None, prompt: Chat.Prompt | None = None) -> Chat:
        if dataset_ids is None:
            dataset_ids = []
        dataset_list = []
        for id in dataset_ids:
            dataset_list.append(id)

        if llm is None:
            llm = Chat.LLM(
                self,
                {
                    "model_name": None,
                    "temperature": 0.1,
                    "top_p": 0.3,
                    "presence_penalty": 0.4,
                    "frequency_penalty": 0.7,
                    "max_tokens": 512,
                },
            )
        if prompt is None:
            prompt = Chat.Prompt(
                self,
                {
                    "similarity_threshold": 0.2,
                    "keywords_similarity_weight": 0.7,
                    "top_n": 8,
                    "top_k": 1024,
                    "variables": [{"key": "knowledge", "optional": True}],
                    "rerank_model": "",
                    "empty_response": None,
                    "opener": None,
                    "show_quote": True,
                    "prompt": None,
                },
            )
            if prompt.opener is None:
                prompt.opener = "Hi! I'm your assistant. What can I do for you?"
            if prompt.prompt is None:
                prompt.prompt = (
                    "You are an intelligent assistant. Your primary function is to answer questions based strictly on the provided knowledge base."
                    "**Essential Rules:**"
                    "- Your answer must be derived **solely** from this knowledge base: `{knowledge}`."
                    "- **When information is available**: Summarize the content to give a detailed answer."
                    "- **When information is unavailable**: Your response must contain this exact sentence: 'The answer you are looking for is not found in the knowledge base!' "
                    "- **Always consider** the entire conversation history."
                )

        temp_dict = {"name": name, "avatar": avatar, "dataset_ids": dataset_list if dataset_list else [], "llm": llm.to_json(), "prompt": prompt.to_json()}
        res = self.post("/chats", temp_dict)
        res = res.json()
        if res.get("code") == 0:
            return Chat(self, res["data"])
        raise Exception(res["message"])

    def delete_chats(self, ids: list[str] | None = None):
        res = self.delete("/chats", {"ids": ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])

    def list_chats(self, page: int = 1, page_size: int = 30, orderby: str = "create_time", desc: bool = True, id: str | None = None, name: str | None = None) -> list[Chat]:
        res = self.get(
            "/chats",
            {
                "page": page,
                "page_size": page_size,
                "orderby": orderby,
                "desc": desc,
                "id": id,
                "name": name,
            },
        )
        res = res.json()
        result_list = []
        if res.get("code") == 0:
            for data in res["data"]:
                result_list.append(Chat(self, data))
            return result_list
        raise Exception(res["message"])

    def retrieve(
        self,
        dataset_ids,
        document_ids=None,
        question="",
        page=1,
        page_size=30,
        similarity_threshold=0.2,
        vector_similarity_weight=0.3,
        top_k=1024,
        rerank_id: str | None = None,
        keyword: bool = False,
        cross_languages: list[str]|None = None,
        metadata_condition: dict | None = None,
        use_kg: bool = False,
        toc_enhance: bool = False,
    ):
        if document_ids is None:
            document_ids = []
        data_json = {
            "page": page,
            "page_size": page_size,
            "similarity_threshold": similarity_threshold,
            "vector_similarity_weight": vector_similarity_weight,
            "top_k": top_k,
            "rerank_id": rerank_id,
            "keyword": keyword,
            "question": question,
            "dataset_ids": dataset_ids,
            "document_ids": document_ids,
            "cross_languages": cross_languages,
            "metadata_condition": metadata_condition,
            "use_kg": use_kg,
            "toc_enhance": toc_enhance
        }
        # Send a POST request to the backend service (using requests library as an example, actual implementation may vary)
        res = self.post("/retrieval", json=data_json)
        res = res.json()
        if res.get("code") == 0:
            chunks = []
            for chunk_data in res["data"].get("chunks"):
                chunk = Chunk(self, chunk_data)
                chunks.append(chunk)
            return chunks
        raise Exception(res.get("message"))

    def list_agents(self, page: int = 1, page_size: int = 30, orderby: str = "update_time", desc: bool = True, id: str | None = None, title: str | None = None) -> list[Agent]:
        res = self.get(
            "/agents",
            {
                "page": page,
                "page_size": page_size,
                "orderby": orderby,
                "desc": desc,
                "id": id,
                "title": title,
            },
        )
        res = res.json()
        result_list = []
        if res.get("code") == 0:
            for data in res["data"]:
                result_list.append(Agent(self, data))
            return result_list
        raise Exception(res["message"])

    def create_agent(self, title: str, dsl: dict, description: str | None = None) -> None:
        req = {"title": title, "dsl": dsl}

        if description is not None:
            req["description"] = description

        res = self.post("/agents", req)
        res = res.json()

        if res.get("code") != 0:
            raise Exception(res["message"])

    def update_agent(self, agent_id: str, title: str | None = None, description: str | None = None, dsl: dict | None = None) -> None:
        req = {}

        if title is not None:
            req["title"] = title

        if description is not None:
            req["description"] = description

        if dsl is not None:
            req["dsl"] = dsl

        res = self.put(f"/agents/{agent_id}", req)
        res = res.json()

        if res.get("code") != 0:
            raise Exception(res["message"])

    def delete_agent(self, agent_id: str) -> None:
        res = self.delete(f"/agents/{agent_id}", {})
        res = res.json()

        if res.get("code") != 0:
            raise Exception(res["message"])

    def create_memory(self, name: str, memory_type: list[str], embd_id: str, llm_id: str):
        payload = {"name": name, "memory_type": memory_type, "embd_id": embd_id, "llm_id": llm_id}
        res = self.post("/memories", payload)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])
        return Memory(self, res["data"])

    def list_memory(self, page: int = 1, page_size: int = 50, tenant_id: str | list[str] = None, memory_type: str | list[str] = None, storage_type: str = None, keywords: str = None) -> dict:
        res = self.get(
            "/memories",
            {
                "page": page,
                "page_size": page_size,
                "tenant_id": tenant_id,
                "memory_type": memory_type,
                "storage_type": storage_type,
                "keywords": keywords,
            }
        )
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])
        result_list = []
        for data in res["data"]["memory_list"]:
            result_list.append(Memory(self, data))
        return {
            "code": res.get("code", 0),
            "message": res.get("message"),
            "memory_list": result_list,
            "total_count": res["data"]["total_count"]
        }

    def delete_memory(self, memory_id: str):
        res = self.delete(f"/memories/{memory_id}", {})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])

    def add_message(self, memory_id: list[str], agent_id: str, session_id: str, user_input: str, agent_response: str, user_id: str = "") -> str:
        payload = {
            "memory_id": memory_id,
            "agent_id": agent_id,
            "session_id": session_id,
            "user_input": user_input,
            "agent_response": agent_response,
            "user_id": user_id
        }
        res = self.post("/messages", payload)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])
        return res["message"]

    def search_message(self, query: str, memory_id: list[str], agent_id: str=None, session_id: str=None, similarity_threshold: float=0.2, keywords_similarity_weight: float=0.7, top_n: int=10) -> list[dict]:
        params = {
            "query": query,
            "memory_id": memory_id,
            "agent_id": agent_id,
            "session_id": session_id,
            "similarity_threshold": similarity_threshold,
            "keywords_similarity_weight": keywords_similarity_weight,
            "top_n": top_n
        }
        res = self.get("/messages/search", params)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])
        return res["data"]

    def get_recent_messages(self, memory_id: list[str], agent_id: str=None, session_id: str=None, limit: int=10) -> list[dict]:
        params = {
            "memory_id": memory_id,
            "agent_id": agent_id,
            "session_id": session_id,
            "limit": limit
        }
        res = self.get("/messages", params)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])
        return res["data"]
