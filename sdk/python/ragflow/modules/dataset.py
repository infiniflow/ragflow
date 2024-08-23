from .base import Base
import os

class DataSet(Base):
    def __init__(self, rag, res_dict):
        self.avatar = None
        self.chunk_num = 0
        self.description = None
        self.doc_num = 0
        self.embd_id = None
        self.id = None
        self.language = "Chinese" if "zh_CN" in os.getenv("LANG","") else "English"
        self.name = None
        self.parser_config = {"pages": [[1, 1000000]]}
        self.parser_id = "naive"
        self.permission = "me"
        self.token_num = 0
        super().__init__(rag, res_dict)

    def delete(self):
        try:
            self.post("/rm", {"kb_id": self.id})
            return True
        except Exception:
            return False

