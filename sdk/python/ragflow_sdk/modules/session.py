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

import json

from .base import Base


class Session(Base):
    def __init__(self, rag, res_dict):
        self.id = None
        self.name = "New session"
        self.messages = [{"role": "assistant", "content": "Hi! I am your assistant，can I help you?"}]
        for key, value in res_dict.items():
            if key == "chat_id" and value is not None:
                self.chat_id = None
                self.__session_type = "chat"
            if key == "agent_id" and value is not None:
                self.agent_id = None
                self.__session_type = "agent"
        super().__init__(rag, res_dict)

    def ask(self, question="", stream=True, **kwargs):
        if self.__session_type == "agent":
            res = self._ask_agent(question, stream)
        elif self.__session_type == "chat":
            res = self._ask_chat(question, stream, **kwargs)

        if stream:
            for line in res.iter_lines():
                line = line.decode("utf-8")
                if line.startswith("{"):
                    json_data = json.loads(line)
                    raise Exception(json_data["message"])
                if not line.startswith("data:"):
                    continue
                json_data = json.loads(line[5:])
                if json_data["data"] is True or json_data["data"].get("running_status"):
                    continue
                answer = json_data["data"]["answer"]
                reference = json_data["data"].get("reference", {})
                temp_dict = {
                    "content": answer,
                    "role": "assistant"
                }
                if reference and "chunks" in reference:
                    chunks = reference["chunks"]
                    temp_dict["reference"] = chunks
                message = Message(self.rag, temp_dict)
                yield message
        else:
            try:
                json_data = json.loads(res.text)
            except ValueError:
                raise Exception(f"Invalid response {res}")
            answer = json_data["data"]["answer"]
            reference = json_data["data"].get("reference", {})
            temp_dict = {
                "content": answer,
                "role": "assistant"
            }
            if reference and "chunks" in reference:
                chunks = reference["chunks"]
                temp_dict["reference"] = chunks
            message = Message(self.rag, temp_dict)
            return message

    def _ask_chat(self, question: str, stream: bool, **kwargs):
        json_data = {"question": question, "stream": stream, "session_id": self.id}
        json_data.update(kwargs)
        res = self.post(f"/chats/{self.chat_id}/completions",
                        json_data, stream=stream)
        return res

    def _ask_agent(self, question: str, stream: bool):
        res = self.post(f"/agents/{self.agent_id}/completions",
                        {"question": question, "stream": stream, "session_id": self.id}, stream=stream)
        return res

    def update(self, update_message):
        res = self.put(f"/chats/{self.chat_id}/sessions/{self.id}",
                       update_message)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res.get("message"))


class Message(Base):
    def __init__(self, rag, res_dict):
        self.content = "Hi! I am your assistant，can I help you?"
        self.reference = None
        self.role = "assistant"
        self.prompt = None
        self.id = None
        super().__init__(rag, res_dict)
