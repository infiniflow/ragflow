#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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


from .base import Base
from .session import Session


class Chat(Base):
    def __init__(self, rag, res_dict):
        self.id = ""
        self.name = "assistant"
        self.icon = ""
        self.dataset_ids = []
        self.llm_id = None
        self.llm_setting = {}
        self.prompt_config = {}
        self.similarity_threshold = 0.2
        self.vector_similarity_weight = 0.3
        self.top_n = 6
        self.top_k = 1024
        self.rerank_id = ""
        super().__init__(rag, res_dict)

    def update(self, update_message: dict):
        if not isinstance(update_message, dict):
            raise Exception("ValueError('`update_message` must be a dict')")
        res = self.patch(f"/chats/{self.id}", update_message)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])

    def create_session(self, name: str = "New session") -> Session:
        res = self.post(f"/chats/{self.id}/sessions", {"name": name})
        res = res.json()
        if res.get("code") == 0:
            return Session(self.rag, res["data"])
        raise Exception(res["message"])

    def list_sessions(self, page: int = 1, page_size: int = 30, orderby: str = "create_time", desc: bool = True, id: str = None, name: str = None) -> list[Session]:
        res = self.get(f"/chats/{self.id}/sessions", {"page": page, "page_size": page_size, "orderby": orderby, "desc": desc, "id": id, "name": name})
        res = res.json()
        if res.get("code") == 0:
            result_list = []
            for data in res["data"]:
                result_list.append(Session(self.rag, data))
            return result_list
        raise Exception(res["message"])

    def delete_sessions(self, ids: list[str] | None = None, delete_all: bool = False):
        res = self.rm(f"/chats/{self.id}/sessions", {"ids": ids, "delete_all": delete_all})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res.get("message"))
