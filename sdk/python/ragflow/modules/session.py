import json

from .base import Base


class Session(Base):
    def __init__(self, rag, res_dict):
        self.id = None
        self.name = "New session"
        self.messages = [{"role": "assistant", "content": "您好，我是您的助手小樱，长得可爱又善良，can I help you?"}]
        self.assistant_id = None
        super().__init__(rag, res_dict)

    def chat(self, question: str, stream: bool = False):
        res = self.post("/session/completion",
                        {"id": self.id, "question": question, "stream": stream, "messages": self.messages})
        res = res.text
        response_lines = res.splitlines()
        message_list = []
        for line in response_lines:
            if line.startswith("data:"):
                json_data = json.loads(line[5:])
                if json_data["data"] != True:
                    answer = json_data["data"]["answer"]
                    reference = json_data["data"]["reference"]
                    temp_dict = {
                        "content": answer,
                        "role": "assistant",
                        "reference": reference
                    }
                    message = Message(self.rag, temp_dict)
                    message_list.append(message)
        return message_list

    def save(self):
        res = self.post("/session/save",
                        {"id": self.id, "dialog_id": self.assistant_id, "name": self.name, "message": self.messages})
        res = res.json()
        if res.get("retmsg") == "success": return True
        raise Exception(res.get("retmsg"))


class Message(Base):
    def __init__(self, rag, res_dict):
        self.content = ""
        self.reference = []
        self.role = None
        super().__init__(rag, res_dict)


class Chunk(Base):
    def __init__(self, rag, res_dict):
        self.id = None
        self.content = None
        self.document_id = None
        self.document_name = None
        self.knowledgebase_id = None
        self.image_id = None
        self.similarity = None
        self.vector_similarity = None
        self.term_similarity = None
        self.positions = None
        super().__init__(rag, res_dict)
