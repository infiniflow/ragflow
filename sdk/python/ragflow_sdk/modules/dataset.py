#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

from typing import Any, Literal, NamedTuple, NotRequired, Optional, TYPE_CHECKING, TypedDict
from .base import Base
from .document import Document, ChunkMethod

if TYPE_CHECKING:
    from ..ragflow import RAGFlow

__all__ = 'DataSet',

class DocumentStatus(NamedTuple):
    document_id: str
    run: str
    chunk_count: int
    token_count: int


class DocumentParams(TypedDict):
    display_name: str
    blob: str|bytes

Permission = Literal["me", "team"]

class UpdateMessage(TypedDict):
    name: NotRequired[str]
    avatar: NotRequired[str]
    embedding_model: NotRequired[str]
    permission: NotRequired[Permission]
    pagerank: NotRequired[int]
    chunk_method: NotRequired[ChunkMethod]

class DataSet(Base):
    __slots__ = (
        'id',
        'name',
        'avatar',
        'tenant_id',
        'description',
        'embedding_model',
        'permission',
        'chunk_method',
        'parser_config',
        'pagerank',
    )

    class ParserConfig(Base):
        # TODO: Proper typing of parser config.

        def __init__(self, rag: "RAGFlow", res_dict: dict[str, Any]) -> None:
            super().__init__(rag, res_dict)

    id: str
    name: str
    avatar: str
    tenant_id: Optional[str]
    description: str
    embedding_model: str
    permission: str
    chunk_method: str
    parser_config: Optional[ParserConfig]
    pagerank: int

    def __init__(self, rag: "RAGFlow", res_dict: dict[str, Any]) -> None:
        self.id = ""
        self.name = ""
        self.avatar = ""
        self.tenant_id = None
        self.description = ""
        self.embedding_model = ""
        self.permission = "me"
        self.document_count = 0
        self.chunk_count = 0
        self.chunk_method = "naive"
        self.parser_config = None
        self.pagerank = 0
        for k in list(res_dict.keys()):
            if not hasattr(self, k):
                res_dict.pop(k)
        super().__init__(rag, res_dict)

    def update(self, update_message: UpdateMessage) -> "DataSet":
        res = self.put(f"/datasets/{self.id}", update_message)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])

        self._update_from_dict(self.rag, res.get("data", {}))
        return self

    def upload_documents(self, document_list: list[DocumentParams]) -> list[Document]:
        url = f"/datasets/{self.id}/documents"
        files = [("file", (ele["display_name"], ele["blob"])) for ele in document_list]
        res = self.post(path=url, json=None, files=files)
        res = res.json()
        if res.get("code") == 0:
            doc_list: list[Document] = []
            for doc in res["data"]:
                document = Document(self.rag, doc)
                doc_list.append(document)
            return doc_list
        raise Exception(res.get("message"))

    def list_documents(
        self,
        id: str | None = None,
        name: str | None = None,
        keywords: str | None = None,
        page: int = 1,
        page_size: int = 30,
        orderby: str = "create_time",
        desc: bool = True,
        create_time_from: int = 0,
        create_time_to: int = 0,
    ) -> list[Document]:
        params = {
            "id": id,
            "name": name,
            "keywords": keywords,
            "page": page,
            "page_size": page_size,
            "orderby": orderby,
            "desc": desc,
            "create_time_from": create_time_from,
            "create_time_to": create_time_to,
        }
        res = self.get(f"/datasets/{self.id}/documents", params=params)
        res = res.json()
        documents: list[Document] = []
        if res.get("code") == 0:
            for document in res["data"].get("docs"):
                documents.append(Document(self.rag, document))
            return documents
        raise Exception(res["message"])

    def delete_documents(self, ids: list[str] | None = None) -> None:
        res = self.rm(f"/datasets/{self.id}/documents", {"ids": ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])
        
    
    def _get_documents_status(self, document_ids: list[str]) -> list[DocumentStatus]:
        import time
        terminal_states = {"DONE", "FAIL", "CANCEL"}
        interval_sec = 1
        pending = set(document_ids)
        finished: list[DocumentStatus] = []
        while pending:
            for doc_id in list(pending):
                def fetch_doc(doc_id: str) -> Document | None:
                    try:
                        docs = self.list_documents(id=doc_id)
                        return docs[0] if docs else None
                    except Exception:
                        return None
                doc = fetch_doc(doc_id)
                if doc is None:
                    continue
                if isinstance(doc.run, str) and doc.run.upper() in terminal_states:
                    finished.append(DocumentStatus(doc_id, doc.run, doc.chunk_count, doc.token_count))
                    pending.discard(doc_id)
                elif float(doc.progress or 0.0) >= 1.0:
                    finished.append(DocumentStatus(doc_id, "DONE", doc.chunk_count, doc.token_count))
                    pending.discard(doc_id)
            if pending:
                time.sleep(interval_sec)
        return finished
    
    def async_parse_documents(self, document_ids: list[str]) -> None:
        res = self.post(f"/datasets/{self.id}/chunks", {"document_ids": document_ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res.get("message"))
        

    def parse_documents(self, document_ids: list[str]) -> list[DocumentStatus]:
        try:
            self.async_parse_documents(document_ids)
            self._get_documents_status(document_ids)
        except KeyboardInterrupt:
            self.async_cancel_parse_documents(document_ids)
            
        return self._get_documents_status(document_ids)


    def async_cancel_parse_documents(self, document_ids: list[str]) -> None:
        res = self.rm(f"/datasets/{self.id}/chunks", {"document_ids": document_ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res.get("message"))
