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
        self.parse_method = "naive"
        self.parser_config = None
        for k in list(res_dict.keys()):
            if k not in self.__dict__:
                res_dict.pop(k)
        super().__init__(rag, res_dict)

    def save(self) -> bool:
        res = self.post('/dataset/save',
                        {"id": self.id, "name": self.name, "avatar": self.avatar, "tenant_id": self.tenant_id,
                         "description": self.description, "language": self.language, "embedding_model": self.embedding_model,
                         "permission": self.permission,
                         "document_count": self.document_count, "chunk_count": self.chunk_count, "parse_method": self.parse_method,
                         "parser_config": self.parser_config.to_json()
                         })
        res = res.json()
        if res.get("retmsg") == "success": return True
        raise Exception(res["retmsg"])

    def delete(self) -> bool:
        res = self.rm('/dataset/delete',
                      {"id": self.id})
        res = res.json()
        if res.get("retmsg") == "success": return True
        raise Exception(res["retmsg"])
