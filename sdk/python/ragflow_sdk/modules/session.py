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
        self.messages = [{"role": "assistant", "content": "Hi! I am your assistant, can I help you?"}]
        for key, value in res_dict.items():
            if key == "chat_id" and value is not None:
                self.chat_id = None
                self.__session_type = "chat"
            if key == "agent_id" and value is not None:
                self.agent_id = None
                self.__session_type = "agent"
        super().__init__(rag, res_dict)


    def ask(self, question="", stream=False, **kwargs):
        """
        Ask a question to the session. If stream=True, yields Message objects as they arrive (SSE streaming).
        If stream=False, returns a single Message object for the final answer.
        """
        if self.__session_type == "agent":
            res = self._ask_agent(question, stream)
        elif self.__session_type == "chat":
            res = self._ask_chat(question, stream, **kwargs)
        else:
            raise Exception(f"Unknown session type: {self.__session_type}")

        if stream:
            for line in res.iter_lines(decode_unicode=True):
                if not line:
                    continue  # Skip empty lines
                line = line.strip()

                if line.startswith("data:"):
                    content = line[len("data:"):].strip()
                    if content == "[DONE]":
                        break  # End of stream
                else:
                    content = line

                try:
                    json_data = json.loads(content)
                except json.JSONDecodeError:
                    continue  # Skip lines that are not valid JSON

                event = json_data.get("event")
                if event == "message":
                    yield self._structure_answer(json_data)
                elif event == "message_end":
                    return  # End of message stream
        else:
            try:
                json_data = res.json()
            except ValueError:
                raise Exception(f"Invalid response {res}")
            yield self._structure_answer(json_data["data"])
        

    def _structure_answer(self, json_data):
        answer = json_data["data"]["content"]
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
        self.content = "Hi! I am your assistant, can I help you?"
        self.reference = None
        self.role = "assistant"
        self.prompt = None
        self.id = None
        super().__init__(rag, res_dict)
