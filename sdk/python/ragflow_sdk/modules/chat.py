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
        self.avatar = "path/to/avatar"
        self.llm = Chat.LLM(rag, {})
        self.prompt = Chat.Prompt(rag, {})
        super().__init__(rag, res_dict)

    class LLM(Base):
        def __init__(self, rag, res_dict):
            self.model_name = None
            self.temperature = 0.1
            self.top_p = 0.3
            self.presence_penalty = 0.4
            self.frequency_penalty = 0.7
            self.max_tokens = 512
            super().__init__(rag, res_dict)

    class Prompt(Base):
        def __init__(self, rag, res_dict):
            self.similarity_threshold = 0.2
            self.keywords_similarity_weight = 0.7
            self.top_n = 8
            self.top_k = 1024
            self.variables = [{"key": "knowledge", "optional": True}]
            self.rerank_model = ""
            self.empty_response = None
            self.opener = "Hi! I'm your assistant. What can I do for you?"
            self.show_quote = True
            self.prompt = (
                "You are an intelligent assistant. Your primary function is to answer questions based strictly on the provided knowledge base."
                "**Essential Rules:**"
                "- Your answer must be derived **solely** from this knowledge base: `{knowledge}`."
                "- **When information is available**: Summarize the content to give a detailed answer."
                "- **When information is unavailable**: Your response must contain this exact sentence: 'The answer you are looking for is not found in the knowledge base!' "
                "- **Always consider** the entire conversation history."
            )
            super().__init__(rag, res_dict)

    def update(self, update_message: dict):
        if not isinstance(update_message, dict):
            raise Exception("ValueError('`update_message` must be a dict')")
        if update_message.get("llm") == {}:
            raise Exception("ValueError('`llm` cannot be empty')")
        if update_message.get("prompt") == {}:
            raise Exception("ValueError('`prompt` cannot be empty')")
        res = self.put(f"/chats/{self.id}", update_message)
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

    def delete_sessions(self, ids: list[str] | None = None):
        res = self.rm(f"/chats/{self.id}/sessions", {"ids": ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res.get("message"))
