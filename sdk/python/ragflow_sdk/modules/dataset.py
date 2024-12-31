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
        self.pagerank = 0
        for k in list(res_dict.keys()):
            if k not in self.__dict__:
                res_dict.pop(k)
        super().__init__(rag, res_dict)

    def update(self, update_message: dict):
        res = self.put(f'/datasets/{self.id}',
                        update_message)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])

    def upload_documents(self,document_list: list[dict]):
        url = f"/datasets/{self.id}/documents"
        files = [("file",(ele["display_name"],ele["blob"])) for ele in document_list]
        res = self.post(path=url,json=None,files=files)
        res = res.json()
        if res.get("code") == 0:
            doc_list=[]
            for doc in res["data"]:
                document = Document(self.rag,doc)
                doc_list.append(document)
            return doc_list
        raise Exception(res.get("message"))

    def list_documents(self, id: str | None = None, keywords: str | None = None, page: int = 1, page_size: int = 30, orderby: str = "create_time", desc: bool = True):
        res = self.get(f"/datasets/{self.id}/documents",params={"id": id,"keywords": keywords,"page": page,"page_size": page_size,"orderby": orderby,"desc": desc})
        res = res.json()
        documents = []
        if res.get("code") == 0:
            for document in res["data"].get("docs"):
                documents.append(Document(self.rag,document))
            return documents
        raise Exception(res["message"])

    def delete_documents(self,ids: list[str] | None = None):
        res = self.rm(f"/datasets/{self.id}/documents",{"ids":ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])

    def async_parse_documents(self,document_ids):
        res = self.post(f"/datasets/{self.id}/chunks",{"document_ids":document_ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res.get("message"))

    def async_cancel_parse_documents(self,document_ids):
        res = self.rm(f"/datasets/{self.id}/chunks",{"document_ids":document_ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res.get("message"))
