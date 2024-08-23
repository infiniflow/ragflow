from .base import Base


class DataSet(Base):
    class ParseConfig(Base):
        def __init__(self, rag, res_dict):
            self.chunk_token_count = 128
            self.layout_recognize = True
            self.delimiter = '\n!?。；！？'
            self.task_page_size = 12
            super().__init__(rag, res_dict)

    def __init__(self, rag, res_dict):
        self.id = ""
        self.name = ""
        self.avatar = ""
        self.tenant_id = None
        self.description = ""
        self.language = "English"
        self.embedding_model = ""
        self.permission = "me"
        self.document_count = 0
        self.chunk_count = 0
        self.parse_method = 0
        self.parser_config = None
        super().__init__(rag, res_dict)

    def delete(self):
        try:
            self.post("/rm", {"kb_id": self.id})
            return True
        except Exception:
            return False
