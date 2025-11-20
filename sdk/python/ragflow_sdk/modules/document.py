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

from typing import Any, Literal, NotRequired, Optional, TYPE_CHECKING, TypedDict

import json

from .base import Base
from .chunk import Chunk

if TYPE_CHECKING:
    from ..ragflow import RAGFlow

__all__ = 'Document',

ChunkMethod = Literal["naive", "manual", "qa", "table", "paper", "book", "laws", "presentation", "picture", "one", "email"]

LayoutRecognize = Literal["DeepDOC", "Plain Text", "Naive"]

class RaptorParams(TypedDict):
    use_raptor: NotRequired[bool]

class GraphragParams(TypedDict):
    use_graphrag: NotRequired[bool]

class ParserConfigParams(TypedDict):
    filename_embd_weight: NotRequired[int|float]

    # chunk_method=naive
    chunk_token_num: NotRequired[int]
    delimiter: NotRequired[str]
    html4excel: NotRequired[bool]
    layout_recognize: NotRequired[LayoutRecognize|bool]

    # chunk_method=raptor
    raptor: NotRequired[RaptorParams]

    # chunk_method=knowledge-graph
    entity_types: NotRequired[list[str]]

    graphrag: NotRequired[GraphragParams]

class UpdateMessage(TypedDict):
    display_name: NotRequired[str]
    meta_fields: NotRequired[dict[str, Any]]
    chunk_method: NotRequired[ChunkMethod]
    parser_config: NotRequired[ParserConfigParams]

class Document(Base):
    __slots__ = (
        'id',
        'name',
        'thumbnail',
        'dataset_id',
        'chunk_method',
        'parser_config',
        'source_type',
        'type',
        'created_by',
        'progress',
        'progress_msg',
        'process_begin_at',
        'process_duration',
        'run',
        'status',
        'meta_fields',
        'blob',
        'keywords',
    )

    class ParserConfig(Base):
        def __init__(self, rag: "RAGFlow", res_dict: dict[str, Any]) -> None:
            super().__init__(rag, res_dict)

    id: str
    name: str
    thumbnail: Optional[str]
    dataset_id: Optional[str]
    chunk_method: Optional[str]
    parser_config: dict[str, Any]
    source_type: str
    type: str
    created_by: str
    progress: float
    progress_msg: str
    process_begin_at: Optional[str]
    process_duration: float
    run: str
    status: str
    meta_fields: dict[str, Any]
    blob: bytes
    keywords: set[str]

    def __init__(self, rag: "RAGFlow", res_dict: dict[str, Any]) -> None:
        self.id = ""
        self.name = ""
        self.thumbnail = None
        self.dataset_id = None
        self.chunk_method = "naive"
        self.parser_config = {"pages": [[1, 1000000]]}
        self.source_type = "local"
        self.type = ""
        self.created_by = ""
        self.size = 0
        self.token_count = 0
        self.chunk_count = 0
        self.progress = 0.0
        self.progress_msg = ""
        self.process_begin_at = None
        self.process_duration = 0.0
        self.run = "0"
        self.status = "1"
        self.meta_fields = {}
        for k in list(res_dict.keys()):
            if not hasattr(self, k):
                res_dict.pop(k)
        super().__init__(rag, res_dict)

    def update(self, update_message: UpdateMessage) -> "Document":
        if "meta_fields" in update_message:
            if not isinstance(update_message["meta_fields"], dict):
                raise Exception("meta_fields must be a dictionary")
        res = self.put(f"/datasets/{self.dataset_id}/documents/{self.id}", update_message)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])

        self._update_from_dict(self.rag, res.get("data", {}))
        return self

    def download(self):
        res = self.get(f"/datasets/{self.dataset_id}/documents/{self.id}")
        error_keys = set(["code", "message"])
        try:
            response = res.json()
            actual_keys = set(response.keys())
            if actual_keys == error_keys:
                raise Exception(response.get("message"))
            else:
                return res.content
        except json.JSONDecodeError:
            return res.content

    def list_chunks(self, page: int=1, page_size: int=30, keywords: str="", id: str="") -> list[Chunk]:
        data = {"keywords": keywords, "page": page, "page_size": page_size, "id": id}
        res = self.get(f"/datasets/{self.dataset_id}/documents/{self.id}/chunks", data)
        res = res.json()
        if res.get("code") == 0:
            chunks: list[Chunk] = []
            for data in res["data"].get("chunks"):
                chunk = Chunk(self.rag, data)
                chunks.append(chunk)
            return chunks
        raise Exception(res.get("message"))

    def add_chunk(self, content: str, important_keywords: list[str] = [], questions: list[str] = []) -> Chunk:
        res = self.post(f"/datasets/{self.dataset_id}/documents/{self.id}/chunks", {"content": content, "important_keywords": important_keywords, "questions": questions})
        res = res.json()
        if res.get("code") == 0:
            return Chunk(self.rag, res["data"].get("chunk"))
        raise Exception(res.get("message"))

    def delete_chunks(self, ids: list[str] | None = None) -> None:
        res = self.rm(f"/datasets/{self.dataset_id}/documents/{self.id}/chunks", {"chunk_ids": ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res.get("message"))
