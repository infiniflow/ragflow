import json

from .base import Base


class Session(Base):
    def __init__(self, rag, res_dict):
        self.id = None
        self.name = "New session"
        self.messages = [{"role": "assistant", "content": "Hi! I am your assistant，can I help you?"}]
        self.assistant_id = None
        super().__init__(rag, res_dict)

    def chat(self, question: str, stream: bool = False):
        for message in self.messages:
            if "reference" in message:
                message.pop("reference")
        res = self.post("/session/completion",
                        {"id": self.id, "question": question, "stream": stream}, stream=True)
        for line in res.iter_lines():
            line = line.decode("utf-8")
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
                                "knowledgebase_id": chunk["kb_id"],
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

    def save(self):
        res = self.post("/session/save",
                        {"id": self.id, "assistant_id": self.assistant_id, "name": self.name})
        res = res.json()
        if res.get("retmsg") == "success": return True
        raise Exception(res.get("retmsg"))

    def delete(self):
        res = self.rm("/session/delete", {"id": self.id})
        res = res.json()
        if res.get("retmsg") == "success": return True
        raise Exception(res.get("retmsg"))


class Message(Base):
    def __init__(self, rag, res_dict):
        self.content = "Hi! I am your assistant，can I help you?"
        self.reference = None
        self.role = "assistant"
        self.prompt = None
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
