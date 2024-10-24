from typing import Optional, List

from transformers.models.bloom.modeling_bloom import bloom_gelu_back

from .document import Document

from .base import Base


class DataSet(Base):
    class ParserConfig(Base):
        def __init__(self, rag, res_dict):
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
        self.chunk_method = "naive"
        self.parser_config = None
        for k in list(res_dict.keys()):
            if k not in self.__dict__:
                res_dict.pop(k)
        super().__init__(rag, res_dict)

    def update(self, update_message: dict):
        res = self.put(f'/dataset/{self.id}',
                        update_message)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])

    def upload_documents(self,document_list: List[dict]):
        url = f"/dataset/{self.id}/document"
        files = [("file",(ele["displayed_name"],ele["blob"])) for ele in document_list]
        res = self.post(path=url,json=None,files=files)
        res = res.json()
        if res.get("code") == 0:
            doc_list=[]
            for doc in res["data"]:
                document = Document(self.rag,doc)
                doc_list.append(document)
            return doc_list
        raise Exception(res.get("message"))

    def list_documents(self, id: str = None, keywords: str = None, offset: int =1, limit: int = 1024, orderby: str = "create_time", desc: bool = True):
        res = self.get(f"/dataset/{self.id}/info",params={"id": id,"keywords": keywords,"offset": offset,"limit": limit,"orderby": orderby,"desc": desc})
        res = res.json()
        documents = []
        if res.get("code") == 0:
            for document in res["data"].get("docs"):
                documents.append(Document(self.rag,document))
            return documents
        raise Exception(res["message"])

    def delete_documents(self,ids: List[str] = None):
        res = self.rm(f"/dataset/{self.id}/document",{"ids":ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])

    def async_parse_documents(self,document_ids):
        res = self.post(f"/dataset/{self.id}/chunk",{"document_ids":document_ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res.get("message"))

    def async_cancel_parse_documents(self,document_ids):
        res = self.rm(f"/dataset/{self.id}/chunk",{"document_ids":document_ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res.get("message"))
