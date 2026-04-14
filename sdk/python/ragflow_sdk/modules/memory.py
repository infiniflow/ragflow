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


class Memory(Base):

    def __init__(self, rag, res_dict):
        self.id = ""
        self.name = ""
        self.avatar = None
        self.tenant_id = None
        self.owner_name = ""
        self.memory_type = ["raw"]
        self.storage_type = "table"
        self.embd_id = ""
        self.llm_id = ""
        self.permissions = "me"
        self.description = ""
        self.memory_size = 5 * 1024 * 1024
        self.forgetting_policy = "FIFO"
        self.temperature = 0.5,
        self.system_prompt = ""
        self.user_prompt = ""
        for k in list(res_dict.keys()):
            if k not in self.__dict__:
                res_dict.pop(k)
        super().__init__(rag, res_dict)

    def update(self, update_dict: dict):
        res = self.put(f"/memories/{self.id}", update_dict)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])
        self._update_from_dict(self.rag, res.get("data", {}))
        return self

    def get_config(self):
        res = self.get(f"/memories/{self.id}/config")
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])
        self._update_from_dict(self.rag, res.get("data", {}))
        return self

    def list_memory_messages(self, agent_id: str | list[str]=None, keywords: str=None, page: int=1, page_size: int=50):
        params = {
            "agent_id": agent_id,
            "keywords": keywords,
            "page": page,
            "page_size": page_size
        }
        res = self.get(f"/memories/{self.id}", params)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])
        return res["data"]

    def forget_message(self, message_id: int):
        res = self.rm(f"/messages/{self.id}:{message_id}", {})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])
        return True

    def update_message_status(self, message_id: int, status: bool):
        update_message = {
            "status": status
        }
        res = self.put(f"/messages/{self.id}:{message_id}", update_message)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])
        return True

    def get_message_content(self, message_id: int) -> dict:
        res = self.get(f"/messages/{self.id}:{message_id}/content")
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])
        return res["data"]
