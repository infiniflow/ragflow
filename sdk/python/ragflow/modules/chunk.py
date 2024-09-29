from .base import Base


class Chunk(Base):
    def __init__(self, rag, res_dict):
        self.id = ""
        self.content = ""
        self.important_keywords = []
        self.create_time = ""
        self.create_timestamp = 0.0
        self.knowledgebase_id = None
        self.document_name = ""
        self.document_id = ""
        self.available = 1
        for k in list(res_dict.keys()):
            if k not in self.__dict__:
                res_dict.pop(k)
        super().__init__(rag, res_dict)

    def delete(self) -> bool:
        """
        Delete the chunk in the document.
        """
        res = self.post('/doc/chunk/rm',
                        {"document_id": self.document_id, 'chunk_ids': [self.id]})
        res = res.json()
        if res.get("retmsg") == "success":
            return True
        raise Exception(res["retmsg"])

    def save(self) -> bool:
        """
        Save the document details to the server.
        """
        res = self.post('/doc/chunk/set',
                        {"chunk_id": self.id,
                         "knowledgebase_id": self.knowledgebase_id,
                         "name": self.document_name,
                         "content": self.content,
                         "important_keywords": self.important_keywords,
                         "document_id": self.document_id,
                         "available": self.available,
                         })
        res = res.json()
        if res.get("retmsg") == "success":
            return True
        raise Exception(res["retmsg"])

