import json

from .base import Base


class Session(Base):
    def __init__(self, rag, res_dict):
        self.id = None
        self.name = "New session"
        self.messages = [{"role": "assistant", "content": "Hi! I am your assistant，can I help you?"}]
        self.chat_id = None
        self.agent_id = None
        for key,value in res_dict.items():
            if key =="chat_id" and value is not None:
                self.__session_type = "chat"
            if key == "agent_id" and value is not None:
                self.__session_type = "agent"
        super().__init__(rag, res_dict)

    def ask(self, question,stream=True):
        if self.__session_type == "agent":
            res=self._ask_agent(question,stream)
        elif self.__session_type == "chat":
            res=self._ask_chat(question,stream)
        for line in res.iter_lines():
            line = line.decode("utf-8")
            if line.startswith("{"):
                json_data = json.loads(line)
                raise Exception(json_data["message"])
            if line.startswith("data:"):
                json_data = json.loads(line[5:])
                if json_data["data"] != True:
                    answer = json_data["data"]["answer"]
                    reference = json_data["data"]["reference"]
                    temp_dict = {
                        "content": answer,
                        "role": "assistant"
                    }
                    if "chunks" in reference:
                        chunks = reference["chunks"]
                        temp_dict["reference"] = chunks
                    message = Message(self.rag, temp_dict)
                    yield message


    def _ask_chat(self, question: str, stream: bool):
        res = self.post(f"/chats/{self.chat_id}/completions",
                        {"question": question, "stream": True,"session_id":self.id}, stream=stream)
        return res
    def _ask_agent(self,question:str,stream:bool):
        res = self.post(f"/agents/{self.agent_id}/completions",
                        {"question": question, "stream": True,"session_id":self.id}, stream=stream)
        return res

    def update(self,update_message):
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