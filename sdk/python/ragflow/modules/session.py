import json

from .base import Base


class Session(Base):
    def __init__(self, rag, res_dict):
        self.id = None
        self.name = "New session"
        self.messages = [{"role": "assistant", "content": "Hi! I am your assistant，can I help you?"}]
        self.chat_id = None
        super().__init__(rag, res_dict)

    def ask(self, question: str, stream: bool = False):
        for message in self.messages:
            if "reference" in message:
                message.pop("reference")
        res = self.post(f"/chat/{self.chat_id}/completion",
                        {"question": question, "stream": True,"session_id":self.id}, stream=stream)
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
                        chunk_list = []
                        for chunk in chunks:
                            new_chunk = {
                                "id": chunk["chunk_id"],
                                "content": chunk["content_with_weight"],
                                "document_id": chunk["doc_id"],
                                "document_name": chunk["docnm_kwd"],
                                "dataset_id": chunk["kb_id"],
                                "image_id": chunk["img_id"],
                                "similarity": chunk["similarity"],
                                "vector_similarity": chunk["vector_similarity"],
                                "term_similarity": chunk["term_similarity"],
                                "positions": chunk["positions"],
                            }
                            chunk_list.append(new_chunk)
                        temp_dict["reference"] = chunk_list
                    message = Message(self.rag, temp_dict)
                    yield message

    def update(self,update_message):
        res = self.put(f"/chat/{self.chat_id}/session/{self.id}",
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


class Chunk(Base):
    def __init__(self, rag, res_dict):
        self.id = None
        self.content = None
        self.document_id = ""
        self.document_name = ""
        self.dataset_id = ""
        self.image_id = ""
        self.similarity = None
        self.vector_similarity = None
        self.term_similarity = None
        self.positions = None
        super().__init__(rag, res_dict)

