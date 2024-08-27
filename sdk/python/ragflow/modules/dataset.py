from .base import Base


class DataSet(Base):
    class ParserConfig(Base):
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
        self.parser_method = "naive"
        self.parser_config = None
        super().__init__(rag, res_dict)

    def save(self):
        res = self.post('/dataset/save',
                        {"id": self.id, "name": self.name, "avatar": self.avatar, "tenant_id": self.tenant_id,
                         "description": self.description, "language": self.language, "embd_id": self.embedding_model,
                         "permission": self.permission,
                         "doc_num": self.document_count, "chunk_num": self.chunk_count, "parser_id": self.parser_method,
                         "parser_config": self.parser_config.to_json()
                         })
        res = res.json()
        if not res.get("retmsg"): return True
        raise Exception(res["retmsg"])